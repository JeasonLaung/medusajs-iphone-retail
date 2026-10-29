package identity_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/paymentimport"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOwnedPaymentImport(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned SQL fixture required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("payment import fixture unavailable")
	}
	defer root.Close()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE payment_import_rehearsal"); err != nil {
		t.Fatal("payment import database unavailable")
	}
	defer func() {
		if _, err := root.Exec("DROP DATABASE payment_import_rehearsal"); err != nil {
			t.Error("payment import cleanup failed")
		}
	}()
	cfg.DBName = "payment_import_rehearsal"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("payment import connection unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("payment import plan unavailable")
	}
	applied, err := schema.Apply(ctx, db, plan)
	if err != nil || applied.Applied != 39 {
		t.Fatal("payment import migrations failed")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal("artificial payment SQL failed")
		}
	}
	count := func(table string, want int) {
		t.Helper()
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n) != nil || n != want {
			t.Fatal("payment import count differs: " + table)
		}
	}
	exec(`INSERT INTO payment_providers(id,is_enabled,created_at,updated_at) VALUES ('pp_system_default',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-provider',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	text := func(v string) rowimport.Cell { b, _ := json.Marshal(v); return rowimport.Cell{Raw: b} }
	raw := func(v string) rowimport.Cell { return rowimport.Cell{Raw: json.RawMessage(v)} }
	newRow := func(fields, id string) rowimport.SourceRow {
		r := rowimport.SourceRow{}
		for _, f := range strings.Fields(fields) {
			r[f] = rowimport.Cell{SQLNull: true}
		}
		r["id"] = text(id)
		r["created_at"] = text("2026-10-28T00:00:00.123456Z")
		r["updated_at"] = r["created_at"]
		return r
	}
	collection := newRow("id currency_code amount raw_amount authorized_amount raw_authorized_amount captured_amount raw_captured_amount refunded_amount raw_refunded_amount created_at updated_at deleted_at completed_at status metadata", "collection")
	collection["currency_code"], collection["status"] = text("eur"), text("completed")
	collection["amount"], collection["raw_amount"] = raw("9007199254740993.000000000000000001"), raw(`{"value":"9007199254740993.000000000000000001","precision":38}`)
	session := newRow("id currency_code amount raw_amount provider_id data context status authorized_at payment_collection_id metadata created_at updated_at deleted_at", "session")
	session["currency_code"], session["provider_id"], session["data"], session["context"], session["status"], session["payment_collection_id"] = text("eur"), text("pp_system_default"), raw(` { } `), raw(`{"customer":{"id":"old-subject","addresses":[],"account_holders":[]}}`), text("captured"), text("collection")
	session["amount"], session["raw_amount"] = collection["amount"], collection["raw_amount"]
	payment := newRow("id amount raw_amount currency_code provider_id data created_at updated_at deleted_at captured_at canceled_at payment_collection_id payment_session_id metadata", "payment")
	payment["amount"], payment["raw_amount"], payment["currency_code"], payment["provider_id"], payment["data"], payment["payment_collection_id"], payment["payment_session_id"] = collection["amount"], collection["raw_amount"], text("eur"), text("pp_system_default"), raw(`{}`), text("collection"), text("session")
	holder := newRow("id provider_id external_id email data metadata created_at updated_at deleted_at", "holder")
	holder["provider_id"], holder["external_id"], holder["email"], holder["data"] = text("pp_system_default"), text("old-subject"), text("same@example.invalid"), raw(`{}`)
	capture := newRow("id amount raw_amount payment_id created_at updated_at deleted_at created_by metadata", "capture")
	capture["amount"], capture["raw_amount"], capture["payment_id"], capture["metadata"] = collection["amount"], collection["raw_amount"], text("payment"), raw(`null`)
	refund := newRow("id amount raw_amount payment_id created_at updated_at deleted_at created_by metadata refund_reason_id note", "refund")
	refund["amount"], refund["raw_amount"], refund["payment_id"], refund["refund_reason_id"] = raw("0.000000000000000001"), raw(`{"value":"0.000000000000000001","precision":38}`), text("payment"), text("retired-reason")
	reason := newRow("id label description metadata created_at updated_at deleted_at code", "reason")
	reason["label"], reason["code"] = text("Original reason"), text("reason-code")
	batch := map[string][]rowimport.SourceRow{"account_holder": {holder}, "payment_collection": {collection}, "payment_session": {session}, "payment": {payment}, "capture": {capture}, "refund": {refund}, "refund_reason": {reason}, "payment_collection_payment_providers": {
		{"payment_collection_id": text("collection"), "payment_provider_id": text("pp_system_default")},
		{"payment_collection_id": text("collection"), "payment_provider_id": text("other-provider")},
	}}
	original, _ := json.Marshal(batch)
	const snapshot = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	key := bytes.Repeat([]byte{45}, 32)
	ring, _ := protected.New("old", map[string][]byte{"old": key})
	importer, _ := paymentimport.New(ring)
	run := func(i *paymentimport.Importer, b map[string][]rowimport.SourceRow, sha string, commit bool) (paymentimport.Result, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return paymentimport.Result{}, err
		}
		defer tx.Rollback()
		r, err := i.Import(ctx, tx, sha, b)
		if err != nil {
			return r, err
		}
		if commit {
			err = tx.Commit()
		}
		return r, err
	}
	tables := []string{"payment_account_holders", "payment_collections", "payment_sessions", "payments", "payment_captures", "payment_refunds", "payment_refund_reasons", "payment_collection_providers", "migration_payment_rows", "migration_payment_dispositions", "migration_holder_dispositions", "payment_migration_reference_holds", "unique_key_owners", "unique_key_claims"}
	empty := func() {
		for _, name := range tables {
			count(name, 0)
		}
	}
	r, err := run(importer, batch, snapshot, false)
	if err != nil || r.Rows != 9 || r.OrdinaryRows != 6 || r.ProtectedRows != 3 || r.ObservedMissingReferences != 4 {
		t.Fatal("payment rollback staging failed")
	}
	empty()
	exec(`CREATE TRIGGER reject_payment_reference BEFORE INSERT ON payment_migration_reference_holds FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial reference failure'`)
	if r, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrStorage) || r != (paymentimport.Result{}) {
		t.Fatal("late reference failure accepted or returned partial success")
	}
	empty()
	exec("DROP TRIGGER reject_payment_reference")
	payment["unknown"] = raw("true")
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrPolicy) {
		t.Fatal("unknown protected field accepted")
	}
	empty()
	delete(payment, "unknown")
	oldTx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("old payment snapshot unavailable")
	}
	defer oldTx.Rollback()
	var before int
	if oldTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM migration_payment_rows").Scan(&before) != nil || before != 0 {
		t.Fatal("old payment snapshot invalid")
	}
	var wg sync.WaitGroup
	results := make(chan paymentimport.Result, 8)
	errs := make(chan error, 8)
	for j := 0; j < 8; j++ {
		wg.Go(func() { r, e := run(importer, batch, snapshot, true); results <- r; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("parallel payment batches failed")
		}
	}
	fresh, replayed := 0, 0
	for r := range results {
		if r.Rows != 9 || r.ObservedMissingReferences != 4 {
			t.Fatal("parallel payment result differs")
		}
		if r.ReplayedRows == 0 {
			fresh++
		} else if r.ReplayedRows == 9 {
			replayed++
		}
	}
	if fresh != 1 || replayed != 7 {
		t.Fatal("payment batch not idempotent")
	}
	r, err = importer.Import(ctx, oldTx, snapshot, batch)
	if err != nil || r.ReplayedRows != 9 || oldTx.Commit() != nil {
		t.Fatal("old snapshot hid payment receipts")
	}
	count("migration_payment_rows", 9)
	count("migration_payment_dispositions", 2)
	count("migration_holder_dispositions", 1)
	count("payment_migration_reference_holds", 4)
	count("payment_collection_providers", 2)
	count("admin_users", 0)
	count("admin_sessions", 0)
	count("historical_customer_subjects", 0)
	var protectedHash, ordinaryHash int
	if db.QueryRowContext(ctx, "SELECT SUM(target_row_sha256 IS NULL),SUM(target_row_sha256 IS NOT NULL) FROM migration_payment_rows").Scan(&protectedHash, &ordinaryHash) != nil || protectedHash != 3 || ordinaryHash != 6 {
		t.Fatal("protected receipt hash ownership differs")
	}
	fingerprint := func() string {
		var v string
		if db.QueryRowContext(ctx, "SELECT CONCAT((SELECT data FROM payments WHERE id='payment'),(SELECT context FROM payment_sessions WHERE id='session'),(SELECT data FROM payment_account_holders WHERE id='holder'))").Scan(&v) != nil {
			t.Fatal("payment ciphertext fingerprint unavailable")
		}
		return v
	}
	cipher := fingerprint()
	rotatedRing, _ := protected.New("new", map[string][]byte{"new": bytes.Repeat([]byte{46}, 32), "old": key})
	rotated, _ := paymentimport.New(rotatedRing)
	if r, err := run(rotated, batch, snapshot, true); err != nil || r.ReplayedRows != 9 || fingerprint() != cipher {
		t.Fatal("rotation replay replaced original ciphertext")
	}
	missingRing, _ := protected.New("new", map[string][]byte{"new": bytes.Repeat([]byte{46}, 32)})
	missing, _ := paymentimport.New(missingRing)
	if _, err := run(missing, batch, snapshot, true); !errors.Is(err, paymentimport.ErrConflict) || fingerprint() != cipher {
		t.Fatal("missing old key accepted")
	}
	if _, err := run(importer, batch, strings.Repeat("b", 64), true); !errors.Is(err, paymentimport.ErrConflict) {
		t.Fatal("payment snapshot changed")
	}
	reason["label"] = text("changed")
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrConflict) {
		t.Fatal("payment source changed")
	}
	reason["label"] = text("Original reason")
	exec("UPDATE payment_captures SET amount=9007199254740993.000000000000000002 WHERE id='capture'")
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrConflict) {
		t.Fatal("payment decimal drift accepted")
	}
	exec("UPDATE payment_captures SET amount=9007199254740993.000000000000000001 WHERE id='capture'")
	batch["payment_collection_payment_providers"] = append(batch["payment_collection_payment_providers"], batch["payment_collection_payment_providers"][0])
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrPolicy) {
		t.Fatal("duplicate composite payment identity accepted")
	}
	batch["payment_collection_payment_providers"] = batch["payment_collection_payment_providers"][:2]
	delete(batch, "refund_reason")
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrPolicy) {
		t.Fatal("missing payment table accepted")
	}
	batch["refund_reason"] = []rowimport.SourceRow{reason}
	exec("UPDATE payment_migration_reference_holds SET reason='parent_domain_unavailable' WHERE column_name='refund_reason_id'")
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrConflict) {
		t.Fatal("payment hold drift accepted")
	}
	exec("UPDATE payment_migration_reference_holds SET reason='missing_parent' WHERE column_name='refund_reason_id'")
	if _, err := db.ExecContext(ctx, "UPDATE payment_migration_reference_holds SET released=TRUE"); err == nil {
		t.Fatal("payment reference hold released")
	}
	if r, err := run(importer, batch, snapshot, true); err != nil || r.ReplayedRows != 9 {
		t.Fatal("restored payment replay failed")
	}

	// Batch receipts must never replace dedicated protected target authenticity.
	if _, err := db.ExecContext(ctx, "UPDATE migration_payment_rows SET target_row_sha256=NULL WHERE source_table=_binary'capture'"); err == nil {
		t.Fatal("ordinary payment receipt lost its target hash")
	}
	if _, err := db.ExecContext(ctx, "UPDATE migration_payment_rows SET target_row_sha256=UNHEX(REPEAT('0',64)) WHERE source_table=_binary'payment'"); err == nil {
		t.Fatal("protected payment receipt accepted a second target hash")
	}
	exec("UPDATE payment_runtime_schema SET reads_enabled=FALSE WHERE id=1")
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrPolicy) {
		t.Fatal("payment import ignored disabled schema")
	}
	exec("UPDATE payment_runtime_schema SET reads_enabled=TRUE WHERE id=1")
	duplicateHolder := rowimport.SourceRow{}
	for k, v := range holder {
		duplicateHolder[k] = v
	}
	duplicateHolder["id"] = text("holder-collision")
	batch["account_holder"] = append(batch["account_holder"], duplicateHolder)
	if _, err := run(importer, batch, snapshot, true); !errors.Is(err, paymentimport.ErrConflict) {
		t.Fatal("historical holder uniqueness lost through batch import")
	}
	batch["account_holder"] = batch["account_holder"][:1]
	count("migration_payment_rows", 9)
	count("migration_holder_dispositions", 1)
	count("payment_account_holders", 1)
	after, _ := json.Marshal(batch)
	if !bytes.Equal(original, after) {
		t.Fatal("payment import mutated source")
	}
	t.Log(`PAYMENT_IMPORT_REHEARSAL {"migrations":39,"tables":8,"columns":83,"rows":9,"ordinary_rows":6,"protected_rows":3,"reference_holds":4,"operation_holds":3,"concurrent_batches":8,"single_fresh_batch":1,"replayed_batches":7,"old_snapshot_replay":true,"rollback_atomic":true,"late_hold_failure_rollback":true,"composite_identity_preserved":true,"decimal_drift_rejected":true,"source_snapshot_hold_drift_rejected":true,"original_ciphertext_rotation_replay":true,"missing_key_rejected":true,"no_account_granted":true,"source_unchanged":true,"receipt_hash_policy_enforced":true,"disabled_schema_rejected":true,"holder_uniqueness_conflict_atomic":true,"provider_called":false}`)
}
