package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/refunds"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Runs in the rollback-only positive native exchange. Account/ownership facts
// are explicit artificial SQL fixtures, not a browser or a verified session.
func checkSupplementalPaymentCommands(t *testing.T, ctx context.Context, tx *gorm.DB, bound orders.SupplementalPayment, o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := tx.Exec(q, args...).Error; e != nil {
			t.Fatal("order payment fixture", e)
		}
	}
	count := func(table, where string, args ...any) int64 {
		t.Helper()
		var n int64
		if tx.Table(table).Where(where, args...).Count(&n).Error != nil {
			t.Fatal("order payment count", table)
		}
		return n
	}
	exec(`INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,auth_version,verified_at,created_at,updated_at) VALUES('order-payment-account','order-payment@example.invalid','artificial-unusable-hash','Payment','Fixture',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('order-payment-other','order-payment-other@example.invalid','artificial-unusable-hash','Other','Fixture',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	var original struct{ CartID string }
	if tx.Table("cart_completion_receipts").Select("cart_id").Where("order_id=?", bound.OrderID).Take(&original).Error != nil {
		t.Fatal("order payment completed trace")
	}
	exec("INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) VALUES(?,'order-payment-account',?,'shop',UTC_TIMESTAMP(6))", bound.OrderID, original.CartID)
	p := orderpayments.Principal{AccountID: "order-payment-account", AuthVersion: 1}
	s := orderpayments.Service{DB: tx, StoreID: "shop", ReturnOrigin: o.ReturnOrigin, Ring: o.Ring, Adapters: o.Adapters}
	in := orderpayments.Input{OperationID: uuid.NewString(), OrderID: bound.OrderID, ExpectedVersion: bound.Version, CollectionID: bound.CollectionID, Amount: bound.Amount, CurrencyCode: bound.CurrencyCode}
	if _, e := s.Prepare(ctx, p, in); e != orderpayments.ErrUnavailable {
		t.Fatal("order payment default on", e)
	}
	exec("UPDATE order_payment_runtime SET enabled=TRUE WHERE id=1")
	// Failure after the protected effect and session insert rolls both back.
	exec("SET @owned_order_payment_insert_failure=1")
	_, failed := s.Prepare(ctx, p, in)
	exec("SET @owned_order_payment_insert_failure=NULL")
	if failed != orderpayments.ErrUnavailable || count("order_payment_operations", "operation_id=?", in.OperationID) != 0 || count("payment_effect_journal", "scope_id=? AND operation_id=?", bound.OrderID, in.OperationID) != 0 || count("payment_sessions", "payment_collection_id=?", bound.CollectionID) != 0 {
		t.Fatal("late stage failure leaked protected effect or session", failed)
	}
	callerRollback := errors.New("owned caller rollback")
	if e := tx.Transaction(func(nested *gorm.DB) error {
		if _, e := s.PrepareTx(ctx, nested, p, in); e != nil {
			t.Fatal("order payment participant", e)
		}
		return callerRollback
	}); !errors.Is(e, callerRollback) {
		t.Fatal("order payment caller rollback", e)
	}
	if count("order_payment_operations", "operation_id=?", in.OperationID) != 0 || count("payment_effect_journal", "operation_id=?", in.OperationID) != 0 {
		t.Fatal("order payment participant committed caller work")
	}
	prepared, e := s.Prepare(ctx, p, in)
	if e != nil || prepared.State != "pending" || prepared.Applied || !prepared.CanCancel || prepared.CanRetry || prepared.Amount != "6.00" {
		t.Fatal("order payment prepare", prepared, e)
	}
	checkOrderPaymentRefundExclusion(t, ctx, tx, s, p, in, o, true)
	for i := 0; i < 6; i++ {
		replay, e := s.Prepare(ctx, p, in)
		if e != nil || replay.OperationID != prepared.OperationID {
			t.Fatal("order payment immutable replay", e)
		}
	}
	if count("order_payment_operations", "collection_id=?", bound.CollectionID) != 1 || count("payment_sessions", "payment_collection_id=?", bound.CollectionID) != 1 || count("payment_effect_journal", "scope_id=? AND operation_id=?", bound.OrderID, in.OperationID) != 1 {
		t.Fatal("order payment replay duplicated facts")
	}
	var saved struct {
		SessionID     string
		RequestSHA256 []byte
	}
	if tx.Table("order_payment_operations").Select("session_id,request_sha256").Where("operation_id=?", in.OperationID).Take(&saved).Error != nil {
		t.Fatal("order payment saved binding")
	}
	k := payments.EffectKey{ScopeID: bound.OrderID, OperationID: in.OperationID, Kind: "create_session", ResourceID: saved.SessionID}
	copy(k.Digest[:], saved.RequestSHA256)
	req, e := payments.ReadSessionRequest(ctx, tx, k, o.Ring)
	if e != nil || req.CollectionID != bound.CollectionID || req.SessionID != saved.SessionID || req.Amount != "6" || req.Currency != "eur" {
		t.Fatal("order payment protected request", e)
	}
	var data map[string]string
	if json.Unmarshal(req.Data, &data) != nil {
		t.Fatal("order payment protected input")
	}
	redirect, e := url.Parse(data["redirect_url"])
	if e != nil || redirect.Scheme+"://"+redirect.Host != o.ReturnOrigin || redirect.Path != "/api/mollie-return" || redirect.Query().Get("cart_id") != original.CartID || redirect.Query().Get("order_id") != bound.OrderID || redirect.Query().Get("order_payment_id") != in.OperationID || data["cart_id"] != original.CartID || data["session_id"] != saved.SessionID {
		t.Fatal("order payment return binding")
	}
	var session struct {
		Data    string
		Context *string
	}
	if tx.Table("payment_sessions").Select("data,context").Where("id=?", saved.SessionID).Take(&session).Error != nil || !strings.HasPrefix(session.Data, "ers1.") || session.Context == nil || !strings.HasPrefix(*session.Context, "ers1.") {
		t.Fatal("order payment plaintext session")
	}
	if _, e = payments.OpenSessionFields(o.Ring, req, session.Data, session.Context); e != nil {
		t.Fatal("order payment session envelope", e)
	}
	for _, change := range []func(*orderpayments.Input){func(i *orderpayments.Input) { i.Amount = "5.99" }, func(i *orderpayments.Input) { i.CurrencyCode = "dkk" }, func(i *orderpayments.Input) { i.ExpectedVersion = 1 }, func(i *orderpayments.Input) { i.CollectionID = "paycol_foreign" }} {
		bad := in
		change(&bad)
		if _, e := s.Prepare(ctx, p, bad); e != orderpayments.ErrConflict {
			t.Fatal("mutable payment instruction accepted", e)
		}
	}
	other := orderpayments.Principal{AccountID: "order-payment-other", AuthVersion: 1}
	if _, e = s.Prepare(ctx, other, in); e != orderpayments.ErrDenied {
		t.Fatal("other account payment prepare", e)
	}
	if _, e = s.Read(ctx, other, in.OrderID, in.OperationID); e != orderpayments.ErrDenied {
		t.Fatal("other account payment read", e)
	}
	foreign := s
	foreign.StoreID = "other-shop"
	if _, e = foreign.Prepare(ctx, p, in); e != orderpayments.ErrDenied {
		t.Fatal("other store payment prepare", e)
	}
	fresh := in
	fresh.OperationID = uuid.NewString()
	if _, e = s.Prepare(ctx, p, fresh); e != orderpayments.ErrConflict {
		t.Fatal("second active order payment", e)
	}
	if _, e = orders.AdvanceVersionTx(ctx, tx, bound.OrderID, 3); e != orders.ErrVersionConflict {
		t.Fatal("pending payment changed amount version", e)
	}
	for _, q := range []string{"UPDATE customer_accounts SET enabled=FALSE WHERE id='order-payment-account'", "UPDATE customer_accounts SET verified_at=NULL WHERE id='order-payment-account'", "UPDATE customer_accounts SET auth_version=2 WHERE id='order-payment-account'"} {
		if tx.SavePoint("payment_authority").Error != nil {
			t.Fatal("authority savepoint")
		}
		exec(q)
		if _, e = s.Prepare(ctx, p, in); e != orderpayments.ErrDenied {
			t.Fatal("revoked payment replay", e)
		}
		if tx.RollbackTo("payment_authority").Error != nil {
			t.Fatal("authority rollback")
		}
	}
	// Current account authority may read/cancel an old unsent instruction after
	// revocation. It cannot reuse its old UUID with the changed auth generation.
	exec("UPDATE customer_accounts SET auth_version=2 WHERE id=?", p.AccountID)
	p.AuthVersion = 2
	if _, e = s.Prepare(ctx, p, in); e != orderpayments.ErrConflict {
		t.Fatal("changed generation rewrote payment", e)
	}
	if _, e = s.Read(ctx, p, in.OrderID, in.OperationID); e != nil {
		t.Fatal("current owner cannot read old payment", e)
	}
	cancel := orderpayments.CancelInput{OperationID: uuid.NewString(), PaymentOperationID: in.OperationID}
	if tx.SavePoint("payment_claim_fence").Error != nil {
		t.Fatal("claim savepoint")
	}
	claim, _, e := payments.ClaimSessionRequest(ctx, tx, k, o.Ring, 30_000_000_000)
	if e != nil || !claim.Dispatch {
		t.Fatal("order payment effect claim", e)
	}
	if _, e = s.Cancel(ctx, p, in.OrderID, cancel); e != orderpayments.ErrConflict {
		t.Fatal("claimed effect locally canceled", e)
	}
	if tx.RollbackTo("payment_claim_fence").Error != nil {
		t.Fatal("claim rollback")
	}
	exec("SET @owned_order_payment_cancel_failure=1")
	_, failed = s.Cancel(ctx, p, in.OrderID, cancel)
	exec("SET @owned_order_payment_cancel_failure=NULL")
	if failed != orderpayments.ErrUnavailable || count("order_payment_operations", "operation_id=? AND state='pending'", in.OperationID) != 1 || count("payment_sessions", "id=? AND status='pending'", saved.SessionID) != 1 || count("order_payment_cancellations", "operation_id=?", cancel.OperationID) != 0 {
		t.Fatal("late cancellation released hold", failed)
	}
	exec("UPDATE order_payment_runtime SET enabled=FALSE WHERE id=1")
	if _, e = orders.AdvanceVersionTx(ctx, tx, bound.OrderID, 3); e != orders.ErrVersionConflict {
		t.Fatal("disabled feature released pending hold", e)
	}
	for i := 0; i < 2; i++ {
		canceled, e := s.Cancel(ctx, p, in.OrderID, cancel)
		if e != nil || canceled.State != "canceled" || !canceled.CanRetry || canceled.CanCancel {
			t.Fatal("local payment cancellation replay", canceled, e)
		}
	}
	if count("order_payment_cancellations", "payment_operation_id=?", in.OperationID) != 1 || count("payment_sessions", "id=? AND status='canceled'", saved.SessionID) != 1 {
		t.Fatal("cancellation facts missing")
	}
	if tx.SavePoint("payment_version_release").Error != nil {
		t.Fatal("version savepoint")
	}
	if next, e := orders.AdvanceVersionTx(ctx, tx, bound.OrderID, 3); e != nil || next != 4 {
		t.Fatal("canceled payment kept version frozen", e)
	}
	if tx.RollbackTo("payment_version_release").Error != nil {
		t.Fatal("version rollback")
	}
	// Long-term new-account ownership survives retirement of the old cart
	// capability and soft-deleted historical cart links. No cart token is used.
	if tx.SavePoint("payment_without_cart_capability").Error != nil {
		t.Fatal("cart capability savepoint")
	}
	exec("UPDATE order_payment_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_ownership SET active=FALSE,generation=?,checkout_attempt_id=NULL WHERE cart_id=?", strings.Repeat("0", 64), original.CartID)
	exec("UPDATE carts SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", original.CartID)
	exec("UPDATE order_carts SET deleted_at=UTC_TIMESTAMP(6) WHERE order_id=?", in.OrderID)
	withoutCart := in
	withoutCart.OperationID = uuid.NewString()
	withoutCart.PreviousOperationID = in.OperationID
	if _, e = s.Prepare(ctx, p, withoutCart); e != nil {
		t.Fatal("attached owner depends on old cart capability", e)
	}
	if _, e = s.Read(ctx, p, in.OrderID, withoutCart.OperationID); e != nil {
		t.Fatal("attached owner cannot read without cart capability", e)
	}
	if _, e = s.Cancel(ctx, p, in.OrderID, orderpayments.CancelInput{OperationID: uuid.NewString(), PaymentOperationID: withoutCart.OperationID}); e != nil {
		t.Fatal("attached owner cannot cancel without cart capability", e)
	}
	if tx.RollbackTo("payment_without_cart_capability").Error != nil {
		t.Fatal("cart capability rollback")
	}
	exec("UPDATE order_payment_runtime SET enabled=TRUE WHERE id=1")
	if _, e = s.Prepare(ctx, p, fresh); e != orderpayments.ErrConflict {
		t.Fatal("implicit payment successor", e)
	}
	checkOrderPaymentRefundExclusion(t, ctx, tx, s, p, in, o, false)
	fresh.PreviousOperationID = in.OperationID
	second, e := s.Prepare(ctx, p, fresh)
	if e != nil || second.PreviousOperationID == nil || *second.PreviousOperationID != in.OperationID {
		t.Fatal("explicit payment successor", second, e)
	}
	if _, e = s.Cancel(ctx, p, in.OrderID, orderpayments.CancelInput{OperationID: uuid.NewString(), PaymentOperationID: fresh.OperationID}); e != nil {
		t.Fatal("successor cancellation", e)
	}
	branch := fresh
	branch.OperationID = uuid.NewString()
	if _, e = s.Prepare(ctx, p, branch); e != orderpayments.ErrConflict {
		t.Fatal("payment successor branch", e)
	}
	branch.PreviousOperationID = fresh.OperationID
	if _, e = s.Prepare(ctx, p, branch); e != nil {
		t.Fatal("next explicit payment successor", e)
	}
	if _, e = s.Cancel(ctx, p, in.OrderID, orderpayments.CancelInput{OperationID: uuid.NewString(), PaymentOperationID: branch.OperationID}); e != nil {
		t.Fatal("last payment cancellation", e)
	}
	if count("cart_payment_collections", "cart_id=? AND deleted_at IS NULL", original.CartID) != 1 || count("carts", "id=? AND completed_at IS NOT NULL", original.CartID) != 1 || count("payments p JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id", "l.order_id=? AND p.deleted_at IS NULL", bound.OrderID) != 1 {
		t.Fatal("supplemental staging reopened cart or created cash")
	}
	t.Log(`NATIVE_ORDER_PAYMENT_COMMANDS_REHEARSAL {"migration":87,"default_disabled":true,"actual_completed_checkout":true,"native_positive_exchange":true,"due":"6.00","protected_request_and_session":true,"order_scope_not_cart_scope":true,"immutable_uuid_replay":true,"current_account_and_explicit_ownership":true,"revoked_generation_replay_denied":true,"one_active_instruction":true,"payment_refund_mutual_exclusion":true,"unapplied_refund_blocks_payment":true,"explicit_successor_chain":true,"local_cancel_even_disabled":true,"late_stage_failure_atomic":true,"late_cancel_failure_atomic":true,"caller_rollback_preserved":true,"claimed_effect_cancel_denied":true,"pending_version_change_denied":true,"disabled_pending_version_change_denied":true,"cancel_releases_version":true,"completed_cart_preserved":true,"independent_retired_cart_capability":true,"artificial_account_ownership":true,"real_customer_session":false,"concurrency_tested":false,"provider_dispatch":false,"cash_created":false,"browser_cli_acceptance":false}`)
}

func checkOrderPaymentRefundExclusion(t *testing.T, ctx context.Context, tx *gorm.DB, s orderpayments.Service, p orderpayments.Principal, in orderpayments.Input, o cart.InitialPaymentOptions, paymentPending bool) {
	t.Helper()
	if tx.SavePoint("payment_refund_exclusion").Error != nil {
		t.Fatal("payment refund savepoint")
	}
	defer func() {
		if tx.RollbackTo("payment_refund_exclusion").Error != nil {
			t.Fatal("payment refund rollback")
		}
	}()
	var trace struct{ RequestID, PaymentID string }
	if tx.Table("order_exchanges e").Select("JSON_UNQUOTE(JSON_EXTRACT(e.metadata,'$.after_sales_request_id')) AS request_id,r.payment_id").Joins("JOIN cart_completion_receipts r ON r.order_id=e.order_id").Where("e.order_id=? AND e.deleted_at IS NULL", in.OrderID).Take(&trace).Error != nil {
		t.Fatal("payment refund trace")
	}
	var actor identity.User
	if tx.Where("id='refund-command-actor'").Take(&actor).Error != nil {
		t.Fatal("payment refund actor")
	}
	channel := &refundChannel{db: tx, t: t, status: "pending"}
	adapters, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": channel})
	if e != nil {
		t.Fatal(e)
	}
	refund := refunds.Service{DB: tx, StoreID: s.StoreID, Ring: o.Ring, Adapters: adapters}
	setTax := func(rate string) {
		t.Helper()
		if tx.Exec("UPDATE order_line_tax_lines t JOIN order_exchange_items i ON i.item_id=t.item_id JOIN order_exchanges e ON e.id=i.exchange_id SET t.rate=?,t.raw_rate=JSON_OBJECT('value',?,'precision',100) WHERE e.order_id=? AND t.deleted_at IS NULL", rate, rate, in.OrderID).Error != nil {
			t.Fatal("payment refund tax fixture")
		}
	}
	setTax("15")
	refundInput := refunds.Input{OperationID: uuid.NewString(), OrderID: in.OrderID, RequestID: trace.RequestID, PaymentID: trace.PaymentID, Amount: "12.00"}
	if paymentPending {
		if _, e := refund.Prepare(ctx, actor, refundInput); e != refunds.ErrConflict {
			t.Fatal("pending payment permitted competing refund", e)
		}
	} else {
		if _, e := refund.Prepare(ctx, actor, refundInput); e != nil {
			t.Fatal("negative credit fixture prepare", e)
		}
		setTax("30")
		next := in
		next.OperationID = uuid.NewString()
		next.PreviousOperationID = in.OperationID
		if _, e := s.Prepare(ctx, p, next); e != orderpayments.ErrConflict {
			t.Fatal("pending refund permitted competing payment", e)
		}
		if tx.Exec("UPDATE order_refund_operations SET state='refunded',dispatched_at=UTC_TIMESTAMP(6) WHERE operation_id=?", refundInput.OperationID).Error != nil {
			t.Fatal("unapplied refund fixture")
		}
		if _, e := s.Prepare(ctx, p, next); e != orderpayments.ErrConflict {
			t.Fatal("unapplied refunded observation permitted competing payment", e)
		}

	}
	if channel.posts.Load() != 0 {
		t.Fatal("payment refund exclusion dispatched provider")
	}
}
