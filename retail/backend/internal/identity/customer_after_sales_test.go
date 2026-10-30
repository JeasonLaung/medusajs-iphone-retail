package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCustomerAfterSales(t *testing.T, ctx context.Context, db *gorm.DB, store *customer.Store, owner, other, orderID string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("after-sales fixture SQL")
		}
	}
	var item struct{ ItemID, VariantID string }
	if db.Table("order_items i").Select("i.item_id,l.variant_id").Joins("JOIN order_line_items l ON l.id=i.item_id").Where("i.order_id=? AND i.deleted_at IS NULL", orderID).Take(&item).Error != nil {
		t.Fatal("after-sales line fixture")
	}
	exec("INSERT INTO device_variant_tracking(id,variant_id,inventory_item_id,created_at,updated_at) VALUES ('customer-as-tracking',?,'add-item',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", item.VariantID)
	defer exec("DELETE FROM device_variant_tracking WHERE id='customer-as-tracking'")
	defer exec("UPDATE customer_after_sales_runtime SET enabled=FALSE WHERE id=1")
	clear := func() {
		exec("DELETE FROM audit_events WHERE action='customer.after_sales.create' AND actor_id IN (SELECT customer_id FROM customer_order_ownership WHERE order_id=?)", orderID)
		exec("DELETE FROM customer_after_sales_operations WHERE order_id=?", orderID)
		exec("DELETE FROM after_sales_requests WHERE order_id=?", orderID)
	}
	defer clear()
	makeInput := func() customer.AfterSalesInput {
		return customer.AfterSalesInput{OperationID: uuid.NewString(), Type: "return", Reason: "Please review this item before delivery.", Items: []customer.AfterSalesLine{{ItemID: item.ItemID, Quantity: 1}}}
	}
	expect := func(in customer.AfterSalesInput, want error) {
		t.Helper()
		_, e := store.CreateAfterSales(ctx, owner, orderID, in)
		if !errors.Is(e, want) {
			t.Fatal("after-sales expected denial", e, want)
		}
	}
	if _, e := store.AfterSales(ctx, other, orderID); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("foreign account read requests", e)
	}
	if _, e := store.OrderDevices(ctx, other, orderID); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("foreign account read devices", e)
	}
	if _, e := store.CreateAfterSales(ctx, other, orderID, makeInput()); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("foreign account create after-sales", e)
	}
	expect(makeInput(), customer.ErrUnavailable)
	exec("UPDATE customer_after_sales_runtime SET enabled=TRUE WHERE id=1")
	for _, kind := range []string{"repair", "exchange"} {
		in := makeInput()
		in.Type = kind
		expect(in, customer.ErrInvalid)
	}
	in := makeInput()
	in.Items[0].Quantity = 2
	expect(in, customer.ErrInvalid)
	in = makeInput()
	in.Items[0].ItemID = "foreign-line"
	expect(in, customer.ErrInvalid)
	in = makeInput()
	in.Items[0].DeviceIDs = []string{"foreign-device"}
	expect(in, customer.ErrInvalid)
	// A late audit failure rolls back request, unique open slot and receipt.
	exec("CREATE TRIGGER after_sales_audit_failure BEFORE INSERT ON audit_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial after-sales failure'")
	in = makeInput()
	expect(in, customer.ErrUnavailable)
	exec("DROP TRIGGER after_sales_audit_failure")
	var count int64
	for _, table := range []string{"after_sales_requests", "customer_after_sales_operations"} {
		if db.Table(table).Where("order_id=?", orderID).Count(&count).Error != nil || count != 0 {
			t.Fatal("partial after-sales transaction")
		}
	}
	// Same UUID concurrently commits one request and one audit event.
	var wg sync.WaitGroup
	results := make(chan customer.AfterSalesRequest, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := store.CreateAfterSales(ctx, owner, orderID, in)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent after-sales retry", e)
		}
	}
	key := ""
	for r := range results {
		if key == "" {
			key = r.ID
		}
		if r.ID != key || r.Status != "pending" || r.Items[0].Title == "" {
			t.Fatal("retry differs")
		}
	}
	for _, table := range []string{"after_sales_requests", "customer_after_sales_operations"} {
		if db.Table(table).Where("order_id=?", orderID).Count(&count).Error != nil || count != 1 {
			t.Fatal("duplicate after-sales request")
		}
	}
	if db.Table("audit_events").Where("resource_id=? AND action='customer.after_sales.create'", key).Count(&count).Error != nil || count != 1 {
		t.Fatal("duplicate audit")
	}
	// Receipt replay remains available when new submissions are paused.
	exec("UPDATE customer_after_sales_runtime SET enabled=FALSE WHERE id=1")
	if r, e := store.CreateAfterSales(ctx, owner, orderID, in); e != nil || r.ID != key {
		t.Fatal("paused same operation replay", e)
	}
	exec("UPDATE customer_after_sales_runtime SET enabled=TRUE WHERE id=1")
	expect(makeInput(), customer.ErrAfterSalesConflict)
	changed := in
	changed.Reason = "A different reason using an existing operation."
	expect(changed, customer.ErrAfterSalesConflict)
	list, e := store.AfterSales(ctx, owner, orderID)
	if e != nil || len(list) != 1 || list[0].ID != key {
		t.Fatal("saved request list", e)
	}
	data, _ := json.Marshal(list)
	for _, private := range []string{"customer_id", "order_id", "operation_id", "request_json", "metadata"} {
		if strings.Contains(string(data), private) {
			t.Fatal("private after-sales field", private)
		}
	}
	checkAfterSalesReview(t, ctx, db, orderID, key)
	// Closing the request frees the original one-open slot for a later case.
	exec("UPDATE after_sales_requests SET status='rejected' WHERE id=?", key)
	if _, e := store.CreateAfterSales(ctx, owner, orderID, makeInput()); e != nil {
		t.Fatal("closed request blocked later application", e)
	}
	clear()
	// Serialized distinct requests enforce one open slot, even different UUIDs.
	errs = make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := store.CreateAfterSales(ctx, owner, orderID, makeInput()); errs <- e }()
	}
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, customer.ErrAfterSalesConflict) {
			conflicts++
		} else {
			t.Fatal("distinct concurrent request", e)
		}
	}
	if success != 1 || conflicts != 3 {
		t.Fatal("one open request invariant")
	}
	clear()
	exec("UPDATE orders SET status='canceled' WHERE id=?", orderID)
	expect(makeInput(), customer.ErrAfterSalesConflict)
	exec("UPDATE orders SET status='pending' WHERE id=?", orderID)
	// A shipped managed item requires the correct device for this order line.
	exec(`INSERT INTO fulfillments(id,location_id,requires_shipping,created_at,updated_at,shipped_at) VALUES('customer-as-parcel','private-location',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	defer exec("DELETE FROM fulfillments WHERE id='customer-as-parcel'")
	exec("INSERT INTO order_fulfillments(order_id,fulfillment_id,id,created_at,updated_at) VALUES (?,'customer-as-parcel','customer-as-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", orderID)
	defer exec("DELETE FROM order_fulfillments WHERE fulfillment_id='customer-as-parcel'")
	exec(`INSERT INTO fulfillment_items(id,title,sku,barcode,quantity,raw_quantity,line_item_id,fulfillment_id,created_at,updated_at) VALUES('customer-as-parcel-item','Phone','','',1,'{"value":"1","precision":38}',?,'customer-as-parcel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, item.ItemID)
	defer exec("DELETE FROM fulfillment_items WHERE id='customer-as-parcel-item'")
	exec(`INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('customer-as-device','AS-SERIAL','123456789012345',?,'private-source','private-location',100,'eur','private-bin','private-notes','shipped','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, item.VariantID)
	defer exec("DELETE FROM devices WHERE id='customer-as-device'")
	exec(`INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('customer-as-assignment','customer-as-device',?,?,'private-source','shipped',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, orderID, item.ItemID)
	defer exec("DELETE FROM device_assignments WHERE id='customer-as-assignment'")
	expect(makeInput(), customer.ErrInvalid)
	devices, e := store.OrderDevices(ctx, owner, orderID)
	if e != nil || len(devices) != 1 || devices[0].SerialNumber == nil || *devices[0].SerialNumber != "AS-SERIAL" {
		t.Fatal("customer shipped devices", e)
	}
	for _, kind := range []string{"return", "exchange", "repair"} {
		in = makeInput()
		in.Type = kind
		in.Items[0].DeviceIDs = []string{"customer-as-device"}
		if _, e := store.CreateAfterSales(ctx, owner, orderID, in); e != nil {
			t.Fatal("shipped after-sales type", kind, e)
		}
		clear()
	}
	exec("UPDATE fulfillments SET canceled_at=UTC_TIMESTAMP(6) WHERE id='customer-as-parcel'")
	if _, e := store.CreateAfterSales(ctx, owner, orderID, makeInput()); e != nil {
		t.Fatal("canceled parcel counted as shipped", e)
	}
	clear()
	t.Log(`CUSTOMER_AFTER_SALES_REHEARSAL {"unshipped_managed_return":true,"repair_exchange_need_shipped_devices":true,"shipped_three_types":true,"canceled_parcel_not_shipped":true,"exact_quantity_validation":true,"foreign_devices_denied":true,"cross_account_reads_denied":true,"late_audit_failure_atomic":true,"same_uuid_concurrent_once":true,"distinct_uuid_one_open":true,"immutable_replay":true,"paused_receipt_recovery":true,"private_projection":true,"external_mail_sent":false}`)
}
