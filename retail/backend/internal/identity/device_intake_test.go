package identity_test

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sync"
	"sync/atomic"
	"testing"
)

func checkDeviceIntake(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("intake fixture", e)
		}
	}
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	defer exec("UPDATE device_runtime_schema SET commands_enabled=FALSE WHERE id=1")
	exec("INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('intake-product','iPhone 14','intake-product',FALSE,'draft',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variants(id,product_id,title,manage_inventory,allow_backorder,created_at,updated_at) VALUES('intake-v','intake-product','Intake SKU',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO inventory_items(id,title,requires_shipping,created_at,updated_at) VALUES('intake-stock','Intake stock',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES('intake-link','intake-v','intake-stock',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('intake-location','Intake warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES('intake-source','Own intake','self','intake-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('intake-grade','Condition',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('intake-battery','Battery',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES('intake-grade-b','B','intake-grade',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('intake-battery-85','85%','intake-battery',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES('intake-v','intake-grade-b'),('intake-v','intake-battery-85')")
	path := "/api/admin/products/intake-product/variants/intake-v/device-intake"
	call("GET", path, "", 200)
	tracking, e := h.ReadTracking(ctx, actor, "intake-product", "intake-v")
	if e != nil {
		t.Fatal("intake SKU read", e)
	}
	if _, e := h.Enroll(ctx, actor, "intake-product", "intake-v", devices.EnrollInput{OperationID: uuid.NewString(), ExpectedRevision: tracking.Revision}); e != nil {
		t.Fatal("intake enrollment", e)
	}
	read := func() devices.IntakeContext {
		t.Helper()
		var out devices.IntakeContext
		if json.Unmarshal(call("GET", path, "", 200), &out) != nil || !out.RequiresRadio {
			t.Fatal("intake context missing")
		}
		return out
	}
	sku := read()
	body := func(in devices.IntakeInput) string { b, _ := json.Marshal(in); return string(b) }
	base := devices.IntakeDevice{Identity: devices.Identity{SerialNumber: " intake-one ", IMEI: "000000000000071"}, SourceID: "intake-source", Category: "sale", Notes: "First line\nSecond line", CostCents: 12345, CurrencyCode: "eur", Photos: []devices.Photo{}}
	in := devices.IntakeInput{OperationID: uuid.NewString(), ExpectedRevision: sku.Revision, Devices: []devices.IntakeDevice{base}}
	fingerprint := func() [32]byte {
		t.Helper()
		var all []any
		for _, table := range []string{"inventory_levels", "inventory_reservations", "inventory_stock_edits"} {
			var rows []map[string]any
			order := "id"
			if table == "inventory_stock_edits" {
				order = "operation_id"
			}
			if db.Table(table).Order(order).Find(&rows).Error != nil {
				t.Fatal("intake stock fingerprint failed")
			}
			all = append(all, rows)
		}
		b, _ := json.Marshal(all)
		return sha256.Sum256(b)
	}
	before := fingerprint()
	// The old workflow accepts valid inventory SKUs without explicit enrollment.
	exec("UPDATE device_variant_tracking SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id='intake-v'")
	var untracked devices.IntakeContext
	if json.Unmarshal(call("GET", path, "", 200), &untracked) != nil {
		t.Fatal("unenrolled context unavailable")
	}
	pre := devices.IntakeInput{OperationID: uuid.NewString(), ExpectedRevision: untracked.Revision, Devices: []devices.IntakeDevice{base}}
	pre.Devices[0].SerialNumber = "UNTRACKED-INTAKE"
	pre.Devices[0].IMEI = "000000000000075"
	call("POST", path, body(pre), 200)
	exec("UPDATE device_variant_tracking SET deleted_at=NULL WHERE variant_id='intake-v'")
	in.ExpectedRevision = read().Revision

	bad := in
	bad.Devices = []devices.IntakeDevice{base}
	bad.Devices[0].IMEI = ""
	call("POST", path, body(bad), 400)
	grade := "A"
	bad.Devices[0] = base
	bad.Devices[0].Grade = &grade
	call("POST", path, body(bad), 400)
	health := 84
	bad.Devices[0] = base
	bad.Devices[0].BatteryHealth = &health
	call("POST", path, body(bad), 400)
	bad.Devices[0] = base
	bad.Devices[0].Photos = []devices.Photo{{ID: "unverified", URL: "https://example.invalid/device-photo.jpg"}}
	call("POST", path, body(bad), 400)
	for _, raw := range []string{`{}`, `{"operation_id":null,"expected_revision":"x","devices":[]}`, body(in) + `{}`} {
		call("POST", path, raw, 400)
	}
	call("GET", path+"?x=1", "", 400)
	exec("UPDATE catalog_option_values SET value='C' WHERE id='intake-grade-b'")
	call("POST", path, body(in), 409)
	exec("UPDATE catalog_option_values SET value='B' WHERE id='intake-grade-b'")
	var wins, replays atomic.Int64
	var unexpected atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.Intake(ctx, actor, "intake-product", "intake-v", in)
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
		t.Fatal("intake concurrent idempotency failed")
	}
	var result devices.IntakeResult
	if json.Unmarshal(call("POST", path, body(in), 200), &result) != nil || !result.Replayed || len(result.Devices) != 1 {
		t.Fatal("intake receipt invalid")
	}
	d := result.Devices[0]
	if d.SerialNumber != "INTAKE-ONE" || d.LocationID != "intake-location" || d.State != "available" || d.Grade == nil || *d.Grade != "B" || d.BatteryHealth == nil || *d.BatteryHealth != 85 || d.CostCents != 12345 {
		t.Fatal("intake fact differs")
	}
	batch := devices.IntakeInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Devices: []devices.IntakeDevice{base, base}}
	batch.Devices[0].SerialNumber = "BATCH-ROLLBACK-ONE"
	batch.Devices[0].IMEI = "000000000000072"
	batch.Devices[1].SerialNumber = "BATCH-ROLLBACK-TWO"
	batch.Devices[1].IMEI = ""
	batch.Devices[1].IMEI2 = base.IMEI
	call("POST", path, body(batch), 409)
	var n int64
	if db.Table("devices").Where("serial_number LIKE 'BATCH-ROLLBACK-%'").Count(&n).Error != nil || n != 0 {
		t.Fatal("partial intake batch survived")
	}
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("intake role missing")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='device.intake'", role)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'device.intake')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", path, "", 403)
	call("POST", path, body(in), 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	fault := devices.IntakeInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Devices: []devices.IntakeDevice{base}}
	fault.Devices[0].SerialNumber = "INTAKE-LATE-FAULT"
	fault.Devices[0].IMEI = "000000000000073"
	fault.Devices[0].Category = "returned"
	exec("CREATE TRIGGER device_intake_receipt_fault BEFORE INSERT ON device_intake_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial intake receipt fault'")
	call("POST", path, body(fault), 503)
	exec("DROP TRIGGER device_intake_receipt_fault")
	if db.Table("devices").Where("serial_number='INTAKE-LATE-FAULT'").Count(&n).Error != nil || n != 0 {
		t.Fatal("late intake failure left device")
	}
	var returned devices.IntakeResult
	if json.Unmarshal(call("POST", path, body(fault), 200), &returned) != nil || returned.Devices[0].State != "inspection" {
		t.Fatal("returned intake state invalid")
	}
	fault.OperationID = uuid.NewString()
	fault.ExpectedRevision = read().Revision
	fault.Devices[0].SerialNumber = "INTAKE-RECYCLED"
	fault.Devices[0].IMEI = "000000000000074"
	fault.Devices[0].Category = "recycled"
	var recycled devices.IntakeResult
	if json.Unmarshal(call("POST", path, body(fault), 200), &recycled) != nil || recycled.Devices[0].State != "inspection" {
		t.Fatal("recycled intake state invalid")
	}
	recent := read()
	if recent.RecentLimit != 50 || len(recent.RecentDevices) != 4 {
		t.Fatal("intake records missing after reload")
	}
	found := false
	for _, record := range recent.RecentDevices {
		if record.SerialNumber == "INTAKE-ONE" {
			found = true
			if record.IMEI == nil || *record.IMEI != base.IMEI || record.LocationID != "intake-location" {
				t.Fatal("reloaded device identity or warehouse lost")
			}
		}
	}
	if !found {
		t.Fatal("saved device absent from current list")
	}
	if before != fingerprint() {
		t.Fatal("intake modified quantity stock")
	}
	checkDevicePhotos(t, db, actor, call)
	if before != fingerprint() {
		t.Fatal("photo intake modified quantity stock")
	}
	t.Log(`DEVICE_INTAKE_REHEARSAL {"real_gin_mysql_redis":true,"valid_unenrolled_sku_allowed":true,"server_source_location":true,"sku_grade_battery_defaults":true,"measured_configuration_validated":true,"iphone_radio_required":true,"sku_change_rejected":true,"concurrent_attempts":4,"single_creation":true,"immutable_replay":true,"batch_collision_atomic":true,"revoked_replay_denied":true,"warehouse_full_store_denied":true,"late_receipt_failure_atomic":true,"sale_available_returned_recycled_inspection":true,"inventory_fingerprint_unchanged":true,"recent_records_after_reload":true,"unverified_photos_rejected":true,"photo_upload_integrated":true}`)
}
