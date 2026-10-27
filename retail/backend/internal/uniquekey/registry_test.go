package uniquekey

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func ptr(s string) *string { return &s }

func TestTupleEncoding(t *testing.T) {
	for _, pair := range [][2][]*string{
		{{ptr("ab"), ptr("c")}, {ptr("a"), ptr("bc")}},
		{{ptr("A")}, {ptr("a")}}, {{ptr("a")}, {ptr("a ")}},
		{{ptr("é")}, {ptr("e\u0301")}}, {{ptr("")}, {ptr("\x00")}},
	} {
		a, _ := encode(pair[0])
		b, _ := encode(pair[1])
		if bytes.Equal(a, b) {
			t.Fatal("distinct full tuples collapsed")
		}
	}
	if b, err := encode([]*string{nil}); b != nil || err != nil {
		t.Fatal("NULL must not claim")
	}
	if _, err := encode([]*string{ptr(string([]byte{0xff}))}); err != ErrInvalid {
		t.Fatal("invalid UTF8 accepted")
	}
}

func TestRegistryMySQL(t *testing.T) {
	if os.Getenv("UNIQUEKEY_TEST") != "1" {
		t.Skip("isolated MySQL fixture only")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "type_fixture"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("fixture connection config failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(12)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, filename := range []string{"0002_unique_key_registry.sql", "0003_unique_key_secret_scopes.sql"} {
		ddl, err := migrations.Files.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(string(ddl), ";") {
			if strings.TrimSpace(stmt) != "" {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatal("fixture DDL failed", err)
				}
			}
		}
	}
	change := func(scope, owner, value string) Change { return Change{scope, owner, true, []*string{ptr(value)}} }
	run := func(changes []Change, collision bool, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if collision {
			err = apply(ctx, tx, changes, func([]byte) [32]byte { return [32]byte{} })
		} else {
			err = Apply(ctx, tx, changes)
		}
		if err != nil {
			return err
		}
		if commit {
			return tx.Commit()
		}
		return nil
	}
	ok := func(cs ...Change) {
		t.Helper()
		if err := run(cs, false, true); err != nil {
			t.Fatal(err)
		}
	}
	conflict := func(cs ...Change) {
		t.Helper()
		if err := run(cs, false, true); !errors.Is(err, ErrConflict) {
			t.Fatal("expected exact conflict", err)
		}
	}

	t.Run("full_value_and_collision", func(t *testing.T) {
		long := strings.Repeat("🚀", 1024)
		for _, c := range []Change{change("collision", "one", long+"A"), change("collision", "two", long+"B")} {
			if err := run([]Change{c}, true, true); err != nil {
				t.Fatal(err)
			}
		}
		if err := run([]Change{change("collision", "three", long+"A")}, true, true); err != ErrConflict {
			t.Fatal("collision check lost complete value", err)
		}
	})
	t.Run("case_trailing_space_and_scopes", func(t *testing.T) {
		ok(change("text", "a", "Name"), change("text", "b", "name"), change("text", "c", "name "), change("other", "a", "name"))
		conflict(change("text", "d", "name"))
	})
	t.Run("null_excluded_release_restore", func(t *testing.T) {
		ok(change("life", "a", "same"))
		x := change("life", "a", "same")
		x.Active = false
		ok(x)
		ok(change("life", "b", "same"))
		conflict(change("life", "a", "same"))
		x = change("life", "b", "same")
		x.Components = []*string{nil}
		ok(x)
		ok(change("life", "a", "same"))
		ok(Change{"life", "null", true, []*string{nil}})
	})
	t.Run("atomic_swap_and_failure_rollback", func(t *testing.T) {
		ok(change("swap", "a", "A"), change("swap", "b", "B"))
		ok(change("swap", "a", "B"), change("swap", "b", "A"))
		conflict(change("swap", "a", "A"))
		conflict(change("swap", "c", "B")) // failed replacement preserved a's B
		if err := run([]Change{change("rollback", "a", "value")}, false, false); err != nil {
			t.Fatal(err)
		}
		ok(change("rollback", "b", "value"))
	})
	t.Run("same_owner_retry_and_duplicate_batch", func(t *testing.T) {
		ok(change("retry", "a", "value"))
		ok(change("retry", "a", "value"))
		if err := run([]Change{change("retry", "a", "x"), change("retry", "a", "y")}, false, true); err != ErrInvalid {
			t.Fatal("duplicate owner batch accepted")
		}
	})
	t.Run("repeatable_read_old_snapshot_cannot_hide_new_claim", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM unique_key_claims WHERE scope='snapshot'").Scan(&count); err != nil {
			t.Fatal(err)
		}
		ok(change("snapshot", "new", "claimed-after-snapshot"))
		if err := Apply(ctx, tx, []Change{change("snapshot", "old", "claimed-after-snapshot")}); err != ErrConflict {
			t.Fatal("stale snapshot bypassed current claim", err)
		}
	})
	t.Run("duplicate_full_values_in_one_batch_roll_back", func(t *testing.T) {
		conflict(change("batch", "a", "same"), change("batch", "b", "same"))
		ok(change("batch", "c", "same"))
	})
	t.Run("concurrent_only_one_winner", func(t *testing.T) {
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				results <- run([]Change{change("race", string(rune('a'+i)), "last-item")}, false, true)
			}(i)
		}
		close(start)
		wg.Wait()
		close(results)
		winners := 0
		for err := range results {
			if err == nil {
				winners++
			} else if !errors.Is(err, ErrConflict) {
				t.Fatal("unexpected concurrent failure", err)
			}
		}
		if winners != 1 {
			t.Fatal("concurrent uniqueness violated", winners)
		}
	})
	t.Run("source_option_adapter_and_domain_row_share_transaction", func(t *testing.T) {
		write := func(id, title string, commit bool) error {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			err = ApplySourceRow(ctx, tx, "catalog_options", row(map[string]any{"id": id, "title": title, "deleted_at": nil, "is_exclusive": false}))
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO relational_fixture.catalog_options(id,title,created_at,updated_at,is_exclusive) VALUES(?,?, '2026-10-27','2026-10-27',false)", id, title)
			if err != nil {
				return err
			}
			if commit {
				return tx.Commit()
			}
			return nil
		}
		if err := write("option-a", "Long exact title ", true); err != nil {
			t.Fatal(err)
		}
		if err := write("option-b", "Long exact title ", true); err != ErrConflict {
			t.Fatal("source adapter duplicate escaped", err)
		}
		if err := write("option-c", "Long exact title", true); err != nil {
			t.Fatal("trailing space collapsed", err)
		}
		if err := write("option-d", "Rollback title", false); err != nil {
			t.Fatal(err)
		}
		if err := write("option-e", "Rollback title", true); err != nil {
			t.Fatal("claim not rolled back with business row", err)
		}
		var count int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relational_fixture.catalog_options WHERE id IN ('option-a','option-b','option-c','option-d','option-e')").Scan(&count); err != nil || count != 3 {
			t.Fatal("business rows mismatch", count, err)
		}
	})
	t.Run("protected_envelope_binary_sql_roundtrip_and_rotation", func(t *testing.T) {
		keys := map[string][]byte{"key1": bytes.Repeat([]byte{1}, 32), "key2": bytes.Repeat([]byte{2}, 32)}
		old, err := protected.New("key1", keys)
		if err != nil {
			t.Fatal(err)
		}
		next, err := protected.New("key2", keys)
		if err != nil {
			t.Fatal(err)
		}
		binding := protected.Context{Purpose: "merchant-config", Table: "mail_accounts", Column: "api_key", Row: "artificial-row"}
		plain := []byte("人工 configuration \x00\xff ")
		sealed, err := old.Seal(binding, plain)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "CREATE TABLE protected_envelope_fixture(id INT PRIMARY KEY, value_bytes LONGBLOB NULL) ENGINE=InnoDB"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO protected_envelope_fixture VALUES(1,?),(2,NULL)", sealed); err != nil {
			t.Fatal("binary persistence failed")
		}
		var stored []byte
		if err := db.QueryRowContext(ctx, "SELECT value_bytes FROM protected_envelope_fixture WHERE id=1").Scan(&stored); err != nil {
			t.Fatal(err)
		}
		opened, err := next.Open(binding, stored)
		if err != nil || !bytes.Equal(opened, plain) {
			t.Fatal("persisted value changed", err)
		}
		wrong := binding
		wrong.Row = "different-row"
		if _, err := next.Open(wrong, stored); err != protected.ErrEnvelope {
			t.Fatal("SQL row substitution accepted")
		}
		rotated, err := next.Seal(binding, opened)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE protected_envelope_fixture SET value_bytes=? WHERE id=1", rotated); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRowContext(ctx, "SELECT value_bytes FROM protected_envelope_fixture WHERE id=1").Scan(&stored); err != nil || !bytes.Equal(stored, sealed) {
			t.Fatal("rotation rollback changed ciphertext")
		}
		if _, err = db.ExecContext(ctx, "UPDATE protected_envelope_fixture SET value_bytes=? WHERE id=1", rotated); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRowContext(ctx, "SELECT value_bytes FROM protected_envelope_fixture WHERE id=1").Scan(&stored); err != nil {
			t.Fatal(err)
		}
		onlyNew, _ := protected.New("key2", map[string][]byte{"key2": keys["key2"]})
		opened, err = onlyNew.Open(binding, stored)
		if err != nil || !bytes.Equal(opened, plain) {
			t.Fatal("rotation failed", err)
		}
		if err = db.QueryRowContext(ctx, "SELECT value_bytes FROM protected_envelope_fixture WHERE id=2").Scan(&stored); err != nil || stored != nil {
			t.Fatal("SQL NULL changed")
		}
		if _, err = onlyNew.Open(binding, stored); err != protected.ErrEnvelope {
			t.Fatal("SQL NULL treated as empty secret")
		}
	})
	t.Run("protected_source_claims_hmac_envelopes_and_fail_closed", func(t *testing.T) {
		ring, _ := protected.New("enc1", map[string][]byte{"enc1": bytes.Repeat([]byte{3}, 32)})
		registry, _ := NewSecretRegistry(ring, "idx1", bytes.Repeat([]byte{4}, 32))
		var document struct {
			Plans []sourcePlan `json:"plans"`
		}
		if err := json.Unmarshal(sourcePlans, &document); err != nil {
			t.Fatal(err)
		}
		scope := ""
		for _, p := range document.Plans {
			if p.Protected {
				scope = p.Scope
			}
		}
		if scope == "" {
			t.Fatal("missing protected scope")
		}
		runSecret := func(s *SecretRegistry, owner, email, token string, deleted, commit bool) error {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			var deletedAt any
			if deleted {
				deletedAt = "2026-10-27T00:00:00Z"
			}
			sealed, err := ring.Seal(protected.Context{Purpose: "business-capability", Table: "mail_contacts", Column: "unsubscribe_token", Row: owner}, []byte(token))
			if err != nil {
				return err
			}
			err = s.ApplySourceRow(ctx, tx, "mail_contacts", row(map[string]any{"id": owner, "email": email, "deleted_at": deletedAt}), map[string][]byte{"unsubscribe_token": sealed})
			if err != nil {
				return err
			}
			var sqlDeleted any
			if deleted {
				sqlDeleted = "2026-10-27 00:00:00"
			}
			_, err = tx.ExecContext(ctx, "INSERT INTO relational_fixture.mail_contacts(id,email,name,subscribed,consent_source,consent_at,suppressed,unsubscribe_token,created_at,updated_at,deleted_at) VALUES(?,?,'artificial',true,'fixture',NULL,false,?,'2026-10-27','2026-10-27',?) ON DUPLICATE KEY UPDATE email=?,unsubscribe_token=?,deleted_at=?", owner, email, sealed, sqlDeleted, email, sealed, sqlDeleted)
			if err != nil {
				return err
			}
			if commit {
				return tx.Commit()
			}
			return nil
		}
		must := func(s *SecretRegistry, owner, email, token string, deleted bool) {
			t.Helper()
			if err := runSecret(s, owner, email, token, deleted, true); err != nil {
				t.Fatal(err)
			}
		}
		// Force HMAC collisions without exposing this switch outside the package.
		registry.testDigest = func(string, []byte) [32]byte { return [32]byte{} }
		must(registry, "secret-a", "a@fixture.test", "private artificial token A", false)
		must(registry, "secret-b", "b@fixture.test", "private artificial token B", false)
		if err := runSecret(registry, "secret-c", "c@fixture.test", "private artificial token A", false, true); err != ErrConflict {
			t.Fatal("full secret duplicate accepted", err)
		}
		if err := runSecret(registry, "secret-c", "a@fixture.test", "private artificial token C", false, true); err != ErrConflict {
			t.Fatal("email constraint skipped", err)
		}
		var stored []byte
		if err := db.QueryRowContext(ctx, "SELECT full_value FROM unique_key_claims WHERE scope=? AND owner='secret-a'", scope).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(stored, []byte("private artificial token")) {
			t.Fatal("secret plaintext persisted")
		}
		opened, err := ring.Open(claimContext(scope, "secret-a"), stored)
		if err != nil || !bytes.Contains(opened, []byte("private artificial token A")) {
			t.Fatal("claim not encrypted", err)
		}
		wrong, _ := NewSecretRegistry(ring, "idx1", bytes.Repeat([]byte{5}, 32))
		if err := runSecret(wrong, "secret-d", "d@fixture.test", "new-token", false, true); err != ErrProtected {
			t.Fatal("wrong HMAC key silently changed bucket", err)
		}
		wrongID, _ := NewSecretRegistry(ring, "idx2", bytes.Repeat([]byte{4}, 32))
		if err := runSecret(wrongID, "secret-d", "d@fixture.test", "new-token", false, true); err != ErrProtected {
			t.Fatal("different HMAC ID accepted", err)
		}
		missingRing, _ := protected.New("enc2", map[string][]byte{"enc2": bytes.Repeat([]byte{6}, 32)})
		missing, _ := NewSecretRegistry(missingRing, "idx1", bytes.Repeat([]byte{4}, 32))
		if err := runSecret(missing, "secret-d", "d@fixture.test", "new-token", false, true); err != ErrProtected {
			t.Fatal("missing envelope key accepted", err)
		}
		if err := runSecret(registry, "secret-d", "d@fixture.test", "rollback-token", false, false); err != nil {
			t.Fatal(err)
		}
		must(registry, "secret-e", "e@fixture.test", "rollback-token", false)
		must(registry, "secret-a", "a@fixture.test", "private artificial token A", true)
		must(registry, "secret-c", "c@fixture.test", "private artificial token A", false)
		if err := runSecret(registry, "secret-a", "a@fixture.test", "private artificial token A", false, true); err != ErrConflict {
			t.Fatal("secret restore conflict escaped", err)
		}
		var domainToken []byte
		if err := db.QueryRowContext(ctx, "SELECT unsubscribe_token FROM relational_fixture.mail_contacts WHERE id='secret-c'").Scan(&domainToken); err != nil {
			t.Fatal(err)
		}
		plainToken, err := ring.Open(protected.Context{Purpose: "business-capability", Table: "mail_contacts", Column: "unsubscribe_token", Row: "secret-c"}, domainToken)
		if err != nil || string(plainToken) != "private artificial token A" {
			t.Fatal("domain secret not independently sealed", err)
		}
		var rolledBack int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relational_fixture.mail_contacts WHERE id='secret-d'").Scan(&rolledBack); err != nil || rolledBack != 0 {
			t.Fatal("failed transaction persisted domain row")
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		outcomes := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				owner := "secret-race-" + string(rune('a'+i))
				outcomes <- runSecret(registry, owner, owner+"@fixture.test", "concurrent-private-token", false, true)
			}(i)
		}
		close(start)
		wg.Wait()
		close(outcomes)
		winners := 0
		for err := range outcomes {
			if err == nil {
				winners++
			} else if err != ErrConflict {
				t.Fatal("secret race failed unexpectedly", err)
			}
		}
		if winners != 1 {
			t.Fatal("secret concurrent uniqueness failed", winners)
		}
		// Ordinary registry calls may never write plaintext under the protected scope.
		if err := run([]Change{change(scope, "bypass", "private")}, false, true); err != ErrProtected {
			t.Fatal("plain bypass accepted", err)
		}
		// A corrupt candidate in the same bucket must fail, never skip a row.
		corrupt := bytes.Clone(stored)
		corrupt[len(corrupt)-1] ^= 1
		if _, err := db.ExecContext(ctx, "UPDATE unique_key_claims SET full_value=? WHERE scope=? AND owner='secret-b'", corrupt, scope); err != nil {
			t.Fatal(err)
		}
		if err := runSecret(registry, "secret-f", "f@fixture.test", "different-token", false, true); err != ErrStorage {
			t.Fatal("corrupt candidate ignored", err)
		}
	})
	t.Run("strict_import_values_survive_actual_sql_without_coercion", func(t *testing.T) {
		amount, err := importvalue.Decimal38_18(json.RawMessage(`"99999999999999999999.123456789012345678"`))
		if err != nil {
			t.Fatal(err)
		}
		instant, err := importvalue.Instant(json.RawMessage(`"2026-10-27T08:00:00.123456+08:00"`))
		if err != nil {
			t.Fatal(err)
		}
		document := json.RawMessage(` {"n":9007199254740993,"scale":1.2300,"n":2} `)
		original, err := importvalue.JSONText(document)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, "CREATE TABLE strict_import_value_fixture(id INT PRIMARY KEY,amount DECIMAL(38,18) NOT NULL,instant DATETIME(6) NOT NULL,raw_json LONGTEXT CHECK(JSON_VALID(raw_json))) ENGINE=InnoDB"); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, "INSERT INTO strict_import_value_fixture VALUES(1,?,?,?)", amount, instant.Format("2006-01-02 15:04:05.000000"), original); err != nil {
			t.Fatal(err)
		}
		var gotAmount, gotInstant, gotJSON string
		if err = db.QueryRowContext(ctx, "SELECT CAST(amount AS CHAR),DATE_FORMAT(instant,'%Y-%m-%d %H:%i:%s.%f'),raw_json FROM strict_import_value_fixture WHERE id=1").Scan(&gotAmount, &gotInstant, &gotJSON); err != nil {
			t.Fatal(err)
		}
		if gotAmount != amount || gotInstant != "2026-10-27 00:00:00.123456" || gotJSON != original {
			t.Fatal("strict importer SQL values changed")
		}
	})
	t.Run("mapped_source_row_bound_sql_and_unique_claim_same_transaction", func(t *testing.T) {
		jsonCell := func(value string) rowimport.Cell { return rowimport.Cell{Raw: json.RawMessage(value)} }
		source := rowimport.SourceRow{
			"id": jsonCell(`"mapped-tag"`), "value": jsonCell(`" Exact tag "`),
			"metadata":   jsonCell(` {"n":9007199254740993,"n":1.2300} `),
			"created_at": jsonCell(`"2026-10-27T08:00:00.123456+08:00"`), "updated_at": jsonCell(`"2026-10-27T08:00:00.123456+08:00"`),
			"deleted_at": {SQLNull: true}, "external_id": {SQLNull: true},
		}
		bound, err := rowimport.Convert("product_tag", source)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err = ApplySourceRow(ctx, tx, bound.Table, row(map[string]any{"id": "mapped-tag", "value": " Exact tag ", "deleted_at": nil})); err != nil {
			t.Fatal(err)
		}
		var columns, markers []string
		for _, column := range bound.Columns {
			columns = append(columns, "`"+column+"`")
			markers = append(markers, "?")
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO relational_fixture.`"+bound.Table+"` ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", bound.Values...); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		var metadata, created, value string
		var deleted, external sql.NullString
		if err = db.QueryRowContext(ctx, "SELECT value,metadata,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s.%f'),deleted_at,external_id FROM relational_fixture.catalog_tags WHERE id='mapped-tag'").Scan(&value, &metadata, &created, &deleted, &external); err != nil {
			t.Fatal(err)
		}
		if value != " Exact tag " || metadata != string(source["metadata"].Raw) || created != "2026-10-27 00:00:00.123456" || deleted.Valid || external.Valid {
			t.Fatal("mapped row roundtrip mismatch")
		}
		// JSON document null remains a non-NULL text value, unlike SQL NULL.
		source["id"] = jsonCell(`"mapped-tag-null"`)
		source["value"] = jsonCell(`"JSON null tag"`)
		source["metadata"] = jsonCell(`null`)
		bound, err = rowimport.Convert("product_tag", source)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, "INSERT INTO relational_fixture.`"+bound.Table+"` ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", bound.Values...); err != nil {
			t.Fatal(err)
		}
		var isNull int
		if err = db.QueryRowContext(ctx, "SELECT metadata IS NULL,metadata FROM relational_fixture.catalog_tags WHERE id='mapped-tag-null'").Scan(&isNull, &metadata); err != nil || isNull != 0 || metadata != "null" {
			t.Fatal("JSON null collapsed to SQL NULL")
		}
	})
	t.Run("mapped_merchant_cipher_to_new_binary_sql_envelope", func(t *testing.T) { testMappedMerchantConfig(t, db, ctx) })
	t.Run("nested_paypal_credential_projection_sql_roundtrip_and_rollback", func(t *testing.T) { testNestedPayPalConfigSQL(t, db, ctx) })
	t.Run("historical_mail_and_disposition_atomic_replay_and_conflict", func(t *testing.T) { testHistoricalMailSQL(t, ctx) })
	t.Run("reviewed_payment_context_ascii_envelope_sql_roundtrip", func(t *testing.T) { testPaymentContextSQL(t, db, ctx) })
	t.Run("complete_payment_history_rows_sql_rollback_and_roundtrip", func(t *testing.T) { testPaymentHistorySQL(t, db, ctx) })
	t.Run("payment_history_atomic_hold_replay_conflict_and_concurrency", func(t *testing.T) { testPaymentHistoryImportSQL(t, ctx) })
}
