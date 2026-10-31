package identity_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/mollieworker"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

type initialDiscoverer struct {
	executionObserver
	discover func(context.Context, payments.SessionRequest) (payments.SessionObservation, error)
}

func (a initialDiscoverer) DiscoverSession(ctx context.Context, r payments.SessionRequest) (payments.SessionObservation, error) {
	return a.discover(ctx, r)
}

func checkInitialPaymentRecovery(t *testing.T, ctx context.Context, db *gorm.DB, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	const provider = "pp_mollie-hosted-checkout_mollie"
	var creates, discovers, gets atomic.Int32
	var status atomic.Value
	status.Store("pending")
	var fail atomic.Bool
	fail.Store(true)
	observation := func(r payments.SessionRequest) payments.SessionObservation {
		s := status.Load().(string)
		action, local := "pending", "pending"
		if s == "paid" {
			action, local = "successful", "authorized"
		}
		raw, _ := json.Marshal(map[string]any{"id": "tr_recovery_" + strings.ReplaceAll(r.SessionID, "-", ""), "status": s, "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"cart_id": inScope(r), "session_id": r.SessionID}, "_links": map[string]any{"checkout": map[string]string{"href": "https://www.mollie.com/checkout/recovered"}}})
		return payments.SessionObservation{PaymentID: "tr_recovery_" + strings.ReplaceAll(r.SessionID, "-", ""), ProviderStatus: s, Action: action, Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: local, Data: raw}}
	}
	a := initialDiscoverer{executionObserver: executionObserver{
		create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			creates.Add(1)
			return payments.CreateUncertain, payments.CreatedSession{}, payments.ErrCreateExecution
		},
		observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			gets.Add(1)
			out := observation(r)
			if id != out.PaymentID {
				return payments.SessionObservation{}, payments.ErrCreateExecution
			}
			return out, nil
		},
	}, discover: func(_ context.Context, r payments.SessionRequest) (payments.SessionObservation, error) {
		discovers.Add(1)
		if fail.Load() {
			return payments.SessionObservation{}, payments.ErrCreateExecution
		}
		return observation(r), nil
	}}
	registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: a})
	if err != nil {
		t.Fatal(err)
	}
	o.Adapters = registry
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	exec("UPDATE inventory_levels SET stocked_quantity=stocked_quantity+2.5 WHERE id='add-level'")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	defer exec("UPDATE mollie_notification_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	ref, in := makeCart()
	if out, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, o); err != nil || out.State != "requires_review" {
		t.Fatal("lost creation fixture", err)
	}
	count := func(table string) int64 {
		t.Helper()
		var n int64
		if db.Table(table).Count(&n).Error != nil {
			t.Fatal("recovery count")
		}
		return n
	}
	orders, results, references := count("orders"), count("mollie_payment_references"), count("mollie_notification_inbox")
	exec("UPDATE mollie_notification_runtime SET enabled=FALSE WHERE id=1")
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", ref, in, o); ok || err != cart.ErrSchema || discovers.Load() != 0 {
		t.Fatal("closed recovery gate queried")
	}
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	untouched, freshCommand := makeCart()
	attempts, collections, effects := count("cart_checkout_attempts"), count("payment_collections"), count("payment_effect_journal")
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", untouched, freshCommand, o); ok || err != cart.ErrConflict {
		t.Fatal("recovery staged new checkout", err)
	}
	var owner struct{ CheckoutAttemptID *string }
	if db.Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", untouched.CartID).Take(&owner).Error != nil || owner.CheckoutAttemptID != nil || count("cart_checkout_attempts") != attempts || count("payment_collections") != collections || count("payment_effect_journal") != effects || discovers.Load() != 0 {
		t.Fatal("recovery froze untouched cart")
	}
	wrong := ref
	wrong.Generation = strings.Repeat("a", 43)
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", wrong, in, o); ok || err == nil || discovers.Load() != 0 {
		t.Fatal("revoked recovery queried provider")
	}
	if ok, err := cart.RecoverInitialPayment(ctx, db, "other", ref, in, o); ok || err == nil || discovers.Load() != 0 {
		t.Fatal("wrong store recovery queried provider")
	}
	exec("UPDATE payment_providers SET is_enabled=FALSE WHERE id=?", provider)
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", ref, in, o); ok || err == nil || discovers.Load() != 0 {
		t.Fatal("disabled provider recovery")
	}
	exec("UPDATE payment_providers SET is_enabled=TRUE WHERE id=?", provider)
	exec("UPDATE payment_effect_journal SET state='inflight',lease_until=DATE_ADD(UTC_TIMESTAMP(6),INTERVAL 1 MINUTE) WHERE scope_id=?", ref.CartID)
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", ref, in, o); ok || err != nil || discovers.Load() != 0 {
		t.Fatal("live creation lease queried")
	}
	exec("UPDATE payment_effect_journal SET lease_until=DATE_SUB(UTC_TIMESTAMP(6),INTERVAL 1 MINUTE) WHERE scope_id=?", ref.CartID)
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", ref, in, o); ok || err != payments.ErrCreateExecution {
		t.Fatal("failed discovery accepted")
	}
	if creates.Load() != 1 || count("orders") != orders || count("mollie_payment_references") != results || count("mollie_notification_inbox") != references {
		t.Fatal("failed discovery wrote business facts")
	}
	fail.Store(false)
	exec(`CREATE TRIGGER fail_recovery_queue BEFORE INSERT ON mollie_notification_inbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned recovery failure'`)
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", ref, in, o); ok || err == nil {
		t.Fatal("recovery queue failure accepted")
	}
	exec("DROP TRIGGER fail_recovery_queue")
	var state string
	if db.Table("payment_effect_journal").Select("state").Where("scope_id=?", ref.CartID).Scan(&state).Error != nil || state != "uncertain" || count("mollie_payment_references") != results {
		t.Fatal("recovery result/reference escaped rollback")
	}
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cart.RecoverInitialPayment(ctx, db, "shop", ref, in, o)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal("concurrent recovery", err)
		}
	}
	if creates.Load() != 1 || count("orders") != orders || count("mollie_payment_references") != results+1 || count("mollie_notification_inbox") != references+1 {
		t.Fatal("recovery repeated creation or order")
	}
	before := discovers.Load()
	if ok, err := cart.RecoverInitialPayment(ctx, db, "shop", ref, in, o); ok || err != nil || discovers.Load() != before {
		t.Fatal("succeeded recovery rediscovered")
	}
	worker := mollieworker.Worker{DB: db, StoreID: "shop", Payment: o}
	if out, err := worker.RunOnce(ctx); err != nil || out.Failed != 0 || out.Queried != 1 || out.Consumed != 1 {
		t.Fatal("recovered evidence not consumable", out, err)
	}
	if out, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, o); err != nil || out.State != "ready" || out.CheckoutURL == nil || creates.Load() != 1 {
		t.Fatal("recovered checkout not resumable", err)
	}
	// A new process can discover another lost response without a browser/cart
	// Redis record; a second process consumes the durable queued reference.
	ref2, in2 := makeCart()
	if _, err := cart.StartInitialPayment(ctx, db, "shop", ref2, in2, o); err != nil {
		t.Fatal(err)
	}
	status.Store("paid")
	worker = mollieworker.Worker{DB: db, StoreID: "shop", Payment: o}
	if out, err := worker.RunOnce(ctx); err != nil || out.Recovered != 1 || out.Failed != 0 {
		t.Fatal("worker discovery", out, err)
	}
	worker = mollieworker.Worker{DB: db, StoreID: "shop", Payment: o}
	if out, err := worker.RunOnce(ctx); err != nil || out.Queried != 1 || out.Consumed != 1 || out.Recovered != 0 || out.Failed != 0 {
		t.Fatal("worker restart recovery", out, err)
	}
	if creates.Load() != 2 || count("orders") != orders+1 {
		t.Fatal("read recovery created payment/order")
	}
	t.Log(`CART_INITIAL_RECOVERY_REHEARSAL {"real_mysql_redis":true,"no_second_post":true,"existing_attempt_only":true,"live_lease_respected":true,"authority_rechecked":true,"queue_failure_atomic":true,"concurrent_recovery":8,"browser_resume":true,"worker_restart":true,"lost_response_to_paid_order":true,"real_provider_called":false}`)
}
