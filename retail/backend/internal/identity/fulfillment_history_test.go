package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/fulfillmentimport"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedFulfillmentHistory(t *testing.T) {
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
			t.Fatal("owned fulfillment database unavailable")
		}
		c := *cfg
		c.DBName = name
		db, err := sql.Open("mysql", c.FormatDSN())
		if err != nil {
			t.Fatal("owned fulfillment connection unavailable")
		}
		t.Cleanup(func() {
			db.Close()
			if _, err := root.Exec("DROP DATABASE `" + name + "`"); err != nil {
				t.Error("owned fulfillment database cleanup failed")
			}
		})
		gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
		if err != nil {
			t.Fatal("owned fulfillment adapter unavailable")
		}
		return db, gdb
	}
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("fulfillment migration plan invalid")
	}
	db, gdb := open("fulfillment_schema_fresh")
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 41 || schema.CheckApplied(ctx, db, plan) != nil || fulfillment.Ready(ctx, gdb) != nil {
		t.Fatal("fresh fulfillment schema failed")
	}
	r, err = schema.Apply(ctx, db, plan)
	if err != nil || r.Skipped != 41 {
		t.Fatal("fulfillment schema replay changed facts")
	}

	ddl, err := migrations.Files.ReadFile("0021_fulfillment_relational.sql")
	if err != nil {
		t.Fatal("fulfillment schema missing")
	}
	matches := regexp.MustCompile("CREATE TABLE `([a-z_]+)`").FindAllSubmatch(ddl, -1)
	if len(matches) != 7 {
		t.Fatal("fulfillment table count differs")
	}
	total := 0
	for _, m := range matches {
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND extra NOT LIKE '%GENERATED%'", string(m[1])).Scan(&n) != nil || n == 0 {
			t.Fatal("fulfillment field count unavailable")
		}
		total += n
	}
	if total != 72 {
		t.Fatal("fulfillment field count differs")
	}
	exec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal("fulfillment artificial SQL failed")
		}
	}
	reject := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err == nil {
			t.Fatal("invalid fulfillment SQL accepted")
		}
	}
	count := func(table string, want int) {
		t.Helper()
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n) != nil || n != want {
			t.Fatal("fulfillment artificial count differs: " + table)
		}
	}
	exec(`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES ('set','Set','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES ('zone','Zone','set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_option_types(id,label,code,created_at,updated_at) VALUES ('type','Type','code',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,shipping_option_type_id,created_at,updated_at) VALUES ('option','Option','flat','zone','type',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	text := func(v string) rowimport.Cell { b, _ := json.Marshal(v); return rowimport.Cell{Raw: b} }
	raw := func(v string) rowimport.Cell { return rowimport.Cell{Raw: json.RawMessage(v)} }
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
	address := row("id company first_name last_name address_1 address_2 city country_code province postal_code phone metadata created_at updated_at deleted_at", "address")
	address["first_name"], address["metadata"] = text("历史收件人"), raw("null")
	shipment := row("id location_id packed_at shipped_at delivered_at canceled_at data provider_id shipping_option_id metadata delivery_address_id created_at updated_at deleted_at marked_shipped_by created_by requires_shipping", "shipment")
	shipment["location_id"], shipment["requires_shipping"], shipment["delivery_address_id"], shipment["shipping_option_id"], shipment["data"] = text("retired-location"), raw("true"), text("address"), text("option"), raw(`{"original":"provider evidence"}`)
	item := row("id title sku barcode quantity raw_quantity line_item_id inventory_item_id fulfillment_id created_at updated_at deleted_at", "item")
	item["title"], item["sku"], item["barcode"], item["quantity"], item["raw_quantity"], item["fulfillment_id"], item["line_item_id"], item["inventory_item_id"] = text("Original phone"), text("0000123456789012345"), text(""), raw("9007199254740993.000000000000000001"), raw(`{"value":"9007199254740993.000000000000000001","precision":38}`), text("shipment"), text("missing-line"), text("missing-stock")
	label := row("id tracking_number tracking_url label_url fulfillment_id created_at updated_at deleted_at", "label")
	label["tracking_number"], label["tracking_url"], label["label_url"], label["fulfillment_id"] = text("000012345678901234567890"), text("https://example.invalid/track"), text("https://example.invalid/private-label"), text("shipment")
	rule := row("id attribute operator value shipping_option_id created_at updated_at deleted_at", "rule")
	rule["attribute"], rule["operator"], rule["value"], rule["shipping_option_id"] = text("country"), text("in"), raw(`["dk"]`), text("option")
	providers, sets := []rowimport.SourceRow{}, []rowimport.SourceRow{}
	for _, id := range []string{"one", "two"} {
		v := row("stock_location_id fulfillment_provider_id id created_at updated_at deleted_at", "location-provider-"+id)
		v["stock_location_id"], v["fulfillment_provider_id"] = text("retired-location"), text("retired-provider-"+id)
		providers = append(providers, v)
		v = row("stock_location_id fulfillment_set_id id created_at updated_at deleted_at", "location-set-"+id)
		v["stock_location_id"], v["fulfillment_set_id"] = text("retired-location"), text("retired-set-"+id)
		sets = append(sets, v)
	}
	geoFields := "id type country_code province_code city service_zone_id postal_expression metadata created_at updated_at deleted_at"
	geo := row(geoFields, "geo")
	geo["type"], geo["country_code"], geo["province_code"], geo["city"], geo["service_zone_id"] = text("zip"), text("DK"), text("01"), text("Århus"), text("zone")
	geo["postal_expression"] = raw(`{ "eq" : ["00123"], "amount":9007199254740993 }`)
	geo["metadata"] = raw(`{"label":"区域", "label":"original duplicate"}`)
	geoNull := row(geoFields, "geo-null")
	geoNull["type"], geoNull["country_code"], geoNull["service_zone_id"], geoNull["metadata"] = text("country"), text("dk"), text("zone"), raw(`null`)
	batch := map[string][]rowimport.SourceRow{"fulfillment_address": {address}, "fulfillment": {shipment}, "fulfillment_item": {item}, "fulfillment_label": {label}, "shipping_option_rule": {rule}, "location_fulfillment_provider": providers, "location_fulfillment_set": sets, "geo_zone": {geo, geoNull}}
	const snapshot = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := func(commit bool) (fulfillmentimport.Result, error) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fulfillmentimport.Result{}, err
		}
		defer tx.Rollback()
		r, err := fulfillmentimport.Import(ctx, tx, snapshot, batch)
		if err != nil {
			return r, err
		}
		if commit {
			err = tx.Commit()
		}
		return r, err
	}
	empty := func() {
		count("fulfillment_geo_zones", 0)
		for _, m := range matches {
			count(string(m[1]), 0)
		}
		count("migration_fulfillment_rows", 0)
		count("fulfillment_migration_reference_holds", 0)
	}
	staged, err := run(false)
	if err != nil || staged.Rows != 11 || staged.ObservedMissingReferences != 11 {
		t.Fatal("fulfillment rollback staging failed")
	}
	empty()
	// A missing physical parent must roll back receipts and all earlier tables.
	geo["service_zone_id"] = text("missing-zone")
	if _, err := run(true); err != fulfillmentimport.ErrStorage {
		t.Fatal("geo missing parent accepted")
	}
	empty()
	geo["service_zone_id"] = text("zone")
	exec(`CREATE TRIGGER reject_fulfillment_hold BEFORE INSERT ON fulfillment_migration_reference_holds FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial hold failure'`)
	if r, err := run(true); !errors.Is(err, fulfillmentimport.ErrStorage) || r != (fulfillmentimport.Result{}) {
		t.Fatal("fulfillment late failure not atomic")
	}
	empty()
	exec("DROP TRIGGER reject_fulfillment_hold")
	rule["operator"] = text("unknown")
	if _, err := run(true); !errors.Is(err, fulfillmentimport.ErrPolicy) {
		t.Fatal("unknown fulfillment rule accepted")
	}
	empty()
	rule["operator"] = text("in")
	oldTx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("fulfillment old snapshot failed")
	}
	defer oldTx.Rollback()
	var before int
	if oldTx.QueryRowContext(ctx, "SELECT COUNT(*) FROM migration_fulfillment_rows").Scan(&before) != nil || before != 0 {
		t.Fatal("fulfillment old snapshot not empty")
	}
	var wg sync.WaitGroup
	results := make(chan fulfillmentimport.Result, 8)
	errs := make(chan error, 8)
	for j := 0; j < 8; j++ {
		wg.Go(func() { v, e := run(true); results <- v; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("fulfillment concurrent batch failed")
		}
	}
	fresh, replays := 0, 0
	for v := range results {
		if v.Rows != 11 || v.ObservedMissingReferences != 11 {
			t.Fatal("fulfillment concurrent counts differ")
		}
		if v.ReplayedRows == 0 {
			fresh++
		} else if v.ReplayedRows == 11 {
			replays++
		}
	}
	if fresh != 1 || replays != 7 {
		t.Fatal("fulfillment duplicate batch committed")
	}
	v, err := fulfillmentimport.Import(ctx, oldTx, snapshot, batch)
	if err != nil || v.ReplayedRows != 11 || oldTx.Commit() != nil {
		t.Fatal("fulfillment old snapshot hid receipts")
	}
	count("migration_fulfillment_rows", 11)
	count("fulfillment_geo_zones", 2)
	count("fulfillment_migration_reference_holds", 11)
	count("admin_users", 0)
	count("inventory_levels", 0)
	count("inventory_reservations", 0)
	var postal, metadata, country, province string
	if db.QueryRowContext(ctx, `SELECT postal_expression,metadata,country_code,province_code FROM fulfillment_geo_zones WHERE id='geo'`).Scan(&postal, &metadata, &country, &province) != nil || postal != string(geo["postal_expression"].Raw) || metadata != string(geo["metadata"].Raw) || country != "DK" || province != "01" {
		t.Fatal("geo exact JSON/text changed")
	}
	var nulls bool
	if db.QueryRowContext(ctx, `SELECT postal_expression IS NULL AND province_code IS NULL AND city IS NULL AND metadata='null' AND metadata IS NOT NULL FROM fulfillment_geo_zones WHERE id='geo-null'`).Scan(&nulls) != nil || !nulls {
		t.Fatal("geo SQL null/JSON null changed")
	}
	exec(`UPDATE fulfillment_geo_zones SET country_code='de' WHERE id='geo'`)
	if _, err := run(true); err != fulfillmentimport.ErrConflict {
		t.Fatal("geo target drift overwritten")
	}
	exec(`UPDATE fulfillment_geo_zones SET country_code='DK' WHERE id='geo'`)
	geo["country_code"] = text("de")
	if _, err := run(true); err != fulfillmentimport.ErrConflict {
		t.Fatal("geo source drift accepted")
	}
	geo["country_code"] = text("DK")
	delete(batch, "geo_zone")
	if _, err := run(true); err != fulfillmentimport.ErrPolicy {
		t.Fatal("missing geo table key accepted")
	}
	batch["geo_zone"] = []rowimport.SourceRow{geo, geoNull}
	exec(`UPDATE geo_runtime_schema SET reads_enabled=FALSE`)
	if _, err := run(true); err != fulfillmentimport.ErrPolicy {
		t.Fatal("disabled geo model accepted on replay")
	}
	exec(`UPDATE geo_runtime_schema SET reads_enabled=TRUE`)
	var geoModel string
	if db.QueryRowContext(ctx, `SELECT model_sha256 FROM geo_runtime_schema WHERE id=1`).Scan(&geoModel) != nil {
		t.Fatal("geo model unavailable")
	}
	exec(`UPDATE geo_runtime_schema SET model_sha256=REPEAT('0',64)`)
	if _, err := run(true); err != fulfillmentimport.ErrPolicy {
		t.Fatal("geo model drift accepted on replay")
	}
	if _, err := db.ExecContext(ctx, `UPDATE geo_runtime_schema SET model_sha256=?`, geoModel); err != nil {
		t.Fatal("geo model restore failed")
	}
	var quantity, sku, tracking string
	if db.QueryRowContext(ctx, "SELECT CAST(quantity AS CHAR),sku FROM fulfillment_items WHERE id='item'").Scan(&quantity, &sku) != nil || quantity != "9007199254740993.000000000000000001" || sku != "0000123456789012345" {
		t.Fatal("fulfillment precision lost")
	}
	if db.QueryRowContext(ctx, "SELECT tracking_number FROM fulfillment_labels WHERE id='label'").Scan(&tracking) != nil || tracking != "000012345678901234567890" {
		t.Fatal("tracking string lost")
	}
	reject("UPDATE fulfillment_items SET fulfillment_id='missing' WHERE id='item'")
	reject("UPDATE fulfillments SET data='invalid-json' WHERE id='shipment'")
	reject("UPDATE fulfillments SET requires_shipping=2 WHERE id='shipment'")
	reject("UPDATE fulfillment_option_rules SET operator='unknown' WHERE id='rule'")
	exec("UPDATE fulfillment_items SET quantity=9007199254740993.000000000000000002 WHERE id='item'")
	if _, err := run(true); !errors.Is(err, fulfillmentimport.ErrConflict) {
		t.Fatal("fulfillment decimal drift accepted")
	}
	exec("UPDATE fulfillment_items SET quantity=9007199254740993.000000000000000001 WHERE id='item'")
	batch["location_fulfillment_provider"] = append(providers, providers[0])
	if _, err := run(true); !errors.Is(err, fulfillmentimport.ErrPolicy) {
		t.Fatal("duplicate fulfillment composite identity accepted")
	}
	batch["location_fulfillment_provider"] = providers
	exec("UPDATE fulfillment_runtime_schema SET reads_enabled=FALSE WHERE id=1")
	if fulfillment.Ready(ctx, gdb) != fulfillment.ErrSchema {
		t.Fatal("disabled fulfillment schema accepted")
	}
	if _, err := run(true); !errors.Is(err, fulfillmentimport.ErrPolicy) {
		t.Fatal("disabled fulfillment import accepted")
	}
	exec("UPDATE fulfillment_runtime_schema SET reads_enabled=TRUE WHERE id=1")
	reject("UPDATE fulfillment_runtime_schema SET writes_enabled=TRUE WHERE id=1")
	reject("UPDATE fulfillment_migration_reference_holds SET released=TRUE")
	previous := fstest.MapFS{}
	entries, _ := fs.ReadDir(migrations.Files, ".")
	for _, e := range entries {
		if e.Name() < "0021" {
			b, _ := fs.ReadFile(migrations.Files, e.Name())
			previous[e.Name()] = &fstest.MapFile{Data: b}
		}
	}
	old, err := schema.Load(previous)
	if err != nil {
		t.Fatal("preceding fulfillment plan unavailable")
	}
	blocked, _ := open("fulfillment_schema_existing")
	if _, err := schema.Apply(ctx, blocked, old); err != nil {
		t.Fatal("preceding fulfillment plan failed")
	}
	if _, err := blocked.ExecContext(ctx, "CREATE TABLE fulfillments(id INT PRIMARY KEY)"); err != nil {
		t.Fatal("existing fulfillment setup failed")
	}
	if _, err := blocked.ExecContext(ctx, "INSERT INTO fulfillments VALUES (17)"); err != nil {
		t.Fatal("existing fulfillment row failed")
	}
	if _, err := schema.Apply(ctx, blocked, plan); err == nil {
		t.Fatal("existing fulfillment overwritten")
	}
	var kept int
	if blocked.QueryRowContext(ctx, "SELECT id FROM fulfillments").Scan(&kept) != nil || kept != 17 {
		t.Fatal("existing fulfillment row lost")
	}
	t.Log(`FULFILLMENT_HISTORY_REHEARSAL {"migrations":41,"tables":8,"source_columns":83,"rows":11,"reference_holds":11,"concurrent_batches":8,"single_fresh_batch":1,"replayed_batches":7,"old_snapshot_replay":true,"rollback_atomic":true,"late_failure_rollback":true,"exact_quantity_and_identifiers":true,"composite_identity_preserved":true,"physical_fk_json_boolean_enum_rejected":true,"target_drift_rejected":true,"disabled_schema_rejected":true,"existing_tables_preserved":true,"inventory_not_modified":true,"geo_exact_json_text_and_nulls":true,"geo_missing_parent_atomic":true,"geo_source_target_drift_rejected":true,"geo_required_table_key":true,"geo_model_checked_on_replay":true,"provider_called":false,"writes_enabled":false}`)
}
