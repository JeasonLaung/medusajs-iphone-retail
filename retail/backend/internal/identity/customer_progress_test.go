package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/customer"
	"gorm.io/gorm"
)

// Only synthetic records in the owned SQL rehearsal, never production orders.
func seedCustomerProgress(t *testing.T, db *gorm.DB, orderID string) func() {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("customer progress fixture")
		}
	}
	exec(`INSERT INTO fulfillments(id,location_id,requires_shipping,data,metadata,created_at,updated_at,shipped_at,delivered_at,canceled_at) VALUES
 ('customer-shipped','private-warehouse',TRUE,'{"secret":"provider-private"}','{"secret":"private-metadata"}','2026-10-30 00:10:00',UTC_TIMESTAMP(6),'2026-10-30 00:20:00',NULL,NULL),
 ('customer-canceled','private-warehouse',TRUE,NULL,NULL,'2026-10-30 00:05:00',UTC_TIMESTAMP(6),'2026-10-30 00:06:00',NULL,'2026-10-30 00:07:00'),
 ('customer-deleted','private-warehouse',TRUE,NULL,NULL,'2026-10-30 00:05:00',UTC_TIMESTAMP(6),'2026-10-30 00:06:00',NULL,NULL)`)
	exec("UPDATE fulfillments SET deleted_at=UTC_TIMESTAMP(6) WHERE id='customer-deleted'")
	for _, id := range []string{"customer-shipped", "customer-canceled", "customer-deleted"} {
		exec("INSERT INTO order_fulfillments(order_id,fulfillment_id,id,created_at,updated_at) VALUES (?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", orderID, id, id+"-link")
	}
	exec(`INSERT INTO fulfillment_labels(id,tracking_number,tracking_url,label_url,fulfillment_id,created_at,updated_at) VALUES
 ('customer-label','000012345678901234567890','https://example.invalid/track','https://private.invalid/label','customer-shipped',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
 ('customer-unsafe','unsafe-label','javascript:alert(1)','https://private.invalid/label','customer-shipped',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),
 ('customer-hidden-label','hidden-label','','','customer-shipped',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("UPDATE fulfillment_labels SET deleted_at=UTC_TIMESTAMP(6) WHERE id='customer-hidden-label'")
	exec(`INSERT INTO device_events(id,order_id,actor_id,action,data,created_at,updated_at) VALUES ('customer-allocated',?,'private-actor','assign','{"cost":"private-cost"}','2026-10-30 00:04:00',UTC_TIMESTAMP(6)),('customer-hidden-event',?,'private-actor','intake','{"cost":"private-cost"}','2026-10-30 00:04:00',UTC_TIMESTAMP(6))`, orderID, orderID)
	return func() {
		exec("DELETE FROM device_events WHERE id IN ('customer-allocated','customer-hidden-event')")
		exec("DELETE FROM fulfillment_labels WHERE fulfillment_id='customer-shipped'")
		exec("DELETE FROM order_fulfillments WHERE fulfillment_id IN ('customer-shipped','customer-canceled','customer-deleted')")
		exec("DELETE FROM fulfillments WHERE id IN ('customer-shipped','customer-canceled','customer-deleted')")
	}
}

func checkCustomerProgress(t *testing.T, ctx context.Context, db *gorm.DB, store *customer.Store, owner, other, orderID string) {
	t.Helper()
	cleanup := seedCustomerProgress(t, db, orderID)
	defer cleanup()
	out, e := store.OrderProgress(ctx, owner, orderID)
	if e != nil || out.OrderID != orderID || out.CurrencyCode != "eur" || len(out.Shipments) != 1 || out.Shipments[0].ID != "customer-shipped" || len(out.Shipments[0].Labels) != 2 {
		t.Fatal("customer progress projection", e)
	}
	if out.Shipments[0].Labels[0].TrackingNumber != "000012345678901234567890" || out.Shipments[0].Labels[1].TrackingURL != "" {
		t.Fatal("tracking links or original string lost")
	}
	kinds := map[string]int{}
	for i, event := range out.Events {
		kinds[event.Kind]++
		if i > 0 && out.Events[i-1].OccurredAt.After(event.OccurredAt) {
			t.Fatal("unsorted progress")
		}
		if event.Kind == "paid" && (event.Amount == nil || *event.Amount != "158.75") {
			t.Fatal("exact captured amount")
		}
	}
	if kinds["placed"] != 1 || kinds["paid"] != 1 || kinds["allocated"] != 1 || kinds["packed"] != 2 || kinds["shipped"] != 2 || kinds["fulfillment_canceled"] != 1 {
		t.Fatal("timeline facts differ", kinds)
	}
	body, _ := json.Marshal(out)
	for _, private := range []string{"provider-private", "private-metadata", "private-warehouse", "private-actor", "private-cost", "label_url", "hidden-label", "hidden-event", "customer-deleted", "raw_amount", "javascript:"} {
		if strings.Contains(string(body), private) {
			t.Fatal("private progress field escaped", private)
		}
	}
	if _, e = store.OrderProgress(ctx, other, orderID); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("cross account progress accepted", e)
	}
	if _, e = store.OrderProgress(ctx, owner, "missing-order"); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("missing order progress accepted", e)
	}
	if db.Exec("UPDATE orders SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", orderID).Error != nil {
		t.Fatal("delete fixture")
	}
	_, e = store.OrderProgress(ctx, owner, orderID)
	if db.Exec("UPDATE orders SET deleted_at=NULL WHERE id=?", orderID).Error != nil {
		t.Fatal("restore fixture")
	}
	if !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("deleted order progress exposed", e)
	}
	if db.Exec("INSERT INTO order_fulfillments(order_id,fulfillment_id,id,created_at,updated_at) VALUES ('foreign-progress-order','customer-shipped','shared-progress-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))").Error != nil {
		t.Fatal("shared fixture")
	}
	_, e = store.OrderProgress(ctx, owner, orderID)
	if db.Exec("DELETE FROM order_fulfillments WHERE order_id='foreign-progress-order'").Error != nil {
		t.Fatal("shared cleanup")
	}
	if !errors.Is(e, customer.ErrUnavailable) {
		t.Fatal("shared parcel exposed", e)
	}
	t.Log(`CUSTOMER_PROGRESS_REHEARSAL {"explicit_ownership":true,"cross_account_denied":true,"deleted_order_denied":true,"shared_parcel_denied":true,"exact_capture_amount":"158.75","private_fields_absent":true,"unsafe_url_suppressed":true,"canceled_parcel_hidden_from_tracking":true,"canceled_parcel_events_retained":true,"sorted_events":true}`)
}
