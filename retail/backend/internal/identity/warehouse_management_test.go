package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/warehouse"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func checkWarehouseManagement(t *testing.T, ctx context.Context, db *gorm.DB, actor identity.User, get func(string, int) []byte, post func(string, int) []byte) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("warehouse fixture setup failed", err)
		}
	}
	get("/api/admin/warehouses", 503)
	exec("UPDATE warehouse_management_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE warehouse_management_runtime SET enabled=FALSE WHERE id=1")
	exec("INSERT INTO commerce_countries(iso_2,iso_3,num_code,name,display_name,created_at,updated_at) VALUES('xy','xyz','999','Fixture','Fixture',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	defer exec("DELETE FROM commerce_countries WHERE iso_2='xy'")
	h := warehouse.Handler{DB: db}
	directory := func() warehouse.Directory {
		t.Helper()
		var d warehouse.Directory
		if json.Unmarshal(get("/api/admin/warehouses", 200), &d) != nil || d.Revision == "" || d.Locations == nil {
			t.Fatal("warehouse directory invalid")
		}
		return d
	}
	call := func(in warehouse.Command, status int) warehouse.Result {
		t.Helper()
		raw, _ := json.Marshal(in)
		var out warehouse.Result
		body := post(string(raw), status)
		if status == 200 && json.Unmarshal(body, &out) != nil {
			t.Fatal("warehouse response invalid")
		}
		return out
	}
	fingerprint := func() string {
		t.Helper()
		r := ""
		for _, table := range []string{"inventory_locations", "inventory_location_addresses", "warehouse_changes"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("warehouse fingerprint failed")
			}
			r += fmt.Sprint(n, ":")
		}
		var revision uint64
		db.Table("warehouse_management_runtime").Select("revision").Where("id=1").Scan(&revision)
		return r + fmt.Sprint(revision)
	}
	in := warehouse.Command{OperationID: uuid.NewString(), ExpectedRevision: directory().Revision, Name: "Warehouse 中文", Address: warehouse.Address{Address1: "Fixture street", CountryCode: "xy", PostalCode: "0012"}}
	var fresh atomic.Int32
	var group sync.WaitGroup
	results := make(chan warehouse.Result, 4)
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			out, e := h.Save(ctx, actor, in)
			if e != nil {
				t.Errorf("concurrent warehouse save failed: %v", e)
				return
			}
			if !out.Replayed {
				fresh.Add(1)
			}
			results <- out
		}()
	}
	group.Wait()
	close(results)
	var created warehouse.Result
	for out := range results {
		if created.ID != "" && (out.ID != created.ID || out.Revision != created.Revision) {
			t.Fatal("warehouse create replay differs")
		}
		created = out
	}
	if fresh.Load() != 1 || !strings.HasPrefix(created.ID, "sloc_") {
		t.Fatal("warehouse create not unique")
	}
	current := fingerprint()
	if out := call(in, 200); !out.Replayed || out.ID != created.ID || fingerprint() != current {
		t.Fatal("warehouse HTTP replay differs")
	}
	for _, row := range directory().Locations {
		if row.ID == created.ID && (row.Address == nil || row.Address.Address1 != "Fixture street" || row.Address.PostalCode != "0012") {
			t.Fatal("warehouse address projection differs")
		}
	}
	// The source unique active-address index allows deleted locations to keep
	// the same historical address. Editing the active location must preserve it.
	exec("INSERT INTO inventory_locations(id,name,address_id,metadata,created_at,updated_at,deleted_at) SELECT 'warehouse-shared','Shared warehouse',address_id,'{\"private\":\"hidden\"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM inventory_locations WHERE id=?", created.ID)
	update := in
	update.ID = created.ID
	update.OperationID = uuid.NewString()
	update.ExpectedRevision = directory().Revision
	update.Name = "Updated warehouse"
	update.Address.Address1 = "Updated street"
	call(update, 200)
	for _, row := range directory().Locations {
		if row.ID == created.ID && (row.Address == nil || row.Address.Address1 != "Updated street") {
			t.Fatal("warehouse edit missing")
		}
		if row.ID == "warehouse-shared" && (row.Address == nil || row.Address.Address1 != "Fixture street") {
			t.Fatal("shared warehouse address changed")
		}
	}
	var historicalAddress string
	if db.Table("inventory_locations l").Select("a.address_1").Joins("JOIN inventory_location_addresses a ON a.id=l.address_id").Where("l.id='warehouse-shared'").Scan(&historicalAddress).Error != nil || historicalAddress != "Fixture street" {
		t.Fatal("historical warehouse address changed")
	}
	if strings.Contains(string(get("/api/admin/warehouses", 200)), "hidden") {
		t.Fatal("warehouse metadata escaped")
	}
	bad := update
	bad.OperationID = uuid.NewString()
	before := fingerprint()
	call(bad, 409)
	if fingerprint() != before {
		t.Fatal("stale warehouse save mutated")
	}
	bad.ExpectedRevision = directory().Revision
	bad.Address.CountryCode = "zz"
	call(bad, 400)
	if fingerprint() != before {
		t.Fatal("invalid country mutated")
	}
	bad.Address.CountryCode = "xy"
	bad.ID = "missing-warehouse"
	call(bad, 404)
	late := update
	late.OperationID = uuid.NewString()
	late.ExpectedRevision = directory().Revision
	late.Name = "Must roll back"
	exec("CREATE TRIGGER warehouse_late_failure BEFORE INSERT ON warehouse_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned warehouse rollback'")
	call(late, 503)
	exec("DROP TRIGGER warehouse_late_failure")
	if fingerprint() != before {
		t.Fatal("warehouse late failure leaked address/location/revision")
	}
	if out := call(late, 200); out.Replayed {
		t.Fatal("failed warehouse save left receipt")
	}
	// Read and write are separate; current grants apply even to old receipts.
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("warehouse grant fixture missing")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.write'", role)
	directory()
	call(in, 403)
	if _, e := h.Save(ctx, actor, in); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("warehouse revoked replay allowed")
	}
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.write')", role)
	for _, kind := range []string{"warehouse", "supplier"} {
		exec("UPDATE authorization_principals SET kind=? WHERE user_id=?", kind, actor.ID)
		get("/api/admin/warehouses", 403)
		call(in, 403)
	}
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	raw, _ := json.Marshal(late)
	for _, bad := range []string{`{}`, `null`, string(raw) + `{}`, strings.Replace(string(raw), `"name":"Must roll back"`, `"name":"Must roll back","name":"changed"`, 1), strings.Replace(string(raw), `"address_1":"Updated street"`, `"address_1":"Updated street","address_1":"other"`, 1), strings.Replace(string(raw), `"country_code":"xy"`, `"country_code":null`, 1), strings.Replace(string(raw), `"phone":""`, `"phone":"","unknown":"x"`, 1)} {
		before := fingerprint()
		post(bad, 400)
		if fingerprint() != before {
			t.Fatal("invalid warehouse wire mutated")
		}
	}
	get("/api/admin/warehouses?fields=*", 400)
	exec("UPDATE warehouse_management_runtime SET enabled=FALSE WHERE id=1")
	get("/api/admin/warehouses", 503)
	call(in, 503)
	t.Log(`WAREHOUSE_MANAGEMENT_REHEARSAL {"real_gin_mysql_redis":true,"concurrent_creates":4,"one_fresh_receipt":true,"immutable_http_replay":true,"shared_address_copy_on_write":true,"stale_revision_rejected":true,"country_must_exist":true,"late_failure_atomic":true,"read_write_separate":true,"revoked_replay_denied":true,"warehouse_supplier_full_store_denied":true,"strict_wire":true,"metadata_omitted":true,"default_disabled":true}`)
}
