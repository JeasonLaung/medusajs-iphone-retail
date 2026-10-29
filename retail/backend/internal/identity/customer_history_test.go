package identity_test

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

	"github.com/eurorefurb/retail/internal/customerimport"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
)

func TestOwnedCustomerHistory(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned SQL fixture required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("customer fixture server unavailable")
	}
	defer root.Close()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE customer_history_rehearsal"); err != nil {
		t.Fatal("customer fixture database unavailable")
	}
	defer func() {
		if _, err := root.ExecContext(context.Background(), "DROP DATABASE `customer_history_rehearsal`"); err != nil {
			t.Error("owned completed fixture database cleanup failed")
		}
	}()
	cfg.DBName = "customer_history_rehearsal"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("customer fixture pool unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("customer fixture plan invalid")
	}
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 41 {
		t.Fatal("customer fixture migrations failed")
	}
	key := bytes.Repeat([]byte{51}, 32)
	ring, _ := protected.New("first", map[string][]byte{"first": key})
	importer, _ := customerimport.New(ring)
	text := func(s string) rowimport.Cell { b, _ := json.Marshal(s); return rowimport.Cell{Raw: b} }
	row := rowimport.SourceRow{"id": text("old-business-subject"), "company_name": {SQLNull: true}, "first_name": text(" Original 姓名 "), "last_name": text(""), "email": text("same@example.invalid"), "phone": {SQLNull: true}, "has_account": {Raw: json.RawMessage(`true`)}, "metadata": {Raw: json.RawMessage(` { } `)}, "created_at": text("2026-10-28T00:00:00.123456Z"), "updated_at": text("2026-10-28T00:00:00.123456Z"), "deleted_at": {SQLNull: true}, "created_by": text("old-actor")}
	const snapshot = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := func(i *customerimport.Importer, sha string, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = i.Import(ctx, tx, sha, row); err != nil {
			return err
		}
		if d, err := customerimport.BindingDisposition(ctx, tx, "old-business-subject"); err != nil || d != "excluded" {
			return errors.New("binding exclusion unavailable")
		}
		if commit {
			return tx.Commit()
		}
		return nil
	}
	exec := func(query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal("customer fixture SQL failed")
		}
	}
	count := func(table string, want int) {
		t.Helper()
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n) != nil || n != want {
			t.Fatal("customer fixture count differs: " + table)
		}
	}
	if err := run(importer, snapshot, false); err != nil {
		t.Fatal("customer rollback staging failed")
	}
	count("historical_customer_subjects", 0)
	count("migration_customer_dispositions", 0)
	exec(`CREATE TRIGGER reject_customer_row BEFORE INSERT ON historical_customer_subjects FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial subject failure'`)
	if err := run(importer, snapshot, true); !errors.Is(err, customerimport.ErrStorage) {
		t.Fatal("customer row failure accepted")
	}
	count("migration_customer_dispositions", 0)
	exec("DROP TRIGGER reject_customer_row")
	oldTx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("customer old snapshot unavailable")
	}
	defer oldTx.Rollback()
	var before int
	if oldTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM migration_customer_dispositions").Scan(&before) != nil || before != 0 {
		t.Fatal("customer old snapshot invalid")
	}
	var wg sync.WaitGroup
	errorsSeen := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Go(func() { errorsSeen <- run(importer, snapshot, true) })
	}
	wg.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal("customer concurrent import failed")
		}
	}
	if err := importer.Import(ctx, oldTx, snapshot, row); err != nil || oldTx.Commit() != nil {
		t.Fatal("customer old snapshot replay failed")
	}
	count("historical_customer_subjects", 1)
	count("migration_customer_dispositions", 1)
	count("admin_users", 0)
	count("admin_sessions", 0)
	var sealed []byte
	var name string
	var wasAccount bool
	if db.QueryRowContext(ctx, "SELECT metadata,first_name,legacy_had_account FROM historical_customer_subjects WHERE id='old-business-subject'").Scan(&sealed, &name, &wasAccount) != nil || name != " Original 姓名 " || !wasAccount {
		t.Fatal("customer business facts changed")
	}
	plain, err := rowimport.OpenCustomerMetadata(ring, "old-business-subject", sealed)
	if err != nil || string(plain) != ` { } ` {
		t.Fatal("customer binary envelope readback failed")
	}
	clear(plain)
	rotated, _ := protected.New("second", map[string][]byte{"first": key, "second": bytes.Repeat([]byte{52}, 32)})
	rotatedImporter, _ := customerimport.New(rotated)
	if err := run(rotatedImporter, snapshot, true); err != nil {
		t.Fatal("customer key rotation replay failed")
	}
	var after []byte
	if db.QueryRowContext(ctx, "SELECT metadata FROM historical_customer_subjects WHERE id='old-business-subject'").Scan(&after) != nil || !bytes.Equal(after, sealed) {
		t.Fatal("customer replay resealed persisted evidence")
	}
	missing, _ := protected.New("second", map[string][]byte{"second": bytes.Repeat([]byte{52}, 32)})
	missingImporter, _ := customerimport.New(missing)
	if err := run(missingImporter, snapshot, true); !errors.Is(err, customerimport.ErrConflict) {
		t.Fatal("missing original customer key accepted")
	}
	if err := run(importer, strings.Repeat("b", 64), true); !errors.Is(err, customerimport.ErrConflict) {
		t.Fatal("customer snapshot change accepted")
	}
	row["first_name"] = text("Changed source")
	if err := run(importer, snapshot, true); !errors.Is(err, customerimport.ErrConflict) {
		t.Fatal("customer source edit accepted")
	}
	row["first_name"] = text(" Original 姓名 ")
	exec("UPDATE historical_customer_subjects SET first_name='Changed target' WHERE id='old-business-subject'")
	if err := run(importer, snapshot, true); !errors.Is(err, customerimport.ErrConflict) {
		t.Fatal("customer target edit accepted")
	}
	exec("UPDATE historical_customer_subjects SET first_name=' Original 姓名 ' WHERE id='old-business-subject'")
	if _, err := db.ExecContext(ctx, "UPDATE migration_customer_dispositions SET disposition='active'"); err == nil {
		t.Fatal("old account binding activated")
	}
	if _, err := db.ExecContext(ctx, "UPDATE migration_customer_dispositions SET reason='automatic-email-match'"); err == nil {
		t.Fatal("historical binding reason changed")
	}
	row["metadata"] = rowimport.Cell{Raw: json.RawMessage(`{"token":"old"}`)}
	if err := run(importer, snapshot, true); !errors.Is(err, customerimport.ErrPolicy) {
		t.Fatal("old token metadata imported")
	}
	count("admin_users", 0)
	count("historical_customer_subjects", 1)
	count("migration_customer_dispositions", 1)
	t.Log(`CUSTOMER_HISTORY_REHEARSAL {"migrations":41,"concurrent_imports":8,"persisted_subjects":1,"binding_exclusions":1,"created_login_accounts":0,"binary_metadata_roundtrip":true,"rollback_atomic":true,"row_failure_rollback":true,"old_snapshot_replay":true,"rotation_preserves_ciphertext":true,"missing_key_rejected":true,"source_snapshot_target_change_rejected":true,"account_binding_activation_rejected":true,"unknown_metadata_rejected":true}`)
}
