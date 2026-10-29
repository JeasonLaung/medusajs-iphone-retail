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

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedOrderSchema(t *testing.T) {
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
	defer root.Close()
	open := func(name string) (*sql.DB, *gorm.DB) {
		// Only fixed artificial names below are supplied to this closure.
		if _, err := root.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
			t.Fatal("owned order database unavailable")
		}
		c := *cfg
		c.DBName = name
		db, err := sql.Open("mysql", c.FormatDSN())
		if err != nil {
			t.Fatal("owned order connection unavailable")
		}
		t.Cleanup(func() { db.Close() })
		gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
		if err != nil {
			t.Fatal("owned order adapter unavailable")
		}
		return db, gdb
	}
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("order migration plan invalid")
	}
	db, gdb := open("order_schema_fresh")
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 59 || schema.CheckApplied(ctx, db, plan) != nil || orders.Ready(ctx, gdb) != nil {
		t.Fatal("fresh order schema failed")
	}
	r, err = schema.Apply(ctx, db, plan)
	if err != nil || r.Skipped != 59 {
		t.Fatal("order schema replay changed facts")
	}
	ddl, err := migrations.Files.ReadFile("0016_order_relational.sql")
	if err != nil {
		t.Fatal("embedded order schema missing")
	}
	matches := regexp.MustCompile("CREATE TABLE `([a-z_]+)`").FindAllSubmatch(ddl, -1)
	tables := make([]string, 0, len(matches))
	for _, match := range matches {
		tables = append(tables, string(match[1]))
	}
	if len(tables) != 29 {
		t.Fatal("order evidence table count differs")
	}
	total := 0
	for _, name := range tables {
		var count int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND extra NOT LIKE '%GENERATED%'", name).Scan(&count) != nil || count == 0 {
			t.Fatal("order evidence table missing")
		}
		total += count
	}
	if total != 390 {
		t.Fatal("order field preservation differs")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal("artificial order fact rejected")
		}
	}
	reject := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err == nil {
			t.Fatal("invalid order fact accepted")
		}
	}
	count := func(query string, want int) {
		t.Helper()
		var got int
		if db.QueryRowContext(ctx, query).Scan(&got) != nil || got != want {
			t.Fatal("artificial order invariant differs")
		}
	}
	exec(`INSERT INTO historical_customer_subjects(id,email,legacy_had_account,created_at,updated_at) VALUES ('historical-only','same@example.invalid',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	count("SELECT COUNT(*) FROM admin_users", 0)
	count("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='historical_customer_subjects' AND (column_name LIKE '%password%' OR column_name LIKE '%token%' OR column_name LIKE '%session%')", 0)
	exec(`INSERT INTO orders(id,customer_id,version,status,is_draft_order,currency_code,display_id,custom_display_id,created_at,updated_at) VALUES ('historical-order','historical-only',2,'pending',FALSE,'eur',17,'9007199254740993123',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	reject(`UPDATE orders SET status='paid' WHERE id='historical-order'`)
	reject(`UPDATE orders SET is_draft_order=2 WHERE id='historical-order'`)
	reject(`UPDATE orders SET metadata='invalid-json' WHERE id='historical-order'`)
	reject(`UPDATE orders SET shipping_address_id='absent-address' WHERE id='historical-order'`)
	// Nullable totals_id breaks the physical cycle without disabling FK checks:
	// create snapshot -> create version rows -> bind the selected totals row.
	exec(`INSERT INTO order_line_items(id,title,variant_id,variant_sku,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,is_custom_price,is_giftcard,created_at,updated_at) VALUES ('historical-line','Original title','retired-product-variant','9007199254740993123',TRUE,TRUE,FALSE,9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, version := range []int{1, 2} {
		exec(`INSERT INTO order_items(id,order_id,version,item_id,quantity,raw_quantity,fulfilled_quantity,raw_fulfilled_quantity,shipped_quantity,raw_shipped_quantity,return_requested_quantity,raw_return_requested_quantity,return_received_quantity,raw_return_received_quantity,return_dismissed_quantity,raw_return_dismissed_quantity,written_off_quantity,raw_written_off_quantity,delivered_quantity,raw_delivered_quantity,created_at,updated_at)
 VALUES (?, 'historical-order',?,'historical-line',2,'{"value":"2","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, []string{"", "version-one", "version-two"}[version], version)
	}
	exec(`UPDATE order_line_items SET totals_id='version-two' WHERE id='historical-line'`)
	count("SELECT COUNT(*) FROM order_items WHERE order_id='historical-order'", 2)
	var price, sku, display string
	if db.QueryRowContext(ctx, `SELECT CAST(l.unit_price AS CHAR),l.variant_sku,o.custom_display_id FROM order_line_items l JOIN order_items i ON i.id=l.totals_id JOIN orders o ON o.id=i.order_id WHERE l.id='historical-line'`).Scan(&price, &sku, &display) != nil || price != "9007199254740993.000000000000000001" || sku != "9007199254740993123" || display != sku {
		t.Fatal("snapshot decimal or string precision lost")
	}
	reject(`UPDATE order_line_items SET totals_id='absent-version' WHERE id='historical-line'`)
	count("SELECT @@foreign_key_checks", 1)
	// Immutable historical references stay unresolved, rather than manufacturing
	// order, fulfillment or payment parents and quietly granting access to them.
	exec(`INSERT INTO order_fulfillments(order_id,fulfillment_id,id,created_at,updated_at) VALUES ('missing-historical-order','missing-historical-fulfillment','orphan-evidence',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	count("SELECT COUNT(*) FROM orders", 1)
	count("SELECT COUNT(*) FROM order_fulfillments", 1)
	reject(`UPDATE order_runtime_schema SET writes_enabled=TRUE WHERE id=1`)
	exec(`UPDATE order_runtime_schema SET reads_enabled=FALSE WHERE id=1`)
	if orders.Ready(ctx, gdb) != orders.ErrSchema {
		t.Fatal("disabled order read schema accepted")
	}
	exec(`UPDATE order_runtime_schema SET reads_enabled=TRUE,model_sha256=REPEAT('0',64) WHERE id=1`)
	if orders.Ready(ctx, gdb) != orders.ErrSchema || orders.Ready(ctx, nil) != orders.ErrSchema {
		t.Fatal("drifted or missing order schema accepted")
	}
	// A previous local prototype must not be silently adopted or overwritten.
	previous := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.Files, ".")
	for _, e := range entries {
		if e.Name() < "0016" {
			b, _ := fs.ReadFile(migrations.Files, e.Name())
			previous[e.Name()] = &fstest.MapFile{Data: b}
		}
	}
	old, err := schema.Load(previous)
	if err != nil {
		t.Fatal("preceding plan invalid")
	}
	blocked, _ := open("order_schema_existing")
	if _, err := schema.Apply(ctx, blocked, old); err != nil {
		t.Fatal("preceding plan application failed")
	}
	if _, err := blocked.ExecContext(ctx, `CREATE TABLE orders (id INT PRIMARY KEY)`); err != nil {
		t.Fatal("existing prototype setup failed")
	}
	if _, err := blocked.ExecContext(ctx, `INSERT INTO orders VALUES (17)`); err != nil {
		t.Fatal("existing prototype row failed")
	}
	if _, err := schema.Apply(ctx, blocked, plan); err == nil {
		t.Fatal("pre-existing order table overwritten")
	}
	var kept int
	if blocked.QueryRowContext(ctx, "SELECT id FROM orders").Scan(&kept) != nil || kept != 17 {
		t.Fatal("existing order row not retained")
	}
	var created int
	if blocked.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('order_items','order_runtime_schema')").Scan(&created) != nil || created != 0 {
		t.Fatal("guard failed after partially creating order schema")
	}
	result := map[string]any{"migrations": 32, "tables": 29, "source_columns": 390,
		"physical_foreign_keys_enabled": true, "exact_decimal_and_string_ids": true,
		"versions_and_nullable_cycle_retained": true, "invalid_enum_json_boolean_rejected": true,
		"historical_subject_without_login": true, "orphan_references_preserved": true,
		"existing_tables_preserved": true, "replay_unchanged": true, "schema_drift_rejected": true,
		"writes_enabled": false, "checkout_integrated": false}
	b, _ := json.Marshal(result)
	t.Log("ORDER_SCHEMA_REHEARSAL " + string(b))
}
