package identity_test

import (
	"encoding/json"
	"fmt"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func checkWarehouseDevices(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("warehouse devices fixture", e)
		}
	}
	exec("UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE warehouse_scope_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	defer exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_scopes(user_id,mode) VALUES(?,'selected')", actor.ID)
	defer exec("DELETE FROM authorization_warehouse_scopes WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,'intake-location')", actor.ID)
	defer exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("warehouse device role")
	}
	nextRadio := 2000
	seed := func(id, category, state, location string) {
		t.Helper()
		nextRadio++
		exec("INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,grade,battery_health,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,?,?,'intake-v','intake-source',?,'B',85,500,'eur','shelf','SECRET-NOTE',?,?,'[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, strings.ToUpper(id), fmt.Sprintf("%015d", nextRadio), location, state, category)
	}
	for i := 0; i < 51; i++ {
		seed(fmt.Sprintf("wdev-%03d", i), "sale", "pending", "intake-location")
	}
	seed("wdev-other", "sale", "available", "user-scope-b")
	seed("wdev-recycled", "recycled", "inspection", "intake-location")
	seed("wdev-returned", "returned", "inspection", "intake-location")
	const path = "/api/admin/warehouse/locations/intake-location/devices/sale"
	read := func(suffix string) devices.WarehouseDevices {
		t.Helper()
		raw := call("GET", path+suffix, "", 200)
		for _, secret := range []string{"cost_cents", "SECRET-NOTE", "notes", "wdev-other", "wdev-recycled", "customer", "HasSalesHistory", "self_source"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("warehouse device field leak", secret)
			}
		}
		var page devices.WarehouseDevices
		if json.Unmarshal(raw, &page) != nil {
			t.Fatal("warehouse device projection")
		}
		return page
	}
	first := read("")
	last := read("?offset=50")
	if len(first.Devices) != 50 || !first.HasMore || len(last.Devices) != 4 || last.HasMore {
		t.Fatal("warehouse device pagination")
	}
	exact := read("?q=wdev-000")
	if len(exact.Devices) != 1 || exact.Devices[0].SerialNumber != "WDEV-000" {
		t.Fatal("exact normalized search")
	}
	call("GET", "/api/admin/warehouse/locations/user-scope-b/devices/sale", "", 403)
	call("GET", "/api/admin/warehouse/locations/intake-location/devices/recycled", "", 400)
	for _, suffix := range []string{"?offset=01", "?offset=-1", "?offset=0&offset=1", "?q=a&q=b", "?other=x"} {
		call("GET", path+suffix, "", 400)
	}
	call("GET", "/api/admin/products/intake-product/variants/intake-v/device-intake", "", 403)
	body := func(in devices.WarehouseDeviceCommand) string { b, _ := json.Marshal(in); return string(b) }
	row := exact.Devices[0]
	in := devices.WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: row.Revision, Action: "make_available"}
	// Fixture radios must be unique to exercise the same identity claims as real intake.
	exec("UPDATE devices SET imei='000000000001001' WHERE id='wdev-000'")
	row = read("?q=wdev-000").Devices[0]
	in.ExpectedRevision = row.Revision
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.CommandWarehouseDevice(ctx, actor, "intake-location", "sale", row.ID, in)
			if e != nil {
				failed.Store(true)
			} else if out.Replayed {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if failed.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("warehouse command concurrent replay")
	}
	call("POST", path+"/"+row.ID, body(in), 200)
	// Activation keeps the original SKU and radio checks; no default filling on stored devices.
	activation := func(sn, action string, status int) {
		t.Helper()
		r := read("?q=" + sn).Devices[0]
		c := devices.WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: r.Revision, Action: action}
		call("POST", path+"/"+r.ID, body(c), status)
	}
	exec("UPDATE devices SET state='inspection',imei=NULL WHERE id='wdev-005'")
	activation("wdev-005", "activate", 400)
	exec("UPDATE devices SET imei='000000000001005',grade='A' WHERE id='wdev-005'")
	activation("wdev-005", "activate", 400)
	exec("UPDATE devices SET grade='B',battery_health=84 WHERE id='wdev-005'")
	activation("wdev-005", "activate", 400)
	exec("UPDATE devices SET battery_health=85 WHERE id='wdev-005'")
	activation("wdev-005", "activate", 200)
	// Object binding remains authoritative even for a previously successful receipt.
	exec("UPDATE devices SET location_id='user-scope-b' WHERE id=?", row.ID)
	call("POST", path+"/"+row.ID, body(in), 404)
	exec("UPDATE devices SET location_id='intake-location' WHERE id=?", row.ID)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.intake.write'", role)
	call("POST", path+"/"+row.ID, body(in), 403)
	if len(read("?q=wdev-000").Devices[0].AllowedActions) != 0 {
		t.Fatal("read-only actor received commands")
	}
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.intake.write')", role)
	r := read("?q=wdev-001").Devices[0]
	retire := devices.WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: r.Revision, Action: "retire"}
	exec("CREATE TRIGGER warehouse_device_receipt_fault BEFORE INSERT ON warehouse_device_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial warehouse device fault'")
	call("POST", path+"/"+r.ID, body(retire), 503)
	exec("DROP TRIGGER warehouse_device_receipt_fault")
	if read("?q=wdev-001").Devices[0].State != "pending" {
		t.Fatal("failed device command left mutation")
	}
	call("POST", path+"/"+r.ID, body(retire), 200)
	// Global revision invalidates stale editor state after another command.
	stale := devices.WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: row.Revision, Action: "retire"}
	call("POST", path+"/"+row.ID, body(stale), 409)
	r = read("?q=wdev-002").Devices[0]
	del := devices.WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: r.Revision, Action: "delete"}
	call("POST", path+"/"+r.ID, body(del), 200)
	call("POST", path+"/"+r.ID, body(del), 200)
	if len(read("?q=wdev-002").Devices) != 0 {
		t.Fatal("deleted device visible")
	}
	// Returned sales history blocks operations, as do currently locked states.
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('warehouse-history','wdev-003','history-order','history-item','intake-source','returned',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	r = read("?q=wdev-003").Devices[0]
	if len(r.AllowedActions) != 0 {
		t.Fatal("sales history permits mutation")
	}
	retire.OperationID = uuid.NewString()
	retire.ExpectedRevision = r.Revision
	call("POST", path+"/"+r.ID, body(retire), 409)
	exec("UPDATE devices SET state='packed' WHERE id='wdev-004'")
	r = read("?q=wdev-004").Devices[0]
	retire.OperationID = uuid.NewString()
	retire.ExpectedRevision = r.Revision
	call("POST", path+"/"+r.ID, body(retire), 409)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.return.read'", role)
	call("GET", "/api/admin/warehouse/locations/intake-location/devices/returned", "", 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.return.read')", role)
	call("POST", path+"/wdev-004", `{"operation_id":null}`, 400)
	exec("UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='intake-location'")
	call("POST", path+"/"+row.ID, body(in), 403)
	exec("UPDATE inventory_locations SET deleted_at=NULL WHERE id='intake-location'")
	t.Log(`WAREHOUSE_DEVICES_REHEARSAL {"location_and_category_scope":true,"recycled_denied":true,"read_only_hides_commands":true,"no_cost_notes_customer_projection":true,"exact_search":true,"pagination":true,"concurrent_attempts":4,"single_mutation":true,"immutable_replay":true,"moved_object_replay_denied":true,"revoked_replay_denied":true,"stale_revision":true,"late_receipt_atomic":true,"soft_delete_replay":true,"sales_history_and_locked_states_denied":true,"deleted_warehouse_denied":true,"activation_radio_grade_battery_checks":true}`)
}
