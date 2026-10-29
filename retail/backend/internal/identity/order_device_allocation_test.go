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
	"time"
)

func seedOrderDeviceAllocation(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("allocation fixture", e)
		}
	}
	exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('allocation-product','Device fixture','allocation-product',FALSE,'published',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variants(id,product_id,title,sku,manage_inventory,allow_backorder,created_at,updated_at) VALUES('allocation-v','allocation-product','Device variant','ALLOCATION-SKU',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO inventory_items(id,title,requires_shipping,created_at,updated_at) VALUES('allocation-stock','Allocation stock',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variant_inventory_items(id,variant_id,inventory_item_id,required_quantity,created_at,updated_at) VALUES('allocation-stock-link','allocation-v','allocation-stock',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('allocation-location','Allocation warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('allocation-other','Other warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES('allocation-source','Allocation own','self','allocation-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('allocation-source-other','Other own','self','allocation-other',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_channel_locations(id,stock_location_id,sales_channel_id,created_at,updated_at) VALUES('allocation-channel-link','allocation-location','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO orders(id,version,status,is_draft_order,currency_code,sales_channel_id,created_at,updated_at) VALUES('allocation-order',1,'pending',FALSE,'eur','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO order_line_items(id,title,variant_id,variant_sku,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,is_custom_price,is_giftcard,created_at,updated_at) VALUES('allocation-line','Allocation snapshot','allocation-v','ALLOCATION-SKU',TRUE,TRUE,FALSE,100,'{"value":"100","precision":38}',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	row := map[string]any{"id": "allocation-order-item", "order_id": "allocation-order", "item_id": "allocation-line", "version": 1, "created_at": time.Now().UTC(), "updated_at": time.Now().UTC()}
	for _, field := range []string{"quantity", "fulfilled_quantity", "shipped_quantity", "return_requested_quantity", "return_received_quantity", "return_dismissed_quantity", "written_off_quantity", "delivered_quantity"} {
		value := "0"
		if field == "quantity" {
			value = "2"
		}
		row[field] = value
		row["raw_"+field] = `{"value":"` + value + `","precision":38}`
	}
	if e := db.Table("order_items").Create(row).Error; e != nil {
		t.Fatal(e)
	}
	exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES('allocation-payment','eur',200,'{"value":"200","precision":38}','authorized',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO order_payment_collections(id,order_id,payment_collection_id,created_at,updated_at) VALUES('allocation-payment-link','allocation-order','allocation-payment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, d := range []struct{ id, sn, source, location string }{{"allocation-a", "ALLOC-A", "allocation-source", "allocation-location"}, {"allocation-b", "ALLOC-B", "allocation-source", "allocation-location"}, {"allocation-c", "ALLOC-C", "allocation-source", "allocation-location"}, {"allocation-foreign", "ALLOC-FOREIGN", "allocation-source-other", "allocation-other"}} {
		exec(`INSERT INTO devices(id,serial_number,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,?, 'allocation-v',?,?,0,'eur','','','available','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, d.id, d.sn, d.source, d.location)
	}
}
func checkOrderDeviceAllocation(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	seedOrderDeviceAllocation(t, db)
	ctx := t.Context()
	h := devices.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1")
	defer exec("UPDATE device_runtime_schema SET commands_enabled=FALSE WHERE id=1")
	const path = "/api/admin/orders/allocation-order/items/allocation-line/allocation"
	read := func() devices.AllocationContext {
		t.Helper()
		var d devices.AllocationContext
		if json.Unmarshal(call("GET", path, "", 200), &d) != nil {
			t.Fatal("allocation context")
		}
		return d
	}
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	old := read()
	if old.Enabled || old.Paid || old.Remaining != 2 || len(old.Devices) != 3 {
		t.Fatal("default gate/payment/channel candidates", old)
	}
	in := devices.AllocationInput{OperationID: uuid.NewString(), ExpectedRevision: old.Revision, Action: "reserve", IDs: []string{"allocation-a", "allocation-b"}}
	call("POST", path, body(in), 503)
	exec("UPDATE order_device_allocation_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE order_device_allocation_runtime SET enabled=FALSE WHERE id=1")
	call("POST", path, body(in), 409)
	exec("UPDATE payment_collections SET status='completed',updated_at=UTC_TIMESTAMP(6) WHERE id='allocation-payment'")
	call("POST", path, body(in), 409)
	in.ExpectedRevision = read().Revision
	fingerprint := func() [32]byte {
		var tables []any
		for _, table := range []string{"inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("id").Find(&rows).Error != nil {
				t.Fatal("inventory fingerprint")
			}
			tables = append(tables, rows)
		}
		b, _ := json.Marshal(tables)
		return sha256.Sum256(b)
	}
	stock := fingerprint()
	exec("CREATE TRIGGER allocation_receipt_fault BEFORE INSERT ON order_device_allocations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial allocation audit fault'")
	call("POST", path, body(in), 503)
	exec("DROP TRIGGER allocation_receipt_fault")
	if now := read(); now.Remaining != 2 || len(now.Assignments) != 0 || now.Revision != in.ExpectedRevision {
		t.Fatal("allocation rollback")
	}
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.Allocate(ctx, actor, "allocation-order", "allocation-line", in)
			if e != nil {
				failed.Store(true)
				return
			}
			if r.Replayed {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if failed.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("allocation concurrent receipt")
	}
	allocated := read()
	if allocated.Remaining != 0 || len(allocated.Assignments) != 2 || len(allocated.Devices) != 1 || fingerprint() != stock {
		t.Fatal("allocation capacity/stock")
	}
	excess := in
	excess.OperationID = uuid.NewString()
	excess.ExpectedRevision = allocated.Revision
	excess.IDs = []string{"allocation-c"}
	call("POST", path, body(excess), 409)
	altered := in
	altered.IDs = []string{"allocation-c"}
	call("POST", path, body(altered), 409)
	release := devices.AllocationInput{OperationID: uuid.NewString(), ExpectedRevision: allocated.Revision, Action: "release", IDs: []string{allocated.Assignments[0].ID}}
	call("POST", path, body(release), 200)
	call("POST", path, body(release), 200)
	if read().Remaining != 1 || fingerprint() != stock {
		t.Fatal("release/stock")
	}
	foreign := in
	foreign.OperationID = uuid.NewString()
	foreign.ExpectedRevision = read().Revision
	foreign.IDs = []string{"allocation-foreign"}
	call("POST", path, body(foreign), 409)
	exec("UPDATE orders SET status='canceled' WHERE id='allocation-order'")
	foreign.IDs = []string{"allocation-c"}
	foreign.ExpectedRevision = read().Revision
	call("POST", path, body(foreign), 409)
	exec("UPDATE orders SET status='pending' WHERE id='allocation-order'")
	// Completed + authorized is still unpaid; empty collections are also unpaid.
	exec("INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES('allocation-unpaid','eur',1,'{\"value\":\"1\",\"precision\":38}','authorized',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO order_payment_collections(id,order_id,payment_collection_id,created_at,updated_at) VALUES('allocation-unpaid-link','allocation-order','allocation-unpaid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	if read().Paid {
		t.Fatal("partial collection gate")
	}
	exec("DELETE FROM order_payment_collections WHERE id='allocation-unpaid-link'")
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='order.allocation.write'", role)
	call("POST", path, body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'order.allocation.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("POST", path, body(in), 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("UPDATE device_assignments SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", allocated.Assignments[0].ID)
	call("POST", path, body(in), 404)
	exec("UPDATE device_assignments SET deleted_at=NULL WHERE id=?", allocated.Assignments[0].ID)
	t.Log(`ORDER_DEVICE_ALLOCATION_REHEARSAL {"default_disabled":true,"new_explicit_action":true,"all_collections_completed":true,"authorization_alone_denied":true,"stale_payment_denied":true,"same_sku_channel_available_candidates":true,"concurrent_attempts":4,"single_batch_and_receipt":true,"capacity_and_overallocation_denied":true,"reserve_and_release":true,"immutable_replay":true,"late_receipt_failure_atomic":true,"quantity_stock_and_reservations_unchanged":true,"current_rights_and_deleted_target_rechecked":true,"warehouse_full_store_denied":true}`)
}
