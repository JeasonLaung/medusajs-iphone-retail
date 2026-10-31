package identity_test

import (
	"bytes"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkWarehouseReservedRelease(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("reserved release fixture", err)
		}
	}
	const device = "warehouse-unpacked"
	const path = "/api/admin/warehouse/locations/intake-location/devices/sale"
	exec("INSERT INTO orders(id,version,status,is_draft_order,currency_code,created_at,updated_at) VALUES('reserved-release-order',1,'pending',FALSE,'eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('reserved-release-other',1,'pending',FALSE,'eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO devices(id,serial_number,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,'WAREHOUSE-UNPACKED','intake-v','intake-source','intake-location',0,'eur','','','reserved','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", device)
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('reserved-release-assignment',?,'reserved-release-order','reserved-release-item','intake-source','reserved',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", device)
	defer exec("DELETE FROM warehouse_device_changes WHERE device_id=?", device)
	defer exec("DELETE FROM device_events WHERE device_id=?", device)
	defer exec("DELETE FROM device_assignments WHERE device_id=?", device)
	defer exec("DELETE FROM devices WHERE id=?", device)
	defer exec("DELETE FROM orders WHERE id IN ('reserved-release-order','reserved-release-other')")
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) VALUES(?,'order.fulfillment.read'),(?,'warehouse.allocation.write')", role, role)
	read := func() devices.WarehouseDevice {
		t.Helper()
		var page devices.WarehouseDevices
		raw := call("GET", path+"?q=WAREHOUSE-UNPACKED", "", 200)
		if bytes.Contains(raw, []byte("reserved-release-order")) || bytes.Contains(raw, []byte("reserved-release-item")) {
			t.Fatal("reservation projection leaked order")
		}
		if json.Unmarshal(raw, &page) != nil || len(page.Devices) != 1 {
			t.Fatal("reservation projection")
		}
		return page.Devices[0]
	}
	input := func() devices.WarehouseDeviceCommand {
		return devices.WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Action: "release_reserved"}
	}
	post := func(in devices.WarehouseDeviceCommand, status int) {
		t.Helper()
		body, _ := json.Marshal(in)
		call("POST", path+"/"+device, string(body), status)
	}
	hasAction := func() bool {
		for _, a := range read().AllowedActions {
			if a == "release_reserved" {
				return true
			}
		}
		return false
	}
	post(input(), 503)
	exec("UPDATE order_device_allocation_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE order_device_allocation_runtime SET enabled=FALSE WHERE id=1")
	if !hasAction() {
		t.Fatal("valid unpacked reservation hidden")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.allocation.write'", role)
	if hasAction() {
		t.Fatal("intake write granted release")
	}
	post(input(), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.allocation.write')", role)
	for _, status := range []string{"draft", "canceled"} {
		exec("UPDATE orders SET status=? WHERE id='reserved-release-order'", status)
		if hasAction() {
			t.Fatal("inactive order offered release")
		}
		post(input(), 409)
	}
	exec("UPDATE orders SET status='pending' WHERE id='reserved-release-order'")
	exec("UPDATE device_assignments SET status='packed' WHERE id='reserved-release-assignment'")
	post(input(), 409)
	exec("UPDATE device_assignments SET status='reserved' WHERE id='reserved-release-assignment'")
	stale := input()
	exec("UPDATE device_assignments SET item_id='changed-item' WHERE id='reserved-release-assignment'")
	post(stale, 409)
	exec("UPDATE device_assignments SET item_id='reserved-release-item' WHERE id='reserved-release-assignment'")
	fingerprint := func() []byte {
		t.Helper()
		var all []any
		for _, table := range []string{"inventory_levels", "inventory_reservations", "fulfillments", "order_items", "payments"} {
			var rows []map[string]any
			if db.Table(table).Order("id").Find(&rows).Error != nil {
				t.Fatal("release unchanged tables")
			}
			all = append(all, rows)
		}
		raw, _ := json.Marshal(all)
		return raw
	}
	before := fingerprint()
	in := input()
	exec("CREATE TRIGGER fail_reserved_release BEFORE INSERT ON warehouse_device_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned release failure'")
	post(in, 503)
	exec("DROP TRIGGER fail_reserved_release")
	if read().State != "reserved" {
		t.Fatal("late failure released device")
	}
	var n int64
	db.Table("device_events").Where("device_id=?", device).Count(&n)
	if n != 0 {
		t.Fatal("late failure retained event")
	}
	// Intake writes are not required for a warehouse allocation operation.
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.intake.write'", role)
	defer exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.intake.write')", role)
	var wins, replays atomic.Int32
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := h.CommandWarehouseDevice(ctx, actor, "intake-location", "sale", device, in)
			if err != nil {
				failures <- err
			} else if out.Replayed {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal("release concurrency", err)
	}
	if wins.Load() != 1 || replays.Load() != 3 || read().State != "available" || !bytes.Equal(before, fingerprint()) {
		t.Fatal("release changed quantities or replay count")
	}
	post(in, 200)
	db.Table("device_events").Where("device_id=? AND action='release'", device).Count(&n)
	if n != 1 {
		t.Fatal("release audit duplicated")
	}
	exec("UPDATE device_assignments SET order_id='reserved-release-other' WHERE id='reserved-release-assignment'")
	post(in, 409)
	exec("UPDATE device_assignments SET order_id='reserved-release-order' WHERE id='reserved-release-assignment'")
	exec("UPDATE devices SET location_id='user-scope-b' WHERE id=?", device)
	post(in, 404)
	exec("UPDATE devices SET location_id='intake-location' WHERE id=?", device)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.allocation.write'", role)
	post(in, 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.allocation.write')", role)
	t.Log(`WAREHOUSE_RESERVED_RELEASE_REHEARSAL {"real_mysql_http":true,"allocation_permission_only":true,"no_order_projection":true,"current_scope_and_binding":true,"unpacked_only":true,"inactive_order_denied":true,"changed_assignment_revision_denied":true,"late_failure_atomic":true,"concurrency":4,"one_release_event":true,"quantity_and_reservations_unchanged":true,"changed_moved_revoked_replay_denied":true}`)
}
