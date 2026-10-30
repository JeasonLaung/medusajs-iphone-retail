package identity_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
)

func checkDeviceReplacement(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte, assignment string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("replacement fixture", e)
		}
	}
	path := "/api/admin/orders/allocation-order/after-sales/asr-receive-exchange/devices/" + assignment + "/replacement"
	read := func() devices.ReplacementContext {
		t.Helper()
		raw := call("GET", path, "", 200)
		for _, key := range []string{"imei", "cost_cents", "notes", "actor_id", "tracking_number"} {
			if strings.Contains(string(raw), key) {
				t.Fatal("replacement private projection", key)
			}
		}
		var out devices.ReplacementContext
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("replacement response")
		}
		return out
	}
	fingerprint := func() string {
		t.Helper()
		var values []struct{ ID, State, Category, VariantID string }
		if db.Table("devices").Select("id,state,category,variant_id").Order("id").Scan(&values).Error != nil {
			t.Fatal("replacement fingerprint")
		}
		b, _ := json.Marshal(values)
		return string(b)
	}
	baseline := fingerprint()
	initial := read()
	if !initial.Eligible || initial.Reason != "eligible" || initial.WriteAvailable || initial.VariantID != "allocation-v" || initial.OriginalDeviceID != "allocation-b" {
		t.Fatal("replacement initial facts", initial.Reason)
	}
	// Inspection changed the original to service-other-v; replacement MUST use
	// the order line SKU. The ordinary order is fully fulfilled already.
	found := false
	for _, d := range initial.Devices {
		if d.ID == "allocation-foreign" || d.ID == "allocation-b" {
			t.Fatal("foreign/original candidate")
		}
		if d.ID == "allocation-c" {
			found = true
		}
	}
	if !found {
		t.Fatal("same SKU available candidate absent")
	}
	exec("UPDATE devices SET state='inspection' WHERE id='allocation-c'")
	changed := read()
	if changed.Revision == initial.Revision {
		t.Fatal("replacement candidate change not versioned")
	}
	for _, d := range changed.Devices {
		if d.ID == "allocation-c" {
			t.Fatal("unavailable candidate")
		}
	}
	exec("UPDATE devices SET state='available' WHERE id='allocation-c'")
	exec("UPDATE devices SET variant_id='service-other-v' WHERE id='allocation-c'")
	for _, d := range read().Devices {
		if d.ID == "allocation-c" {
			t.Fatal("wrong SKU candidate")
		}
	}
	exec("UPDATE devices SET variant_id='allocation-v' WHERE id='allocation-c'")
	exec("UPDATE devices SET source_id='allocation-source-other' WHERE id='allocation-c'")
	for _, d := range read().Devices {
		if d.ID == "allocation-c" {
			t.Fatal("mismatched source location candidate")
		}
	}
	exec("UPDATE devices SET source_id='allocation-source' WHERE id='allocation-c'")
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,replacement_of,after_sales_request_id,created_at,updated_at) VALUES('replacement-planning-active','allocation-c','allocation-order','allocation-line','allocation-source','reserved',?,'asr-receive-exchange',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", assignment)
	if r := read(); r.Eligible || r.Reason != "replacement_exists" || len(r.Devices) != 0 {
		t.Fatal("duplicate replacement not blocked")
	}
	exec("UPDATE device_assignments SET status='canceled' WHERE id='replacement-planning-active'")
	if !read().Eligible {
		t.Fatal("canceled replacement blocks new allocation")
	}
	exec("DELETE FROM device_assignments WHERE id='replacement-planning-active'")
	for _, f := range []struct{ sql, undo, reason string }{
		{"UPDATE orders SET status='canceled' WHERE id='allocation-order'", "UPDATE orders SET status='pending' WHERE id='allocation-order'", "order_inactive"},
		{"UPDATE after_sales_requests SET status='completed' WHERE id='asr-receive-exchange'", "UPDATE after_sales_requests SET status='processing' WHERE id='asr-receive-exchange'", "not_processing"},
		{"UPDATE after_sales_requests SET type='repair' WHERE id='asr-receive-exchange'", "UPDATE after_sales_requests SET type='exchange' WHERE id='asr-receive-exchange'", "not_exchange"},
		{"UPDATE device_assignments SET status='shipped' WHERE id=?", "UPDATE device_assignments SET status='returned' WHERE id=?", "original_not_returned"},
		{"UPDATE device_assignments SET return_id='another-return' WHERE id=?", "UPDATE device_assignments SET return_id=NULL WHERE id=?", "native_return_exists"},
	} {
		args := []any{}
		if strings.Contains(f.sql, "?") {
			args = append(args, assignment)
		}
		exec(f.sql, args...)
		r := read()
		if r.Eligible || r.Reason != f.reason || len(r.Devices) != 0 {
			t.Fatal("replacement gate", f.reason, r.Reason)
		}
		exec(f.undo, args...)
	}
	exec("UPDATE payment_collections p JOIN order_payment_collections l ON l.payment_collection_id=p.id SET p.status='authorized' WHERE l.order_id='allocation-order'")
	if read().Reason != "payment_required" {
		t.Fatal("authorized is not paid")
	}
	exec("UPDATE payment_collections p JOIN order_payment_collections l ON l.payment_collection_id=p.id SET p.status='completed' WHERE l.order_id='allocation-order'")
	call("GET", path+"?unknown=1", "", 400)
	call("GET", path+"?", "", 400)
	call("GET", strings.Replace(path, "allocation-order", "foreign-order", 1), "", 404)
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='order.allocation.write'", role)
	call("GET", path, "", 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'order.allocation.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("GET", path, "", 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("UPDATE devices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='allocation-b'")
	call("GET", path, "", 404)
	exec("UPDATE devices SET deleted_at=NULL WHERE id='allocation-b'")
	if fingerprint() != baseline {
		t.Fatal("preparation mutated device facts")
	}
	checkOrderVersion(t, db)
	checkExchangeItem(t, db, actor, assignment)
	t.Log(`DEVICE_REPLACEMENT_PREPARATION {"real_gin_mysql_redis":true,"order_sku_after_inspection":true,"fulfilled_order_candidates":true,"same_sku_channel_source_and_availability":true,"active_replacement_blocked":true,"native_return_binding":true,"payment_and_processing_required":true,"revision_changes_with_candidates":true,"revoked_warehouse_deleted_foreign_denied":true,"query_rejected":true,"no_device_mutation":true,"write_available":false}`)
}
