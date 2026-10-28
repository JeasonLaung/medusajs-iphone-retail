package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/orderimport"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
)

func TestOwnedOrderImport(t *testing.T) {
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
		t.Fatal("owned order import server unavailable")
	}
	defer root.Close()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE order_import_rehearsal"); err != nil {
		t.Fatal("owned order import database failed")
	}
	defer func() {
		if _, err := root.ExecContext(context.Background(), "DROP DATABASE `order_import_rehearsal`"); err != nil {
			t.Error("owned completed fixture database cleanup failed")
		}
	}()
	cfg.DBName = "order_import_rehearsal"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned order import pool failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("order import plan invalid")
	}
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 28 {
		t.Fatal("order import migrations failed")
	}
	names, _ := orderimport.SourceTables()
	batch := map[string][]rowimport.SourceRow{}
	for _, name := range names {
		batch[name] = nil
	}
	text := func(s string) rowimport.Cell { b, _ := json.Marshal(s); return rowimport.Cell{Raw: b} }
	raw := func(s string) rowimport.Cell { return rowimport.Cell{Raw: json.RawMessage(s)} }
	newRow := func(fields, id string) rowimport.SourceRow {
		row := rowimport.SourceRow{}
		for _, f := range strings.Fields(fields) {
			row[f] = rowimport.Cell{SQLNull: true}
		}
		row["id"] = text(id)
		row["created_at"] = text("2026-10-28T00:00:00.000001Z")
		row["updated_at"] = row["created_at"]
		return row
	}
	address := newRow("id customer_id company first_name last_name address_1 address_2 city country_code province postal_code phone metadata created_at updated_at deleted_at", "source-address")
	address["customer_id"] = text("unresolved-historical-customer")
	address["first_name"] = text("")
	batch["order_address"] = []rowimport.SourceRow{address}
	order := newRow("id region_id display_id customer_id version sales_channel_id status is_draft_order email currency_code shipping_address_id billing_address_id no_notification metadata created_at updated_at deleted_at canceled_at custom_display_id locale", "source-order")
	order["version"], order["status"], order["is_draft_order"], order["currency_code"] = raw("1"), text("pending"), raw("false"), text("eur")
	order["shipping_address_id"] = text("source-address")
	order["custom_display_id"] = text("9007199254740993123")
	order["metadata"] = raw("null")
	batch["order"] = []rowimport.SourceRow{order}
	line := newRow("id totals_id title subtitle thumbnail variant_id product_id product_title product_description product_subtitle product_type product_collection product_handle variant_sku variant_barcode variant_title variant_option_values requires_shipping is_discountable is_tax_inclusive compare_at_unit_price raw_compare_at_unit_price unit_price raw_unit_price metadata created_at updated_at deleted_at is_custom_price product_type_id is_giftcard", "source-line")
	line["totals_id"], line["title"], line["variant_sku"] = text("source-item"), text("Original snapshot"), text("9007199254740993123")
	for _, key := range []string{"requires_shipping", "is_discountable", "is_tax_inclusive", "is_custom_price", "is_giftcard"} {
		line[key] = raw("false")
	}
	line["unit_price"] = raw("9007199254740993.000000000000000001")
	line["raw_unit_price"] = raw(`{"value":"9007199254740993.000000000000000001","precision":38}`)
	batch["order_line_item"] = []rowimport.SourceRow{line}
	item := newRow("id order_id version item_id quantity raw_quantity fulfilled_quantity raw_fulfilled_quantity shipped_quantity raw_shipped_quantity return_requested_quantity raw_return_requested_quantity return_received_quantity raw_return_received_quantity return_dismissed_quantity raw_return_dismissed_quantity written_off_quantity raw_written_off_quantity metadata created_at updated_at deleted_at delivered_quantity raw_delivered_quantity unit_price raw_unit_price compare_at_unit_price raw_compare_at_unit_price", "source-item")
	item["order_id"], item["item_id"], item["version"] = text("source-order"), text("source-line"), raw("1")
	for _, key := range []string{"quantity", "fulfilled_quantity", "shipped_quantity", "return_requested_quantity", "return_received_quantity", "return_dismissed_quantity", "written_off_quantity", "delivered_quantity"} {
		item[key], item["raw_"+key] = raw("0"), raw(`{"value":"0","precision":38}`)
	}
	item["quantity"], item["raw_quantity"] = raw("2"), raw(`{"value":"2","precision":38}`)
	batch["order_item"] = []rowimport.SourceRow{item}
	for _, pair := range [][2]string{{"reason-child", "reason-parent"}, {"reason-parent", "reason-child"}} {
		reason := newRow("id value label description metadata parent_return_reason_id created_at updated_at deleted_at", pair[0])
		reason["value"], reason["label"], reason["parent_return_reason_id"] = text(pair[0]), text(pair[0]), text(pair[1])
		batch["return_reason"] = append(batch["return_reason"], reason)
	}
	for _, pair := range [][2]string{{"source-order", "missing-one"}, {"missing-order", "missing-two"}} {
		link := newRow("order_id fulfillment_id id created_at updated_at deleted_at", "same-nonunique-source-id")
		link["order_id"], link["fulfillment_id"] = text(pair[0]), text(pair[1])
		batch["order_fulfillment"] = append(batch["order_fulfillment"], link)
	}
	const snapshot = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := func(input map[string][]rowimport.SourceRow, sha string, commit bool) (orderimport.Result, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return orderimport.Result{}, err
		}
		defer tx.Rollback()
		result, err := orderimport.Import(ctx, tx, sha, input)
		if err != nil {
			return result, err
		}
		if commit {
			err = tx.Commit()
		}
		return result, err
	}
	exec := func(query string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal("order import fixture SQL failed")
		}
	}
	count := func(table string, want int) {
		t.Helper()
		var got int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got) != nil || got != want {
			t.Fatal("order import count differs: " + table)
		}
	}
	if result, err := run(batch, snapshot, false); err != nil || result.Rows != 8 || result.ObservedMissingReferences != 5 {
		t.Fatal("order import rollback staging failed")
	}
	for _, table := range []string{"orders", "order_line_items", "order_items", "return_reasons", "order_fulfillments", "migration_order_rows", "order_migration_reference_holds"} {
		count(table, 0)
	}
	// Failure occurs after business rows and cyclic edges have been written.
	exec(`CREATE TRIGGER reject_order_hold BEFORE INSERT ON order_migration_reference_holds FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial hold failure'`)
	if _, err := run(batch, snapshot, true); !errors.Is(err, orderimport.ErrStorage) {
		t.Fatal("hold insertion failure accepted")
	}
	count("orders", 0)
	count("migration_order_rows", 0)
	exec("DROP TRIGGER reject_order_hold")
	oldTx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("old import snapshot unavailable")
	}
	defer oldTx.Rollback()
	var oldCount int
	if oldTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM migration_order_rows").Scan(&oldCount) != nil || oldCount != 0 {
		t.Fatal("old import snapshot invalid")
	}
	var wg sync.WaitGroup
	results := make(chan orderimport.Result, 8)
	errorsSeen := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Go(func() { result, err := run(batch, snapshot, true); results <- result; errorsSeen <- err })
	}
	wg.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal("concurrent order import replay failed")
		}
	}
	fresh, replayed := 0, 0
	for r := range results {
		if r.Rows != 8 || r.ObservedMissingReferences != 5 {
			t.Fatal("concurrent order result differs")
		}
		if r.ReplayedRows == 0 {
			fresh++
		} else if r.ReplayedRows == 8 {
			replayed++
		}
	}
	if fresh != 1 || replayed != 7 {
		t.Fatal("order import not a single committed batch")
	}
	oldResult, err := orderimport.Import(ctx, oldTx, snapshot, batch)
	if err != nil || oldResult.ReplayedRows != 8 || oldTx.Commit() != nil {
		t.Fatal("old RR snapshot hid committed order receipts")
	}
	count("migration_order_rows", 8)
	count("order_migration_reference_holds", 5)
	count("historical_customer_subjects", 0)
	count("admin_users", 0)
	var price, totals, reason string
	var jsonNull bool
	if db.QueryRowContext(ctx, "SELECT CAST(unit_price AS CHAR),totals_id FROM order_line_items WHERE id='source-line'").Scan(&price, &totals) != nil || price != "9007199254740993.000000000000000001" || totals != "source-item" {
		t.Fatal("exact price or totals cycle lost")
	}
	if db.QueryRowContext(ctx, "SELECT parent_return_reason_id FROM return_reasons WHERE id='reason-parent'").Scan(&reason) != nil || reason != "reason-child" {
		t.Fatal("reason cycle lost")
	}
	if db.QueryRowContext(ctx, "SELECT metadata='null' AND email IS NULL FROM orders WHERE id='source-order'").Scan(&jsonNull) != nil || !jsonNull {
		t.Fatal("SQL NULL and JSON null conflated")
	}
	if _, err := run(batch, strings.Repeat("b", 64), true); !errors.Is(err, orderimport.ErrConflict) {
		t.Fatal("snapshot change accepted")
	}
	line["title"] = text("Changed source")
	if _, err := run(batch, snapshot, true); !errors.Is(err, orderimport.ErrConflict) {
		t.Fatal("source edit accepted")
	}
	line["title"] = text("Original snapshot")
	exec("UPDATE order_line_items SET unit_price=9007199254740993.000000000000000002 WHERE id='source-line'")
	if _, err := run(batch, snapshot, true); !errors.Is(err, orderimport.ErrConflict) {
		t.Fatal("decimal target drift accepted")
	}
	exec("UPDATE order_line_items SET unit_price=9007199254740993.000000000000000001,totals_id=NULL WHERE id='source-line'")
	if _, err := run(batch, snapshot, true); !errors.Is(err, orderimport.ErrConflict) {
		t.Fatal("replay repaired a changed cycle edge")
	}
	exec("UPDATE order_line_items SET totals_id='source-item' WHERE id='source-line'")
	batch["order_fulfillment"] = append(batch["order_fulfillment"], batch["order_fulfillment"][0])
	if _, err := run(batch, snapshot, true); !errors.Is(err, orderimport.ErrPolicy) {
		t.Fatal("duplicate composite identity accepted")
	}
	batch["order_fulfillment"] = batch["order_fulfillment"][:2]
	batch["customer"] = nil
	if _, err := run(batch, snapshot, true); !errors.Is(err, orderimport.ErrPolicy) {
		t.Fatal("protected customer ordinary import accepted")
	}
	delete(batch, "customer")
	if _, err := db.ExecContext(ctx, "UPDATE order_migration_reference_holds SET released=TRUE"); err == nil {
		t.Fatal("historical order hold released")
	}
	count("migration_order_rows", 8)
	count("order_migration_reference_holds", 5)
	t.Log(`ORDER_IMPORT_REHEARSAL {"migrations":28,"tables":28,"columns":378,"rows":8,"reference_holds":5,"concurrent_batches":8,"single_fresh_batch":1,"replayed_batches":7,"old_snapshot_replay":true,"nullable_cycle_restored":true,"reason_cycle_preserved":true,"rollback_atomic":true,"hold_failure_rollback":true,"decimal_drift_rejected":true,"changed_cycle_not_repaired":true,"source_and_snapshot_change_rejected":true,"composite_identity_preserved":true,"sql_null_json_null_distinct":true,"protected_customer_excluded":true,"no_account_granted":true,"hold_release_rejected":true}`)
}
