package identity_test

import (
	"errors"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Rehearse the release against a real packed fixture, then roll this extra
// branch back so the original parcel lifecycle continues with identical facts.
func checkWarehouseRelease(t *testing.T, db *gorm.DB, actor identity.User, f string) {
	t.Helper()
	ctx := t.Context()
	rollback := errors.New("owned release rehearsal rollback")
	e := db.Transaction(func(tx *gorm.DB) error {
		exec := func(q string, args ...any) {
			t.Helper()
			if tx.Exec(q, args...).Error != nil {
				t.Fatal("release fixture")
			}
		}
		var location, assignment, role string
		if tx.Table("fulfillments").Select("location_id").Where("id=?", f).Scan(&location).Error != nil || location == "" {
			t.Fatal("release location")
		}
		if tx.Table("device_assignments").Select("id").Where("fulfillment_id=? AND device_id='allocation-b' AND status='packed' AND deleted_at IS NULL", f).Limit(1).Scan(&assignment).Error != nil || assignment == "" {
			t.Fatal("release assignment")
		}
		tx.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
		exec("UPDATE order_device_parcel_runtime SET enabled=TRUE WHERE id=1")
		exec("UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1")
		exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
		exec("INSERT INTO authorization_warehouse_scopes(user_id,mode) VALUES(?,'selected')", actor.ID)
		exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, location)
		// Rehearse a two-device parcel so cancellation must retain the sibling
		// as reserved while releasing only the explicitly selected machine.
		exec("UPDATE device_assignments SET fulfillment_id=? WHERE device_id='allocation-a' AND status='packed' AND deleted_at IS NULL", f)
		exec("UPDATE fulfillment_items SET quantity=2,raw_quantity='{\"value\":\"2\",\"precision\":38}' WHERE fulfillment_id=? AND deleted_at IS NULL", f)
		h := devices.Handler{DB: tx}
		before, e := h.ReadWarehouseRelease(ctx, actor, location, "allocation-order", f)
		if e != nil || len(before.Devices) != 2 {
			t.Fatal("release context", e)
		}
		var stocked, fulfilled string
		tx.Table("inventory_levels").Select("stocked_quantity").Where("id='packing-target'").Scan(&stocked)
		tx.Table("order_items").Select("fulfilled_quantity").Where("id='allocation-order-item'").Scan(&fulfilled)
		in := devices.ParcelInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Action: "cancel"}
		invalid := in
		invalid.Action = "ship"
		invalid.TrackingNumber = "x"
		if _, e = h.ReleaseWarehouseParcelDevice(ctx, actor, location, "allocation-order", f, assignment, invalid); !errors.Is(e, devices.ErrInput) {
			t.Fatal("release accepted ship")
		}
		if _, e = h.ReleaseWarehouseParcelDevice(ctx, actor, location, "allocation-order", f, "foreign-assignment", in); !errors.Is(e, gorm.ErrRecordNotFound) {
			t.Fatal("foreign assignment release", e)
		}
		out, e := h.ReleaseWarehouseParcelDevice(ctx, actor, location, "allocation-order", f, assignment, in)
		if e != nil || out.Replayed {
			t.Fatal("release failed", e)
		}
		var row struct {
			State, Status string
			FulfillmentID *string
		}
		if tx.Table("device_assignments a").Select("d.state,a.status,a.fulfillment_id").Joins("JOIN devices d ON d.id=a.device_id").Where("a.id=?", assignment).Take(&row).Error != nil || row.State != "available" || row.Status != "canceled" || row.FulfillmentID != nil {
			t.Fatal("release state")
		}
		if tx.Table("device_assignments a").Select("d.state,a.status,a.fulfillment_id").Joins("JOIN devices d ON d.id=a.device_id").Where("a.device_id='allocation-a' AND a.status='reserved' AND a.deleted_at IS NULL").Take(&row).Error != nil || row.State != "reserved" || row.Status != "reserved" || row.FulfillmentID != nil {
			t.Fatal("release did not preserve sibling reservation")
		}
		var nextStock, nextFulfilled string
		tx.Table("inventory_levels").Select("stocked_quantity").Where("id='packing-target'").Scan(&nextStock)
		tx.Table("order_items").Select("fulfilled_quantity").Where("id='allocation-order-item'").Scan(&nextFulfilled)
		if nextStock == stocked || nextFulfilled == fulfilled {
			t.Fatal("release did not restore parcel quantity")
		}
		again, e := h.ReleaseWarehouseParcelDevice(ctx, actor, location, "allocation-order", f, assignment, in)
		if e != nil || !again.Replayed {
			t.Fatal("release receipt recovery", e)
		}
		var replayStock string
		tx.Table("inventory_levels").Select("stocked_quantity").Where("id='packing-target'").Scan(&replayStock)
		if replayStock != nextStock {
			t.Fatal("release replay restored twice")
		}
		exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.allocation.write'", role)
		if _, e = h.ReleaseWarehouseParcelDevice(ctx, actor, location, "allocation-order", f, assignment, in); e == nil {
			t.Fatal("revoked release replay")
		}
		exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.allocation.write')", role)
		exec("UPDATE devices SET location_id='user-scope-b' WHERE id='allocation-b'")
		if _, e = h.ReleaseWarehouseParcelDevice(ctx, actor, location, "allocation-order", f, assignment, in); !errors.Is(e, gorm.ErrRecordNotFound) {
			t.Fatal("moved release replay", e)
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal("release rehearsal rollback", e)
	}
	t.Log(`WAREHOUSE_RELEASE_REHEARSAL {"real_mysql":true,"production_shared_cancel_transaction":true,"selected_device_available_assignment_canceled":true,"parcel_stock_restored":true,"immutable_retry_no_double_restore":true,"foreign_assignment_and_ship_denied":true,"revoked_and_moved_replay_denied":true,"fixture_branch_rolled_back":true}`)
}
