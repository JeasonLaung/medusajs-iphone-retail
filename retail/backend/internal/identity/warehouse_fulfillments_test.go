package identity_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
)

func checkWarehouseFulfillments(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte, parcelID string) {
	t.Helper()
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("warehouse fulfillment fixture SQL")
		}
	}
	var location, role string
	if db.Table("fulfillments").Select("location_id").Where("id=?", parcelID).Scan(&location).Error != nil || location == "" {
		t.Fatal("parcel location missing")
	}
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("warehouse role missing")
	}
	exec("UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE warehouse_scope_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	defer exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_scopes(user_id,mode) VALUES(?,'selected')", actor.ID)
	defer exec("DELETE FROM authorization_warehouse_scopes WHERE user_id=?", actor.ID)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, location)
	defer exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	path := "/api/admin/warehouse/locations/" + location + "/fulfillments"
	detail := "/api/admin/warehouse/locations/" + location + "/orders/allocation-order/fulfillments/" + parcelID
	raw := call("GET", detail, "", 200)
	var out devices.WarehouseFulfillmentDetail
	if json.Unmarshal(raw, &out) != nil || out.ID != parcelID || out.ShippedAt == nil || len(out.Devices) != 1 || len(out.Labels) != 1 || len(out.Items) != 1 || out.Labels[0].TrackingNumber != "TRACK-001" {
		t.Fatal("warehouse scoped shipped parcel projection")
	}
	for _, secret := range []string{"cost_cents", "raw_quantity", "payment", "metadata", "imei", "source_id", "inventory_item_id"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("warehouse parcel field disclosure", secret)
		}
	}
	var list devices.WarehouseFulfillments
	if json.Unmarshal(call("GET", path+"?q="+parcelID, "", 200), &list) != nil || len(list.Fulfillments) != 1 || list.Fulfillments[0].ID != parcelID || list.HasMore {
		t.Fatal("scoped parcel search")
	}
	call("GET", path+"?q=%25_!", "", 200)
	call("GET", path+"?offset=01", "", 400)
	call("GET", path+"?fields=*", "", 400)
	call("GET", path+"?q=a&q=b", "", 400)
	call("GET", detail+"?fields=*", "", 400)
	call("GET", "/api/admin/warehouse/locations/"+location+"/orders/missing/fulfillments/"+parcelID, "", 404)
	call("GET", "/api/admin/orders/allocation-order/fulfillments/"+parcelID+"/operations", "", 403)
	// A moved parcel and a revoked binding must cease to be visible immediately.
	exec("UPDATE fulfillments SET location_id='user-scope-b' WHERE id=?", parcelID)
	call("GET", detail, "", 404)
	exec("UPDATE fulfillments SET location_id=? WHERE id=?", location, parcelID)
	exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	call("GET", path, "", 403)
	call("GET", detail, "", 403)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, location)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='order.fulfillment.read'", role)
	call("GET", detail, "", 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'order.fulfillment.read')", role)
	exec("UPDATE authorization_principals SET kind='supplier' WHERE user_id=?", actor.ID)
	if _, e := h.ReadWarehouseFulfillments(ctx, actor, location, "", 0); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("supplier accessed warehouse parcels")
	}
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	exec("UPDATE fulfillments SET canceled_at=UTC_TIMESTAMP(6) WHERE id=?", parcelID)
	call("GET", detail, "", 404)
	exec("UPDATE fulfillments SET canceled_at=NULL WHERE id=?", parcelID)
	t.Log(`WAREHOUSE_FULFILLMENTS_REHEARSAL {"real_gin_mysql":true,"authorized_location_only":true,"existing_shipped_parcel_and_tracking":true,"no_payment_cost_other_warehouse_fields":true,"strict_query":true,"moved_canceled_wrong_order_hidden":true,"scope_and_action_revocation":true,"full_store_endpoint_denied":true,"supplier_denied":true,"warehouse_write_implemented":false}`)
}
