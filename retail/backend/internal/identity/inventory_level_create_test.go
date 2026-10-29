package identity_test

import (
	"encoding/json"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"testing"
)

func seedLevelCreateLocations(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('level-create-a','SKU warehouse A',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('level-create-b','SKU warehouse B',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('level-create-c','SKU warehouse C',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO commerce_channel_locations(id,stock_location_id,sales_channel_id,created_at,updated_at) VALUES('level-create-channel','level-create-a','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	} {
		if e := db.Exec(q).Error; e != nil {
			t.Fatal("level fixture", e)
		}
	}
}
func checkInventoryLevelCreate(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	seedLevelCreateLocations(t, db)
	ctx := t.Context()
	h := inventory.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	var binding struct{ ID, InventoryItemID string }
	if db.Table("catalog_variants v").Select("v.id,vi.inventory_item_id").Joins("JOIN catalog_variant_inventory_items vi ON vi.variant_id=v.id AND vi.deleted_at IS NULL").Where("v.product_id='variant-create-product' AND v.title='128 GB / Black' AND v.deleted_at IS NULL").Take(&binding).Error != nil {
		t.Fatal("new SKU fixture")
	}
	base := "/api/admin/products/variant-create-product/variants/" + binding.ID + "/inventory/" + binding.InventoryItemID
	read := func() inventory.CreateLevelContext {
		t.Helper()
		var d inventory.CreateLevelContext
		if json.Unmarshal(call("GET", base+"/level-creation", "", 200), &d) != nil {
			t.Fatal("level context")
		}
		return d
	}
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	old := read()
	if old.Enabled || old.SharedVariants != 1 {
		t.Fatal("default create gate")
	}
	in := inventory.CreateLevelInput{OperationID: uuid.NewString(), ExpectedRevision: old.Revision, LocationID: "level-create-a", StockedQuantity: "12.000000000000000001", IncomingQuantity: "2.000000000000000000"}
	call("POST", base+"/levels", body(in), 503)
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.create_level'")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.create_level'")
	exec("CREATE TRIGGER level_create_fault BEFORE INSERT ON inventory_level_creations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial level audit fault'")
	call("POST", base+"/levels", body(in), 503)
	exec("DROP TRIGGER level_create_fault")
	if read().Revision != old.Revision {
		t.Fatal("late level failure not atomic")
	}
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	results := make(chan inventory.CreateLevelResult, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.CreateLevel(ctx, actor, "variant-create-product", binding.ID, binding.InventoryItemID, in)
			if e != nil {
				failed.Store(true)
				return
			}
			results <- r
			if r.Replayed {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	close(results)
	if failed.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("concurrent level creation")
	}
	var first inventory.CreateLevelResult
	for r := range results {
		if first.ID == "" {
			first = r
		}
		if r.ID != first.ID || r.Revision != first.Revision {
			t.Fatal("level receipt changed")
		}
	}
	var quantities struct{ StockedQuantity, ReservedQuantity, IncomingQuantity, Raw string }
	if db.Table("inventory_levels").Select("stocked_quantity,reserved_quantity,incoming_quantity,JSON_UNQUOTE(JSON_EXTRACT(raw_stocked_quantity,'$.value')) AS raw").Where("id=?", first.ID).Take(&quantities).Error != nil || quantities.StockedQuantity != in.StockedQuantity || quantities.Raw != in.StockedQuantity || quantities.IncomingQuantity != in.IncomingQuantity || quantities.ReservedQuantity != "0.000000000000000000" {
		t.Fatal("initial quantities/raw", quantities)
	}
	availability, e := inventory.ReadAvailability(db, []inventory.AvailableVariant{{ID: binding.ID, ManageInventory: true, Complete: true}}, "price-edit-channel", "12")
	if e != nil || !availability[binding.ID].CanFulfill {
		t.Fatal("new SKU public quantity", e)
	}
	unavailable, e := inventory.ReadAvailability(db, []inventory.AvailableVariant{{ID: binding.ID, ManageInventory: true, Complete: true}}, "price-edit-channel", "13")
	if e != nil || unavailable[binding.ID].CanFulfill {
		t.Fatal("oversell", e)
	}
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = read().Revision
	call("POST", base+"/levels", body(duplicate), 409)
	changed := in
	changed.IncomingQuantity = "1.000000000000000000"
	call("POST", base+"/levels", body(changed), 409)
	next := duplicate
	next.LocationID = "level-create-b"
	exec("UPDATE inventory_locations SET name='Warehouse changed',updated_at=UTC_TIMESTAMP(6) WHERE id='level-create-b'")
	call("POST", base+"/levels", body(next), 409)
	next.ExpectedRevision = read().Revision
	// An orphan reservation at a new location is not erased or silently adopted.
	exec("INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,created_at,updated_at) VALUES('level-create-orphan',?,'level-create-b',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", binding.InventoryItemID)
	next.ExpectedRevision = read().Revision
	call("POST", base+"/levels", body(next), 409)
	exec("DELETE FROM inventory_reservations WHERE id='level-create-orphan'")
	exec("INSERT INTO inventory_migration_holds(inventory_item_id,location_id,reason,snapshot_sha256,evidence_sha256,evidence_json,policy_version) VALUES(?,'level-create-b','reservation_mismatch',UNHEX(REPEAT('00',32)),UNHEX(REPEAT('00',32)),'{}',1)", binding.InventoryItemID)
	next.ExpectedRevision = read().Revision
	call("POST", base+"/levels", body(next), 409)
	exec("DELETE FROM inventory_migration_holds WHERE inventory_item_id=? AND location_id='level-create-b'", binding.InventoryItemID)
	next.ExpectedRevision = read().Revision
	next.StockedQuantity = "0.000000000000000000"
	call("POST", base+"/levels", body(next), 200)
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	for _, action := range []string{"inventory.stock.write", "warehouse.read"} {
		exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code=?", role, action)
		call("POST", base+"/levels", body(in), 403)
		exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,?)", role, action)
	}
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("POST", base+"/levels", body(in), 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("UPDATE inventory_levels SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", first.ID)
	call("POST", base+"/levels", body(in), 404)
	exec("UPDATE inventory_levels SET deleted_at=NULL WHERE id=?", first.ID)
	exec("UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='level-create-a'")
	call("POST", base+"/levels", body(in), 404)
	exec("UPDATE inventory_locations SET deleted_at=NULL WHERE id='level-create-a'")
	exec("UPDATE catalog_variant_inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id=? AND inventory_item_id=?", binding.ID, binding.InventoryItemID)
	call("POST", base+"/levels", body(in), 404)
	exec("UPDATE catalog_variant_inventory_items SET deleted_at=NULL WHERE variant_id=? AND inventory_item_id=?", binding.ID, binding.InventoryItemID)
	t.Log(`LEVEL_CREATE_REHEARSAL {"default_disabled":true,"real_new_sku_inventory":true,"current_product_item_warehouse_binding":true,"concurrent_attempts":4,"single_level_receipt":true,"immutable_replay":true,"duplicate_pair_denied":true,"raw_precision":true,"zero_reserved":true,"incoming_not_sellable":true,"public_availability_and_oversell":true,"stale_context_denied":true,"orphan_reservations_and_holds_denied":true,"zero_stock_allowed":true,"late_audit_failure_atomic":true,"current_rights_rechecked":true,"deleted_and_unlinked_replay_denied":true}`)
}
