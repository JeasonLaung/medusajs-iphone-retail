package uniquekey

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func paymentImportFixture(id string) rowimport.SourceRow {
	text := func(s string) rowimport.Cell { raw, _ := json.Marshal(s); return rowimport.Cell{Raw: raw} }
	data, _ := json.Marshal(map[string]any{"id": "provider-" + id, "mode": "test", "resource": "payment", "status": "open",
		"amount": map[string]string{"currency": "EUR", "value": "1.23"}, "session_id": id, "cart_id": "fixture-cart",
		"metadata": map[string]string{"session_id": id, "cart_id": "fixture-cart"}})
	return rowimport.SourceRow{"id": text(id), "currency_code": text("eur"), "amount": {Raw: json.RawMessage(`"1.2300"`)},
		"raw_amount":  {Raw: json.RawMessage(`{"value":"1.2300000000000000000","precision":20}`)},
		"provider_id": text("pp_mollie-hosted-checkout_mollie"), "data": {Raw: data},
		"context": {Raw: json.RawMessage(`{"customer":{"addresses":[{"token":"old-auth"}],"billing_address":{"address_1":"交易历史"}}}`)},
		"status":  text("pending"), "authorized_at": {SQLNull: true}, "payment_collection_id": text("import-payment-parent"),
		"metadata": {Raw: json.RawMessage(` { } `)}, "created_at": text("2026-10-27T00:00:00.123456Z"),
		"updated_at": text("2026-10-27T00:00:00.123456Z"), "deleted_at": {SQLNull: true}}
}

func testPaymentHistoryImportSQL(t *testing.T, ctx context.Context) {
	t.Helper()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "relational_fixture"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("payment import fixture configuration failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	ddl, err := migrations.Files.ReadFile("0005_migration_payment_dispositions.sql")
	if err != nil {
		t.Fatal("payment ledger migration missing")
	}
	if _, err := db.ExecContext(ctx, string(ddl)); err != nil {
		t.Fatal("payment ledger migration failed")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO payment_collections(id,currency_code,amount,raw_amount,created_at,updated_at,status)
	 VALUES('import-payment-parent','eur',1.23,'{"value":"1.23","precision":20}','2026-10-27','2026-10-27','not_paid')`); err != nil {
		t.Fatal("payment parent fixture failed")
	}
	oldKey, newKey := bytes.Repeat([]byte{53}, 32), bytes.Repeat([]byte{59}, 32)
	ring, _ := protected.New("old", map[string][]byte{"old": oldKey})
	converter, _ := rowimport.NewPaymentHistory(ring)
	snapshot := strings.Repeat("b", 64)
	run := func(converter *rowimport.PaymentHistory, table string, source rowimport.SourceRow, hash string, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := converter.Import(ctx, tx, hash, table, source); err != nil {
			return err
		}
		if commit {
			return tx.Commit()
		}
		return tx.Rollback()
	}
	count := func(table, column, id string) int {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"` WHERE `"+column+"`=?", id).Scan(&n); err != nil {
			t.Fatal("payment fixture count failed")
		}
		return n
	}
	row := paymentImportFixture("import-session")
	if err := run(converter, "payment_session", row, snapshot, false); err != nil {
		t.Fatal("payment rollback staging failed", err)
	}
	if count("payment_sessions", "id", "import-session") != 0 || count("migration_payment_dispositions", "source_id", "import-session") != 0 {
		t.Fatal("payment or hold survived rollback")
	}
	if err := run(converter, "payment_session", row, snapshot, true); err != nil {
		t.Fatal("payment atomic import failed", err)
	}
	var dataBefore, contextBefore, metadataBefore string
	if err := db.QueryRowContext(ctx, "SELECT data,context,metadata FROM payment_sessions WHERE id='import-session'").Scan(&dataBefore, &contextBefore, &metadataBefore); err != nil {
		t.Fatal("payment import readback unavailable")
	}
	// Newly generated random nonces are discarded on replay; old ciphertext
	// remains exact, including after activating a new independent target key.
	rotatedRing, _ := protected.New("new", map[string][]byte{"old": oldKey, "new": newKey})
	rotated, _ := rowimport.NewPaymentHistory(rotatedRing)
	for _, importer := range []*rowimport.PaymentHistory{converter, rotated} {
		if err := run(importer, "payment_session", row, snapshot, true); err != nil {
			t.Fatal("exact encrypted payment replay failed", err)
		}
	}
	var dataAfter, contextAfter, metadataAfter string
	if err := db.QueryRowContext(ctx, "SELECT data,context,metadata FROM payment_sessions WHERE id='import-session'").Scan(&dataAfter, &contextAfter, &metadataAfter); err != nil || dataBefore != dataAfter || contextBefore != contextAfter || metadataBefore != metadataAfter {
		t.Fatal("encrypted replay overwrote original evidence")
	}
	tx, _ := db.BeginTx(ctx, nil)
	disposition, err := rowimport.HistoricalPaymentDisposition(ctx, tx, "payment_session", "import-session")
	_ = tx.Rollback()
	if err != nil || disposition != "hold" {
		t.Fatal("imported payment operation not held")
	}
	tx, _ = db.BeginTx(ctx, nil)
	if disposition, err := rowimport.HistoricalPaymentDisposition(ctx, tx, "payment_session", "missing"); err == nil || disposition != "" {
		t.Fatal("missing hold granted permission")
	}
	_ = tx.Rollback()
	if err := run(converter, "payment_session", row, strings.Repeat("c", 64), true); err != rowimport.ErrPaymentConflict {
		t.Fatal("different snapshot replay accepted")
	}
	// Even excluded old capability changes must conflict with source evidence.
	changed := paymentImportFixture("import-session")
	changed["context"] = rowimport.Cell{Raw: json.RawMessage(`{"customer":{"addresses":[{"token":"different-old-auth"}],"billing_address":{"address_1":"交易历史"}}}`)}
	if err := run(converter, "payment_session", changed, snapshot, true); err != rowimport.ErrPaymentConflict {
		t.Fatal("changed redacted source accepted")
	}
	newOnly, _ := protected.New("new", map[string][]byte{"new": newKey})
	withoutOld, _ := rowimport.NewPaymentHistory(newOnly)
	if err := run(withoutOld, "payment_session", row, snapshot, true); err != rowimport.ErrPaymentConflict {
		t.Fatal("replay without original decryption key accepted")
	}
	// A target failure AFTER the ledger insert must roll both back.
	broken := paymentImportFixture("import-missing-parent")
	broken["payment_collection_id"] = rowimport.Cell{Raw: json.RawMessage(`"missing-parent"`)}
	if err := run(converter, "payment_session", broken, snapshot, true); err != rowimport.ErrPaymentStorage {
		t.Fatal("missing payment parent not rejected")
	}
	if count("migration_payment_dispositions", "source_id", "import-missing-parent") != 0 || count("payment_sessions", "id", "import-missing-parent") != 0 {
		t.Fatal("target failure left hold or payment residue")
	}
	// Eight identical imports serialize on the same ledger row. Every replay
	// succeeds, with exactly one persisted target and its original ciphertext.
	var wg sync.WaitGroup
	start := make(chan struct{})
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errors <- run(converter, "payment_session", paymentImportFixture("import-race"), snapshot, true)
		}()
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal("concurrent exact payment import failed", err)
		}
	}
	if count("payment_sessions", "id", "import-race") != 1 || count("migration_payment_dispositions", "source_id", "import-race") != 1 {
		t.Fatal("concurrent payment imported more than once")
	}
	// Nullable payment fields still require an atomic durable hold.
	payment := rowimport.SourceRow{}
	for k, v := range row {
		payment[k] = v
	}
	for _, k := range []string{"context", "status", "authorized_at"} {
		delete(payment, k)
	}
	payment["id"] = rowimport.Cell{Raw: json.RawMessage(`"import-payment"`)}
	payment["payment_session_id"] = rowimport.Cell{Raw: json.RawMessage(`"import-session"`)}
	payment["captured_at"], payment["canceled_at"] = rowimport.Cell{SQLNull: true}, rowimport.Cell{SQLNull: true}
	payment["data"], payment["metadata"] = rowimport.Cell{SQLNull: true}, rowimport.Cell{SQLNull: true}
	for i := 0; i < 2; i++ {
		if err := run(converter, "payment", payment, snapshot, true); err != nil {
			t.Fatal("nullable payment import/replay failed", err)
		}
	}
	if count("payments", "id", "import-payment") != 1 || count("migration_payment_dispositions", "source_id", "import-payment") != 1 {
		t.Fatal("nullable payment missing its hold")
	}
	for _, test := range []struct{ id, update string }{
		{"import-tampered-scalar", "UPDATE payment_sessions SET status='error' WHERE id=?"},
		{"import-tampered-final-decimal", "UPDATE payment_sessions SET amount=1.230000000000000001 WHERE id=?"},
		{"import-tampered-cipher", "UPDATE payment_sessions SET data='plaintext-not-envelope' WHERE id=?"},
		{"import-missing-target", "DELETE FROM payment_sessions WHERE id=?"},
		{"import-tampered-digest", "UPDATE migration_payment_dispositions SET target_row_sha256=UNHEX(REPEAT('00',32)) WHERE source_id=?"},
	} {
		source := paymentImportFixture(test.id)
		if err := run(converter, "payment_session", source, snapshot, true); err != nil {
			t.Fatal("conflict fixture import failed", err)
		}
		if _, err := db.ExecContext(ctx, test.update, test.id); err != nil {
			t.Fatal("artificial conflict mutation failed")
		}
		if err := run(converter, "payment_session", source, snapshot, true); err != rowimport.ErrPaymentConflict {
			t.Fatal("persisted evidence conflict accepted", err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE migration_payment_dispositions SET disposition='allow' WHERE source_id='import-session'"); err == nil {
		t.Fatal("payment ledger permits allow")
	}
	// A separately valid re-encryption of identical data is still a persisted
	// target change. Authentication alone must not bypass target-row digest.
	id := "import-valid-reseal"
	source := paymentImportFixture(id)
	if err := run(converter, "payment_session", source, snapshot, true); err != nil {
		t.Fatal("valid reseal fixture import failed")
	}
	identity := rowimport.PaymentDataIdentity{SourceTable: "payment_session", SourceID: id, SessionID: id, ProviderID: "pp_mollie-hosted-checkout_mollie", CurrencyCode: "eur", Amount: source["amount"].Raw}
	resealed, err := rowimport.SealPaymentData(ring, identity, source["data"].Raw)
	if err != nil {
		t.Fatal("valid reseal fixture unavailable")
	}
	if _, err := db.ExecContext(ctx, "UPDATE payment_sessions SET data=? WHERE id=?", resealed, id); err != nil {
		t.Fatal("valid reseal fixture mutation failed")
	}
	if err := run(converter, "payment_session", source, snapshot, true); err != rowimport.ErrPaymentConflict {
		t.Fatal("valid but changed ciphertext bypassed target digest")
	}
}
