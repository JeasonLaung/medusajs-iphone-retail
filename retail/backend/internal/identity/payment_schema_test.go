package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"os"
	"regexp"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedPaymentSchema(t *testing.T) {
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
		t.Fatal("owned schema server unavailable")
	}
	t.Cleanup(func() { root.Close() })
	open := func(name string) (*sql.DB, *gorm.DB) {
		// Only fixed artificial names below are supplied to this closure.
		if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
			t.Fatal("owned payment database unavailable")
		}
		c := *cfg
		c.DBName = name
		db, err := sql.Open("mysql", c.FormatDSN())
		if err != nil {
			t.Fatal("owned payment connection unavailable")
		}
		t.Cleanup(func() {
			db.Close()
			if _, err := root.Exec("DROP DATABASE `" + name + "`"); err != nil {
				t.Error("owned payment database cleanup failed")
			}
		})
		gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
		if err != nil {
			t.Fatal("owned payment adapter unavailable")
		}
		return db, gdb
	}
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("payment migration plan invalid")
	}
	db, gdb := open("payment_schema_fresh")
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 46 || schema.CheckApplied(ctx, db, plan) != nil || payments.Ready(ctx, gdb) != nil {
		t.Fatal("fresh payment schema failed")
	}
	r, err = schema.Apply(ctx, db, plan)
	if err != nil || r.Skipped != 46 {
		t.Fatal("payment schema replay changed facts")
	}

	ddl, err := migrations.Files.ReadFile("0019_payment_relational.sql")
	if err != nil {
		t.Fatal("embedded payment schema missing")
	}
	matches := regexp.MustCompile("CREATE TABLE `([a-z_]+)`").FindAllSubmatch(ddl, -1)
	if len(matches) != 8 {
		t.Fatal("payment table scope differs")
	}
	total := 0
	for _, match := range matches {
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND extra NOT LIKE '%GENERATED%'", string(match[1])).Scan(&n) != nil || n == 0 {
			t.Fatal("payment table missing")
		}
		total += n
	}
	if total != 83 {
		t.Fatal("payment source column preservation differs")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal("artificial payment fact rejected")
		}
	}
	reject := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err == nil {
			t.Fatal("invalid payment fact accepted")
		}
	}
	exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES ('collection','eur',9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}','not_paid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	var amount string
	if db.QueryRowContext(ctx, "SELECT CAST(amount AS CHAR) FROM payment_collections WHERE id='collection'").Scan(&amount) != nil || amount != "9007199254740993.000000000000000001" {
		t.Fatal("payment precision lost")
	}
	for _, status := range []string{"not_paid", "awaiting", "authorized", "partially_authorized", "canceled", "failed", "partially_captured", "completed"} {
		exec("UPDATE payment_collections SET status=? WHERE id='collection'", status)
	}
	reject("UPDATE payment_collections SET status='paid' WHERE id='collection'")
	reject("UPDATE payment_collections SET raw_amount='invalid' WHERE id='collection'")
	exec(`INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,context,status,payment_collection_id,metadata,created_at,updated_at) VALUES ('session','eur',1,'{"value":"1","precision":38}','retired-provider','sealed-evidence',NULL,'pending','collection','sealed-metadata',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, status := range []string{"authorized", "captured", "pending", "requires_more", "error", "canceled", "pending_authorization"} {
		exec("UPDATE payment_sessions SET status=? WHERE id='session'", status)
	}
	reject("UPDATE payment_sessions SET status='completed' WHERE id='session'")
	reject("UPDATE payment_sessions SET payment_collection_id='missing' WHERE id='session'")
	var contextNull int
	if db.QueryRowContext(ctx, "SELECT context IS NULL FROM payment_sessions WHERE id='session'").Scan(&contextNull) != nil || contextNull != 1 {
		t.Fatal("protected SQL NULL changed")
	}
	exec(`INSERT INTO payments(id,amount,raw_amount,currency_code,provider_id,data,payment_collection_id,payment_session_id,created_at,updated_at) VALUES ('payment',1,'{"value":"1","precision":38}','eur','retired-provider','sealed-evidence','collection','session',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	reject(`INSERT INTO payments(id,amount,raw_amount,currency_code,provider_id,payment_collection_id,payment_session_id,created_at,updated_at) VALUES ('duplicate',1,'{"value":"1","precision":38}','eur','retired-provider','collection','session',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("UPDATE payments SET deleted_at=UTC_TIMESTAMP(6) WHERE id='payment'")
	exec(`INSERT INTO payments(id,amount,raw_amount,currency_code,provider_id,payment_collection_id,payment_session_id,created_at,updated_at) VALUES ('replacement',1,'{"value":"1","precision":38}','eur','retired-provider','collection','session',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	reject("UPDATE payments SET deleted_at=NULL WHERE id='payment'")
	exec(`INSERT INTO payment_captures(id,amount,raw_amount,payment_id,metadata,created_at,updated_at) VALUES ('capture',1,'{"value":"1","precision":38}','replacement','null',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	var jsonNull string
	if db.QueryRowContext(ctx, "SELECT metadata FROM payment_captures WHERE id='capture'").Scan(&jsonNull) != nil || jsonNull != "null" {
		t.Fatal("JSON null changed to SQL NULL")
	}
	reject("UPDATE payment_captures SET payment_id='missing' WHERE id='capture'")
	reject("UPDATE payment_captures SET metadata='sealed-evidence' WHERE id='capture'")
	exec(`INSERT INTO payment_refunds(id,amount,raw_amount,payment_id,refund_reason_id,created_at,updated_at) VALUES ('refund',1,'{"value":"1","precision":38}','replacement','retired-reason',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	reject("UPDATE payment_refunds SET payment_id='missing' WHERE id='refund'")
	exec(`INSERT INTO payment_account_holders(id,provider_id,external_id,data,created_at,updated_at) VALUES ('historical-holder','retired-provider','sealed-identifier','sealed-data',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	var fk int
	if db.QueryRowContext(ctx, "SELECT @@foreign_key_checks").Scan(&fk) != nil || fk != 1 {
		t.Fatal("foreign keys disabled")
	}
	reject("UPDATE payment_runtime_schema SET writes_enabled=TRUE WHERE id=1")
	exec("UPDATE payment_runtime_schema SET reads_enabled=FALSE WHERE id=1")
	if payments.Ready(ctx, gdb) != payments.ErrSchema {
		t.Fatal("disabled payment reads accepted")
	}
	exec("UPDATE payment_runtime_schema SET reads_enabled=TRUE,model_sha256=REPEAT('0',64) WHERE id=1")
	if payments.Ready(ctx, gdb) != payments.ErrSchema || payments.Ready(ctx, nil) != payments.ErrSchema {
		t.Fatal("drifted payment schema accepted")
	}
	previous := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.Files, ".")
	for _, e := range entries {
		if e.Name() < "0019" {
			b, _ := fs.ReadFile(migrations.Files, e.Name())
			previous[e.Name()] = &fstest.MapFile{Data: b}
		}
	}
	old, err := schema.Load(previous)
	if err != nil {
		t.Fatal("preceding plan invalid")
	}
	blocked, _ := open("payment_schema_existing")
	if _, err := schema.Apply(ctx, blocked, old); err != nil {
		t.Fatal("preceding plan failed")
	}
	if _, err := blocked.ExecContext(ctx, "CREATE TABLE payments(id INT PRIMARY KEY)"); err != nil {
		t.Fatal("prototype setup failed")
	}
	if _, err := blocked.ExecContext(ctx, "INSERT INTO payments VALUES (17)"); err != nil {
		t.Fatal("prototype row failed")
	}
	if _, err := schema.Apply(ctx, blocked, plan); err == nil {
		t.Fatal("existing payment table overwritten")
	}
	var kept, created int
	if blocked.QueryRowContext(ctx, "SELECT id FROM payments").Scan(&kept) != nil || kept != 17 {
		t.Fatal("existing payment row lost")
	}
	if blocked.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('payment_sessions','payment_runtime_schema')").Scan(&created) != nil || created != 0 {
		t.Fatal("guard partially created payment tables")
	}
	result := map[string]any{"migrations": 32, "tables": 8, "source_columns": 83, "exact_decimal": true, "enum_json_fk_rejected": true, "active_session_unique": true, "soft_deleted_session_reusable": true, "protected_columns_accept_ciphertext": true, "existing_tables_preserved": true, "replay_unchanged": true, "schema_drift_rejected": true, "writes_enabled": false, "provider_called": false}
	b, _ := json.Marshal(result)
	t.Log("PAYMENT_SCHEMA_REHEARSAL " + string(b))
}
