package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/refunds"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"testing"
)

type supplementalRefundChannel struct {
	refundChannel
	session string
}

func (a *supplementalRefundChannel) check(in payments.RefundRequest) {
	if in.Session.SessionID != a.session || in.RemotePaymentID != "tr_supplement_"+strings.ReplaceAll(a.session, "-", "") || in.Session.Amount != "6" || in.Session.Currency != "eur" {
		a.t.Error("supplement refund targeted another payment")
	}
}
func (a *supplementalRefundChannel) CreateRefund(ctx context.Context, in payments.RefundRequest) (payments.RefundObservation, error) {
	a.check(in)
	return a.refundChannel.CreateRefund(ctx, in)
}
func (a *supplementalRefundChannel) ObserveRefund(ctx context.Context, in payments.RefundRequest, id string) (payments.RefundObservation, error) {
	a.check(in)
	return a.refundChannel.ObserveRefund(ctx, in, id)
}
func (a *supplementalRefundChannel) DiscoverRefund(ctx context.Context, in payments.RefundRequest) (payments.RefundObservation, error) {
	a.check(in)
	return a.refundChannel.DiscoverRefund(ctx, in)
}

// The checkout, native exchange and supplemental cash are real commands. As in
// the existing refund SQL tests, returned device/request facts are artificial;
// this does not claim a customer browser/CLI return of the replacement device.
func checkSupplementalRefund(t *testing.T, ctx context.Context, db *gorm.DB, paid orderpayments.Service, in orderpayments.Input) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("supplement refund fixture", e)
		}
	}
	var actor identity.User
	if db.Where("id='refund-command-actor'").Take(&actor).Error != nil {
		t.Fatal("refund actor")
	}
	var assignment struct{ ItemID, DeviceID string }
	if db.Table("device_assignments").Where("id='supplement-assignment'").Take(&assignment).Error != nil {
		t.Fatal("supplement returned fixture")
	}
	wire, _ := json.Marshal(map[string]any{"lines": []any{map[string]any{"item_id": assignment.ItemID, "quantity": 1, "device_ids": []string{assignment.DeviceID}}}})
	exec("UPDATE after_sales_requests SET status='completed' WHERE id='supplement-exchange'")
	exec("INSERT INTO after_sales_requests(id,order_id,customer_id,type,reason,items,status,created_at,updated_at) VALUES('supplement-cash-return',?,'order-payment-execution-account','return','Owned refund trace fixture',?,'processing',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", in.OrderID, string(wire))
	exec("UPDATE device_assignments SET after_sales_request_id='supplement-cash-return' WHERE id='supplement-assignment'")
	var session string
	if db.Table("order_payment_operations").Select("session_id").Where("operation_id=?", in.OperationID).Scan(&session).Error != nil {
		t.Fatal("supplement session")
	}
	channel := &supplementalRefundChannel{refundChannel: refundChannel{db: db, t: t, status: "refunded"}, session: session}
	registry, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": channel})
	if e != nil {
		t.Fatal(e)
	}
	s := refunds.Service{DB: db, StoreID: "shop", Ring: paid.Ring, Adapters: registry}
	command := refunds.Input{OperationID: uuid.NewString(), OrderID: in.OrderID, RequestID: "supplement-cash-return", PaymentID: "pay_supp_" + in.OperationID, Amount: "6.00"}
	var data struct{ Data []byte }
	if db.Table("payments").Select("data").Where("id=?", command.PaymentID).Take(&data).Error != nil {
		t.Fatal("supplement cash data")
	}
	rollback := errors.New("supplement trace probe rollback")
	for _, sql := range []string{
		"UPDATE payments SET data='{}' WHERE id=?",
		"UPDATE payment_effect_journal SET result_envelope=REPEAT('x',40) WHERE CONCAT('pay_supp_',operation_id)=? AND effect_kind='create_session'",
		"UPDATE order_payment_operations o JOIN cart_completion_receipts r ON r.order_id=o.order_id JOIN cart_checkout_attempts a ON a.cart_id=r.cart_id AND a.operation_id=r.operation_id SET o.session_id=a.session_id WHERE CONCAT('pay_supp_',o.operation_id)=?",
		"UPDATE payment_captures SET deleted_at=UTC_TIMESTAMP(6) WHERE payment_id=?",
		"UPDATE payment_captures SET amount=7,raw_amount=JSON_OBJECT('value','7','precision',100) WHERE payment_id=?",
		"INSERT INTO payment_captures(id,payment_id,amount,raw_amount,created_at,updated_at) VALUES('supplement-forged-capture',?,6,JSON_OBJECT('value','6','precision',100),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))",
	} {
		if e := db.Transaction(func(tx *gorm.DB) error {
			query := sql
			if e := tx.Exec(query, command.PaymentID).Error; e != nil {
				return e
			}
			local := s
			local.DB = tx
			probe := command
			probe.OperationID = uuid.NewString()
			if _, e := local.Prepare(ctx, actor, probe); e == nil {
				t.Fatal("untrusted supplemental provenance accepted", sql)
			}
			return rollback
		}); !errors.Is(e, rollback) {
			t.Fatal(e)
		}
	}
	foreign := s
	foreign.StoreID = "other-shop"
	if _, e := foreign.Prepare(ctx, actor, command); e != refunds.ErrConflict {
		t.Fatal("foreign supplement refund", e)
	}
	excessive := command
	excessive.OperationID = uuid.NewString()
	excessive.Amount = "6.01"
	if _, e := s.Prepare(ctx, actor, excessive); e != refunds.ErrConflict {
		t.Fatal("supplement exceeded capture", e)
	}
	exec("UPDATE customer_accounts SET enabled=FALSE,auth_version=auth_version+1 WHERE id=(SELECT account_id FROM order_payment_operations WHERE operation_id=?)", in.OperationID)
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Prepare(ctx, actor, command); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("supplement refund concurrent replay", e)
		}
	}
	exec("UPDATE payments SET data='{}' WHERE id=?", command.PaymentID)
	if _, e := s.Run(ctx, command.OperationID); e != refunds.ErrUnavailable || channel.posts.Load() != 0 {
		t.Fatal("untrusted dispatch", e)
	}
	exec("UPDATE payments SET data=? WHERE id=?", data.Data, command.PaymentID)
	channel.lost.Store(true)
	if _, e := s.Run(ctx, command.OperationID); e != payments.ErrRefundExecution || channel.posts.Load() != 1 {
		t.Fatal("supplement refund lost POST", e)
	}
	exec("UPDATE order_refund_operations SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE operation_id=?", command.OperationID)
	exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=(SELECT account_id FROM order_payment_operations WHERE operation_id=?)", in.OperationID)
	channel.lost.Store(false)
	exec("CREATE TRIGGER supplement_refund_notice_fault BEFORE INSERT ON customer_account_notices FOR EACH ROW BEGIN IF NEW.kind='refund.created' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned supplement refund notice failure'; END IF; END")
	_, e = s.Run(ctx, command.OperationID)
	exec("DROP TRIGGER supplement_refund_notice_fault")
	if e == nil {
		t.Fatal("supplement refund notice fault accepted")
	}
	for _, q := range []string{"SELECT COUNT(*) FROM payment_refunds WHERE payment_id=?", "SELECT COUNT(*) FROM order_transactions WHERE reference='refund' AND order_id=(SELECT order_id FROM order_payment_operations WHERE CONCAT('pay_supp_',operation_id)=?)"} {
		var n int64
		if db.Raw(q, command.PaymentID).Scan(&n).Error != nil || n != 0 {
			t.Fatal("partial supplement refund survived")
		}
	}
	gets := channel.gets.Load()
	for i := 0; i < 3; i++ {
		if r, e := s.Run(ctx, command.OperationID); e != nil || !r.Applied || r.State != "refunded" {
			t.Fatal("supplement refund local reapply", r, e)
		}
	}
	if channel.posts.Load() != 1 || channel.gets.Load() != gets {
		t.Fatal("supplement refund called provider on local reapply")
	}
	for _, q := range []string{
		"SELECT COUNT(*) FROM payment_refunds WHERE payment_id=? AND amount=6",
		"SELECT COUNT(*) FROM payments p JOIN payment_collections c ON c.id=p.payment_collection_id AND c.refunded_amount=6 WHERE p.id=?",
		"SELECT COUNT(*) FROM order_payment_operations o JOIN order_summaries s ON s.order_id=o.order_id AND s.version=3 WHERE CONCAT('pay_supp_',o.operation_id)=? AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.paid_total'))='164.75' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.refunded_total'))='6' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.pending_difference'))='6'",
		"SELECT COUNT(*) FROM payment_refunds f JOIN order_transactions x ON x.reference_id=f.id AND x.reference='refund' AND x.amount=-6 WHERE f.payment_id=?",
		"SELECT COUNT(*) FROM payment_refunds f JOIN order_refund_operations o ON CONCAT('ref_',o.operation_id)=f.id JOIN customer_account_notices n ON n.event_key=CONCAT('refund.created:',f.id) AND n.resource_id=o.order_id WHERE f.payment_id=?",
	} {
		var n int64
		if e := db.Raw(q, command.PaymentID).Scan(&n).Error; e != nil || n != 1 {
			t.Fatal("supplement refund not unique", q, n, e)
		}
	}
	var original int64
	if db.Table("payment_refunds f").Joins("JOIN cart_completion_receipts r ON r.payment_id=f.payment_id").Where("r.order_id=?", in.OrderID).Count(&original).Error != nil || original != 0 {
		t.Fatal("supplement refund touched original cash")
	}
	t.Log(`SUPPLEMENTAL_REFUND_TRACE_REHEARSAL {"actual_native_checkout_exchange_supplement_cash":true,"return_facts_artificial":true,"protected_creation_and_paid_payload_required":true,"corrupt_and_cross_session_denied":true,"first_dispatch_rechecks_evidence":true,"cross_store_denied":true,"capture_limit":"6.00","concurrent_same_uuid":6,"lost_post_get_only":true,"late_notice_fault_rolls_back_cash":true,"saved_refund_local_reapply":true,"unique_refund_negative_transaction_notice":true,"original_capture_not_refunded":true,"retired_cart_revoked_customer_independent":true,"refunded_total":"6","pending_difference_without_return_amount_change":"6","customer_browser_cli_joint":false,"real_provider":false}`)
}
