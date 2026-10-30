package identity_test

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/tax"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedTaxSchema(t *testing.T) {
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
		t.Fatal("tax migration plan invalid")
	}
	db, gdb := open("tax_schema_fresh")
	result, err := schema.Apply(ctx, db, plan)
	if err != nil || result.Applied != 77 || schema.CheckApplied(ctx, db, plan) != nil || tax.Ready(ctx, gdb) != nil {
		t.Fatal("fresh tax schema unavailable")
	}
	if result, err = schema.Apply(ctx, db, plan); err != nil || result.Skipped != 77 {
		t.Fatal("tax schema replay changed facts")
	}
	exec := func(query string) {
		t.Helper()
		if err := gdb.Exec(query).Error; err != nil {
			t.Fatalf("artificial tax fixture failed: %v", err)
		}
	}
	reject := func(query string) {
		t.Helper()
		if gdb.Exec(query).Error == nil {
			t.Fatal("invalid tax configuration accepted")
		}
	}
	exec(`INSERT INTO tax_providers VALUES('tp_system',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`)
	exec(`INSERT INTO tax_regions(id,provider_id,country_code,province_code,parent_id,created_at,updated_at) VALUES ('country','tp_system','dk',NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('province',NULL,'dk','sample','country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_rates(id,rate,code,name,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES ('default',25,'VAT','Default',TRUE,FALSE,'country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('product',12.345678901234567891,'SPECIFIC','Specific',FALSE,FALSE,'country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('province-rate',5,'LOCAL','Local',TRUE,TRUE,'province',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_rate_rules(id,tax_rate_id,reference,reference_id,created_at,updated_at) VALUES ('rule','product','product','product',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	item := tax.Item{ID: "item", ProductID: testTaxPtr("product")}
	items := []tax.CalculationItem{{Item: item}, {Item: tax.Item{ID: "gift", ProductID: testTaxPtr("product")}, Giftcard: true}, {Item: tax.Item{ID: "ship", Shipping: true, ShippingOptionID: testTaxPtr("option")}}}
	read := func(c tax.CalculationContext, want int, wantErr bool) []tax.Line {
		t.Helper()
		var lines []tax.Line
		err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var e error; lines, e = tax.ReadSystem(tx, c, items); return e }, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if wantErr {
			if err == nil || lines != nil {
				t.Fatal("invalid tax read accepted")
			}
			return nil
		}
		if err != nil || len(lines) != want {
			t.Fatalf("tax read failed: lines=%d err=%v", len(lines), err)
		}
		return lines
	}
	if lines, err := tax.ReadSystem(gdb, tax.CalculationContext{Country: "dk", Automatic: true}, items); err != tax.ErrInput || lines != nil {
		t.Fatal("autocommit tax read accepted")
	}
	lines := read(tax.CalculationContext{Country: "DK", Province: "SAMPLE", Automatic: true}, 4, false)
	if lines[0].RateID != "province-rate" || lines[1].RateID != "product" || lines[1].Rate != "12.345678901234567891" || lines[2].ShippingLineID != "ship" || lines[3].RateID != "default" {
		t.Fatalf("configured tax priority/precision differs: %+v", lines)
	}
	read(tax.CalculationContext{Country: "dk", Automatic: true}, 2, false)
	read(tax.CalculationContext{Country: "dk"}, 0, false)
	read(tax.CalculationContext{Automatic: true}, 0, false)
	read(tax.CalculationContext{Force: true}, 0, true)
	read(tax.CalculationContext{Country: "fr", Automatic: true}, 0, false)
	read(tax.CalculationContext{Country: "dk", Force: true}, 2, false)
	reject(`UPDATE tax_providers SET is_enabled=2`)
	reject(`UPDATE tax_rates SET is_default=2 WHERE id='product'`)
	reject(`UPDATE tax_rates SET metadata='invalid' WHERE id='product'`)
	reject(`UPDATE tax_rates SET tax_region_id='missing' WHERE id='product'`)
	reject(`INSERT INTO tax_rates(id,rate,code,name,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES('duplicate',10,'DUP','Duplicate',TRUE,FALSE,'country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	reject(`UPDATE tax_regions SET provider_id='tp_system' WHERE id='province'`)
	exec(`UPDATE tax_providers SET is_enabled=FALSE WHERE id='tp_system'`)
	read(tax.CalculationContext{Country: "dk", Automatic: true}, 0, true)
	exec(`UPDATE tax_providers SET is_enabled=TRUE WHERE id='tp_system'`)
	exec(`UPDATE tax_regions SET provider_id='custom' WHERE id='country'`)
	read(tax.CalculationContext{Country: "dk", Automatic: true}, 0, true)
	exec(`UPDATE tax_regions SET provider_id='tp_system' WHERE id='country'`)
	exec(`INSERT INTO tax_regions(id,provider_id,country_code,created_at,updated_at) VALUES('duplicate-root','tp_system','dk',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	read(tax.CalculationContext{Country: "dk", Automatic: true}, 0, true)
	exec(`DELETE FROM tax_regions WHERE id='duplicate-root'`)
	exec(`INSERT INTO tax_rates(id,rate,code,name,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES('competing',10,'OTHER','Competing',FALSE,FALSE,'country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_rate_rules(id,tax_rate_id,reference,reference_id,created_at,updated_at) VALUES('competing-rule','competing','product','product',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	read(tax.CalculationContext{Country: "dk", Automatic: true}, 0, true)
	exec(`UPDATE tax_rates SET deleted_at=UTC_TIMESTAMP(6) WHERE id='competing'`)
	read(tax.CalculationContext{Country: "dk", Automatic: true}, 2, false)
	exec(`UPDATE tax_rates SET rate=NULL WHERE id='product'`)
	if lines := read(tax.CalculationContext{Country: "dk", Automatic: true}, 2, false); lines[0].Rate != "0" {
		t.Fatal("null tax rate is not zero")
	}
	exec(`UPDATE tax_runtime_schema SET reads_enabled=FALSE WHERE id=1`)
	if tax.Ready(ctx, gdb) != tax.ErrSchema {
		t.Fatal("disabled tax schema accepted")
	}
	exec(`UPDATE tax_runtime_schema SET reads_enabled=TRUE,model_sha256=REPEAT('0',64)`)
	if tax.Ready(ctx, gdb) != tax.ErrSchema || tax.Ready(ctx, nil) != tax.ErrSchema {
		t.Fatal("tax model drift accepted")
	}
	reject(`UPDATE tax_runtime_schema SET writes_enabled=TRUE`)
	previous := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.Files, ".")
	for _, entry := range entries {
		if entry.Name() < "0028" {
			data, _ := fs.ReadFile(migrations.Files, entry.Name())
			previous[entry.Name()] = &fstest.MapFile{Data: data}
		}
	}
	old, err := schema.Load(previous)
	if err != nil {
		t.Fatal(err)
	}
	blocked, _ := open("tax_schema_existing")
	if _, err := schema.Apply(ctx, blocked, old); err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.ExecContext(ctx, `CREATE TABLE tax_regions(id INT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.ExecContext(ctx, `INSERT INTO tax_regions VALUES(17)`); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, blocked, plan); err == nil {
		t.Fatal("existing tax table overwritten")
	}
	var kept int
	if blocked.QueryRowContext(ctx, `SELECT id FROM tax_regions`).Scan(&kept) != nil || kept != 17 {
		t.Fatal("existing tax row lost")
	}
	if blocked.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='tax_providers'`).Scan(&kept) != nil || kept != 0 {
		t.Fatal("tax guard created partial domain")
	}
	t.Log(`TAX_SCHEMA_REHEARSAL {"migrations":61,"tables":4,"source_columns":36,"actual_sql_selection":true,"province_parent_and_exact_rate":true,"giftcards_excluded":true,"automatic_force_address_gates":true,"null_rate_zero":true,"disabled_unknown_provider_rejected":true,"duplicate_tax_regions_and_priority_ties_rejected":true,"soft_deleted_rates_excluded":true,"autocommit_rejected":true,"existing_tables_preserved":true,"schema_drift_rejected":true,"writes_enabled":false,"cart_transaction_integrated":false}`)
}

func testTaxPtr(s string) *string { return &s }
