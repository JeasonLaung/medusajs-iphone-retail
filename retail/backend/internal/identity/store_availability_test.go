package identity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/catalog"
	"gorm.io/gorm"
)

func checkStoreAvailability(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if db.Exec(sql, args...).Error != nil {
			t.Fatal("availability fixture mutation failed")
		}
	}
	exec(`INSERT INTO inventory_items(id,title,requires_shipping,created_at,updated_at,metadata) VALUES('availability-item','Private stock item',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"availability-secret"}'),('availability-second','Second component',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('availability-retired','Retired',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`)
	exec(`UPDATE inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='availability-retired'`)
	exec(`INSERT INTO inventory_locations(id,name,created_at,updated_at,metadata) VALUES('availability-a','Private warehouse A',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"availability-secret"}'),('availability-b','Private warehouse B',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('availability-foreign','Foreign warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('availability-deleted','Removed warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`)
	exec(`UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='availability-deleted'`)
	exec(`INSERT INTO commerce_channel_locations(sales_channel_id,stock_location_id,id,created_at,updated_at) VALUES('public-main','availability-a','availability-channel-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-main','availability-b','availability-channel-b',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-main','availability-deleted','availability-channel-deleted',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-other','availability-foreign','availability-channel-other',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, v := range []struct {
		id                string
		manage, backorder bool
	}{{"public-untracked", false, false}, {"public-backorder", true, true}, {"public-no-items", true, false}, {"public-invalid", true, false}, {"public-dead-item", true, false}, {"public-bundle", true, false}, {"public-incomplete-backorder", true, true}} {
		exec(`INSERT INTO catalog_variants(id,product_id,title,manage_inventory,allow_backorder,variant_rank,created_at,updated_at) VALUES(?,'public-a','Availability fixture',?,?,10,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, v.id, v.manage, v.backorder)
		if v.id != "public-incomplete-backorder" {
			exec(`INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES(?,'public-128')`, v.id)
		}
	}
	for _, r := range []struct{ variant, item, required string }{{"public-v", "availability-item", "2.5"}, {"public-shared", "availability-item", "5"}, {"public-backorder", "availability-item", "2.5"}, {"public-invalid", "availability-item", "0"}, {"public-dead-item", "availability-retired", "1"}, {"public-bundle", "availability-item", "2.5"}, {"public-bundle", "availability-second", "1"}} {
		exec(`INSERT INTO catalog_variant_inventory_items(variant_id,inventory_item_id,required_quantity,id,created_at,updated_at) VALUES(?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, r.variant, r.item, r.required, r.variant+"-"+r.item)
	}
	for _, l := range []struct{ id, item, location, stocked, reserved string }{{"availability-level-a", "availability-item", "availability-a", "12", "2"}, {"availability-level-b", "availability-item", "availability-b", "10", "0"}, {"availability-level-other", "availability-item", "availability-foreign", "1000", "0"}, {"availability-level-deleted-location", "availability-item", "availability-deleted", "1000", "0"}, {"availability-second-a", "availability-second", "availability-a", "1", "0"}, {"availability-second-b", "availability-second", "availability-b", "100", "0"}} {
		exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at,metadata) VALUES(?,?,?,?,?,999,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"availability-secret"}')`, l.id, l.item, l.location, l.stocked, l.reserved)
	}
	exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at,deleted_at) VALUES('availability-soft-deleted','availability-item','availability-a',1000,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,created_at,updated_at,metadata) VALUES('availability-reservation','availability-item','availability-a',2,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"availability-secret"}')`)
	exec(`INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,created_at,updated_at,deleted_at) VALUES('availability-deleted-reservation','availability-item','availability-a',999,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	fingerprint := func() []byte {
		t.Helper()
		out := []json.RawMessage{}
		for _, table := range []string{"catalog_variants", "catalog_variant_inventory_items", "inventory_items", "inventory_locations", "inventory_levels", "inventory_reservations", "inventory_migration_holds", "commerce_channel_locations", "inventory_runtime_schema"} {
			var rows []map[string]any
			if db.Table(table).Order("1,2").Find(&rows).Error != nil {
				t.Fatal("availability fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			out = append(out, b)
		}
		b, _ := json.Marshal(out)
		return b
	}
	call := func(query string, status int) catalog.StoreProduct {
		t.Helper()
		before := fingerprint()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/store/products/public-a"+query, nil)
		r.Header.Set("Cookie", "retail_admin_session=invalid")
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("availability HTTP status %d expected %d", w.Code, status)
		}
		if w.Header().Get("Set-Cookie") != "" || !bytes.Equal(before, fingerprint()) {
			t.Fatal("availability read mutated data or identity")
		}
		for _, secret := range []string{"availability-secret", "availability-item", "availability-a", "availability-b", "availability-foreign", "Private stock", "Private warehouse", "inventory_item_id", "location_id", "required_quantity", "reserved_quantity", "incoming_quantity", "migration_holds"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("availability private field escaped")
			}
		}
		if status != 200 {
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("availability error cacheable")
			}
			return catalog.StoreProduct{}
		}
		var response struct {
			Product      catalog.StoreProduct
			Capabilities map[string]bool
		}
		if json.Unmarshal(w.Body.Bytes(), &response) != nil || !response.Capabilities["availability"] || response.Capabilities["checkout"] {
			t.Fatal("availability capability incorrect")
		}
		return response.Product
	}
	check := func(p catalog.StoreProduct, id, state, quantity, maximum string, allowed bool) {
		t.Helper()
		for _, v := range p.Variants {
			if v.ID != id {
				continue
			}
			a := v.Availability
			if a.State != state || a.CanFulfill != allowed || quantity == "" && v.InventoryQuantity != nil || quantity != "" && (v.InventoryQuantity == nil || *v.InventoryQuantity != quantity) || maximum == "" && a.MaxFulfillableQuantity != nil || maximum != "" && (a.MaxFulfillableQuantity == nil || *a.MaxFulfillableQuantity != maximum) {
				t.Fatalf("availability observation mismatch for artificial variant %s", id)
			}
			return
		}
		t.Fatal("availability fixture variant missing")
	}
	p := call("", 200)
	check(p, "public-v", "available", "8", "4", true)
	check(p, "public-shared", "available", "4", "2", true)
	check(p, "public-untracked", "untracked", "", "", true)
	check(p, "public-no-items", "unavailable", "", "", false)
	check(p, "public-invalid", "unavailable", "", "", false)
	check(p, "public-dead-item", "unavailable", "", "", false)
	check(p, "public-incomplete-backorder", "incomplete", "", "", false)
	p = call("?region_id=price-region&quantity=5", 200)
	check(p, "public-v", "out_of_stock", "8", "4", false)
	check(p, "public-backorder", "backorder", "8", "4", true)
	check(p, "public-bundle", "out_of_stock", "8", "4", false)
	exec(`UPDATE inventory_levels SET reserved_quantity=3 WHERE id='availability-level-a'`)
	p = call("", 200)
	check(p, "public-v", "available", "4", "4", true)
	exec(`UPDATE inventory_levels SET reserved_quantity=2 WHERE id='availability-level-a'`)
	for _, r := range []struct{ id, q string }{{"availability-negative", "-1"}, {"availability-positive", "1"}} {
		exec(`INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,created_at,updated_at) VALUES(?,'availability-item','availability-a',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, r.id, r.q)
	}
	p = call("", 200)
	check(p, "public-v", "available", "4", "4", true)
	exec(`DELETE FROM inventory_reservations WHERE id IN ('availability-negative','availability-positive')`)
	for _, location := range []string{"availability-a", "availability-b"} {
		exec(`INSERT INTO inventory_migration_holds(inventory_item_id,location_id,reason,snapshot_sha256,evidence_sha256,evidence_json,policy_version) VALUES('availability-item',?,'reservation_mismatch',UNHEX(REPEAT('11',32)),UNHEX(REPEAT('22',32)),'{"private":"availability-secret"}',1)`, location)
	}
	p = call("?region_id=price-region&quantity=5", 200)
	check(p, "public-v", "unavailable", "0", "0", false)
	check(p, "public-backorder", "unavailable", "0", "0", false)
	check(p, "public-untracked", "untracked", "", "", true)
	exec(`DELETE FROM inventory_migration_holds WHERE inventory_item_id='availability-item' AND location_id='availability-b'`)
	p = call("", 200)
	check(p, "public-v", "available", "4", "4", true)
	exec(`DELETE FROM inventory_migration_holds WHERE inventory_item_id='availability-item'`)
	exec(`UPDATE inventory_levels SET stocked_quantity=1 WHERE id='availability-level-a'`)
	p = call("", 200)
	check(p, "public-v", "available", "4", "4", true)
	exec(`UPDATE inventory_levels SET stocked_quantity=12 WHERE id='availability-level-a'`)
	exec(`UPDATE inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='availability-item'`)
	p = call("", 200)
	check(p, "public-v", "unavailable", "", "", false)
	exec(`UPDATE inventory_items SET deleted_at=NULL WHERE id='availability-item'`)
	exec(`UPDATE commerce_channel_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE sales_channel_id='public-main'`)
	p = call("", 200)
	check(p, "public-v", "unavailable", "", "", false)
	exec(`UPDATE commerce_channel_locations SET deleted_at=NULL WHERE sales_channel_id='public-main'`)
	exec(`UPDATE inventory_runtime_schema SET reads_enabled=FALSE WHERE id=1`)
	call("", 503)
	exec(`UPDATE inventory_runtime_schema SET reads_enabled=TRUE WHERE id=1`)
	for start := 0; start < 20000; start += 500 {
		rows := []map[string]any{}
		for i := start; i < start+500; i++ {
			rows = append(rows, map[string]any{"id": fmt.Sprintf("availability-overflow-%05d", i), "inventory_item_id": "availability-item", "location_id": "availability-a", "quantity": "0", "created_at": "2026-10-28 00:00:00", "updated_at": "2026-10-28 00:00:00"})
		}
		if db.Table("inventory_reservations").Create(rows).Error != nil {
			t.Fatal("availability overflow seed failed")
		}
		if start == 10000 {
			p = call("", 200)
			check(p, "public-v", "available", "4", "4", true)
		}
	}
	call("", 503)
	exec(`DELETE FROM inventory_reservations WHERE id LIKE 'availability-overflow-%'`)
	checkWholeCartStock(t, db)
	t.Log(`STORE_AVAILABILITY_REHEARSAL {"current_channel_only":true,"required_quantity_exact":true,"shared_inventory":true,"single_warehouse_limit":true,"bundle_component_warehouses":true,"untracked_and_backorder":true,"incomplete_and_missing_links":true,"soft_delete_filters":true,"persistent_holds":true,"live_reservation_mismatch":true,"negative_reservation_rejected":true,"per_pair_reservation_limit":true,"relation_limit_503":true,"disabled_model_503":true,"private_fields_excluded":true,"read_fingerprint_unchanged":true}`)
}
