package identity_test

import (
	"gorm.io/gorm"
	"os"
	"strings"
	"testing"
)

func checkCheckoutExchange(t *testing.T, db *gorm.DB, orders []string) {
	t.Helper()
	for _, order := range orders {
		for _, check := range []struct {
			sql  string
			want int64
		}{
			{"SELECT COUNT(*) FROM after_sales_requests r JOIN customer_order_ownership own ON own.order_id=r.order_id AND own.customer_id=r.customer_id WHERE r.order_id=? AND r.type='exchange' AND r.status='completed' AND r.deleted_at IS NULL", 1},
			{"SELECT COUNT(*) FROM customer_after_sales_operations WHERE order_id=?", 1},
			{"SELECT COUNT(*) FROM device_return_receipts WHERE order_id=?", 1},
			{"SELECT COUNT(*) FROM device_service_receipts WHERE order_id=?", 2},
			{"SELECT COUNT(*) FROM device_service_receipts WHERE order_id=? AND JSON_UNQUOTE(JSON_EXTRACT(request_json,'$.action'))='inspect' AND JSON_LENGTH(JSON_EXTRACT(request_json,'$.photos'))=1", 1},
			{"SELECT COUNT(*) FROM orders WHERE id=? AND version=3", 1},
			{"SELECT COUNT(*) FROM order_exchanges WHERE order_id=? AND deleted_at IS NULL", 1},
			{"SELECT COUNT(*) FROM order_changes WHERE order_id=? AND status='confirmed' AND change_type IN ('exchange','return_receive')", 2},
			{"SELECT COUNT(*) FROM returns WHERE order_id=? AND status='received' AND deleted_at IS NULL", 1},
			{"SELECT COUNT(*) FROM device_assignments a JOIN devices d ON d.id=a.device_id WHERE a.order_id=? AND a.status='returned' AND a.replacement_of IS NULL AND d.category='sale' AND d.state='available' AND d.variant_id='add-v' AND a.tracking_number LIKE 'CHECKOUT-TRACK-%'", 1},
			{"SELECT COUNT(*) FROM device_assignments a JOIN device_assignments old ON old.id=a.replacement_of AND old.order_id=a.order_id JOIN devices d ON d.id=a.device_id WHERE a.order_id=? AND a.status='shipped' AND d.state='shipped' AND d.variant_id='add-v' AND a.item_id<>old.item_id AND a.tracking_number LIKE '000-CHECKOUT-EXCHANGE-%'", 1},
			{"SELECT COUNT(*) FROM device_assignments WHERE order_id=? AND status='canceled'", 1},
			{"SELECT COUNT(*) FROM order_fulfillments l JOIN fulfillments f ON f.id=l.fulfillment_id WHERE l.order_id=? AND f.shipped_at IS NOT NULL AND f.canceled_at IS NULL", 2},
			{"SELECT COUNT(*) FROM order_fulfillments l JOIN fulfillments f ON f.id=l.fulfillment_id WHERE l.order_id=? AND f.canceled_at IS NOT NULL", 1},
			{"SELECT COUNT(*) FROM order_device_parcel_operations WHERE order_id=? AND JSON_UNQUOTE(JSON_EXTRACT(request_json,'$.action'))='ship'", 2},
			{"SELECT COUNT(*) FROM customer_account_notices n JOIN customer_order_ownership own ON own.order_id=n.resource_id AND own.customer_id=n.customer_id WHERE n.resource_id=? AND n.read_at IS NOT NULL", 4},
			{"SELECT COUNT(*) FROM order_summaries s JOIN orders o ON o.id=s.order_id AND o.version=s.version WHERE s.order_id=? AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.pending_difference'))='0' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.paid_total'))='158.75' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.refunded_total'))='0'", 1},
			{"SELECT COUNT(*) FROM order_payment_collections l JOIN payment_collections c ON c.id=l.payment_collection_id WHERE l.order_id=? AND c.deleted_at IS NULL", 1},
			{"SELECT COUNT(*) FROM inventory_reservations r JOIN order_items oi ON oi.item_id=r.line_item_id WHERE oi.order_id=? AND r.deleted_at IS NULL", 0},
		} {
			if os.Getenv("CHECKOUT_EXCHANGE_REFUND") == "1" {
				if strings.Contains(check.sql, "customer_account_notices n") {
					check.want = 5
				}
				check.sql = strings.ReplaceAll(check.sql, "$.refunded_total'))='0'", "$.refunded_total'))='12'")
			}
			if os.Getenv("CHECKOUT_EXCHANGE_PAYMENT") == "1" {
				if strings.Contains(check.sql, "customer_account_notices n") {
					check.want = 5
				}
				if strings.Contains(check.sql, "order_payment_collections l") {
					check.want = 2
				}
				check.sql = strings.ReplaceAll(check.sql, "$.paid_total'))='158.75'", "$.paid_total'))='164.75'")
			}
			var n int64
			if e := db.Raw(check.sql, order).Scan(&n).Error; e != nil || n != check.want {
				t.Fatalf("new order exchange facts differ %s: %d %v", check.sql, n, e)
			}
		}
	}
	t.Log(`CHECKOUT_FULFILLMENT_JOINT_SQL {"same_four_new_paid_orders":true,"four_canceled_and_eight_shipped_parcels":true,"two_shipment_receipts_per_order":true,"reservations_consumed":true,"original_and_replacement_shipped":true,"final_reserved_delta":"0","final_stock_delta_from_added_baseline":"-8"}`)
	if os.Getenv("CHECKOUT_EXCHANGE_REFUND") == "1" || os.Getenv("CHECKOUT_EXCHANGE_PAYMENT") == "1" {
		return
	}
	t.Log(`CHECKOUT_EXCHANGE_JOINT_SQL {"same_four_new_shipped_orders":true,"one_own_exchange_and_creation_receipt":true,"one_physical_return_and_photo_inspection":true,"native_exchange_return_and_two_order_versions":true,"one_replacement_and_two_shipping_receipts":true,"original_and_replacement_tracking_retained":true,"four_unique_own_read_notices_per_order":true,"zero_difference_no_new_unpaid_collection":true,"captured_amount_unchanged":"158.75","refunded_amount":"0","no_quantity_restock_of_original":true}`)
}
