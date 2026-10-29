package identity_test

import (
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"testing"
)

func checkDeviceSources(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := devices.SourceHandler{DB: db, StoreID: "device-source-store"}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("source fixture SQL", e)
		}
	}
	call("GET", "/api/admin/device-sources", "", 503)
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	exec("UPDATE warehouse_management_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE device_runtime_schema SET identity_enabled=FALSE,commands_enabled=FALSE WHERE id=1")
	defer exec("UPDATE warehouse_management_runtime SET enabled=FALSE WHERE id=1")
	read := func() devices.Sources {
		t.Helper()
		var out devices.Sources
		if json.Unmarshal(call("GET", "/api/admin/device-sources", "", 200), &out) != nil || out.Sources == nil {
			t.Fatal("source projection invalid")
		}
		return out
	}
	body := func(in devices.SourceInput) string { b, _ := json.Marshal(in); return string(b) }
	in := devices.SourceInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Name: "Main intake", Type: "self"}
	call("POST", "/api/admin/device-sources", body(in), 503)
	exec("INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('device-source-channel','Source channel',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES('device-source-store','Source store','device-source-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	call("POST", "/api/admin/device-sources", body(in), 503)
	exec("INSERT INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('manual_manual',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE commerce_stores SET default_sales_channel_id=NULL WHERE id='device-source-store'")
	call("POST", "/api/admin/device-sources", body(in), 503)
	exec("UPDATE commerce_stores SET default_sales_channel_id='device-source-channel' WHERE id='device-source-store'")
	exec("UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='device-source-channel'")
	call("POST", "/api/admin/device-sources", body(in), 503)
	exec("UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='device-source-channel'")
	exec("UPDATE fulfillment_providers SET is_enabled=FALSE WHERE id='manual_manual'")
	call("POST", "/api/admin/device-sources", body(in), 503)
	exec("UPDATE fulfillment_providers SET is_enabled=TRUE WHERE id='manual_manual'")
	exec("INSERT INTO commerce_stores(id,name,created_at,updated_at) VALUES('a-source-decoy','Unconfigured decoy',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	var warehouseBefore uint64
	if db.Raw("SELECT revision FROM warehouse_management_runtime WHERE id=1").Scan(&warehouseBefore).Error != nil {
		t.Fatal("warehouse revision missing")
	}
	var wins, replays atomic.Int64
	var unexpected atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.Save(ctx, actor, in)
			if e != nil {
				unexpected.Store(true)
			} else if out.Replayed {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if unexpected.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("source concurrent idempotency failed")
	}
	var result devices.SourceResult
	if json.Unmarshal(call("POST", "/api/admin/device-sources", body(in), 200), &result) != nil || !result.Replayed || result.Source.LocationName == nil || *result.Source.LocationName != in.Name {
		t.Fatal("source immutable replay invalid")
	}
	var n int64
	for _, q := range []string{"SELECT COUNT(*) FROM inventory_locations WHERE id=? AND name='Main intake' AND deleted_at IS NULL", "SELECT COUNT(*) FROM commerce_channel_locations WHERE stock_location_id=? AND sales_channel_id='device-source-channel' AND deleted_at IS NULL", "SELECT COUNT(*) FROM fulfillment_location_providers WHERE stock_location_id=? AND fulfillment_provider_id='manual_manual' AND deleted_at IS NULL"} {
		if db.Raw(q, result.Source.LocationID).Scan(&n).Error != nil || n != 1 {
			t.Fatal("source relationship missing")
		}
	}
	var warehouseAfter uint64
	if db.Raw("SELECT revision FROM warehouse_management_runtime WHERE id=1").Scan(&warehouseAfter).Error != nil || warehouseAfter != warehouseBefore+1 {
		t.Fatal("source did not advance warehouse revision")
	}
	other := in
	other.Name = "Changed payload"
	call("POST", "/api/admin/device-sources", body(other), 409)
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = read().Revision
	call("POST", "/api/admin/device-sources", body(duplicate), 409)
	for _, bad := range []string{`{}`, `{"operation_id":null,"expected_revision":"1","name":"x","type":"self"}`, `{"operation_id":"a","operation_id":"b","expected_revision":"1","name":"x","type":"self"}`, body(in) + `{}`, `{"operation_id":"a","expected_revision":"1","name":"x","type":"self","extra":"x"}`} {
		call("POST", "/api/admin/device-sources", bad, 400)
	}
	call("GET", "/api/admin/device-sources?x=1", "", 400)
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("source role missing")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='device.source.write'", role)
	call("POST", "/api/admin/device-sources", body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'device.source.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", "/api/admin/device-sources", "", 403)
	if _, e := h.Save(ctx, actor, in); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("warehouse source creation allowed")
	}
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	counts := func() string {
		t.Helper()
		var rows []int64
		for _, table := range []string{"inventory_locations", "commerce_channel_locations", "fulfillment_location_providers", "device_sources", "device_source_changes", "inventory_levels", "inventory_stock_edits"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("source rollback counts failed")
			}
			rows = append(rows, n)
		}
		b, _ := json.Marshal(rows)
		return string(b)
	}
	before := counts()
	revision := read().Revision
	exec("CREATE TRIGGER device_source_receipt_fault BEFORE INSERT ON device_source_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial source receipt fault'")
	fault := devices.SourceInput{OperationID: uuid.NewString(), ExpectedRevision: revision, Name: "Supplier intake", Type: "supplier"}
	call("POST", "/api/admin/device-sources", body(fault), 503)
	exec("DROP TRIGGER device_source_receipt_fault")
	if counts() != before || read().Revision != revision {
		t.Fatal("source late failure left partial objects")
	}
	var supplier devices.SourceResult
	if json.Unmarshal(call("POST", "/api/admin/device-sources", body(fault), 200), &supplier) != nil || supplier.Source.Type != "supplier" {
		t.Fatal("basic supplier source lost")
	}
	exec("UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", supplier.Source.LocationID)
	call("POST", "/api/admin/device-sources", body(fault), 404)
	found := false
	for _, source := range read().Sources {
		if source.ID == supplier.Source.ID {
			found = true
			if source.LocationName != nil {
				t.Fatal("deleted source location displayed active")
			}
		}
	}
	if !found {
		t.Fatal("historical source hidden")
	}
	t.Log(`DEVICE_SOURCES_REHEARSAL {"real_gin_mysql_redis":true,"default_disabled":true,"configured_store_required":true,"active_default_channel_required":true,"manual_provider_required":true,"source_creates_warehouse_and_both_links":true,"warehouse_revision_advanced":true,"concurrent_attempts":4,"single_creation":true,"immutable_replay":true,"duplicate_name_denied":true,"revoked_replay_denied":true,"warehouse_creation_denied":true,"late_receipt_failure_atomic":true,"deleted_location_replay_denied":true,"strict_wire":true,"intake_integrated":false}`)
}
