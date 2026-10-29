package identity_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/taximport"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"sync"
)

func TestOwnedTaxImport(t *testing.T) {
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
	db, gdb := open("tax_import_fresh")
	if r, err := schema.Apply(ctx, db, plan); err != nil || r.Applied != 38 {
		t.Fatal("tax import migrations failed")
	}
	db.SetMaxOpenConns(12)
	exec := func(q string) {
		t.Helper()
		if gdb.Exec(q).Error != nil {
			t.Fatal("artificial tax import SQL failed")
		}
	}
	count := func(table string, want int) {
		t.Helper()
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n) != nil || n != want {
			t.Fatalf("unexpected %s count %d want %d", table, n, want)
		}
	}
	text := func(s string) rowimport.Cell { b, _ := json.Marshal(s); return rowimport.Cell{Raw: b} }
	raw := func(s string) rowimport.Cell { return rowimport.Cell{Raw: json.RawMessage(s)} }
	row := func(fields, id string) rowimport.SourceRow {
		r := rowimport.SourceRow{}
		for _, f := range strings.Fields(fields) {
			r[f] = rowimport.Cell{SQLNull: true}
		}
		r["id"] = text(id)
		r["created_at"] = text("2026-10-28T00:00:00.123456Z")
		r["updated_at"] = r["created_at"]
		return r
	}
	provider := row("id is_enabled created_at updated_at deleted_at", "tp_system")
	provider["is_enabled"] = raw("true")
	regionFields := "id provider_id country_code province_code parent_id metadata created_at updated_at created_by deleted_at"
	rootRegion := row(regionFields, "z-country")
	rootRegion["provider_id"], rootRegion["country_code"] = text("tp_system"), text("dk")
	child := row(regionFields, "a-province")
	child["country_code"], child["province_code"], child["parent_id"] = text("dk"), text("sample"), text("z-country")
	orphan := row(regionFields, "orphan")
	orphan["country_code"], orphan["province_code"], orphan["parent_id"] = text("xx"), text("old"), text("missing-parent")
	rate := row("id rate code name is_default is_combinable tax_region_id metadata created_at updated_at created_by deleted_at", "rate")
	rate["rate"], rate["code"], rate["name"], rate["is_default"], rate["is_combinable"], rate["tax_region_id"], rate["metadata"] = raw("12.345678901234567891"), text("VAT"), text("历史税率"), raw("true"), raw("false"), text("z-country"), raw("null")
	rule := row("id tax_rate_id reference_id reference metadata created_at updated_at created_by deleted_at", "rule")
	rule["tax_rate_id"], rule["reference_id"], rule["reference"] = text("rate"), text("product"), text("product")
	batch := map[string][]rowimport.SourceRow{"tax_provider": {provider}, "tax_region": {child, rootRegion, orphan}, "tax_rate": {rate}, "tax_rate_rule": {rule}}
	const snapshot = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := func(input map[string][]rowimport.SourceRow, commit bool) (taximport.Result, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return taximport.Result{}, err
		}
		defer tx.Rollback()
		r, err := taximport.Import(ctx, tx, snapshot, input)
		if err == nil && commit {
			err = tx.Commit()
		}
		return r, err
	}
	empty := func() {
		for _, table := range []string{"tax_providers", "tax_regions", "tax_rates", "tax_rate_rules", "migration_tax_rows", "tax_migration_reference_holds", "unique_key_owners", "unique_key_claims"} {
			count(table, 0)
		}
	}
	if r, err := run(batch, false); err != nil || r.Rows != 6 || r.ObservedMissingReferences != 1 {
		t.Fatalf("tax import staging failed %+v %v", r, err)
	}
	empty()
	exec(`CREATE TRIGGER reject_tax_hold BEFORE INSERT ON tax_migration_reference_holds FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial hold failure'`)
	if r, err := run(batch, true); err != taximport.ErrStorage || r != (taximport.Result{}) {
		t.Fatal("late hold failure not atomic")
	}
	empty()
	exec("DROP TRIGGER reject_tax_hold")
	duplicate := row(regionFields, "duplicate")
	duplicate["country_code"], duplicate["provider_id"] = text("dk"), text("tp_system")
	batch["tax_region"] = append(batch["tax_region"], duplicate)
	if _, err := run(batch, true); err != taximport.ErrConflict {
		t.Fatal("duplicate full country identity accepted")
	}
	empty()
	batch["tax_region"] = batch["tax_region"][:3]
	duplicate["country_code"], duplicate["province_code"], duplicate["parent_id"], duplicate["provider_id"] = text("dk"), text("sample"), text("z-country"), rowimport.Cell{SQLNull: true}
	batch["tax_region"] = append(batch["tax_region"], duplicate)
	if _, err := run(batch, true); err != taximport.ErrConflict {
		t.Fatal("duplicate full province identity accepted")
	}
	empty()
	batch["tax_region"] = batch["tax_region"][:3]
	old, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer old.Rollback()
	var before int
	if old.QueryRowContext(ctx, "SELECT COUNT(*) FROM migration_tax_rows").Scan(&before) != nil || before != 0 {
		t.Fatal("old snapshot setup failed")
	}
	var wg sync.WaitGroup
	results := make(chan taximport.Result, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() { r, e := run(batch, true); results <- r; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("concurrent import failed: %v", e)
		}
	}
	fresh, replays := 0, 0
	for r := range results {
		if r.Rows != 6 || r.ObservedMissingReferences != 1 {
			t.Fatal("incomplete concurrent import")
		}
		if r.ReplayedRows == 0 {
			fresh++
		} else if r.ReplayedRows == 6 {
			replays++
		} else {
			t.Fatal("partial replay")
		}
	}
	if fresh != 1 || replays != 7 {
		t.Fatal("concurrent import duplicated rows")
	}
	if r, err := taximport.Import(ctx, old, snapshot, batch); err != nil || r.ReplayedRows != 6 {
		t.Fatalf("old snapshot replay failed %+v %v", r, err)
	}
	old.Rollback()
	count("migration_tax_rows", 6)
	count("tax_migration_reference_holds", 1)
	var parent, amount, metadata string
	if db.QueryRowContext(ctx, "SELECT parent_id FROM tax_regions WHERE id='a-province'").Scan(&parent) != nil || parent != "z-country" {
		t.Fatal("deferred parent not restored")
	}
	if db.QueryRowContext(ctx, "SELECT rate,metadata FROM tax_rates WHERE id='rate'").Scan(&amount, &metadata) != nil || amount != "12.345678901234567891" || metadata != "null" {
		t.Fatal("decimal or JSON null changed")
	}
	rootRegion["country_code"] = text("de")
	if _, err := run(batch, true); err != taximport.ErrConflict {
		t.Fatal("changed source row accepted")
	}
	rootRegion["country_code"] = text("dk")
	if r, err := run(batch, true); err != nil || r.ReplayedRows != 6 {
		t.Fatal("conflict rollback damaged registry")
	}
	exec(`UPDATE tax_rates SET rate=12.345678901234567892 WHERE id='rate'`)
	if _, err := run(batch, true); err != taximport.ErrConflict {
		t.Fatal("last decimal target drift accepted")
	}
	exec(`UPDATE tax_rates SET rate=12.345678901234567891 WHERE id='rate'`)
	// A source-hash change and unrelated extra table keys cannot restore or grant accounts.
	batch["customer"] = nil
	if _, err := run(batch, true); !errors.Is(err, taximport.ErrPolicy) {
		t.Fatal("unexpected account table accepted")
	}
	delete(batch, "customer")
	exec(`UPDATE tax_runtime_schema SET reads_enabled=FALSE`)
	if _, err := run(batch, true); err != taximport.ErrPolicy {
		t.Fatal("disabled schema accepted")
	}
	exec(`UPDATE tax_runtime_schema SET reads_enabled=TRUE`)
	if gdb.Exec(`UPDATE tax_migration_reference_holds SET released=TRUE`).Error == nil {
		t.Fatal("reference hold released without policy")
	}
	t.Log(`TAX_IMPORT_REHEARSAL {"migrations":38,"tables":4,"source_columns":36,"rows":6,"reference_holds":1,"concurrent_batches":8,"single_fresh_batch":1,"replayed_batches":7,"old_snapshot_replay":true,"deferred_parent_restored":true,"exact_decimal_and_json_null":true,"country_province_full_value_registry":true,"rollback_atomic":true,"late_hold_failure_atomic":true,"source_and_target_drift_rejected":true,"disabled_schema_rejected":true,"accounts_restored":false,"provider_called":false}`)
}
