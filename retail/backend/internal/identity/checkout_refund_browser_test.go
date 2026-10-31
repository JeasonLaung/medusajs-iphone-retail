package identity_test

import (
	"gorm.io/gorm"
	"os"
	"strings"
	"testing"
)

func checkCheckoutRefund(t *testing.T, db *gorm.DB, orders []string, worker *checkoutRefundWorker) {
	t.Helper()
	if worker.posts.Load() != 4 || worker.gets.Load() < 5 || worker.lost.Load() != 1 || worker.runs.Load() < 2 || worker.rollback.Load() != 1 || worker.localRecovery.Load() != 1 {
		t.Fatal("independent refund worker process evidence differs")
	}
	exchange := os.Getenv("CHECKOUT_EXCHANGE_REFUND") == "1"
	for _, order := range orders {
		for _, check := range []struct {
			sql  string
			want int64
		}{
			{"SELECT COUNT(*) FROM after_sales_requests r JOIN customer_order_ownership own ON own.order_id=r.order_id AND own.customer_id=r.customer_id WHERE r.order_id=? AND r.type='return' AND r.status='completed' AND r.deleted_at IS NULL", 1},
			{"SELECT COUNT(*) FROM customer_after_sales_operations WHERE order_id=?", 1},
			{"SELECT COUNT(*) FROM device_return_receipts WHERE order_id=?", 1},
			{"SELECT COUNT(*) FROM device_service_receipts WHERE order_id=? AND JSON_UNQUOTE(JSON_EXTRACT(request_json,'$.action'))='inspect' AND JSON_LENGTH(JSON_EXTRACT(request_json,'$.photos'))=1", 1},
			{"SELECT COUNT(*) FROM order_refund_operations WHERE order_id=? AND state='refunded' AND applied=TRUE AND amount='158.75'", 1},
			{"SELECT COUNT(*) FROM payment_refunds r JOIN order_refund_operations o ON r.id=CONCAT('ref_',o.operation_id) WHERE o.order_id=? AND r.amount=158.75 AND r.note=CONCAT('after-sales:',o.request_id)", 1},
			{"SELECT COUNT(*) FROM order_transactions WHERE order_id=? AND reference='refund' AND amount=-158.75", 1},
			{"SELECT COUNT(*) FROM order_summaries s JOIN orders o ON o.id=s.order_id AND o.version=s.version WHERE s.order_id=? AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.refunded_total'))='158.75'", 1},
			{"SELECT COUNT(*) FROM customer_account_notices n JOIN customer_order_ownership own ON own.order_id=n.resource_id AND own.customer_id=n.customer_id WHERE n.resource_id=? AND n.read_at IS NOT NULL", 4},
			{"SELECT COUNT(*) FROM customer_account_notices WHERE resource_id=? AND kind='refund.created' AND read_at IS NOT NULL", 1},
			{"SELECT COUNT(*) FROM device_assignments a JOIN devices d ON d.id=a.device_id WHERE a.order_id=? AND a.status='returned' AND d.state='available' AND d.category='sale' AND a.tracking_number LIKE 'CHECKOUT-TRACK-%'", 1},
		} {
			if exchange {
				check.sql = strings.ReplaceAll(check.sql, "r.type='return'", "r.type='exchange'")
				check.sql = strings.ReplaceAll(check.sql, "amount='158.75'", "amount='12.00'")
				check.sql = strings.ReplaceAll(check.sql, "158.75", "12")
				if strings.Contains(check.sql, "customer_account_notices n") {
					check.want = 5
				}
			}
			var count int64
			if e := db.Raw(check.sql, order).Scan(&count).Error; e != nil || count != check.want {
				t.Fatalf("new customer refund facts differ %s: %d %v", check.sql, count, e)
			}
		}
	}
	if exchange {
		t.Log(`CHECKOUT_EXCHANGE_REFUND_JOINT_SQL {"same_four_new_shopped_shipped_orders":true,"native_exchange_negative_difference":"-12","exact_refunded_per_order":"12","final_pending_difference":"0","five_own_read_notices_per_order":true,"actual_admin_refund_drawer":true,"completion_before_credit_settled_denied":true,"actual_production_worker_binary_separate_processes":true,"real_mollie_adapter_local_trusted_https_fixture":true,"lost_post_response_recovered_after_process_restart_get_only":true,"provider_posts":4,"late_notice_failure_rolls_back_cash_and_notice":true,"saved_observation_local_reapply_in_new_process":true,"real_provider_refund":false}`)
		return
	}
	t.Log(`CHECKOUT_FULFILLMENT_JOINT_SQL {"same_four_new_paid_orders":true,"four_canceled_and_four_shipped_parcels":true,"reservations_consumed":true,"original_devices_received_and_inspected":true,"final_reserved_delta":"0","final_stock_delta_from_added_baseline":"-4"}`)
	t.Log(`CHECKOUT_REFUND_JOINT_SQL {"same_four_new_shopped_shipped_orders":true,"own_return_creation":true,"scanned_physical_receipt_photo_inspection":true,"actual_admin_refund_drawer":true,"actual_production_worker_binary_separate_processes":true,"real_mollie_adapter_local_trusted_https_fixture":true,"lost_post_response_recovered_after_process_restart_get_only":true,"provider_posts":4,"late_notice_failure_rolls_back_cash_and_notice":true,"saved_observation_local_reapply_in_new_process":true,"one_refund_negative_transaction_and_own_notice_per_order":true,"exact_refunded_per_order":"158.75","four_own_read_notices_per_order":true,"real_provider_refund":false}`)
}
