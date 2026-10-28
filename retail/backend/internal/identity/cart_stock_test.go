package identity_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/inventory"
	"gorm.io/gorm"
)

func checkWholeCartStock(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("owned whole-cart stock fixture failed")
		}
	}
	fingerprint := func() [32]byte {
		t.Helper()
		var all []json.RawMessage
		for _, table := range []string{"inventory_levels", "inventory_reservations", "inventory_migration_holds", "catalog_variant_inventory_items", "inventory_locations", "inventory_items", "commerce_channel_locations"} {
			var rows []map[string]any
			order := "id"
			if table == "inventory_migration_holds" {
				order = "inventory_item_id,location_id,reason"
			}
			if db.Table(table).Order(order).Find(&rows).Error != nil {
				t.Fatal("whole-cart fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return sha256.Sum256(b)
	}
	variants := []inventory.AvailableVariant{{ID: "public-v", ManageInventory: true, Complete: true}, {ID: "public-shared", ManageInventory: true, Complete: true}, {ID: "public-backorder", ManageInventory: true, AllowBackorder: true, Complete: true}}
	lines := []inventory.CartStockLine{{ID: "one", VariantID: "public-v", Quantity: "4"}, {ID: "two", VariantID: "public-shared", Quantity: "2"}}
	call := func(state string, allowed bool) inventory.CartStockPlan {
		t.Helper()
		before := fingerprint()
		var out inventory.CartStockPlan
		err := db.Transaction(func(tx *gorm.DB) error {
			var e error
			out, e = inventory.ReadCartStock(tx, variants, lines, "public-main")
			return e
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil || out.State != state || out.CanFulfill != allowed {
			t.Fatalf("real SQL whole-cart state differs: %s %v", out.State, err)
		}
		if before != fingerprint() {
			t.Fatal("stock observation reserved or mutated inventory")
		}
		public, _ := json.Marshal(out)
		for _, field := range []string{"LocationID", "InventoryItemID", "Assignments", "availability-a"} {
			if strings.Contains(string(public), field) {
				t.Fatal("stock allocation exposed")
			}
		}
		return out
	}
	out := call("available", true)
	if len(out.Assignments) != 2 || out.Assignments[0].LocationID == out.Assignments[1].LocationID {
		t.Fatal("real shared stock not spread across line warehouses")
	}
	// Component one exists only at a, component two only at b. One bundle may
	// use those two component warehouses; no component's quantity is split.
	savedLines := append([]inventory.CartStockLine(nil), lines...)
	variants = append(variants, inventory.AvailableVariant{ID: "public-bundle", ManageInventory: true, Complete: true})
	exec(`UPDATE inventory_levels SET deleted_at=UTC_TIMESTAMP(6) WHERE id IN ('availability-level-b','availability-second-a')`)
	lines = []inventory.CartStockLine{{ID: "bundle-line", VariantID: "public-bundle", Quantity: "1"}}
	bundle := call("available", true)
	if len(bundle.Assignments) != 2 || bundle.Assignments[0].LocationID == bundle.Assignments[1].LocationID {
		t.Fatal("bundle components wrongly required one warehouse")
	}
	exec(`UPDATE inventory_levels SET deleted_at=NULL WHERE id IN ('availability-level-b','availability-second-a')`)
	lines = savedLines
	// Each line individually fits at a. The whole cart needs twice its capacity.
	exec(`UPDATE commerce_channel_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='availability-channel-b'`)
	call("out_of_stock", false)
	lines[1].VariantID = "public-backorder"
	lines[1].Quantity = "4"
	call("backorder", true)
	exec(`UPDATE inventory_levels SET reserved_quantity=3 WHERE id='availability-level-a'`)
	call("unavailable", false)
	exec(`UPDATE inventory_levels SET reserved_quantity=2 WHERE id='availability-level-a'`)
	exec(`INSERT INTO inventory_migration_holds(inventory_item_id,location_id,reason,snapshot_sha256,evidence_sha256,evidence_json,policy_version) VALUES('availability-item','availability-a','reservation_mismatch',UNHEX(REPEAT('11',32)),UNHEX(REPEAT('22',32)),'{}',1)`)
	call("unavailable", false)
	exec(`DELETE FROM inventory_migration_holds WHERE inventory_item_id='availability-item' AND location_id='availability-a'`)
	exec(`UPDATE commerce_channel_locations SET deleted_at=NULL WHERE id='availability-channel-b'`)
	exec(`UPDATE catalog_variant_inventory_items SET required_quantity=0.000000000000000001 WHERE variant_id='public-v'`)
	exec(`UPDATE inventory_levels SET stocked_quantity=2.000000000000000001 WHERE id='availability-level-a'`)
	exec(`UPDATE commerce_channel_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='availability-channel-b'`)
	lines = lines[:1]
	lines[0].Quantity = "1"
	out = call("available", true)
	if len(out.Assignments) != 1 || out.Assignments[0].Quantity != "0.000000000000000001" {
		t.Fatal("SQL stock requirement lost final decimal")
	}
	lines[0].Quantity = "2"
	call("out_of_stock", false)
	exec(`UPDATE catalog_variant_inventory_items SET required_quantity=2.5 WHERE variant_id='public-v'`)
	exec(`UPDATE inventory_levels SET stocked_quantity=12 WHERE id='availability-level-a'`)
	exec(`UPDATE commerce_channel_locations SET deleted_at=NULL WHERE id='availability-channel-b'`)
	if _, err := inventory.ReadCartStock(db, variants, lines, "public-main"); err != inventory.ErrCartStockInput {
		t.Fatal("autocommit stock check accepted")
	}
	t.Log(`CART_STOCK_REHEARSAL {"real_mysql":true,"whole_cart_shared_capacity":true,"separate_line_warehouses":true,"separate_component_warehouses":true,"exact_scale18":true,"foreign_and_incoming_excluded":true,"holds_and_live_mismatch":true,"backorder_separate":true,"no_inventory_mutation":true,"checkout_integrated":false}`)
}
