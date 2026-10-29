package identity_test

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkOrderDeviceParcels(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("parcel fixture", e)
		}
	}
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	defer exec("UPDATE order_device_parcel_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE order_device_allocation_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE order_device_packing_runtime SET enabled=FALSE WHERE id=1")
	var bID, aID string
	db.Table("device_assignments").Select("fulfillment_id").Where("device_id='allocation-b' AND status='packed'").Scan(&bID)
	db.Table("device_assignments").Select("fulfillment_id").Where("device_id='allocation-a' AND status='packed'").Scan(&aID)
	if bID == "" || aID == "" || aID == bID {
		t.Fatal("two owned parcel fixtures required")
	}
	path := func(f string) string { return "/api/admin/orders/allocation-order/fulfillments/" + f + "/operations" }
	body := func(in devices.ParcelInput) string { b, _ := json.Marshal(in); return string(b) }
	read := func(f string) devices.ParcelContext {
		t.Helper()
		var out devices.ParcelContext
		if json.Unmarshal(call("GET", path(f), "", 200), &out) != nil {
			t.Fatal("parcel context")
		}
		return out
	}
	stock := func() string {
		t.Helper()
		var all []json.RawMessage
		for _, table := range []string{"inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("id").Find(&rows).Error != nil {
				t.Fatal("parcel stock fingerprint")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	before := read(bID)
	if before.Enabled || before.Status != "packed" || len(before.Devices) != 1 || !before.Supported {
		t.Fatal("parcel default")
	}
	cancel := devices.ParcelInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Action: "cancel"}
	call("POST", path(bID), body(cancel), 503)
	exec("UPDATE order_device_parcel_runtime SET enabled=TRUE WHERE id=1")
	// Cancellation remains available with new packing/allocation disabled.
	exec("UPDATE order_device_allocation_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE order_device_packing_runtime SET enabled=FALSE WHERE id=1")

	call("POST", path(bID), `{"action":"cancel"}`, 400)
	call("GET", path(bID)+"?unexpected=1", "", 400)
	call("GET", "/api/admin/orders/missing-order/fulfillments/"+bID+"/operations", "", 404)
	exec("UPDATE inventory_levels SET reserved_quantity=1 WHERE id='packing-target'")
	cancel.ExpectedRevision = read(bID).Revision
	call("POST", path(bID), body(cancel), 409)
	exec("UPDATE inventory_levels SET reserved_quantity=0 WHERE id='packing-target'")
	oldRevision := read(bID).Revision
	exec("UPDATE fulfillments SET updated_at=DATE_ADD(updated_at,INTERVAL 1 SECOND) WHERE id=?", bID)
	cancel.ExpectedRevision = oldRevision
	call("POST", path(bID), body(cancel), 409)
	cancel.ExpectedRevision = read(bID).Revision
	exec("CREATE TRIGGER parcel_receipt_fault BEFORE INSERT ON order_device_parcel_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial parcel audit fault'")
	baseline := stock()
	call("POST", path(bID), body(cancel), 503)
	exec("DROP TRIGGER parcel_receipt_fault")
	if stock() != baseline || read(bID).Status != "packed" {
		t.Fatal("parcel rollback")
	}
	var fresh, replay, bad atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.ChangeParcel(ctx, actor, "allocation-order", bID, cancel)
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
		t.Fatal("parcel cancel concurrency", fresh.Load(), replay.Load(), bad.Load())
	}
	check := func(stock, reserved, fulfilled, shipped string) {
		t.Helper()
		var row struct{ StockedQuantity, ReservedQuantity string }
		db.Table("inventory_levels").Select("stocked_quantity,reserved_quantity").Where("id='packing-target'").Take(&row)
		var line struct{ FulfilledQuantity, ShippedQuantity string }
		db.Table("order_items").Select("fulfilled_quantity,shipped_quantity").Where("id='allocation-order-item'").Take(&line)
		if row.StockedQuantity != stock || row.ReservedQuantity != reserved || line.FulfilledQuantity != fulfilled || line.ShippedQuantity != shipped {
			t.Fatal("parcel quantities", row, line)
		}
	}
	check("4.000000000000000001", "1.000000000000000000", "1.000000000000000000", "0.000000000000000000")
	var state, status string
	var fID *string
	db.Raw("SELECT d.state,a.status,a.fulfillment_id FROM devices d JOIN device_assignments a ON a.device_id=d.id AND a.status='reserved' AND a.deleted_at IS NULL WHERE d.id='allocation-b'").Row().Scan(&state, &status, &fID)
	if state != "reserved" || status != "reserved" || fID != nil || read(bID).Status != "canceled" {
		t.Fatal("cancel preserves preallocation")
	}
	call("POST", path(bID), body(cancel), 200)
	changed := cancel
	changed.Action = "ship"
	changed.TrackingNumber = "DIFFERENT"
	call("POST", path(bID), body(changed), 409)
	// Repack the same still-reserved device. The restored quantity is consumed once.
	exec("UPDATE order_device_allocation_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE order_device_packing_runtime SET enabled=TRUE WHERE id=1")
	packing, e := h.ReadPacking(ctx, actor, "allocation-order")
	if e != nil {
		t.Fatal(e)
	}
	if len(packing.Devices) != 1 || packing.Devices[0].DeviceID != "allocation-b" {
		t.Fatal("cancelled device not available for repacking")
	}
	repacked, e := h.Pack(ctx, actor, "allocation-order", devices.AllocationInput{OperationID: uuid.NewString(), ExpectedRevision: packing.Revision, Action: "reserve", IDs: []string{packing.Devices[0].AssignmentID}})
	if e != nil {
		t.Fatal("repacking", e)
	}
	check("3.000000000000000001", "0.000000000000000000", "2.000000000000000000", "0.000000000000000000")
	// Original cancellation adds back to an existing remainder at its original
	// location, while actual stock returns to the parcel's warehouse.
	exec("UPDATE order_items SET quantity=3,raw_quantity='{\"value\":\"3\",\"precision\":38}' WHERE id='allocation-order-item'")
	exec("UPDATE inventory_levels SET reserved_quantity=1,raw_reserved_quantity='{\"value\":\"1\",\"precision\":38}' WHERE id='packing-original'")
	exec("INSERT INTO inventory_reservations(id,inventory_item_id,location_id,line_item_id,quantity,raw_quantity,allow_backorder,created_at,updated_at) VALUES('parcel-remainder','allocation-stock','allocation-other','allocation-line',1,'{\"value\":\"1\",\"precision\":38}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	ship := devices.ParcelInput{OperationID: uuid.NewString(), ExpectedRevision: read(repacked.FulfillmentID).Revision, Action: "ship", TrackingNumber: "TRACK-001", TrackingURL: "https://tracking.example.invalid/TRACK-001"}
	// The entire parcel must be allocated in full before shipment.
	exec("UPDATE fulfillment_items SET quantity=2 WHERE fulfillment_id=?", repacked.FulfillmentID)
	ship.ExpectedRevision = read(repacked.FulfillmentID).Revision
	call("POST", path(repacked.FulfillmentID), body(ship), 409)
	exec("UPDATE fulfillment_items SET quantity=1 WHERE fulfillment_id=?", repacked.FulfillmentID)
	ship.ExpectedRevision = read(repacked.FulfillmentID).Revision
	baseline = stock()
	exec("CREATE TRIGGER parcel_ship_fault BEFORE INSERT ON order_device_parcel_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial shipment fault'")
	call("POST", path(repacked.FulfillmentID), body(ship), 503)
	exec("DROP TRIGGER parcel_ship_fault")
	if stock() != baseline || read(repacked.FulfillmentID).Status != "packed" {
		t.Fatal("shipment rollback")
	}
	fresh.Store(0)
	replay.Store(0)
	bad.Store(0)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.ChangeParcel(ctx, actor, "allocation-order", repacked.FulfillmentID, ship)
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
	if fresh.Load() != 1 || replay.Load() != 3 || bad.Load() != 0 || stock() != baseline {
		t.Fatal("shipment uniqueness or inventory changed", fresh.Load(), replay.Load(), bad.Load())
	}
	shipped := read(repacked.FulfillmentID)
	if shipped.Status != "shipped" || len(shipped.Labels) != 1 || shipped.Labels[0].TrackingNumber != "TRACK-001" || shipped.Labels[0].TrackingURL != ship.TrackingURL || len(shipped.Devices) != 1 || shipped.Devices[0].Status != "shipped" {
		t.Fatal("shipment status and tracking")
	}
	check("3.000000000000000001", "0.000000000000000000", "2.000000000000000000", "1.000000000000000000")
	twice := ship
	twice.OperationID = uuid.NewString()
	twice.ExpectedRevision = shipped.Revision
	call("POST", path(repacked.FulfillmentID), body(twice), 409)
	twice.Action = "cancel"
	twice.TrackingNumber = ""
	twice.TrackingURL = ""
	call("POST", path(repacked.FulfillmentID), body(twice), 409)
	secondCancel := devices.ParcelInput{OperationID: uuid.NewString(), ExpectedRevision: read(aID).Revision, Action: "cancel"}
	call("POST", path(aID), body(secondCancel), 200)
	check("4.000000000000000001", "0.000000000000000000", "1.000000000000000000", "1.000000000000000000")
	var remainder, oldReserved string
	db.Raw("SELECT quantity FROM inventory_reservations WHERE id='parcel-remainder'").Row().Scan(&remainder)
	db.Raw("SELECT reserved_quantity FROM inventory_levels WHERE id='packing-original'").Row().Scan(&oldReserved)
	if remainder != "2.000000000000000000" || oldReserved != remainder {
		t.Fatal("existing cross warehouse reservation not restored")
	}
	call("GET", "/api/admin/orders/allocation-order/fulfillments", "", 200)
	exec("UPDATE devices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='allocation-b'")
	call("POST", path(repacked.FulfillmentID), body(ship), 404)
	exec("UPDATE devices SET deleted_at=NULL WHERE id='allocation-b'")
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='order.allocation.write'", role)
	call("POST", path(repacked.FulfillmentID), body(ship), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'order.allocation.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", path(aID), "", 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	t.Log(`ORDER_DEVICE_PARCEL_REHEARSAL {"default_disabled":true,"strict_http_and_order_binding":true,"stale_parcel_and_reservation_discrepancy_rejected":true,"independent_from_new_packing_capability":true,"cancel_concurrent_attempts":4,"one_cancel_receipt":true,"restore_stock_and_new_reservation":true,"devices_remain_reserved":true,"repack_consumes_restored_quantity_once":true,"restore_existing_reservation_at_original_location":true,"ship_concurrent_attempts":4,"single_shipment_label_and_receipt":true,"shipment_does_not_change_inventory":true,"whole_parcel_quantity_required":true,"cancel_and_ship_late_failure_atomic":true,"tracking_and_shipped_counters":true,"repeat_or_cancel_shipped_denied":true,"immutable_replay":true,"deleted_target_replay_denied":true,"revoked_and_warehouse_denied":true}`)
}
