package identity_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkWarehouseInventory(t *testing.T, dbActor identity.User, call func(string, string, string, string, string, []*http.Cookie, int) *httptest.ResponseRecorder, session *http.Cookie, csrf string, db *gorm.DB) {
	t.Helper()
	ctx := t.Context()
	const origin = "https://admin.example"
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("warehouse inventory fixture", e)
		}
	}
	get := func(path string, status int) []byte {
		return call("GET", path, "", origin, "", []*http.Cookie{session}, status).Body.Bytes()
	}
	var cap struct{ Enabled bool }
	if db.Table("inventory_command_capabilities").Where("command='inventory.edit_stock'").Take(&cap).Error != nil {
		t.Fatal("capability missing")
	}
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.edit_stock'")
	defer exec("UPDATE inventory_command_capabilities SET enabled=? WHERE command='inventory.edit_stock'", cap.Enabled)
	for i := 0; i < 51; i++ {
		item := fmt.Sprintf("warehouse-stock-%03d", i)
		exec("INSERT INTO inventory_items(id,title,sku,requires_shipping,metadata,created_at,updated_at) VALUES(?,'Scoped stock',?,TRUE,'{\"private\":\"warehouse-hidden\"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", item, item)
		exec("INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES(?,?,'user-scope-a',100,0,1.25,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", fmt.Sprintf("warehouse-level-%03d", i), item)
	}
	exec("INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES('warehouse-other-level','warehouse-stock-000','user-scope-b',999,0,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	access := func() authorization.WarehouseView {
		t.Helper()
		raw := get("/api/admin/warehouse/access", 200)
		var out authorization.WarehouseView
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("access invalid")
		}
		return out
	}
	a := access()
	if len(a.Locations) != 1 || a.Locations[0].ID != "user-scope-a" || len(a.Actions) != 2 {
		t.Fatal("scope projection invalid")
	}
	const path = "/api/admin/warehouse/locations/user-scope-a/inventory"
	read := func(offset int) inventory.WarehouseInventory {
		t.Helper()
		raw := get(fmt.Sprintf("%s?offset=%d", path, offset), 200)
		for _, secret := range []string{"metadata", "warehouse-hidden", "999.000000000000000000", "password_hash", "customer_id"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("warehouse projection leak", secret)
			}
		}
		var out inventory.WarehouseInventory
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("inventory invalid")
		}
		return out
	}
	first := read(0)
	last := read(50)
	if len(first.Levels) != 50 || !first.HasMore || len(last.Levels) != 1 || last.HasMore || first.Levels[0].Revision == "" || first.Levels[0].AvailableQuantity != "100.000000000000000000" {
		t.Fatal("pagination or quantities invalid")
	}
	get("/api/admin/warehouse/locations/user-scope-b/inventory", 403)
	for _, suffix := range []string{"?offset=01", "?offset=0&offset=1", "?other=1", "?offset=-1"} {
		get(path+suffix, 400)
	}
	h := inventory.Handler{DB: db}
	in := inventory.StockInput{OperationID: uuid.NewString(), ExpectedRevision: first.Levels[0].Revision, StockedQuantity: "99.000000000000000001", IncomingQuantity: "2.000000000000000000"}
	post := func(level string, input inventory.StockInput, status int) inventory.StockResult {
		t.Helper()
		raw, _ := json.Marshal(input)
		out := call("POST", path+"/"+level+"/stock", string(raw), origin, csrf, []*http.Cookie{session}, status)
		var r inventory.StockResult
		if status == 200 && json.Unmarshal(out.Body.Bytes(), &r) != nil {
			t.Fatal("stock result invalid")
		}
		return r
	}
	post("warehouse-other-level", in, 403)
	if _, e := h.EditStock(ctx, first.Levels[0].ID, dbActor, in); !errors.Is(e, inventory.ErrStockDenied) {
		t.Fatal("full store editor accepted warehouse actor", e)
	}
	result := post(first.Levels[0].ID, in, 200)
	if result.Replayed {
		t.Fatal("first edit replayed")
	}
	if !post(first.Levels[0].ID, in, 200).Replayed {
		t.Fatal("retry not replayed")
	}
	if read(0).Levels[0].StockedQuantity != in.StockedQuantity {
		t.Fatal("exact stock not saved")
	}
	stale := in
	stale.OperationID = uuid.NewString()
	post(first.Levels[0].ID, stale, 409)
	var role string
	if db.Table("warehouse_user_accounts").Where("user_id=?", dbActor.ID).Pluck("role_id", &role).Error != nil || role == "" {
		t.Fatal("role missing")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='inventory.stock.write'", role)
	post(first.Levels[0].ID, in, 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'inventory.stock.write')", role)
	exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", dbActor.ID)
	if len(access().Locations) != 0 {
		t.Fatal("empty scope disclosed warehouse")
	}
	post(first.Levels[0].ID, in, 403)
	get(path, 403)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,'user-scope-a')", dbActor.ID)
	post(first.Levels[0].ID, in, 200)
	next := in
	next.OperationID = uuid.NewString()
	next.ExpectedRevision = read(0).Levels[0].Revision
	next.StockedQuantity = "98.000000000000000001"
	exec("CREATE TRIGGER warehouse_inventory_receipt_fault BEFORE INSERT ON inventory_stock_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial warehouse receipt failure'")
	post(first.Levels[0].ID, next, 503)
	exec("DROP TRIGGER warehouse_inventory_receipt_fault")
	if read(0).Levels[0].Revision != next.ExpectedRevision {
		t.Fatal("stock write survived receipt failure")
	}
	post(first.Levels[0].ID, next, 200)
	exec("UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='user-scope-a'")
	if len(access().Locations) != 0 {
		t.Fatal("deleted warehouse disclosed")
	}
	post(first.Levels[0].ID, next, 403)
	exec("UPDATE inventory_locations SET deleted_at=NULL WHERE id='user-scope-a'")
	call("POST", path+"/"+first.Levels[0].ID+"/stock", "{}", origin, "", []*http.Cookie{session}, 403)
	t.Log(`WAREHOUSE_INVENTORY_REHEARSAL {"real_gin_mysql_redis":true,"scope_directory_only":true,"cross_warehouse_read_write_denied":true,"row_location_binding":true,"exact_decimal":true,"pagination_50_plus_1":true,"immutable_replay":true,"revoked_action_and_scope_replay_denied":true,"late_receipt_failure_atomic":true,"full_store_editor_denied":true,"soft_deleted_location_denied":true,"csrf_and_strict_query":true,"no_metadata_cost_or_customer_projection":true}`)
}
