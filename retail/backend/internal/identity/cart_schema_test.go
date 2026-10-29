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

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedCartSchema(t *testing.T) {
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
			t.Fatal("owned cart database unavailable")
		}
		c := *cfg
		c.DBName = name
		db, err := sql.Open("mysql", c.FormatDSN())
		if err != nil {
			t.Fatal("owned cart connection unavailable")
		}
		t.Cleanup(func() { db.Close() })
		gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
		if err != nil {
			t.Fatal("owned cart adapter unavailable")
		}
		return db, gdb
	}
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("cart migration plan invalid")
	}
	db, gdb := open("cart_schema_fresh")
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 33 || schema.CheckApplied(ctx, db, plan) != nil || cart.Ready(ctx, gdb) != nil {
		t.Fatal("fresh cart schema failed")
	}
	r, err = schema.Apply(ctx, db, plan)
	if err != nil || r.Skipped != 33 {
		t.Fatal("cart schema replay changed facts")
	}
	ddl, err := migrations.Files.ReadFile("0025_cart_relational.sql")
	if err != nil {
		t.Fatal("embedded cart schema missing")
	}
	matches := regexp.MustCompile("CREATE TABLE `([a-z_]+)`").FindAllSubmatch(ddl, -1)
	tables := make([]string, 0, len(matches))
	for _, match := range matches {
		tables = append(tables, string(match[1]))
	}
	if len(tables) != 10 {
		t.Fatal("cart evidence table count differs")
	}
	total := 0
	for _, name := range tables {
		var count int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND extra NOT LIKE '%GENERATED%'", name).Scan(&count) != nil || count == 0 {
			t.Fatal("cart evidence table missing")
		}
		total += count
	}
	if total != 136 {
		t.Fatal("cart field preservation differs")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal("artificial cart fact rejected")
		}
	}
	reject := func(query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err == nil {
			t.Fatal("invalid cart fact accepted")
		}
	}
	count := func(query string, want int) {
		t.Helper()
		var got int
		if db.QueryRowContext(ctx, query).Scan(&got) != nil || got != want {
			t.Fatal("cart count differs")
		}
	}
	exec(`INSERT INTO cart_addresses(id,first_name,created_at,updated_at) VALUES ('address','Artificial',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO carts(id,currency_code,shipping_address_id,created_at,updated_at,metadata) VALUES ('cart','eur','address',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"amount":9007199254740993.000000000000000001}')`)
	exec(`INSERT INTO cart_line_items(id,cart_id,title,quantity,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,is_custom_price,is_giftcard,created_at,updated_at,variant_sku) VALUES ('line','cart','Artificial phone',2,TRUE,TRUE,TRUE,9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'9007199254740993123')`)
	var price, sku, metadata string
	if db.QueryRowContext(ctx, `SELECT CAST(unit_price AS CHAR),variant_sku FROM cart_line_items WHERE id='line'`).Scan(&price, &sku) != nil || price != "9007199254740993.000000000000000001" || sku != "9007199254740993123" {
		t.Fatal("cart exact money or SKU lost")
	}
	if db.QueryRowContext(ctx, `SELECT metadata FROM carts WHERE id='cart'`).Scan(&metadata) != nil || metadata != `{"amount":9007199254740993.000000000000000001}` {
		t.Fatal("cart JSON numeric lexeme lost")
	}
	exec(`INSERT INTO cart_line_tax_lines(id,code,rate,item_id,created_at,updated_at) VALUES ('tax','VAT',25.000000000000000001,'line',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	if db.QueryRowContext(ctx, `SELECT CAST(rate AS CHAR) FROM cart_line_tax_lines WHERE id='tax'`).Scan(&price) != nil || price != "25.000000000000000001" {
		t.Fatal("cart tax precision lost")
	}
	exec(`INSERT INTO cart_line_adjustments(id,amount,raw_amount,item_id,is_tax_inclusive,created_at,updated_at) VALUES ('discount',1.000000000000000001,'{"value":"1.000000000000000001","precision":38}','line',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_shipping_methods(id,cart_id,name,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES ('shipping','cart','Artificial shipping',12.5,'{"value":"12.5","precision":38}',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_shipping_tax_lines(id,code,rate,shipping_method_id,created_at,updated_at) VALUES ('shipping-tax','VAT',25,'shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_shipping_adjustments(id,amount,raw_amount,shipping_method_id,created_at,updated_at) VALUES ('shipping-discount',1,'{"value":"1","precision":38}','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	reject(`UPDATE carts SET shipping_address_id='missing' WHERE id='cart'`)
	reject(`UPDATE carts SET metadata='invalid json' WHERE id='cart'`)
	reject(`UPDATE cart_line_items SET cart_id='missing' WHERE id='line'`)
	reject(`UPDATE cart_line_items SET unit_price=-1 WHERE id='line'`)
	reject(`UPDATE cart_line_items SET requires_shipping=2 WHERE id='line'`)
	reject(`UPDATE cart_line_items SET raw_unit_price='invalid' WHERE id='line'`)
	reject(`UPDATE cart_line_tax_lines SET item_id='missing' WHERE id='tax'`)
	reject(`UPDATE cart_line_adjustments SET amount=-1 WHERE id='discount'`)
	reject(`UPDATE cart_shipping_methods SET amount=-1 WHERE id='shipping'`)
	reject(`UPDATE cart_shipping_tax_lines SET shipping_method_id='missing' WHERE id='shipping-tax'`)
	// Source semantic links without physical foreign keys remain evidence only.
	exec(`INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) VALUES ('missing-cart','missing-payment','orphan',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_promotions(cart_id,promotion_id,id,created_at,updated_at) VALUES ('missing-cart','missing-promotion','orphan',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	count("SELECT COUNT(*) FROM carts", 1)
	count("SELECT @@foreign_key_checks", 1)
	reject(`INSERT INTO cart_payment_collections SELECT * FROM cart_payment_collections`)
	exec(`DELETE FROM cart_addresses WHERE id='address'`)
	count("SELECT COUNT(*) FROM carts WHERE shipping_address_id IS NULL", 1)
	exec(`DELETE FROM carts WHERE id='cart'`)
	for _, name := range []string{"cart_line_items", "cart_line_adjustments", "cart_line_tax_lines", "cart_shipping_methods", "cart_shipping_tax_lines", "cart_shipping_adjustments"} {
		count("SELECT COUNT(*) FROM "+name, 0)
	}
	reject(`UPDATE cart_runtime_schema SET writes_enabled=TRUE WHERE id=1`)
	exec(`UPDATE cart_runtime_schema SET reads_enabled=FALSE WHERE id=1`)
	if cart.Ready(ctx, gdb) != cart.ErrSchema {
		t.Fatal("disabled cart schema accepted")
	}
	exec(`UPDATE cart_runtime_schema SET reads_enabled=TRUE,model_sha256=REPEAT('0',64) WHERE id=1`)
	if cart.Ready(ctx, gdb) != cart.ErrSchema || cart.Ready(ctx, nil) != cart.ErrSchema {
		t.Fatal("drifted cart schema accepted")
	}
	previous := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.Files, ".")
	for _, entry := range entries {
		if entry.Name() < "0025" {
			b, _ := fs.ReadFile(migrations.Files, entry.Name())
			previous[entry.Name()] = &fstest.MapFile{Data: b}
		}
	}
	old, err := schema.Load(previous)
	if err != nil {
		t.Fatal("preceding cart plan invalid")
	}
	blocked, _ := open("cart_schema_existing")
	if _, err := schema.Apply(ctx, blocked, old); err != nil {
		t.Fatal("preceding cart migrations failed")
	}
	if _, err := blocked.ExecContext(ctx, `CREATE TABLE carts(id INT PRIMARY KEY)`); err != nil {
		t.Fatal("existing cart setup failed")
	}
	if _, err := blocked.ExecContext(ctx, `INSERT INTO carts VALUES (17)`); err != nil {
		t.Fatal("existing cart row failed")
	}
	if _, err := schema.Apply(ctx, blocked, plan); err == nil {
		t.Fatal("existing cart overwritten")
	}
	var kept int
	if blocked.QueryRowContext(ctx, "SELECT id FROM carts").Scan(&kept) != nil || kept != 17 {
		t.Fatal("existing cart lost")
	}
	if blocked.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('cart_line_items','cart_runtime_schema')").Scan(&kept) != nil || kept != 0 {
		t.Fatal("cart guard created partial domain")
	}
	result := map[string]any{"migrations": 32, "tables": 10, "source_columns": 136, "exact_money_tax_and_string_ids": true, "json_numeric_lexeme_preserved": true, "physical_fk_cascade_and_set_null": true, "invalid_boolean_json_negative_amount_rejected": true, "composite_identity_preserved": true, "unresolved_semantic_references_preserved": true, "existing_tables_preserved": true, "replay_unchanged": true, "schema_drift_rejected": true, "writes_enabled": false, "possession_restored": false, "checkout_integrated": false}
	b, _ := json.Marshal(result)
	t.Log("CART_SCHEMA_REHEARSAL " + string(b))
}
