package identity_test

import (
	"encoding/base64"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func checkWarehouseIntake(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("warehouse intake fixture", e)
		}
	}
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	const base = "/api/admin/warehouse/locations/intake-location/device-intake/sale"
	const path = base + "/products/intake-product/variants/intake-v"
	exec("INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('warehouse-intake-supplier-location','Supplier location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('warehouse-intake-moved-location','Moved location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES('warehouse-intake-other','SECRET-OTHER-SOURCE','self','user-scope-b',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('warehouse-intake-supplier','Supplier','supplier','warehouse-intake-supplier-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	read := func() devices.IntakeContext {
		t.Helper()
		raw := call("GET", path, "", 200)
		if strings.Contains(string(raw), "SECRET-OTHER-SOURCE") || strings.Contains(string(raw), "wdev-returned") || strings.Contains(string(raw), "cost_cents") {
			t.Fatal("warehouse intake projection leak")
		}
		var out devices.IntakeContext
		if json.Unmarshal(raw, &out) != nil || len(out.Sources) != 1 || out.Sources[0].ID != "intake-source" {
			t.Fatal("warehouse intake source scope")
		}
		for _, d := range out.RecentDevices {
			if d.LocationID != "intake-location" || d.Category != "sale" {
				t.Fatal("warehouse recent device leak")
			}
		}
		return out
	}
	var variants devices.WarehouseIntakeVariants
	raw := call("GET", base+"/variants?q=Intake&offset=0", "", 200)
	if json.Unmarshal(raw, &variants) != nil || len(variants.Variants) == 0 || variants.Limit != 20 {
		t.Fatal("warehouse SKU picker missing")
	}
	for _, key := range []string{"metadata", "cost_cents", "inventory_levels", "password", "customer"} {
		if strings.Contains(string(raw), key) {
			t.Fatal("variant picker leaks relations")
		}
	}
	exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('warehouse-search-color','Color',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('warehouse-search-capacity','Storage',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES('warehouse-search-blue','Blue','warehouse-search-color',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('warehouse-search-128','128 GB','warehouse-search-capacity',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES('intake-v','warehouse-search-blue'),('intake-v','warehouse-search-128')")
	for _, query := range []string{"i14+B+85+128GB+%E8%93%9D%E8%89%B2", "IPHONE+14+128+85%25+%E8%93%9D"} {
		var result devices.WarehouseIntakeVariants
		if json.Unmarshal(call("GET", base+"/variants?q="+query, "", 200), &result) != nil || len(result.Variants) != 1 || result.Variants[0].ID != "intake-v" {
			t.Fatal("original SKU search semantics lost")
		}
	}
	var noMatch devices.WarehouseIntakeVariants
	if json.Unmarshal(call("GET", base+"/variants?q=i14pm+128GB", "", 200), &noMatch) != nil || len(noMatch.Variants) != 0 {
		t.Fatal("model exact search includes wrong suffix")
	}
	call("GET", base+"/variants?offset=01", "", 400)
	call("GET", base+"/variants?q=a&q=b", "", 400)
	call("GET", "/api/admin/warehouse/locations/user-scope-b/device-intake/sale/variants", "", 403)
	call("GET", "/api/admin/warehouse/locations/intake-location/device-intake/recycled/variants", "", 400)
	call("GET", "/api/admin/products/intake-product/variants/intake-v/device-intake", "", 403)
	photoInput := devices.PhotoInput{OperationID: uuid.NewString(), Filename: "worker.jpg", MimeType: "image/jpeg", Content: base64.StdEncoding.EncodeToString([]byte{255, 216, 255, 1, 2, 3})}
	var uploaded struct {
		Photo    devices.Photo
		Replayed bool
	}
	if json.Unmarshal(call("POST", base+"/photos", body(photoInput), 200), &uploaded) != nil {
		t.Fatal("scoped photo upload")
	}
	call("POST", base+"/photos", body(photoInput), 200)
	call("POST", "/api/admin/warehouse/locations/intake-location/device-intake/returned/photos", body(photoInput), 409)
	call("POST", "/api/admin/device-photos", body(photoInput), 403)
	in := devices.IntakeInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Devices: []devices.IntakeDevice{{Identity: devices.Identity{SerialNumber: "WORKER-INTAKE", IMEI: "000000000008001"}, SourceID: "intake-source", Category: "sale", CurrencyCode: "eur", Photos: []devices.Photo{uploaded.Photo}}}}
	h := devices.Handler{DB: db, WarehouseScope: &devices.WarehouseIntakeScope{LocationID: "intake-location", Category: "sale"}}
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.Intake(ctx, actor, "intake-product", "intake-v", in)
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
		t.Fatal("warehouse intake concurrent save")
	}
	var result devices.IntakeResult
	if json.Unmarshal(call("POST", path, body(in), 200), &result) != nil || len(result.Devices) != 1 || result.Devices[0].State != "available" || result.Devices[0].LocationID != "intake-location" || len(result.Devices[0].Photos) != 1 {
		t.Fatal("warehouse intake result")
	}
	read()
	// Path category, source ownership and self-source checks precede immutable replay.
	bad := in
	bad.OperationID = uuid.NewString()
	bad.ExpectedRevision = read().Revision
	bad.Devices = append([]devices.IntakeDevice(nil), in.Devices...)
	bad.Devices[0].SerialNumber = "WORKER-FORGED"
	bad.Devices[0].IMEI = "000000000008002"
	bad.Devices[0].SourceID = "warehouse-intake-other"
	call("POST", path, body(bad), 403)
	bad.Devices[0].SourceID = "warehouse-intake-supplier"
	call("POST", path, body(bad), 403)
	bad.Devices[0].SourceID = "intake-source"
	bad.Devices[0].Category = "returned"
	call("POST", path, body(bad), 403)
	bad.Devices[0].Category = "recycled"
	call("POST", path, body(bad), 403)
	bad.Devices[0].Category = "sale"
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("warehouse intake role")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.intake.write'", role)
	call("POST", path, body(in), 403)
	call("POST", base+"/photos", body(photoInput), 403)
	read()
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.intake.write')", role)
	exec("UPDATE device_sources SET location_id='warehouse-intake-moved-location' WHERE id='intake-source'")
	call("POST", path, body(in), 403)
	exec("UPDATE device_sources SET location_id='intake-location' WHERE id='intake-source'")
	// Late receipt fault rolls back the whole physical device and identity claims.
	bad.ExpectedRevision = read().Revision
	exec("CREATE TRIGGER warehouse_intake_fault BEFORE INSERT ON device_intake_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial warehouse intake fault'")
	call("POST", path, body(bad), 503)
	exec("DROP TRIGGER warehouse_intake_fault")
	var n int64
	if db.Table("devices").Where("serial_number='WORKER-FORGED'").Count(&n).Error != nil || n != 0 {
		t.Fatal("warehouse intake partial save")
	}
	call("POST", path, body(bad), 200)
	returnedPath := "/api/admin/warehouse/locations/intake-location/device-intake/returned/products/intake-product/variants/intake-v"
	var returned devices.IntakeContext
	if json.Unmarshal(call("GET", returnedPath, "", 200), &returned) != nil {
		t.Fatal("returned context")
	}
	bad.OperationID = uuid.NewString()
	bad.ExpectedRevision = returned.Revision
	bad.Devices[0].SerialNumber = "WORKER-RETURNED"
	bad.Devices[0].IMEI = "000000000008003"
	bad.Devices[0].Category = "returned"
	if json.Unmarshal(call("POST", returnedPath, body(bad), 200), &result) != nil || result.Devices[0].State != "inspection" {
		t.Fatal("returned intake state")
	}
	t.Log(`WAREHOUSE_INTAKE_REHEARSAL {"scoped_variant_picker":true,"original_model_capacity_grade_battery_color_search":true,"sources_and_recent_scope":true,"full_store_denied":true,"cross_warehouse_source_denied":true,"supplier_source_denied":true,"category_override_denied":true,"recycled_denied":true,"scoped_photo_upload":true,"photo_receipt_context_bound":true,"concurrent_attempts":4,"single_creation":true,"immutable_replay":true,"revoked_write_replay_denied":true,"source_moved_replay_denied":true,"late_receipt_atomic":true,"sale_available_returned_inspection":true}`)
}
