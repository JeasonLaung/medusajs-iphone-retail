package identity_test

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedOrderDevicePacking(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES('packing-set','Packing','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES('packing-zone','Packing zone','packing-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO fulfillment_option_types(id,label,code,created_at,updated_at) VALUES('packing-type','Packing','packing',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO fulfillment_shipping_profiles(id,name,type,created_at,updated_at) VALUES('packing-profile','Packing','default',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT IGNORE INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('manual_manual',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE fulfillment_providers SET is_enabled=TRUE WHERE id='manual_manual'`,
		`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,shipping_profile_id,provider_id,shipping_option_type_id,created_at,updated_at) VALUES('packing-option','Packing standard','flat','packing-zone','packing-profile','manual_manual','packing-type',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO fulfillment_location_providers(id,stock_location_id,fulfillment_provider_id,created_at,updated_at) VALUES('packing-provider-link','allocation-location','manual_manual',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO fulfillment_location_sets(id,stock_location_id,fulfillment_set_id,created_at,updated_at) VALUES('packing-set-link','allocation-location','packing-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_product_shipping_profiles(id,product_id,shipping_profile_id,created_at,updated_at) VALUES('packing-profile-link','allocation-product','packing-profile',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_addresses(id,first_name,last_name,address_1,city,country_code,postal_code,created_at,updated_at) VALUES('packing-address','Artificial','Recipient','Fixture road','Fixture city','dk','0000',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE order_line_items SET variant_barcode='PACK-BC' WHERE id='allocation-line'`,
		`UPDATE orders SET shipping_address_id='packing-address' WHERE id='allocation-order'`,
		`INSERT INTO order_shipping_methods(id,name,amount,raw_amount,is_tax_inclusive,is_custom_amount,shipping_option_id,created_at,updated_at) VALUES('packing-method','Packing standard',0,'{"value":"0","precision":38}',FALSE,FALSE,'packing-option',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_shipping_versions(id,order_id,version,shipping_method_id,created_at,updated_at) VALUES('packing-shipping-version','allocation-order',1,'packing-method',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,raw_stocked_quantity,raw_reserved_quantity,created_at,updated_at) VALUES('packing-target','allocation-stock','allocation-location',5.000000000000000001,0,0,'{"value":"5.000000000000000001","precision":38}','{"value":"0","precision":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('packing-original','allocation-stock','allocation-other',5,2,0,'{"value":"5","precision":38}','{"value":"2","precision":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO inventory_reservations(id,inventory_item_id,location_id,line_item_id,quantity,raw_quantity,allow_backorder,created_at,updated_at) VALUES('packing-reservation','allocation-stock','allocation-other','allocation-line',2,'{"value":"2","precision":38}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	} {
		if e := db.Exec(q).Error; e != nil {
			t.Fatal("packing fixture", e)
		}
	}
}
func checkOrderDevicePacking(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	seedOrderDevicePacking(t, db)
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	exec("UPDATE order_device_allocation_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE order_device_allocation_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE order_device_packing_runtime SET enabled=FALSE WHERE id=1")
	const path = "/api/admin/orders/allocation-order/packing"
	h := devices.Handler{DB: db}
	ctx := t.Context()
	body := func(in devices.AllocationInput) string { b, _ := json.Marshal(in); return string(b) }
	read := func() devices.PackingContext {
		t.Helper()
		b := call("GET", path, "", 200)
		var out devices.PackingContext
		if json.Unmarshal(b, &out) != nil {
			t.Fatal("packing read")
		}
		return out
	}
	// The prior release chooses a UUID-sorted assignment. Reserve whichever
	// of A/B is available; fixtures must not rely on UUID ordering.
	alloc, e := h.ReadAllocation(ctx, actor, "allocation-order", "allocation-line")
	if e != nil {
		t.Fatal(e)
	}
	available := ""
	for _, d := range alloc.Devices {
		if d.ID == "allocation-a" || d.ID == "allocation-b" {
			available = d.ID
		}
	}
	if available == "" {
		t.Fatal("packing fixture missing available A/B")
	}
	if _, e = h.Allocate(ctx, actor, "allocation-order", "allocation-line", devices.AllocationInput{OperationID: uuid.NewString(), ExpectedRevision: alloc.Revision, Action: "reserve", IDs: []string{available}}); e != nil {
		t.Fatal(e)
	}
	before := read()
	if before.Enabled || len(before.Devices) != 2 || len(before.Options) != 1 || !before.Options[0].Supported {
		t.Fatal("packing default and candidates")
	}
	var bID, aID string
	for _, d := range before.Devices {
		if d.DeviceID == "allocation-b" {
			bID = d.AssignmentID
		} else if d.DeviceID == "allocation-a" {
			aID = d.AssignmentID
		}
	}
	in := devices.AllocationInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Action: "reserve", IDs: []string{bID}}
	call("POST", path, body(in), 503)
	exec("UPDATE order_device_packing_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE payment_collections SET status='authorized' WHERE id='allocation-payment'")
	in.ExpectedRevision = read().Revision
	call("POST", path, body(in), 409)
	exec("UPDATE payment_collections SET status='completed' WHERE id='allocation-payment'")
	in.ExpectedRevision = read().Revision

	exec("UPDATE devices SET source_id='allocation-source-other',location_id='allocation-other' WHERE id='allocation-a'")
	exec("UPDATE device_assignments SET source_id='allocation-source-other' WHERE id=?", aID)
	mixed := in
	mixed.IDs = []string{aID, bID}
	mixed.ExpectedRevision = read().Revision
	call("POST", path, body(mixed), 409)
	exec("UPDATE devices SET source_id='allocation-source',location_id='allocation-location' WHERE id='allocation-a'")
	exec("UPDATE device_assignments SET source_id='allocation-source' WHERE id=?", aID)
	exec("UPDATE inventory_levels SET stocked_quantity=0.5 WHERE id='packing-target'")
	in.ExpectedRevision = read().Revision
	call("POST", path, body(in), 409)
	exec("UPDATE inventory_levels SET stocked_quantity=5.000000000000000001 WHERE id='packing-target'")
	exec("INSERT INTO inventory_reservations(id,inventory_item_id,location_id,line_item_id,quantity,raw_quantity,allow_backorder,created_at,updated_at) VALUES('packing-duplicate','allocation-stock','allocation-other','allocation-line',1,'{\"value\":\"1\",\"precision\":38}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	in.ExpectedRevision = read().Revision
	call("POST", path, body(in), 409)
	exec("DELETE FROM inventory_reservations WHERE id='packing-duplicate'")
	exec("INSERT INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('packing-external',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE fulfillment_shipping_options SET provider_id='packing-external' WHERE id='packing-option'")
	in.ExpectedRevision = read().Revision
	call("POST", path, body(in), 503)
	exec("UPDATE fulfillment_shipping_options SET provider_id='manual_manual' WHERE id='packing-option'")
	in.ExpectedRevision = read().Revision

	// A stock writer can commit after this request established an RR snapshot.
	// Packing waits for its marker lock and must then see the writer's stock.
	blocker := db.Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	if blocker.Exec("SELECT id FROM inventory_runtime_schema WHERE id=1 FOR SHARE").Error != nil {
		blocker.Rollback()
		t.Fatal("packing inventory blocker")
	}
	if blocker.Exec("UPDATE inventory_levels SET incoming_quantity=7 WHERE id='packing-target'").Error != nil {
		blocker.Rollback()
		t.Fatal("packing pending stock")
	}
	finished := make(chan error, 1)
	go func() { _, e := h.Pack(ctx, actor, "allocation-order", in); finished <- e }()
	select {
	case e := <-finished:
		blocker.Rollback()
		t.Fatal("packing skipped inventory writer lock", e)
	case <-time.After(150 * time.Millisecond):
	}
	if blocker.Commit().Error != nil {
		t.Fatal("packing blocker commit")
	}
	select {
	case e := <-finished:
		if !errors.Is(e, devices.ErrConflict) {
			t.Fatal("packing used stale stock snapshot", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("packing lock did not resume")
	}
	exec("UPDATE inventory_levels SET incoming_quantity=0 WHERE id='packing-target'")
	in.ExpectedRevision = read().Revision
	// Concurrent stock change invalidates the form even without device changes.
	exec("UPDATE inventory_levels SET incoming_quantity=1 WHERE id='packing-target'")
	call("POST", path, body(in), 409)
	in.ExpectedRevision = read().Revision
	exec("CREATE TRIGGER packing_receipt_fault BEFORE INSERT ON order_device_packings FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial packing audit fault'")
	call("POST", path, body(in), 503)
	exec("DROP TRIGGER packing_receipt_fault")
	var n int64
	db.Table("fulfillments").Where("shipping_option_id='packing-option'").Count(&n)
	if n != 0 || len(read().Devices) != 2 || read().Revision != in.ExpectedRevision {
		t.Fatal("packing late rollback")
	}
	var fresh, replay, bad atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.Pack(ctx, actor, "allocation-order", in)
			if e != nil {
				bad.Add(1)
			} else if r.Replayed {
				replay.Add(1)
			} else {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if fresh.Load() != 1 || replay.Load() != 3 || bad.Load() != 0 {
		t.Fatal("packing concurrency", fresh.Load(), replay.Load(), bad.Load())
	}
	check := func(target, original, reserved, fulfilled string) {
		t.Helper()
		var stock, raw, old, remain, done string
		db.Raw("SELECT stocked_quantity,JSON_UNQUOTE(JSON_EXTRACT(raw_stocked_quantity,'$.value')) FROM inventory_levels WHERE id='packing-target'").Row().Scan(&stock, &raw)
		db.Raw("SELECT reserved_quantity FROM inventory_levels WHERE id='packing-original'").Row().Scan(&old)
		db.Raw("SELECT quantity FROM inventory_reservations WHERE id='packing-reservation'").Row().Scan(&remain)
		db.Raw("SELECT fulfilled_quantity FROM order_items WHERE id='allocation-order-item'").Row().Scan(&done)
		if stock != target || raw != target || old != original || remain != reserved || done != fulfilled {
			t.Fatal("packing stock ledger", stock, raw, old, remain, done)
		}
	}
	check("4.000000000000000001", "1.000000000000000000", "1.000000000000000000", "1.000000000000000000")
	var receipt struct{ BeforeJSON, AfterJSON string }
	if db.Table("order_device_packings").Select("before_json,after_json").Where("operation_id=?", in.OperationID).Take(&receipt).Error != nil {
		t.Fatal("packing audit missing")
	}
	auditQuantity := func(raw string) string {
		t.Helper()
		var audit struct {
			Stock []struct {
				ID       string `json:"id"`
				Quantity string `json:"stocked_quantity"`
			} `json:"stock"`
		}
		if json.Unmarshal([]byte(raw), &audit) != nil {
			t.Fatal("packing audit invalid")
		}
		for _, row := range audit.Stock {
			if row.ID == "packing-target" {
				return row.Quantity
			}
		}
		return ""
	}
	if auditQuantity(receipt.BeforeJSON) != "5.000000000000000001" || auditQuantity(receipt.AfterJSON) != "4.000000000000000001" {
		t.Fatal("packing exact audit ledger")
	}

	call("GET", "/api/admin/orders/allocation-order/fulfillments", "", 200)
	var barcode string
	db.Table("fulfillment_items").Select("barcode").Where("line_item_id=?", "allocation-line").Limit(1).Scan(&barcode)
	if barcode != "PACK-BC" {
		t.Fatal("fulfillment barcode snapshot")
	}
	after := read()
	if len(after.Devices) != 1 || after.Devices[0].AssignmentID != aID {
		t.Fatal("packing remainder")
	}
	call("POST", path, body(in), 200)
	altered := in
	altered.IDs = []string{aID}
	call("POST", path, body(altered), 409)
	// A migration discrepancy must never be silently repaired while packing.
	next := devices.AllocationInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, Action: "reserve", IDs: []string{aID}}
	exec("UPDATE inventory_levels SET reserved_quantity=2 WHERE id='packing-original'")
	next.ExpectedRevision = read().Revision
	call("POST", path, body(next), 409)
	exec("UPDATE inventory_levels SET reserved_quantity=1 WHERE id='packing-original'")
	// Packing without an order reservation still consumes available stock once.
	exec("UPDATE inventory_reservations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='packing-reservation'")
	exec("UPDATE inventory_levels SET reserved_quantity=0 WHERE id='packing-original'")
	next.ExpectedRevision = read().Revision
	call("POST", path, body(next), 200)
	var final struct{ StockedQuantity string }
	db.Table("inventory_levels").Select("stocked_quantity").Where("id='packing-target'").Take(&final)
	if final.StockedQuantity != "3.000000000000000001" || len(read().Devices) != 0 {
		t.Fatal("no-reservation pack")
	}
	exec("UPDATE devices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='allocation-b'")
	call("POST", path, body(in), 404)
	exec("UPDATE devices SET deleted_at=NULL WHERE id='allocation-b'")
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='order.allocation.write'", role)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'order.allocation.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", path, "", 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	t.Log(`ORDER_DEVICE_PACKING_REHEARSAL {"default_disabled":true,"paid_gate":true,"mixed_source_rejected":true,"insufficient_stock_and_multiple_reservations_rejected":true,"unsupported_provider_not_replaced":true,"deleted_replay_target_denied":true,"stale_stock_rejected":true,"waited_writer_fresh_snapshot":true,"concurrent_attempts":4,"single_parcel_and_receipt":true,"late_receipt_failure_atomic":true,"partial_pack_cross_warehouse_reservation_remainder":true,"exact_stock_raw_and_fulfilled_quantities":true,"existing_fulfillment_read":true,"barcode_snapshot_preserved":true,"readable_exact_inventory_audit":true,"immutable_replay":true,"reservation_discrepancy_rejected":true,"no_reservation_consumes_once":true,"current_rights_rechecked":true,"warehouse_full_store_denied":true}`)
}
