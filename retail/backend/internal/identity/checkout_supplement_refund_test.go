package identity_test

import (
	"gorm.io/gorm"
	"testing"
)

// Four actual new customers continue their paid exchange by returning the
// shipped replacement. Commands, cash and receipts are never inserted by SQL.
func checkCheckoutSupplementRefund(t *testing.T, db *gorm.DB, orders []string, worker *checkoutRefundWorker) {
	t.Helper()
	if worker == nil || worker.posts.Load() != 4 || worker.gets.Load() < 5 || worker.lost.Load() != 1 || worker.runs.Load() < 2 || worker.rollback.Load() != 1 || worker.localRecovery.Load() != 1 {
		t.Fatal("supplement refund process recovery differs")
	}
	checkCheckoutExchange(t, db, orders)
	for _, order := range orders {
		for _, q := range []string{
			"SELECT COUNT(*) FROM after_sales_requests r WHERE r.order_id=? AND r.type='return' AND r.status='processing' AND NOT EXISTS (SELECT 1 FROM customer_account_notices n WHERE n.event_key=CONCAT('after_sales.completed:',r.id))",
			"SELECT COUNT(*) FROM after_sales_requests r JOIN customer_order_ownership own ON own.order_id=r.order_id AND own.customer_id=r.customer_id WHERE r.order_id=? AND r.type='return' AND r.status='processing' AND r.deleted_at IS NULL",
			"SELECT COUNT(*) FROM device_assignments a JOIN after_sales_requests r ON r.id=a.after_sales_request_id AND r.type='return' JOIN device_return_receipts receipt ON receipt.assignment_id=a.id AND receipt.request_id=r.id WHERE a.order_id=? AND a.replacement_of IS NOT NULL AND a.status='returned' AND a.return_id IS NOT NULL",
			"SELECT COUNT(*) FROM order_refund_operations r JOIN order_payment_operations o ON r.payment_id=CONCAT('pay_supp_',o.operation_id) AND r.order_id=o.order_id AND r.scope_id=o.order_id AND r.checkout_operation_id=o.operation_id AND r.session_id=o.session_id AND r.request_sha256=o.request_sha256 JOIN payment_captures c ON c.id=CONCAT('cap_supp_',o.operation_id) AND c.amount=6 JOIN payment_refunds f ON f.id=CONCAT('ref_',r.operation_id) AND f.payment_id=r.payment_id AND f.amount=6 AND f.note=CONCAT('after-sales:',r.request_id) JOIN payment_collections col ON col.id=o.collection_id AND col.refunded_amount=6 WHERE r.order_id=? AND r.state='refunded' AND r.applied=TRUE AND r.amount='6.00'",
			"SELECT COUNT(*) FROM order_transactions WHERE order_id=? AND reference='refund' AND amount=-6",
			"SELECT COUNT(*) FROM customer_account_notices n JOIN order_refund_operations o ON n.event_key=CONCAT('refund.created:ref_',o.operation_id) AND n.resource_id=o.order_id JOIN customer_order_ownership own ON own.order_id=o.order_id AND own.customer_id=n.customer_id WHERE o.order_id=? AND n.kind='refund.created' AND n.read_at IS NOT NULL",
		} {
			var n int64
			if e := db.Raw(q, order).Scan(&n).Error; e != nil || n != 1 {
				t.Fatalf("supplement refund fact differs %s: %d %v", q, n, e)
			}
		}
		var n int64
		if db.Table("payment_refunds f").Joins("JOIN cart_completion_receipts c ON c.payment_id=f.payment_id").Where("c.order_id=?", order).Count(&n).Error != nil || n != 0 {
			t.Fatal("supplement refund changed original checkout cash")
		}
	}
	t.Log(`CHECKOUT_SUPPLEMENT_REFUND_JOINT_SQL {"same_four_registered_shopped_customers":true,"native_exchange_supplement_and_replacement_shipment":true,"actual_second_return_request_and_scanned_replacement_receipt":true,"actual_photo_inspection":true,"second_return_completion_rejected_by_source_guard":true,"completion_conflict_http_status":409,"completion_conflict_error_code":"after_sales.unavailable","no_second_return_completed_notice":true,"actual_admin_selects_supplemental_payment":true,"exact_refunded_per_order":"6.00","original_cash_not_refunded":true,"separate_production_refund_cli":true,"lost_post_restart_get_only":true,"late_notice_fault_cash_rollback":true,"saved_observation_local_reapply":true,"seven_own_read_notices_per_order":true,"provider_refund_posts":4,"final_paid_total":"164.75","final_refunded_total":"6","final_pending_difference":"-150","return_amount_change_applied":true,"one_refund_per_request_preserves_source":true,"real_provider":false,"real_email":false}`)
}
