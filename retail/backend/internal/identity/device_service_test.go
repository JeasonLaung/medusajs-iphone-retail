package identity_test

import (
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"testing"
)

func checkDeviceService(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte, assignment string) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("device service fixture", e)
		}
	}
	path := "/api/admin/orders/allocation-order/after-sales/asr-receive-repair/devices/" + assignment + "/service"
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	read := func() devices.ServiceContext {
		t.Helper()
		raw := call("GET", path, "", 200)
		if strings.Contains(string(raw), "cost_cents") || strings.Contains(string(raw), "IMEI") {
			t.Fatal("service secret leaked")
		}
		var out devices.ServiceContext
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("service context")
		}
		return out
	}
	stock := func() string {
		var s string
		db.Raw("SELECT CONCAT(stocked_quantity,'/',reserved_quantity) FROM inventory_levels WHERE id='packing-target'").Row().Scan(&s)
		return s
	}
	baseline := stock()
	before := read()
	if before.Enabled || before.State != "repair" {
		t.Fatal("default disabled service / repair state")
	}
	in := devices.ServiceInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Action: "inspect", Result: "pass", VariantID: "allocation-v", Grade: "B", BatteryHealth: 85, Notes: "Original repaired device inspected", Photos: []devices.Photo{}}
	call("POST", path, body(in), 503)
	exec("UPDATE device_service_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE device_service_runtime SET enabled=FALSE WHERE id=1")
	exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('service-grade','Condition',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('service-battery','Battery',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES('service-grade-b','B','service-grade',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('service-battery-85','85%','service-battery',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES('allocation-v','service-grade-b'),('allocation-v','service-battery-85')")
	exec("INSERT INTO catalog_variants(id,product_id,title,sku,manage_inventory,allow_backorder,created_at,updated_at) VALUES('service-other-v','allocation-product','Alternate inspected SKU','SERVICE-OTHER',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES('service-other-link','service-other-v','allocation-stock',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	in.ExpectedRevision = read().Revision
	for _, bad := range []string{`{"photos":null}`, `{"action":"inspect","action":"repair_ship"}`, body(in) + ` {}`} {
		call("POST", path, bad, 400)
	}
	call("GET", path+"?unknown=1", "", 400)
	bad := in
	bad.VariantID = "service-other-v"
	call("POST", path, body(bad), 409)
	bad = in
	bad.Grade = "C"
	call("POST", path, body(bad), 409)
	bad = in
	bad.BatteryHealth = 84
	call("POST", path, body(bad), 409)
	bad = in
	bad.Photos = []devices.Photo{{ID: "dphoto_" + uuid.NewString(), URL: "/api/store/device-photos/mismatch"}}
	call("POST", path, body(bad), 400)
	bad = in
	id := "dphoto_" + uuid.NewString()
	bad.Photos = []devices.Photo{{ID: id, URL: "/api/store/device-photos/" + id}}
	call("POST", path, body(bad), 400)
	exec("UPDATE devices SET updated_at=DATE_ADD(updated_at,INTERVAL 1 SECOND) WHERE id='allocation-b'")
	call("POST", path, body(in), 409)
	in.ExpectedRevision = read().Revision
	exec("CREATE TRIGGER service_receipt_fault BEFORE INSERT ON device_service_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial service receipt failure'")
	call("POST", path, body(in), 503)
	exec("DROP TRIGGER service_receipt_fault")
	if read().State != "repair" || stock() != baseline {
		t.Fatal("inspection rollback")
	}
	concurrent := func(input devices.ServiceInput) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := h.ServiceDevice(ctx, actor, "allocation-order", "asr-receive-repair", assignment, input)
				errs <- e
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal("service concurrent replay", e)
			}
		}
	}
	concurrent(in)
	if read().State != "repair_ready" {
		t.Fatal("original repaired device not ready")
	}
	var n int64
	if db.Table("device_service_receipts").Where("assignment_id=?", assignment).Count(&n).Error != nil || n != 1 {
		t.Fatal("duplicate inspection")
	}
	exec("UPDATE device_service_runtime SET enabled=FALSE WHERE id=1")
	call("POST", path, body(in), 200)
	exec("UPDATE device_service_runtime SET enabled=TRUE WHERE id=1")
	var review customer.ReviewContext
	readReview := func() {
		t.Helper()
		if e := db.Transaction(func(tx *gorm.DB) error {
			var e error
			review, e = customer.ReviewAfterSalesTx(ctx, tx, actor, "allocation-order", nil)
			return e
		}); e != nil {
			t.Fatal(e)
		}
	}
	exec("UPDATE after_sales_review_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE after_sales_review_runtime SET enabled=FALSE WHERE id=1")
	readReview()
	complete := func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			_, e := customer.ReviewAfterSalesTx(ctx, tx, actor, "allocation-order", &customer.ReviewInput{OperationID: uuid.NewString(), ExpectedRevision: review.Revision, RequestID: "asr-receive-repair", Status: "completed"})
			return e
		})
	}
	if !errors.Is(complete(), customer.ErrAfterSalesConflict) {
		t.Fatal("repair completed before dispatch")
	}
	shipping := devices.ServiceInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Action: "repair_ship", TrackingNumber: "REPAIR-TRACK", Photos: []devices.Photo{}}
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('service-conflict','allocation-b','allocation-order','allocation-line','allocation-source','reserved',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	shipping.ExpectedRevision = read().Revision
	call("POST", path, body(shipping), 409)
	exec("UPDATE device_assignments SET status='canceled',deleted_at=UTC_TIMESTAMP(6) WHERE id='service-conflict'")
	shipping.ExpectedRevision = read().Revision
	exec("CREATE TRIGGER service_ship_fault BEFORE INSERT ON device_service_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial shipping receipt failure'")
	call("POST", path, body(shipping), 503)
	exec("DROP TRIGGER service_ship_fault")
	if read().State != "repair_ready" || read().Status != "returned" {
		t.Fatal("repair shipping rollback")
	}
	concurrent(shipping)
	after := read()
	if after.State != "shipped" || after.Status != "shipped" || after.TrackingNumber == nil || *after.TrackingNumber != "REPAIR-TRACK" || stock() != baseline {
		t.Fatal("repair dispatch facts")
	}
	if db.Table("device_service_receipts").Where("assignment_id=?", assignment).Count(&n).Error != nil || n != 2 {
		t.Fatal("duplicate repair shipping")
	}
	readReview()
	if complete() != nil {
		t.Fatal("repair cannot complete after dispatch")
	}
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='device.intake'", role)
	call("POST", path, body(shipping), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'device.intake')", role)
	exec("UPDATE devices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='allocation-b'")
	call("POST", path, body(shipping), 404)
	exec("UPDATE devices SET deleted_at=NULL WHERE id='allocation-b'")
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", path, "", 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	t.Log(`DEVICE_SERVICE_REHEARSAL {"real_gin_mysql_redis":true,"default_disabled":true,"original_repair_sku_required":true,"grade_and_battery_checked":true,"unverified_photos_denied":true,"stale_context_denied":true,"inspection_late_failure_atomic":true,"repair_dispatch_late_failure_atomic":true,"concurrent_attempts_per_command":4,"single_inspection_and_dispatch":true,"active_assignment_denied":true,"repair_ready_then_shipped":true,"quantity_stock_unchanged":true,"repair_completion_requires_dispatch":true,"revoked_deleted_warehouse_denied":true,"paused_write_replay":true}`)
}

func checkDeviceInspectionOutcomes(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte, assignment string) {
	t.Helper()
	h := devices.Handler{DB: db}
	ctx := t.Context()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("inspection outcomes fixture", e)
		}
	}
	exec("UPDATE device_service_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE device_service_runtime SET enabled=FALSE WHERE id=1")
	path := "/api/admin/orders/allocation-order/after-sales/asr-receive-exchange/devices/" + assignment + "/service"
	read := func() devices.ServiceContext {
		t.Helper()
		var out devices.ServiceContext
		if json.Unmarshal(call("GET", path, "", 200), &out) != nil {
			t.Fatal("inspection outcomes context")
		}
		return out
	}
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	photo, _, e := h.SavePhoto(ctx, actor, devices.PhotoInput{OperationID: uuid.NewString(), Filename: "inspection.png", MimeType: "image/png", Content: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII="})
	if e != nil {
		t.Fatal("inspection photo fixture", e)
	}
	inspect := devices.ServiceInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Action: "inspect", Result: "supplier_return", VariantID: "allocation-v", Grade: "B", BatteryHealth: 85, Notes: "Source checked", Photos: []devices.Photo{photo}}
	call("POST", path, body(inspect), 409)
	exec("UPDATE device_sources SET type='supplier' WHERE id='allocation-source'")
	defer exec("UPDATE device_sources SET type='self' WHERE id='allocation-source'")
	for _, result := range []string{"supplier_return", "retire", "repair", "pass"} {
		exec("UPDATE devices SET state='inspection',category='returned' WHERE id='allocation-b'")
		inspect.OperationID = uuid.NewString()
		inspect.ExpectedRevision = read().Revision
		inspect.Result = result
		if result == "pass" {
			inspect.VariantID = "service-other-v"
		}
		call("POST", path, body(inspect), 200)
		out := read()
		expected := map[string]string{"supplier_return": "supplier_returned", "retire": "retired", "repair": "repair", "pass": "available"}[result]
		if out.State != expected || result == "pass" && (out.VariantID != "service-other-v" || out.Category != "sale") {
			t.Fatal("inspection outcome", result, out.State)
		}
		call("POST", path, body(inspect), 200)
	}
	var events int64
	if db.Table("device_events").Where("device_id='allocation-b' AND action='inspect' AND JSON_LENGTH(JSON_EXTRACT(data,'$.photos'))=1").Count(&events).Error != nil || events != 4 {
		t.Fatal("inspection evidence not retained")
	}
	t.Log(`DEVICE_INSPECTION_OUTCOMES {"real_mysql":true,"supplier_origin_required":true,"supplier_return_retire_repair_pass":true,"ordinary_pass_can_change_sku":true,"photos_verified_and_audited":true,"immutable_replay":true}`)
}
