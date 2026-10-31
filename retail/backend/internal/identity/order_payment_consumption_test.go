package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
)

// Evidence comes through the actual notification claim/GET/encryption path.
// The isolated adapter is not Mollie, and the account ownership is artificial.
func checkOrderPaymentConsumption(t *testing.T, ctx context.Context, db *gorm.DB, s orderpayments.Service, in orderpayments.Input, cartID string, channel *orderPaymentChannel) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("supplement consumption fixture", e)
		}
	}
	stockFingerprint := func() string {
		t.Helper()
		var snapshots []json.RawMessage
		for _, table := range []string{"inventory_levels", "inventory_reservations", "device_assignments"} {
			var rows []map[string]any
			if db.Table(table).Order("id").Find(&rows).Error != nil {
				t.Fatal("cash stock fingerprint")
			}
			raw, _ := json.Marshal(rows)
			snapshots = append(snapshots, raw)
		}
		raw, _ := json.Marshal(snapshots)
		return string(raw)
	}
	stockBefore := stockFingerprint()
	ordersBefore := int64(0)
	if db.Table("orders").Count(&ordersBefore).Error != nil {
		t.Fatal("cash initial order count")
	}
	var remote string
	if db.Table("mollie_payment_references").Select("remote_id").Where("scope_id=? AND operation_id=?", in.OrderID, in.OperationID).Scan(&remote).Error != nil || remote == "" {
		t.Fatal("supplement remote reference")
	}
	// These statuses pass the same immutable remote request verification as paid.
	status := "authorized"
	registry, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			t.Error("consumer issued POST")
			return payments.CreateUncertain, payments.CreatedSession{}, payments.ErrCreateExecution
		},
		observe: func(ctx context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			o, e := channel.ObserveSession(ctx, r, id)
			if e != nil {
				return o, e
			}
			var raw map[string]any
			_ = json.Unmarshal(o.Session.Data, &raw)
			raw["status"] = status
			o.Session.Data, _ = json.Marshal(raw)
			o.ProviderStatus = status
			switch status {
			case "authorized":
				o.Action = "authorized"
				o.Session.Status = "authorized"
			case "open":
				o.Action = "not_supported"
				o.Session.Status = "pending"
			case "paid":
				o.Action = "successful"
				o.Session.Status = "authorized"
			}
			return o, nil
		},
	}})
	if e != nil {
		t.Fatal(e)
	}
	observe := func() string {
		t.Helper()
		if e := db.Transaction(func(tx *gorm.DB) error {
			known, e := payments.EnqueueMollieNotification(ctx, tx, "pp_mollie-hosted-checkout_mollie", remote)
			if e == nil && !known {
				t.Error("unknown supplement reference")
			}
			return e
		}); e != nil {
			t.Fatal("enqueue supplement", e)
		}
		observed, e := payments.ProcessMollieNotification(ctx, db, "pp_mollie-hosted-checkout_mollie", remote, s.Ring, registry)
		if e != nil || !observed.Called || observed.Receipt.AttemptID == nil {
			t.Fatal("observe supplement", e)
		}
		return *observed.Receipt.AttemptID
	}
	count := func(table, where string, args ...any) int64 {
		t.Helper()
		var n int64
		if db.Table(table).Where(where, args...).Count(&n).Error != nil {
			t.Fatal("consumption count", table)
		}
		return n
	}
	assertUnpaid := func() {
		t.Helper()
		if count("customer_account_notices", "event_key=?", "payment.received:pay_supp_"+in.OperationID) != 0 || count("payments", "payment_collection_id=?", in.CollectionID) != 0 || count("payment_captures", "id=?", "cap_supp_"+in.OperationID) != 0 || count("order_transactions", "id=?", "ordtr_supp_"+in.OperationID) != 0 {
			t.Fatal("partial supplement cash leaked")
		}
		var row struct {
			State   string
			Applied bool
		}
		if db.Table("order_payment_operations").Where("operation_id=?", in.OperationID).Take(&row).Error != nil || row.Applied || row.State != "authorized" {
			t.Fatal("unpaid operation mutated", row)
		}
		var collection struct{ Status string }
		if db.Table("payment_collections").Where("id=?", in.CollectionID).Take(&collection).Error != nil || collection.Status != "not_paid" {
			t.Fatal("unpaid collection mutated")
		}
	}
	authorized := observe()
	if r, e := s.Consume(ctx, authorized); e != nil || r.State != "authorized" || r.Applied {
		t.Fatal("authorized booked cash", r, e)
	}
	assertUnpaid()
	status = "open"
	staleOpen := observe()
	if r, e := s.Consume(ctx, staleOpen); e != nil || r.State != "authorized" || r.Applied {
		t.Fatal("open regressed authorization", r, e)
	}
	status = "paid"
	paid := observe()
	gets := channel.gets.Load()
	foreign := s
	foreign.StoreID = "other-shop"
	if _, e = foreign.Consume(ctx, paid); e != orderpayments.ErrConflict {
		t.Fatal("foreign store consumed payment", e)
	}
	var savedObservation struct{ ResultEnvelope []byte }
	if db.Table("mollie_notification_observations").Select("result_envelope").Where("attempt_id=?", paid).Take(&savedObservation).Error != nil || len(savedObservation.ResultEnvelope) == 0 {
		t.Fatal("protected observation absent")
	}
	envelope := savedObservation.ResultEnvelope
	bad := append([]byte(nil), envelope...)
	bad[len(bad)-1] ^= 1
	exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", bad, paid)
	if _, e = s.Consume(ctx, paid); e == nil {
		t.Fatal("corrupt paid envelope accepted")
	}
	exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", envelope, paid)
	assertUnpaid()
	// The old generation remains revoked; a currently enabled verified recipient
	// receives this cash event. The independent CLI case retains disabled recovery.
	var owner struct{ AccountID string }
	if db.Table("order_payment_operations").Select("account_id").Where("operation_id=?", in.OperationID).Take(&owner).Error != nil {
		t.Fatal("notice owner")
	}
	exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=?", owner.AccountID)
	for _, change := range []string{"UPDATE customer_accounts SET enabled=FALSE WHERE id=?", "UPDATE customer_accounts SET verified_at=NULL WHERE id=?", "DELETE FROM customer_order_ownership WHERE customer_id=?"} {
		rollback := errors.New("notice recipient rollback")
		if e := db.Transaction(func(tx *gorm.DB) error {
			if e := tx.Exec(change, owner.AccountID).Error; e != nil {
				return e
			}
			if e := customernotices.EmitOrderTx(tx, in.OrderID, "payment.received", "pay_supp_"+in.OperationID); e != nil {
				return e
			}
			var n int64
			if tx.Table("customer_account_notices").Where("event_key=?", "payment.received:pay_supp_"+in.OperationID).Count(&n).Error != nil || n != 0 {
				t.Fatal("notice granted unverified/disabled/absent ownership")
			}
			return rollback
		}); !errors.Is(e, rollback) {
			t.Fatal("notice recipient check", e)
		}
	}
	setTax := func(rate string) {
		exec("UPDATE order_line_tax_lines t JOIN order_exchange_items i ON i.item_id=t.item_id JOIN order_exchanges e ON e.id=i.exchange_id SET t.rate=?,t.raw_rate=JSON_OBJECT('value',?,'precision',100) WHERE e.order_id=?", rate, rate, in.OrderID)
	}
	setTax("29")
	if _, e = s.Consume(ctx, paid); e != orderpayments.ErrConflict {
		t.Fatal("changed exact balance consumed", e)
	}
	setTax("30")
	assertUnpaid()
	exec("CREATE TRIGGER owned_supplement_ledger_failure BEFORE INSERT ON order_transactions FOR EACH ROW BEGIN IF NEW.id='ordtr_supp_" + in.OperationID + "' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned supplement ledger failure'; END IF; END")
	if _, e = s.Consume(ctx, paid); e == nil {
		t.Fatal("late ledger failure accepted")
	}
	exec("DROP TRIGGER owned_supplement_ledger_failure")
	assertUnpaid()
	exec("CREATE TRIGGER owned_supplement_notice_failure BEFORE INSERT ON customer_account_notices FOR EACH ROW BEGIN IF NEW.kind='payment.received' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned supplement notice failure'; END IF; END")
	if _, e = s.Consume(ctx, paid); e == nil {
		t.Fatal("notice failure accepted cash")
	}
	exec("DROP TRIGGER owned_supplement_notice_failure")
	assertUnpaid()
	// Last ACK fails after cash, summary and applied have all been written.
	exec("CREATE TRIGGER owned_supplement_ack_failure BEFORE UPDATE ON mollie_notification_observations FOR EACH ROW BEGIN IF NEW.attempt_id='" + paid + "' AND NEW.consumed_at IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned supplement consumption failure'; END IF; END")
	if _, e = s.Consume(ctx, paid); e == nil {
		t.Fatal("last consumption ACK failure accepted")
	}
	exec("DROP TRIGGER owned_supplement_ack_failure")
	assertUnpaid()
	if count("mollie_notification_observations", "attempt_id=? AND consumed_at IS NULL", paid) != 1 {
		t.Fatal("paid evidence lost after rollback")
	}
	var summary string
	if db.Table("order_summaries").Select("JSON_UNQUOTE(JSON_EXTRACT(totals,'$.pending_difference'))").Where("order_id=? AND version=3", in.OrderID).Scan(&summary).Error != nil || summary != "6" {
		t.Fatal("failed cash changed summary", summary)
	}
	// Already dispatched cash survives old-generation revocation/cart retirement and a
	// canceled order. It grants no permission to alter that canceled order.
	exec("UPDATE orders SET status='canceled',canceled_at=UTC_TIMESTAMP(6) WHERE id=?", in.OrderID)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Go(func() {
			w := orderpayments.Worker{Service: s}
			if r, e := w.RunOnce(ctx); e != nil || r.Failed != 0 {
				t.Error("concurrent supplement consumption worker", r, e)
			}
		})
	}
	wg.Wait()
	if t.Failed() {
		t.Fatal("supplement consumption failed")
	}
	if channel.gets.Load() != gets || channel.posts.Load() != 2 {
		t.Fatal("local paid application called provider")
	}
	if count("payments", "payment_collection_id=?", in.CollectionID) != 1 || count("payment_captures", "id=?", "cap_supp_"+in.OperationID) != 1 || count("order_transactions", "id=?", "ordtr_supp_"+in.OperationID) != 1 || count("mollie_notification_observations", "attempt_id=? AND consumed_at IS NOT NULL", paid) != 1 {
		t.Fatal("supplement cash not unique")
	}
	var collection struct{ Status, Amount, RawCapturedAmount string }
	if db.Table("payment_collections").Select("status,CAST(captured_amount AS CHAR) AS amount,raw_captured_amount").Where("id=?", in.CollectionID).Take(&collection).Error != nil || collection.Status != "completed" || collection.Amount != "6.000000000000000000" {
		t.Fatal("supplement collection capture", collection)
	}
	if db.Table("order_summaries").Select("JSON_UNQUOTE(JSON_EXTRACT(totals,'$.pending_difference'))").Where("order_id=? AND version=3", in.OrderID).Scan(&summary).Error != nil || summary != "0" {
		t.Fatal("paid summary difference", summary)
	}
	var cash string
	if db.Table("order_summaries").Select("JSON_UNQUOTE(JSON_EXTRACT(totals,'$.transaction_total'))").Where("order_id=? AND version=3", in.OrderID).Scan(&cash).Error != nil || cash != "164.75" {
		t.Fatal("paid summary cash", cash)
	}
	var orderStatus string
	if db.Table("orders").Select("status").Where("id=?", in.OrderID).Scan(&orderStatus).Error != nil || orderStatus != "canceled" {
		t.Fatal("cash application reopened canceled order")
	}
	if count("cart_completion_receipts", "order_id=?", in.OrderID) != 1 || count("cart_payment_collections", "cart_id=?", cartID) != 1 || count("carts", "id=? AND completed_at IS NOT NULL AND deleted_at IS NOT NULL", cartID) != 1 {
		t.Fatal("cash application changed completed retired cart")
	}
	if count("customer_account_notices", "event_key=? AND customer_id=? AND resource_id=? AND read_at IS NULL", "payment.received:pay_supp_"+in.OperationID, owner.AccountID, in.OrderID) != 1 {
		t.Fatal("paid cash did not create unique owned notice")
	}
	status = "open"
	late := observe()
	if r, e := s.Consume(ctx, late); e != nil || !r.Applied || r.State != "paid" {
		t.Fatal("late unpaid observation regressed paid", r, e)
	}
	if count("payments", "payment_collection_id=?", in.CollectionID) != 1 {
		t.Fatal("late observation duplicated cash")
	}
	if count("customer_account_notices", "event_key=?", "payment.received:pay_supp_"+in.OperationID) != 1 {
		t.Fatal("late observation duplicated notice")
	}
	checkOrderPaymentNoticeCustomer(t, ctx, db, s, owner.AccountID, in)
	exec("UPDATE orders SET status='pending',canceled_at=NULL WHERE id=?", in.OrderID)
	rollback := db.Begin()
	if next, e := orders.AdvanceVersionTx(ctx, rollback, in.OrderID, 3); e != nil || next != 4 {
		_ = rollback.Rollback()
		t.Fatal("applied cash kept monetary hold", e)
	}
	if rollback.Rollback().Error != nil {
		t.Fatal("paid version rollback")
	}
	var ordersAfter int64
	if db.Table("orders").Count(&ordersAfter).Error != nil || ordersAfter != ordersBefore || stockFingerprint() != stockBefore {
		t.Fatal("cash consumption changed stock, devices or order count")
	}
	idle := orderpayments.Worker{Service: s}
	if r, e := idle.RunOnce(ctx); e != nil || r.Selected+r.Queried+r.Consumed+r.Failed != 0 {
		t.Fatal("idle supplement worker", r, e)
	}
	foreignWorker := orderpayments.Worker{Service: foreign}
	if r, e := foreignWorker.RunOnce(ctx); e != nil || r.Selected+r.Queried+r.Consumed+r.Failed != 0 {
		t.Fatal("cross-store worker picked another shop", r, e)
	}
	// Current authenticated-generation reader sees both local supplemental
	// receipts and zero exact due, including after the original cart retired.
	presentationRollback := errors.New("paid presentation rollback")
	if e := db.Transaction(func(tx *gorm.DB) error {
		var owner struct{ AccountID string }
		if e := tx.Table("order_payment_operations").Select("account_id").Where("operation_id=?", in.OperationID).Take(&owner).Error; e != nil {
			return e
		}
		if e := tx.Exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=?", owner.AccountID).Error; e != nil {
			return e
		}
		context, e := s.ContextTx(ctx, tx, orderpayments.Principal{AccountID: owner.AccountID, AuthVersion: 2}, in.OrderID)
		if e != nil || context.PendingDifference != "0" || context.Next != nil || len(context.Payments) != 3 {
			t.Fatal("paid supplemental context", context, e)
		}
		found := false
		for _, view := range context.Payments {
			if view.OperationID == in.OperationID {
				found = view.Applied && view.State == "paid" && view.CheckoutURL == nil && view.Amount == "6.00"
			}
		}
		if !found {
			t.Fatal("paid current receipt absent")
		}
		return presentationRollback
	}); !errors.Is(e, presentationRollback) {
		t.Fatal(e)
	}
	t.Log(`ORDER_PAYMENT_PRESENTATION_REHEARSAL {"exact_zero_due_after_cash":true,"paid_receipt_only_after_application":true,"retired_cart_independent":true,"paid_checkout_url_absent":true,"new_generation_read":true,"provider_called_by_presentation":false,"customer_browser_cli":false}`)
	t.Log(`NATIVE_ORDER_PAYMENT_CONSUMPTION_REHEARSAL {"actual_notification_get_and_encrypted_evidence":true,"paid_due":"6.00","exact_total_cash":"164.75","authorized_not_cash":true,"open_does_not_regress_authorized":true,"cross_store_denied":true,"corrupt_envelope_denied":true,"changed_balance_retains_paid_evidence":true,"late_ledger_failure_atomic":true,"last_consumption_failure_atomic":true,"concurrent_consumers":6,"worker_run_once_accepted":true,"worker_cli_process_accepted":false,"idle_cross_store_worker":true,"unique_payment_capture_transaction":true,"collection_completed_session_captured":true,"summary_zero_difference":true,"local_retry_without_network":true,"revoked_account_retired_cart_recovery":true,"canceled_order_cash_without_reopening":true,"late_unpaid_does_not_regress_cash":true,"applied_releases_version_hold":true,"original_cart_preserved":true,"no_new_order_stock_or_device_mutation":true,"real_provider":false,"customer_session_browser_cli":false,"payment_notice_accepted":true}`)
}
