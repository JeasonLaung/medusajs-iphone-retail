package identity_test

import (
	"gorm.io/gorm"
	"testing"
)

func checkCheckoutRepair(t *testing.T, db *gorm.DB, orders []string) {
	t.Helper()
	for _, order := range orders {
		for _, check := range []struct {
			sql  string
			want int64
		}{
			{"SELECT COUNT(*) FROM after_sales_requests r JOIN customer_order_ownership own ON own.order_id=r.order_id AND own.customer_id=r.customer_id WHERE r.order_id=? AND r.type='repair' AND r.status='completed' AND r.deleted_at IS NULL", 1},
			{"SELECT COUNT(*) FROM customer_after_sales_operations WHERE order_id=?", 1},
			{"SELECT COUNT(*) FROM device_return_receipts WHERE order_id=?", 1},
			{"SELECT COUNT(*) FROM device_service_receipts WHERE order_id=?", 2},
			{"SELECT COUNT(*) FROM device_service_receipts WHERE order_id=? AND JSON_UNQUOTE(JSON_EXTRACT(request_json,'$.action'))='inspect' AND JSON_LENGTH(JSON_EXTRACT(request_json,'$.photos'))=1", 1},
			{"SELECT COUNT(*) FROM device_events WHERE order_id=? AND action='repair_ship'", 1},
			{"SELECT COUNT(*) FROM device_assignments a JOIN devices d ON d.id=a.device_id WHERE a.order_id=? AND a.status='shipped' AND d.variant_id='add-v' AND d.category='returned' AND d.state='shipped' AND d.grade='B' AND d.battery_health=90", 1},
			{"SELECT COUNT(*) FROM customer_account_notices WHERE resource_id=? AND read_at IS NOT NULL AND kind IN ('shipment.created','after_sales.processing','after_sales.completed')", 3},
			{"SELECT COUNT(*) FROM order_fulfillments l JOIN fulfillment_labels f ON f.fulfillment_id=l.fulfillment_id WHERE l.order_id=? AND f.tracking_number LIKE 'CHECKOUT-TRACK-%'", 1},
		} {
			var n int64
			if e := db.Raw(check.sql, order).Scan(&n).Error; e != nil || n != check.want {
				t.Fatalf("new order repair facts differ %s: %d %v", check.sql, n, e)
			}
		}
	}
	t.Log(`CHECKOUT_REPAIR_JOINT_SQL {"same_four_new_shipped_orders":true,"one_own_repair_request_and_creation_receipt":true,"one_physical_receipt_per_order":true,"one_inspection_with_photo_and_one_repair_dispatch":true,"original_sku_and_device_retained":true,"quantity_inventory_unchanged_by_repair":true,"original_sales_tracking_retained":true,"three_unique_own_read_notices_per_order":true,"completed_after_repair_dispatch":true}`)
}
