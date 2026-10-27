package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/inventoryimport"
	"github.com/eurorefurb/retail/internal/rowimport"
)

// Uses only the artificial owned inventory fixture, never a business database.
func verifyInventoryImport(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES ('import-location','Import fixture',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`); err != nil {
		t.Fatal("import parent fixture failed")
	}
	text := func(s string) rowimport.Cell { b, _ := json.Marshal(s); return rowimport.Cell{Raw: b} }
	null := rowimport.Cell{SQLNull: true}
	level := rowimport.SourceRow{}
	for _, field := range []string{"id", "created_at", "updated_at", "deleted_at", "inventory_item_id", "location_id", "stocked_quantity", "reserved_quantity", "incoming_quantity", "metadata", "raw_stocked_quantity", "raw_reserved_quantity", "raw_incoming_quantity"} {
		level[field] = null
	}
	level["id"] = text("import-level")
	level["created_at"] = text("2026-10-27T00:00:00.000000Z")
	level["updated_at"] = level["created_at"]
	level["inventory_item_id"] = text("fixture-stock")
	level["location_id"] = text("import-location")
	level["stocked_quantity"] = rowimport.Cell{Raw: json.RawMessage(`2.000000000000000001`)}
	level["reserved_quantity"] = rowimport.Cell{Raw: json.RawMessage(`3`)}
	level["incoming_quantity"] = rowimport.Cell{Raw: json.RawMessage(`0`)}
	level["raw_stocked_quantity"] = rowimport.Cell{Raw: json.RawMessage(`{"value":"2.000000000000000001","precision":38}`)}
	level["raw_reserved_quantity"] = rowimport.Cell{Raw: json.RawMessage(`{"value":"3","precision":38}`)}
	level["raw_incoming_quantity"] = rowimport.Cell{Raw: json.RawMessage(`{"value":"0","precision":38}`)}
	if _, err := rowimport.Convert("inventory_level", level); err != nil {
		t.Fatal("artificial row conversion invalid")
	}
	snapshot := strings.Repeat("a", 64)
	run := func(row rowimport.SourceRow, sha string, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err = inventoryimport.Import(ctx, tx, sha, "inventory_level", row); err != nil {
			return err
		}
		if _, err = inventoryimport.PreserveHolds(ctx, tx, sha); err != nil {
			return err
		}
		if commit {
			return tx.Commit()
		}
		return nil
	}
	count := func(table string) int {
		var n int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n) != nil {
			t.Fatal("import count unavailable")
		}
		return n
	}
	before := count("inventory_levels")
	if err := run(level, snapshot, false); err != nil {
		t.Fatal("inventory rollback staging failed")
	}
	if count("inventory_levels") != before || count("migration_inventory_rows") != 0 || count("inventory_migration_holds") != 0 {
		t.Fatal("inventory rollback leaked evidence")
	}
	if err := run(level, snapshot, true); err != nil {
		t.Fatal("inventory commit failed")
	}
	if err := run(level, snapshot, true); err != nil {
		t.Fatal("inventory exact replay failed")
	}
	if count("inventory_levels") != before+1 || count("migration_inventory_rows") != 1 || count("inventory_migration_holds") != 5 {
		t.Fatal("inventory replay duplicated facts or lost holds")
	}
	changed := rowimport.SourceRow{}
	for k, v := range level {
		changed[k] = v
	}
	changed["incoming_quantity"] = rowimport.Cell{Raw: json.RawMessage(`1`)}
	changed["raw_incoming_quantity"] = rowimport.Cell{Raw: json.RawMessage(`{"value":"1","precision":38}`)}
	if !errors.Is(run(changed, snapshot, true), inventoryimport.ErrConflict) {
		t.Fatal("changed source accepted")
	}
	if !errors.Is(run(level, strings.Repeat("b", 64), true), inventoryimport.ErrConflict) {
		t.Fatal("different snapshot accepted")
	}
	if _, err := db.ExecContext(ctx, "UPDATE inventory_levels SET stocked_quantity=2.000000000000000002 WHERE id='import-level'"); err != nil {
		t.Fatal("target precision fixture failed")
	}
	if !errors.Is(run(level, snapshot, true), inventoryimport.ErrConflict) {
		t.Fatal("target precision edit overwritten")
	}
	var quantity string
	if db.QueryRowContext(ctx, "SELECT stocked_quantity FROM inventory_levels WHERE id='import-level'").Scan(&quantity) != nil || quantity != "2.000000000000000002" {
		t.Fatal("target precision edit lost")
	}
	if _, err := db.ExecContext(ctx, "UPDATE inventory_migration_holds SET released=TRUE"); err == nil {
		t.Fatal("unreviewed hold release accepted")
	}
	if count("migration_inventory_rows") != 1 || count("inventory_migration_holds") != 5 {
		t.Fatal("failed import altered evidence")
	}
	t.Log(`INVENTORY_IMPORT_REHEARSAL {"exact_replay":true,"rollback_atomic":true,"source_change_rejected":true,"snapshot_change_rejected":true,"decimal_target_change_rejected":true,"immutable_holds":5,"unreviewed_release_rejected":true}`)
}
