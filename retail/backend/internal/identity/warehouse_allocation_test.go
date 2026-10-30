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

func checkWarehouseAllocation(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte, f string) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	stock := func() string {
		t.Helper()
		all := []any{}
		for _, table := range []string{"inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("id").Find(&rows).Error != nil {
				t.Fatal("warehouse allocation stock snapshot")
			}
			all = append(all, rows)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	baseline := stock()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("warehouse allocation fixture")
		}
	}
	var role, location string
	var enabled bool
	if db.Table("fulfillments").Select("location_id").Where("id=?", f).Scan(&location).Error != nil || location == "" {
		t.Fatal("allocation parcel location")
	}
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("allocation role")
	}
	if db.Table("order_device_allocation_runtime").Select("enabled").Where("id=1").Scan(&enabled).Error != nil {
		t.Fatal("allocation gate")
	}
	exec("UPDATE order_device_allocation_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE order_device_allocation_runtime SET enabled=? WHERE id=1", enabled)
	exec("UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE warehouse_scope_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	defer exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_scopes(user_id,mode) VALUES(?,'selected')", actor.ID)
	defer exec("DELETE FROM authorization_warehouse_scopes WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, location)
	defer exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	path := "/api/admin/warehouse/locations/" + location + "/orders/allocation-order/fulfillments/" + f + "/items/allocation-line/allocation"
	body := func(in devices.AllocationInput) string { b, _ := json.Marshal(in); return string(b) }
	read := func() devices.WarehouseAllocation {
		t.Helper()
		var out devices.WarehouseAllocation
		raw := call("GET", path, "", 200)
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("allocation projection")
		}
		for _, key := range []string{"cost_cents", "imei", "payment", "source_id", "inventory_item_id"} {
			if strings.Contains(string(raw), key) {
				t.Fatal("allocation field leak", key)
			}
		}
		return out
	}
	before := read()
	if !before.Editable || before.Quantity != 1 || len(before.Assigned) != 1 || before.Assigned[0].ID != "allocation-b" {
		t.Fatal("initial parcel assignment")
	}
	candidate := false
	for _, d := range before.Candidates {
		if d.ID == "allocation-c" {
			candidate = true
		}
		if d.ID == "allocation-foreign" {
			t.Fatal("foreign candidate")
		}
	}
	if !candidate {
		t.Fatal("replacement candidate missing")
	}
	in := devices.AllocationInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Action: "reserve", IDs: []string{"allocation-c"}}
	call("GET", path+"?fields=*", "", 400)
	invalid := in
	invalid.Action = "release"
	call("POST", path, body(invalid), 400)
	invalid = in
	invalid.IDs = []string{"allocation-c", "allocation-b"}
	call("POST", path, body(invalid), 409)
	invalid = in
	invalid.IDs = []string{"allocation-foreign"}
	call("POST", path, body(invalid), 409)
	exec("UPDATE payment_collections SET status='awaiting' WHERE id='allocation-payment'")
	call("POST", path, body(in), 409)
	exec("UPDATE payment_collections SET status='completed' WHERE id='allocation-payment'")
	// Payment status participates in the version even after being restored.
	in.ExpectedRevision = read().Revision
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.allocation.write'", role)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.allocation.write')", role)
	exec("CREATE TRIGGER warehouse_allocation_fault BEFORE INSERT ON order_device_allocations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial warehouse allocation receipt fault'")
	call("POST", path, body(in), 503)
	exec("DROP TRIGGER warehouse_allocation_fault")
	if got := read(); len(got.Assigned) != 1 || got.Assigned[0].ID != "allocation-b" || got.Revision != in.ExpectedRevision {
		t.Fatal("allocation rollback did not restore facts")
	}
	var fresh, replay, bad atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.AllocateWarehouseParcel(ctx, actor, location, "allocation-order", f, "allocation-line", in)
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
		t.Fatal("allocation concurrency", fresh.Load(), replay.Load(), bad.Load())
	}
	after := read()
	if len(after.Assigned) != 1 || after.Assigned[0].ID != "allocation-c" || after.Assigned[0].AssignmentID != before.Assigned[0].AssignmentID {
		t.Fatal("replacement lost assignment identity")
	}
	var state string
	if db.Table("devices").Select("state").Where("id='allocation-b'").Scan(&state).Error != nil || state != "available" {
		t.Fatal("old device not released")
	}
	call("POST", path, body(in), 200)
	changed := in
	changed.IDs = []string{"allocation-b"}
	call("POST", path, body(changed), 409)
	exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, location)
	exec("UPDATE devices SET location_id='user-scope-b' WHERE id='allocation-c'")
	call("POST", path, body(in), 404)
	exec("UPDATE devices SET location_id=? WHERE id='allocation-c'", location)
	// Restore the original machine using the same production replacement command.
	changed.OperationID = uuid.NewString()
	changed.ExpectedRevision = read().Revision
	call("POST", path, body(changed), 200)
	final := read()
	unchanged := changed
	unchanged.OperationID = uuid.NewString()
	unchanged.ExpectedRevision = final.Revision
	call("POST", path, body(unchanged), 200)
	if got := read(); len(got.Assigned) != 1 || got.Assigned[0].ID != "allocation-b" {
		t.Fatal("unchanged device duplicated")
	}
	// An already fulfilled parcel imported without physical-device assignments
	// is filled without consuming its quantity inventory a second time.
	exec("UPDATE device_assignments SET status='canceled',fulfillment_id=NULL WHERE id=?", final.Assigned[0].AssignmentID)
	exec("UPDATE devices SET state='available' WHERE id='allocation-b'")
	empty := read()
	if len(empty.Assigned) != 0 {
		t.Fatal("empty historical parcel fixture")
	}
	initial := changed
	initial.OperationID = uuid.NewString()
	initial.ExpectedRevision = empty.Revision
	call("POST", path, body(initial), 200)
	if got := read(); len(got.Assigned) != 1 || got.Assigned[0].ID != "allocation-b" {
		t.Fatal("existing parcel initial allocation")
	}
	if stock() != baseline {
		t.Fatal("warehouse allocation changed quantity stock or reservations")
	}
	t.Log(`WAREHOUSE_ALLOCATION_REHEARSAL {"real_gin_mysql":true,"same_warehouse_sku_candidates":true,"private_projection":true,"payment_and_capacity_checked":true,"replace_preserves_assignment_id":true,"old_device_available":true,"concurrent_attempts":4,"one_fresh_receipt":true,"late_failure_atomic":true,"immutable_replay":true,"revoked_scope_and_moved_device_replay_denied":true,"original_device_restored_by_command":true,"same_device_no_duplicate":true,"parcel_release_pending":true}`)
}
