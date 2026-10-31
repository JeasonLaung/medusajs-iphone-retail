package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/refunds"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type refundChannel struct {
	posts  atomic.Int32
	gets   atomic.Int32
	lost   atomic.Bool
	status string
	db     *gorm.DB
	t      *testing.T
}

func (a *refundChannel) CreateSession(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	return payments.CreateUncertain, payments.CreatedSession{}, payments.ErrCreateExecution
}
func (a *refundChannel) observation(in payments.RefundRequest) payments.RefundObservation {
	var committed int64
	if a.db.Table("order_refund_operations").Where("operation_id=? AND dispatched_at IS NOT NULL", in.OperationID).Count(&committed).Error != nil || committed != 1 {
		a.t.Error("refund before durable claim")
	}
	return payments.RefundObservation{RefundID: "re_" + in.OperationID, PaymentID: in.RemotePaymentID, Status: a.status, Amount: in.Amount, Currency: in.Session.Currency, CreatedAt: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}
}
func (a *refundChannel) CreateRefund(_ context.Context, in payments.RefundRequest) (payments.RefundObservation, error) {
	a.posts.Add(1)
	v := a.observation(in)
	if a.lost.Load() {
		return payments.RefundObservation{}, payments.ErrRefundExecution
	}
	return v, nil
}
func (a *refundChannel) ObserveRefund(_ context.Context, in payments.RefundRequest, _ string) (payments.RefundObservation, error) {
	a.gets.Add(1)
	return a.observation(in), nil
}
func (a *refundChannel) DiscoverRefund(ctx context.Context, in payments.RefundRequest) (payments.RefundObservation, error) {
	return a.ObserveRefund(ctx, in, "")
}

// Uses an actual completed native checkout and encrypted original effect. Only
// original device/return facts and channel responses are artificial SQL fixtures;
// this does not claim a customer browser return or an external Mollie refund.
func checkNativeRefundCommands(t *testing.T, ctx context.Context, db *gorm.DB, order string, o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("refund fixture", e)
		}
	}
	hash, e := bcrypt.GenerateFromPassword([]byte("artificial-refund-password"), bcrypt.DefaultCost)
	if e != nil {
		t.Fatal(e)
	}
	actor := identity.User{ID: "refund-command-actor", Email: "refund-commands@example.invalid", Name: "Refund", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: string(hash), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if db.Create(&actor).Error != nil {
		t.Fatal("refund actor")
	}
	if db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, actor.ID) }) != nil {
		t.Fatal("refund grants")
	}
	var receipt struct{ PaymentID string }
	if db.Table("cart_completion_receipts").Where("order_id=?", order).Take(&receipt).Error != nil {
		t.Fatal("refund original checkout")
	}
	var line struct{ ItemID string }
	if db.Table("order_items").Where("order_id=? AND version=1", order).Take(&line).Error != nil {
		t.Fatal("refund original line")
	}
	// Device tables preserve legacy identifiers without importing old login subjects.
	exec("INSERT INTO devices(id,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('refund-device','add-v','refund-source','refund-location',0,'eur','','','inspection','returned','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	makeRequest := func(id string) {
		wire, _ := json.Marshal(map[string]any{"lines": []any{map[string]any{"item_id": line.ItemID, "quantity": 1, "device_ids": []string{"refund-device"}}}})
		exec("INSERT INTO after_sales_requests(id,order_id,customer_id,type,reason,items,status,created_at,updated_at) VALUES(?,?,'historical-subject','return','Fixture return',?,'processing',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, order, string(wire))
		exec("UPDATE device_assignments SET after_sales_request_id=? WHERE id='refund-assignment'", id)
	}
	makeRequest("refund-return")
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,after_sales_request_id,created_at,updated_at) VALUES('refund-assignment','refund-device',?,?,'refund-source','returned','refund-return',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", order, line.ItemID)
	channel := &refundChannel{db: db, t: t, status: "pending"}
	registry, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": channel})
	if e != nil {
		t.Fatal(e)
	}
	s := refunds.Service{StoreID: "shop", DB: db, Ring: o.Ring, Adapters: registry}
	httpFixture := newRefundHTTPFixture(t, ctx, s, actor)
	path := "/api/admin/orders/" + order + "/after-sales/refund-return/refunds"
	httpFixture.call("GET", path, "", false, false, 401)
	httpFixture.call("GET", path, "", true, false, 200)
	httpFixture.call("POST", path, `{}`, true, false, 403)
	httpFixture.call("POST", path, `{}`, true, true, 400)
	in := refunds.Input{OperationID: uuid.NewString(), OrderID: order, RequestID: "refund-return", PaymentID: receipt.PaymentID, Amount: "100.01"}
	if _, e := s.Prepare(ctx, actor, in); e != refunds.ErrUnavailable {
		t.Fatal("refund default off", e)
	}
	exec("UPDATE order_refund_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE order_refund_runtime SET enabled=FALSE WHERE id=1")
	checkExchangeRefundCredit(t, ctx, db, s, actor, in, channel)
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Prepare(ctx, actor, in); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("refund prepare replay", e)
		}
	}
	wire := func(input refunds.Input) string {
		b, _ := json.Marshal(map[string]string{"operation_id": input.OperationID, "payment_id": input.PaymentID, "amount": input.Amount})
		return string(b)
	}
	httpFixture.call("POST", path, wire(in), true, true, 202)
	for _, bad := range []string{`{"operation_id":"x","operation_id":"y","payment_id":"p","amount":"1"}`, `{"operation_id":"x","payment_id":"p","amount":1}`, `{"operation_id":"x","payment_id":"p","amount":"1","remote_payment_id":"tr_other"}`, `{"operation_id":"x","payment_id":"p","amount":null}`, `{} {}`} {
		httpFixture.call("POST", path, bad, true, true, 400)
	}
	httpFixture.call("GET", path+"?unknown=1", "", true, false, 400)
	var count int64
	db.Table("order_refund_operations").Where("payment_id=?", in.PaymentID).Count(&count)
	if count != 1 || channel.posts.Load() != 0 {
		t.Fatal("prepare not unique/offline")
	}
	exec("UPDATE after_sales_review_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE after_sales_review_runtime SET enabled=FALSE WHERE id=1")
	var review customer.ReviewContext
	if e := db.Transaction(func(tx *gorm.DB) error {
		var err error
		review, err = customer.ReviewAfterSalesTx(ctx, tx, actor, order, nil)
		return err
	}); e != nil {
		t.Fatal("refund review read", e)
	}
	for _, status := range []string{"rejected", "completed"} {
		command := customer.ReviewInput{OperationID: uuid.NewString(), ExpectedRevision: review.Revision, RequestID: in.RequestID, Status: status}
		if e := db.Transaction(func(tx *gorm.DB) error {
			_, err := customer.ReviewAfterSalesTx(ctx, tx, actor, order, &command)
			return err
		}); e != customer.ErrAfterSalesConflict {
			t.Fatal("unfinished refund request closed", status, e)
		}
	}
	changed := in
	changed.Amount = "100.02"
	if _, e := s.Prepare(ctx, actor, changed); e != refunds.ErrConflict {
		t.Fatal("mutable amount", e)
	}
	changed = in
	changed.OperationID = uuid.NewString()
	if _, e := s.Prepare(ctx, actor, changed); e != refunds.ErrConflict {
		t.Fatal("second UUID same return", e)
	}
	channel.lost.Store(true)
	if _, e := s.Run(ctx, in.OperationID); e != payments.ErrRefundExecution || channel.posts.Load() != 1 {
		t.Fatal("lost refund response", e)
	}
	if _, e := s.Run(ctx, in.OperationID); e != nil || channel.posts.Load() != 1 || channel.gets.Load() != 0 {
		t.Fatal("live lease re-dispatched", e)
	}
	if _, e := s.Cancel(ctx, actor, order, in.RequestID, refunds.CancelInput{OperationID: uuid.NewString(), RefundOperationID: in.OperationID}); e != refunds.ErrConflict {
		t.Fatal("dispatched refund canceled", e)
	}
	// Revoked principals cannot replay or create, but internal recovery remains possible.
	exec("DELETE FROM authorization_role_actions WHERE role_id IN (SELECT role_id FROM authorization_memberships WHERE user_id=?) AND action_code='order.refund.write'", actor.ID)
	if _, e := s.Prepare(ctx, actor, in); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("revoked refund replay", e)
	}
	exec("UPDATE order_refund_operations SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE operation_id=?", in.OperationID)
	channel.lost.Store(false)
	channel.status = "processing"
	if r, e := s.Run(ctx, in.OperationID); e != nil || r.State != "processing" || r.Applied || channel.posts.Load() != 1 || channel.gets.Load() != 1 {
		t.Fatal("GET-only discovery", r, e)
	}
	db.Table("payment_refunds").Where("payment_id=?", in.PaymentID).Count(&count)
	if count != 0 {
		t.Fatal("processing booked money")
	}
	exec("UPDATE order_refund_operations SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE operation_id=?", in.OperationID)
	channel.status = "refunded"
	exec("CREATE TRIGGER refund_ledger_fault BEFORE INSERT ON order_transactions FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial refund ledger failure'")
	if _, e := s.Run(ctx, in.OperationID); e == nil {
		t.Fatal("late refund ledger failure accepted")
	}
	exec("DROP TRIGGER refund_ledger_fault")
	db.Table("payment_refunds").Where("payment_id=?", in.PaymentID).Count(&count)
	if count != 0 {
		t.Fatal("partial ledger survived rollback")
	}
	gets := channel.gets.Load()
	for i := 0; i < 3; i++ {
		r, e := s.Run(ctx, in.OperationID)
		if e != nil || !r.Applied || r.State != "refunded" {
			t.Fatal("saved observation not applied", r, e)
		}
	}
	if channel.posts.Load() != 1 || channel.gets.Load() != gets {
		t.Fatal("local ledger retry touched provider")
	}
	exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT role_id,'order.refund.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	exec("UPDATE after_sales_requests SET status='completed' WHERE id='refund-return'")
	exec("UPDATE device_assignments SET status='shipped' WHERE id='refund-assignment'")
	makeRequest("refund-return-two")
	second := in
	second.OperationID = uuid.NewString()
	second.RequestID = "refund-return-two"
	second.Amount = "58.75"
	if _, e := s.Prepare(ctx, actor, second); e != refunds.ErrConflict {
		t.Fatal("unreceived device", e)
	}
	exec("UPDATE device_assignments SET status='returned' WHERE id='refund-assignment'")
	if _, e := s.Prepare(ctx, actor, second); e != refunds.ErrConflict {
		t.Fatal("unsettled reservation exceeded original 158.75", e)
	}
	second.Amount = "58.74"
	if _, e := s.Prepare(ctx, actor, second); e != nil {
		t.Fatal("second partial remaining", e)
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id IN (SELECT role_id FROM authorization_memberships WHERE user_id=?) AND action_code='order.refund.write'", actor.ID)
	if _, e := s.Run(ctx, second.OperationID); !errors.Is(e, authorization.ErrDenied) || channel.posts.Load() != 1 {
		t.Fatal("revoked actor dispatched pending refund", e)
	}
	exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT role_id,'order.refund.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	// Cancel must be atomic and must never contact the provider.
	secondPath := "/api/admin/orders/" + order + "/after-sales/" + second.RequestID + "/refunds"
	cancellation := refunds.CancelInput{OperationID: uuid.NewString(), RefundOperationID: second.OperationID}
	cancelPath := secondPath + "/" + second.OperationID + "/cancel"
	cancelWire := `{"operation_id":"` + cancellation.OperationID + `"}`
	exec("CREATE TRIGGER refund_cancel_fault BEFORE INSERT ON order_refund_cancellations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial cancel receipt failure'")
	httpFixture.call("POST", cancelPath, cancelWire, true, true, 503)
	exec("DROP TRIGGER refund_cancel_fault")
	var canceledCount int64
	db.Table("order_refund_operations").Where("operation_id=? AND canceled_at IS NOT NULL", second.OperationID).Count(&canceledCount)
	if canceledCount != 0 {
		t.Fatal("cancel did not roll back")
	}
	httpFixture.call("POST", cancelPath, cancelWire, true, true, 200)
	httpFixture.call("POST", cancelPath, cancelWire, true, true, 200)
	if r, e := s.Run(ctx, second.OperationID); e != nil || r.State != "canceled" || channel.posts.Load() != 1 {
		t.Fatal("locally canceled command dispatched", r, e)
	}
	// All new attempts need an explicit predecessor; old commands never re-dispatch.
	next := second
	next.OperationID = uuid.NewString()
	httpFixture.call("POST", secondPath, wire(next), true, true, 409)
	next.PreviousOperationID = second.OperationID
	attemptWire := func(input refunds.Input) string {
		b, _ := json.Marshal(map[string]string{"operation_id": input.OperationID, "payment_id": input.PaymentID, "amount": input.Amount, "previous_operation_id": input.PreviousOperationID})
		return string(b)
	}
	httpFixture.call("POST", secondPath, attemptWire(next), true, true, 202)
	channel.status = "failed"
	work, _, e := s.RunDue(ctx, "")
	if e != nil || work.Selected != 1 || work.Processed != 1 {
		t.Fatal("failed refund worker", work, e)
	}
	if r, e := s.Run(ctx, next.OperationID); e != nil || r.State != "failed" || r.Applied {
		t.Fatal("failed command replay", r, e)
	}
	successor := next
	successor.OperationID = uuid.NewString()
	successor.PreviousOperationID = second.OperationID
	httpFixture.call("POST", secondPath, attemptWire(successor), true, true, 409)
	successor.PreviousOperationID = next.OperationID
	httpFixture.call("POST", secondPath, attemptWire(successor), true, true, 202)
	channel.status = "canceled"
	work, _, e = s.RunDue(ctx, "")
	if e != nil || work.Selected != 1 || work.Processed != 1 {
		t.Fatal("provider canceled worker", work, e)
	}
	final := successor
	final.OperationID = uuid.NewString()
	final.PreviousOperationID = successor.OperationID
	httpFixture.call("POST", secondPath, attemptWire(final), true, true, 202)
	channel.status = "refunded"
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fresh := refunds.Service{StoreID: s.StoreID, DB: db, Ring: s.Ring, Adapters: s.Adapters}
			_, _, e := fresh.RunDue(ctx, "")
			if e != nil {
				t.Error("concurrent refund worker", e)
			}
		}()
	}
	wg.Wait()
	if r, e := s.Run(ctx, final.OperationID); e != nil || !r.Applied {
		t.Fatal("explicit final attempt", r, e)
	}
	work, _, e = s.RunDue(ctx, "")
	if e != nil || work.Selected != 0 {
		t.Fatal("idle refund worker", work, e)
	}
	foreign := s
	foreign.StoreID = "other-shop"
	if _, e := foreign.Read(ctx, actor, order, second.RequestID); e != refunds.ErrConflict {
		t.Fatal("cross-store refund read", e)
	}
	var view refunds.Context
	if json.Unmarshal(httpFixture.call("GET", secondPath, "", true, false, 200), &view) != nil || len(view.Operations) != 4 {
		t.Fatal("refund attempt history")
	}
	db.Table("payment_refunds").Where("payment_id=?", in.PaymentID).Count(&count)
	if count != 2 {
		t.Fatal("refund movement uniqueness")
	}
	var totals struct{ RefundedAmount, RawRefundedAmount string }
	if db.Table("payment_collections").Select("CAST(refunded_amount AS CHAR) AS refunded_amount,raw_refunded_amount").Where("id=(SELECT payment_collection_id FROM payments WHERE id=?)", in.PaymentID).Take(&totals).Error != nil || totals.RefundedAmount != "158.750000000000000000" {
		t.Fatal("exact collection refunded total", totals)
	}
	var negative string
	if db.Table("order_transactions").Select("CAST(SUM(amount) AS CHAR)").Where("order_id=? AND reference='refund'", order).Scan(&negative).Error != nil || negative != "-158.750000000000000000" {
		t.Fatal("exact negative ledger", negative)
	}
	if channel.posts.Load() != 4 {
		t.Fatal("refund command POST count")
	}
	t.Log(`NATIVE_REFUND_COMMAND_REHEARSAL {"real_mysql":true,"actual_completed_checkout":true,"explicit_default_off":true,"concurrent_prepare":6,"single_instruction_per_return":true,"immutable_amount":true,"unreceived_denied":true,"remaining_balance_enforced":true,"pending_review_close_denied":true,"two_partial_refunds":true,"lost_response_get_only":true,"late_ledger_failure_atomic":true,"saved_observation_apply_without_network":true,"revoked_actor_recovery":true,"revoked_first_dispatch_denied":true,"exact_refunded":"158.75","provider_posts":4,"explicit_failed_canceled_successors":true,"undispatched_cancel_atomic":true,"cancel_after_dispatch_denied":true,"http_real_auth_csrf":true,"worker_concurrency":6,"real_provider_refund":false,"browser_return_flow":false}`)
}
