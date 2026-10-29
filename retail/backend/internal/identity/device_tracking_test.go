package identity_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"testing"
)

func checkDeviceTracking(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("tracking fixture SQL failed")
		}
	}
	fingerprint := func() [32]byte {
		t.Helper()
		var all []any
		for _, table := range []string{"inventory_items", "inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("id").Find(&rows).Error != nil {
				t.Fatal("tracking inventory fingerprint failed")
			}
			all = append(all, rows)
		}
		b, _ := json.Marshal(all)
		return sha256.Sum256(b)
	}
	exec("INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('tracking-product','Tracking','tracking-product',FALSE,'draft',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	for i := 1; i <= 3; i++ {
		exec("INSERT INTO catalog_variants(id,product_id,title,allow_backorder,manage_inventory,created_at,updated_at) VALUES(?,'tracking-product','Tracking',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", fmt.Sprintf("tracking-v%d", i))
	}
	for i := 1; i <= 2; i++ {
		exec("INSERT INTO inventory_items(id,sku,title,requires_shipping,created_at,updated_at) VALUES(?,?,?,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", fmt.Sprintf("tracking-stock%d", i), fmt.Sprintf("tracking-%d", i), "Tracking")
		exec("INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES(?,?,?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", fmt.Sprintf("tracking-link%d", i), fmt.Sprintf("tracking-v%d", i), fmt.Sprintf("tracking-stock%d", i))
	}
	before := fingerprint()
	path := "/api/admin/products/tracking-product/variants/tracking-v1/device-tracking"
	call("GET", path, "", 503)
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	defer exec("UPDATE device_runtime_schema SET commands_enabled=FALSE WHERE id=1")
	read := func(variant string) devices.Tracking {
		t.Helper()
		var out devices.Tracking
		if json.Unmarshal(call("GET", "/api/admin/products/tracking-product/variants/"+variant+"/device-tracking", "", 200), &out) != nil {
			t.Fatal("tracking projection failed")
		}
		return out
	}
	current := read("tracking-v1")
	if !current.Eligible || current.Enabled || current.InventoryItemID != "tracking-stock1" {
		t.Fatal("eligible SKU unavailable")
	}
	body := func(in devices.EnrollInput) string { b, _ := json.Marshal(in); return string(b) }
	in := devices.EnrollInput{OperationID: uuid.NewString(), ExpectedRevision: current.Revision}
	for _, bad := range []string{`{}`, `{"operation_id":null,"expected_revision":"x"}`, `{"operation_id":"a","operation_id":"b","expected_revision":"x"}`, body(in) + `{}`, `{"extra":1}`} {
		call("POST", path, bad, 400)
	}
	call("GET", path+"?x=1", "", 400)
	call("GET", "/api/admin/products/wrong/variants/tracking-v1/device-tracking", "", 404)
	// A catalog change after the read invalidates the expected state.
	exec("UPDATE catalog_variant_inventory_items SET required_quantity=2 WHERE id='tracking-link1'")
	call("POST", path, body(in), 409)
	if read("tracking-v1").Eligible {
		t.Fatal("quantity other than one accepted")
	}
	exec("UPDATE catalog_variant_inventory_items SET required_quantity=1 WHERE id='tracking-link1'")
	exec("INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES('tracking-sharing','tracking-v3','tracking-stock1',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	if got := read("tracking-v1"); got.Eligible || got.Reason != "shared_inventory_item" {
		t.Fatal("shared stock accepted")
	}
	call("POST", path, body(in), 409)
	exec("UPDATE catalog_variant_inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='tracking-sharing'")
	current = read("tracking-v1")
	in.ExpectedRevision = current.Revision
	var wins, replays atomic.Int64
	var unexpected atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.Enroll(ctx, actor, "tracking-product", "tracking-v1", in)
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
	if wins.Load() != 1 || replays.Load() != 3 || unexpected.Load() {
		t.Fatal("concurrent enrollment not idempotent")
	}
	var result devices.EnrollResult
	raw := call("POST", path, body(in), 200)
	if json.Unmarshal(raw, &result) != nil || !result.Replayed || !result.Tracking.Enabled {
		t.Fatal("HTTP receipt replay failed")
	}
	if got := read("tracking-v1"); got.Eligible || !got.Enabled || got.Reason != "already_enabled" {
		t.Fatal("tracked state lost")
	}
	changed := in
	changed.ExpectedRevision = result.Tracking.Revision
	call("POST", path, body(changed), 409)
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("tracking actor role missing")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='device.enroll'", role)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'device.enroll')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", path, "", 403)
	call("POST", path, body(in), 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	// A failure after tracking+event creation must roll everything back.
	second := read("tracking-v2")
	fault := devices.EnrollInput{OperationID: uuid.NewString(), ExpectedRevision: second.Revision}
	exec("CREATE TRIGGER tracking_receipt_fault BEFORE INSERT ON device_tracking_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial tracking receipt fault'")
	call("POST", "/api/admin/products/tracking-product/variants/tracking-v2/device-tracking", body(fault), 503)
	exec("DROP TRIGGER tracking_receipt_fault")
	if got := read("tracking-v2"); got.Enabled || got.Revision != second.Revision {
		t.Fatal("late failure left tracking or revision")
	}
	var n int64
	if db.Table("device_events").Where("action='enroll'").Count(&n).Error != nil || n != 1 {
		t.Fatal("tracking event atomicity failed")
	}
	if before != fingerprint() {
		t.Fatal("enrollment changed quantity inventory")
	}
	t.Log(`DEVICE_TRACKING_REHEARSAL {"real_gin_mysql_redis":true,"explicit_grants":true,"default_disabled":true,"current_sku_configuration":true,"shared_item_denied":true,"quantity_exactly_one":true,"concurrent_attempts":4,"single_creation":true,"immutable_replay":true,"revoked_replay_denied":true,"warehouse_full_store_denied":true,"late_receipt_failure_atomic":true,"inventory_fingerprint_unchanged":true,"strict_wire":true,"intake_integrated":false}`)
}
