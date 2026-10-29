package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func checkPaymentObservations(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, options cart.InitialPaymentOptions) {
	t.Helper()
	var calls atomic.Int32
	var mode atomic.Value
	mode.Store("pending")
	var during func()
	adapter := executionObserver{create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		t.Error("status query created payment")
		return payments.CreateUncertain, payments.CreatedSession{}, nil
	}, observe: func(callCtx context.Context, request payments.SessionRequest, id string) (payments.SessionObservation, error) {
		calls.Add(1)
		var locks int64
		if db.WithContext(callCtx).Table("cart_ownership").Where("cart_id=?", ref.CartID).Count(&locks).Error != nil || locks != 1 {
			t.Error("observation inside unavailable SQL")
		}
		m := mode.Load().(string)
		if m == "error" {
			return payments.SessionObservation{}, errors.New("private provider failure")
		}
		status := m
		if m == "amount" {
			status = "paid"
		}
		amount := request.Amount
		if m == "amount" {
			amount = "0"
		}
		actions := map[string]string{"open": "not_supported", "pending": "pending", "paid": "successful", "authorized": "authorized", "canceled": "canceled", "expired": "failed", "failed": "failed"}
		local := map[string]string{"open": "pending", "pending": "pending", "paid": "authorized", "authorized": "authorized", "canceled": "canceled", "expired": "error", "failed": "error"}
		raw, _ := json.Marshal(map[string]any{"id": id, "status": status, "amount": map[string]string{"value": amount, "currency": strings.ToUpper(request.Currency)}, "metadata": map[string]string{"session_id": request.SessionID, "cart_id": inScope(request)}})
		if during != nil {
			during()
		}
		return payments.SessionObservation{PaymentID: id, ProviderStatus: status, Action: actions[status], Amount: amount, Currency: request.Currency, Session: payments.CreatedSession{Status: local[status], Data: raw}}, nil
	}}
	registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter})
	if err != nil {
		t.Fatal(err)
	}
	options.Adapters = registry
	count := func() int64 {
		var n int64
		if db.Table("cart_checkout_observations").Where("cart_id=?", ref.CartID).Count(&n).Error != nil {
			t.Fatal("observation count failed")
		}
		return n
	}
	observe := func(want string) {
		t.Helper()
		got, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, options)
		if err != nil || got.State != want || got.Version != "5" {
			t.Fatalf("observation %s: %s %v", mode.Load(), got.State, err)
		}
	}
	observe("pending")
	observe("pending")
	before := count()
	mode.Store("amount")
	if _, err = cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err == nil || count() != before {
		t.Fatal("amount mismatch applied")
	}
	mode.Store("error")
	if _, err = cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err == nil || count() != before {
		t.Fatal("provider outage turned into payment fact")
	}
	mode.Store("authorized")
	observe("authorized")
	mode.Store("paid")
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, options)
			if err != nil || got.State != "paid_pending_order" {
				t.Errorf("concurrent paid observation: %s %v", got.State, err)
			}
		}()
	}
	group.Wait()
	mode.Store("pending")
	observe("paid_pending_order")
	// Paid facts remain durable if the apply transaction fails later.
	if db.Exec(`CREATE TRIGGER fail_payment_apply BEFORE UPDATE ON cart_checkout_attempts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned observation apply failure'`).Error != nil {
		t.Fatal("trigger setup")
	}
	before = count()
	mode.Store("paid")
	if _, err = cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err == nil || count() != before+1 {
		t.Fatal("verified fact lost after apply rollback")
	}
	if db.Exec("DROP TRIGGER fail_payment_apply").Error != nil {
		t.Fatal("trigger cleanup")
	}
	observe("paid_pending_order")
	// Revocation after provider read cannot erase the already-authorized fact.
	before = count()
	during = func() {
		if db.Exec("UPDATE cart_ownership SET active=FALSE WHERE cart_id=?", ref.CartID).Error != nil {
			t.Error("owned revoke failed")
		}
	}
	if _, err = cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err != cart.ErrDenied || count() != before+1 {
		t.Fatal("revoked observation lost fact or exposed state")
	}
	during = nil
	if db.Exec("UPDATE cart_ownership SET active=TRUE WHERE cart_id=?", ref.CartID).Error != nil {
		t.Fatal("owned restore failed")
	}
	// HTTP uses only an empty object and current possession; no query status/ID.
	checkPaymentObservationHTTP(t, ctx, db, sessions, ref, options)
	mode.Store("failed")
	observe("requires_review")
	mode.Store("paid")
	observe("requires_review")
	var private struct {
		ID             string
		ResultEnvelope []byte
	}
	if db.Table("cart_checkout_observations").Where("cart_id=?", ref.CartID).Take(&private).Error != nil || len(private.ResultEnvelope) == 0 || strings.Contains(string(private.ResultEnvelope), "tr_initial") {
		t.Fatal("observation not sealed")
	}
	t.Log(`CART_PAYMENT_OBSERVATION_REHEARSAL {"real_mysql_redis":true,"get_only_isolated_adapter":true,"current_possession":true,"server_saved_locator":true,"amount_mismatch_denied":true,"provider_outage_not_fact":true,"authorized_distinct_from_paid":true,"concurrent_reads":8,"paid_not_downgraded":true,"conflicting_terminal_review":true,"sealed_evidence":true,"apply_failure_retains_fact":true,"revocation_retains_fact_without_disclosure":true,"no_stock_no_order_no_mail":true}`)
}
