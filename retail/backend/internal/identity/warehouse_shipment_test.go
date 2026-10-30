package identity_test

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkWarehouseShipment(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte, f string, in devices.ParcelInput) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("warehouse shipment fixture")
		}
	}
	var location, role string
	if db.Table("fulfillments").Select("location_id").Where("id=?", f).Scan(&location).Error != nil || location == "" {
		t.Fatal("shipment location")
	}
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("shipment role")
	}
	exec("UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE warehouse_scope_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	defer exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_scopes(user_id,mode) VALUES(?,'selected')", actor.ID)
	defer exec("DELETE FROM authorization_warehouse_scopes WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, location)
	defer exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	path := "/api/admin/warehouse/locations/" + location + "/orders/allocation-order/fulfillments/" + f + "/shipment"
	body := func(in devices.ParcelInput) string { v, _ := json.Marshal(in); return string(v) }
	var before devices.WarehouseShipment
	raw := call("GET", path, "", 200)
	if json.Unmarshal(raw, &before) != nil || before.Revision != in.ExpectedRevision || before.Status != "packed" {
		t.Fatal("warehouse shipment revision")
	}
	for _, key := range []string{"payment", "cost", "source_id", "imei", "address", "devices", "items"} {
		if strings.Contains(string(raw), key) {
			t.Fatal("private shipping projection", key)
		}
	}
	call("GET", path+"?fields=*", "", 400)
	call("POST", "/api/admin/orders/allocation-order/fulfillments/"+f+"/operations", body(in), 403)
	invalid := in
	invalid.Action = "cancel"
	invalid.TrackingNumber = ""
	invalid.TrackingURL = ""
	call("POST", path, body(invalid), 400)
	stale := in
	stale.ExpectedRevision = strings.Repeat("0", 64)
	call("POST", path, body(stale), 409)
	exec("UPDATE fulfillments SET location_id='user-scope-b' WHERE id=?", f)
	call("POST", path, body(in), 404)
	exec("UPDATE fulfillments SET location_id=? WHERE id=?", location, f)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.shipment.write'", role)
	call("GET", path, "", 403)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.shipment.write')", role)
	exec("CREATE TRIGGER warehouse_ship_fault BEFORE INSERT ON order_device_parcel_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial warehouse shipment fault'")
	call("POST", path, body(in), 503)
	exec("DROP TRIGGER warehouse_ship_fault")
	var fresh, replays, bad atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.ChangeWarehouseShipment(ctx, actor, location, "allocation-order", f, in)
			if e != nil {
				bad.Add(1)
			} else if r.Replayed {
				replays.Add(1)
			} else {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if fresh.Load() != 1 || replays.Load() != 3 || bad.Load() != 0 {
		t.Fatal("warehouse shipment concurrency", fresh.Load(), replays.Load(), bad.Load())
	}
	call("POST", path, body(in), 200)
	var assignmentID string
	if db.Table("device_assignments").Select("id").Where("device_id='allocation-b' AND fulfillment_id=? AND deleted_at IS NULL", f).Scan(&assignmentID).Error != nil || assignmentID == "" {
		t.Fatal("shipment assignment fixture")
	}
	exec("UPDATE device_assignments SET fulfillment_id=NULL WHERE id=?", assignmentID)
	call("POST", path, body(in), 404)
	exec("UPDATE device_assignments SET fulfillment_id=? WHERE id=?", f, assignmentID)
	next := in
	next.OperationID = uuid.NewString()
	call("POST", path, body(next), 409)
	exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, location)
	exec("UPDATE devices SET location_id='user-scope-b' WHERE id='allocation-b'")
	call("POST", path, body(in), 404)
	exec("UPDATE devices SET location_id=? WHERE id='allocation-b'", location)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.shipment.write'", role)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.shipment.write')", role)
	t.Log(`WAREHOUSE_SHIPMENT_REHEARSAL {"real_gin_mysql":true,"same_shared_shipping_command":true,"no_full_store_access":true,"strict_projection":true,"cancel_denied":true,"stale_moved_and_revoked_denied":true,"concurrent_attempts":4,"fresh":1,"replayed":3,"late_failure_rollback":true,"revoked_scope_and_moved_device_replay_denied":true}`)
}
