package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type orderPaymentChannel struct {
	db                    *gorm.DB
	t                     *testing.T
	posts, gets           atomic.Int32
	failed, lost, invalid atomic.Bool
	started, release      chan struct{}
}

func (a *orderPaymentChannel) CreateSession(ctx context.Context, req payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	a.posts.Add(1)
	var committed int64
	if a.db.WithContext(ctx).Table("order_payment_operations o").Joins("JOIN payment_effect_journal e ON e.scope_id=o.order_id AND e.operation_id=o.operation_id AND e.resource_id=o.session_id").Where("o.session_id=? AND o.dispatched_at IS NOT NULL AND o.state='uncertain' AND e.state='inflight' AND e.attempts=1", req.SessionID).Count(&committed).Error != nil || committed != 1 {
		a.t.Error("order payment POST before committed claim")
	}
	if a.failed.Load() {
		return payments.CreateFailed, payments.CreatedSession{}, nil
	}
	if a.started != nil {
		close(a.started)
		<-a.release
	}
	if a.lost.Load() {
		return payments.CreateUncertain, payments.CreatedSession{}, payments.ErrCreateExecution
	}
	return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(req, "tr_supplement", "https://www.mollie.com/checkout/supplement")}, nil
}
func (a *orderPaymentChannel) DiscoverSession(_ context.Context, req payments.SessionRequest) (payments.SessionObservation, error) {
	a.gets.Add(1)
	id := "tr_supplement_" + strings.ReplaceAll(req.SessionID, "-", "")
	raw, _ := json.Marshal(map[string]any{"id": id, "status": "paid", "amount": map[string]string{"value": req.Amount, "currency": strings.ToUpper(req.Currency)}, "metadata": map[string]string{"session_id": req.SessionID, "cart_id": inScope(req)}})
	amount := req.Amount
	if a.invalid.Load() {
		amount = "999"
	}
	return payments.SessionObservation{PaymentID: id, ProviderStatus: "paid", Action: "successful", Amount: amount, Currency: req.Currency, Session: payments.CreatedSession{Status: "authorized", Data: raw}}, nil
}

func (a *orderPaymentChannel) ObserveSession(ctx context.Context, req payments.SessionRequest, id string) (payments.SessionObservation, error) {
	o, e := a.DiscoverSession(ctx, req)
	if e != nil || o.PaymentID != id {
		return payments.SessionObservation{}, payments.ErrCreateExecution
	}
	return o, nil
}

// A separate committed native checkout keeps the original refund regression
// unchanged. Only the catalog, device receipt and channel are SQL test fixtures;
// this does not claim registration, browser ownership or a real provider.
func newOrderPaymentExecutionFixture(t *testing.T, ctx context.Context, db *gorm.DB, o cart.InitialPaymentOptions) (orderpayments.Service, orderpayments.Principal, orderpayments.Input, string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("order payment execution fixture", e)
		}
	}
	id := uuid.NewString()
	created, e := cart.Create(ctx, db, cart.CreateCommand{OperationID: id, ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: "en"})
	if e != nil {
		t.Fatal("execution fresh cart", e)
	}
	ref := created.Reference
	if _, e = cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"}); e != nil {
		t.Fatal("execution cart item", e)
	}
	address := cart.ContactAddress{FirstName: "Payment", LastName: "Fixture", Address1: "Owned test street 1", City: "Copenhagen", PostalCode: "0012", Country: "dk", Phone: "+45 111"}
	if _, e = cart.SaveContacts(ctx, db, "shop", ref, cart.ContactCommand{OperationID: uuid.NewString(), Version: "2", Email: "supplement@example.invalid", Shipping: address, Billing: address}, o.ManualProviders); e != nil {
		t.Fatal("execution contacts", e)
	}
	if _, e = cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: "3", OptionID: "delivery-standard"}, o.ManualProviders); e != nil {
		t.Fatal("execution delivery", e)
	}
	checkout := cart.InitialPaymentCommand{OperationID: uuid.NewString(), Version: "4"}
	registry, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(r, "tr_supplement_original", "https://www.mollie.com/checkout/original")}, nil
		},
		observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			raw, _ := json.Marshal(map[string]any{"id": id, "status": "paid", "amount": map[string]string{"value": r.Amount, "currency": "EUR"}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}})
			return payments.SessionObservation{PaymentID: id, ProviderStatus: "paid", Action: "successful", Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: "authorized", Data: raw}}, nil
		},
	}})
	if e != nil {
		t.Fatal(e)
	}
	o.Adapters = registry
	if _, e = cart.StartInitialPayment(ctx, db, "shop", ref, checkout, o); e != nil {
		t.Fatal("execution original payment", e)
	}
	if _, e = cart.ObserveInitialPayment(ctx, db, "shop", ref, o); e != nil {
		t.Fatal("execution original observation", e)
	}
	complete, e := cart.CompletePaidOrder(ctx, db, "shop", ref, checkout, o)
	if e != nil {
		t.Fatal("execution original completion", e)
	}
	order := complete.OrderID
	var line string
	if db.Table("order_items").Select("item_id").Where("order_id=? AND version=1", order).Scan(&line).Error != nil || line == "" {
		t.Fatal("execution original line")
	}
	exec("INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,auth_version,verified_at,created_at,updated_at) VALUES('order-payment-execution-account','supplement@example.invalid','artificial-unusable-hash','Payment','Fixture',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) VALUES(?,'order-payment-execution-account',?,'shop',UTC_TIMESTAMP(6))", order, ref.CartID)
	exec("INSERT INTO devices(id,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('supplement-device','add-v','supplement-source','add-location',0,'eur','','','inspection','returned','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	wire, _ := json.Marshal(map[string]any{"lines": []any{map[string]any{"item_id": line, "quantity": 1, "device_ids": []string{"supplement-device"}}}})
	exec("INSERT INTO after_sales_requests(id,order_id,customer_id,type,reason,items,status,created_at,updated_at) VALUES('supplement-exchange',?,'order-payment-execution-account','exchange','Owned fixture',?,'processing',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", order, string(wire))
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,after_sales_request_id,created_at,updated_at) VALUES('supplement-assignment','supplement-device',?,?,'supplement-source','returned','supplement-exchange',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", order, line)
	exec(`UPDATE order_items SET fulfilled_quantity=1,raw_fulfilled_quantity='{"value":"1","precision":100}',shipped_quantity=1,raw_shipped_quantity='{"value":"1","precision":100}' WHERE order_id=? AND version=1`, order)
	exec("UPDATE tax_rates SET rate=30 WHERE is_default=TRUE AND deleted_at IS NULL")
	defer exec("UPDATE tax_rates SET rate=25 WHERE is_default=TRUE AND deleted_at IS NULL")
	var bound orders.SupplementalPayment
	e = db.Transaction(func(tx *gorm.DB) error {
		if _, e := orders.ApplyDeviceExchangeTx(ctx, tx, orders.DeviceExchangeInput{OrderID: order, OriginalItemID: line, LocationID: "add-location", OriginalAssignmentID: "supplement-assignment", RequestID: "supplement-exchange", ActorID: "refund-command-actor", ExpectedVersion: 1}); e != nil {
			return e
		}
		var e error
		bound, e = orders.ReadSupplementalPaymentTx(ctx, tx, order, 3)
		return e
	})
	if e != nil || bound.Amount != "6.00" {
		t.Fatal("committed positive exchange", bound, e)
	}
	return orderpayments.Service{DB: db, StoreID: "shop", ReturnOrigin: o.ReturnOrigin, Ring: o.Ring, Adapters: registry}, orderpayments.Principal{AccountID: "order-payment-execution-account", AuthVersion: 1}, orderpayments.Input{OperationID: uuid.NewString(), OrderID: order, ExpectedVersion: 3, CollectionID: bound.CollectionID, Amount: "6.00", CurrencyCode: "eur"}, ref.CartID
}

func checkOrderPaymentExecution(t *testing.T, ctx context.Context, db *gorm.DB, o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("order payment execution", e)
		}
	}
	s, p, in, cartID := newOrderPaymentExecutionFixture(t, ctx, db, o)
	channel := &orderPaymentChannel{db: db, t: t}
	registry, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": channel})
	if e != nil {
		t.Fatal(e)
	}
	s.Adapters = registry
	exec("UPDATE order_payment_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE order_payment_runtime SET enabled=FALSE WHERE id=1")
	var intake bool
	if db.Table("mollie_notification_runtime").Select("enabled").Where("id=1").Scan(&intake).Error != nil {
		t.Fatal("intake fixture")
	}
	defer exec("UPDATE mollie_notification_runtime SET enabled=? WHERE id=1", intake)
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Go(func() {
			if _, e := s.Prepare(ctx, p, in); e != nil {
				t.Error("concurrent supplemental prepare", e)
			}
		})
	}
	wg.Wait()
	if t.Failed() {
		t.Fatal("supplemental prepare failed")
	}
	if e := db.Transaction(func(tx *gorm.DB) error {
		nested := s
		nested.DB = tx
		if _, e := nested.Run(ctx, in.OperationID); e != orderpayments.ErrUnavailable {
			t.Error("nested effect accepted", e)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	exec("UPDATE customer_accounts SET enabled=FALSE WHERE id=?", p.AccountID)
	if _, e = s.Run(ctx, in.OperationID); e != orderpayments.ErrDenied {
		t.Fatal("revoked initial dispatch", e)
	}
	exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=?", p.AccountID)
	exec("UPDATE order_payment_runtime SET enabled=FALSE WHERE id=1")
	if _, e = s.Run(ctx, in.OperationID); e != orderpayments.ErrUnavailable {
		t.Fatal("disabled initial dispatch", e)
	}
	exec("UPDATE order_payment_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE order_line_tax_lines t JOIN order_exchange_items i ON i.item_id=t.item_id JOIN order_exchanges e ON e.id=i.exchange_id SET t.rate=29,t.raw_rate=JSON_OBJECT('value','29','precision',100) WHERE e.order_id=?", in.OrderID)
	if _, e = s.Run(ctx, in.OperationID); e != orderpayments.ErrConflict {
		t.Fatal("changed amount initial dispatch", e)
	}
	exec("UPDATE order_line_tax_lines t JOIN order_exchange_items i ON i.item_id=t.item_id JOIN order_exchanges e ON e.id=i.exchange_id SET t.rate=30,t.raw_rate=JSON_OBJECT('value','30','precision',100) WHERE e.order_id=?", in.OrderID)
	for _, q := range []string{"UPDATE customer_accounts SET auth_version=2 WHERE id='order-payment-execution-account'", "UPDATE customer_accounts SET verified_at=NULL WHERE id='order-payment-execution-account'"} {
		exec(q)
		if _, e = s.Run(ctx, in.OperationID); e != orderpayments.ErrDenied {
			t.Fatal("stale/unverified initial dispatch", e)
		}
		exec("UPDATE customer_accounts SET auth_version=1,verified_at=UTC_TIMESTAMP(6) WHERE id=?", p.AccountID)
	}
	exec("UPDATE payment_providers SET is_enabled=FALSE WHERE id='pp_mollie-hosted-checkout_mollie'")
	if _, e = s.Run(ctx, in.OperationID); e != orderpayments.ErrConflict {
		t.Fatal("disabled provider initial dispatch", e)
	}
	exec("UPDATE payment_providers SET is_enabled=TRUE WHERE id='pp_mollie-hosted-checkout_mollie'")
	if channel.posts.Load() != 0 {
		t.Fatal("denied dispatch called provider")
	}
	channel.failed.Store(true)
	for i := 0; i < 6; i++ {
		wg.Go(func() {
			w := orderpayments.Worker{Service: s}
			if r, e := w.RunOnce(ctx); e != nil || r.Failed != 0 {
				t.Error("concurrent first dispatch worker", r, e)
			}
		})
	}
	wg.Wait()
	if r, e := s.Read(ctx, p, in.OrderID, in.OperationID); e != nil || r.State != "failed" || !r.CanRetry || channel.posts.Load() != 1 {
		t.Fatal("definitive failed session", r, e)
	}
	prev := in.OperationID
	in.PreviousOperationID = prev
	in.OperationID = uuid.NewString()
	if _, e = s.Prepare(ctx, p, in); e != nil {
		t.Fatal("explicit failure successor", e)
	}
	channel.failed.Store(false)
	channel.lost.Store(true)
	channel.started = make(chan struct{})
	channel.release = make(chan struct{})
	finished := make(chan error, 1)
	go func() { _, e := s.Run(ctx, in.OperationID); finished <- e }()
	select {
	case <-channel.started:
	case <-ctx.Done():
		t.Fatal("dispatch did not start")
	}
	for i := 0; i < 6; i++ {
		wg.Go(func() {
			if _, e := s.Run(ctx, in.OperationID); e != nil {
				t.Error("live initial lease", e)
			}
		})
	}
	wg.Wait()
	if channel.posts.Load() != 2 || channel.gets.Load() != 0 {
		t.Fatal("live lease resent/discovered")
	}
	exec("UPDATE customer_accounts SET enabled=FALSE,auth_version=2 WHERE id=?", p.AccountID)
	exec("UPDATE cart_ownership SET active=FALSE,generation=REPEAT('0',64),checkout_attempt_id=NULL WHERE cart_id=?", cartID)
	exec("UPDATE carts SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", cartID)
	exec("UPDATE orders SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", in.OrderID)
	close(channel.release)
	if e = <-finished; e != payments.ErrCreateExecution {
		t.Fatal("lost response did not persist uncertainty", e)
	}
	if _, e = s.Cancel(ctx, p, in.OrderID, orderpayments.CancelInput{OperationID: uuid.NewString(), PaymentOperationID: in.OperationID}); e != orderpayments.ErrDenied {
		t.Fatal("revoked owner can cancel", e)
	}
	channel.invalid.Store(true)
	if _, e = s.Run(ctx, in.OperationID); e != payments.ErrCreateExecution {
		t.Fatal("foreign discovery accepted", e)
	}
	channel.invalid.Store(false)
	exec("CREATE TRIGGER owned_supplement_reference_failure BEFORE INSERT ON mollie_payment_references FOR EACH ROW BEGIN IF NEW.scope_id='" + in.OrderID + "' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned supplement reference failure'; END IF; END")
	if _, e = s.Run(ctx, in.OperationID); e == nil {
		t.Fatal("late protected creation failure accepted")
	}
	exec("DROP TRIGGER owned_supplement_reference_failure")
	var saved struct {
		State          string
		ResultEnvelope []byte
	}
	if db.Table("payment_effect_journal").Select("state,result_envelope").Where("scope_id=? AND operation_id=?", in.OrderID, in.OperationID).Take(&saved).Error != nil || saved.State != "uncertain" || len(saved.ResultEnvelope) != 0 {
		t.Fatal("partial creation receipt survived rollback")
	}
	exec("UPDATE mollie_notification_runtime SET enabled=FALSE WHERE id=1")
	if _, e = s.Run(ctx, in.OperationID); e == nil {
		t.Fatal("disabled inbox accepted queue")
	}
	if db.Table("payment_effect_journal").Select("state,result_envelope").Where("scope_id=? AND operation_id=?", in.OrderID, in.OperationID).Take(&saved).Error != nil || saved.State != "succeeded" || len(saved.ResultEnvelope) == 0 {
		t.Fatal("disabled inbox discarded protected creation")
	}
	gets := channel.gets.Load()
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE order_payment_runtime SET enabled=FALSE WHERE id=1")
	fresh := s
	if r, e := fresh.Run(ctx, in.OperationID); e != nil || r.State != "uncertain" || r.Applied || channel.posts.Load() != 2 || channel.gets.Load() != gets {
		t.Fatal("saved creation recovery after revocation/disable", r, e)
	}
	var counts int64
	if db.Table("payments p").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id").Where("l.order_id=?", in.OrderID).Count(&counts).Error != nil || counts != 1 {
		t.Fatal("creation or paid discovery booked cash")
	}
	exec("UPDATE orders SET deleted_at=NULL WHERE id=?", in.OrderID)
	t.Log(`NATIVE_ORDER_PAYMENT_EXECUTION_REHEARSAL {"actual_separate_completed_checkout":true,"committed_native_positive_exchange":true,"due":"6.00","concurrent_prepare":6,"concurrent_run":6,"one_post_per_instruction":true,"durable_claim_before_post":true,"live_lease_no_post_or_discovery":true,"revoked_first_dispatch_denied":true,"disabled_first_dispatch_denied":true,"stale_unverified_dispatch_denied":true,"disabled_provider_dispatch_denied":true,"changed_amount_dispatch_denied":true,"explicit_failed_successor":true,"lost_response_get_only":true,"foreign_discovery_denied":true,"late_creation_failure_atomic":true,"protected_creation_survives_disabled_inbox":true,"recovery_after_account_cart_order_retirement":true,"disabled_feature_recovery":true,"reconstructed_service_no_network_for_saved_creation":true,"creation_or_discovery_not_cash":true,"real_provider":false,"customer_session_or_browser":false,"cash_consumed":false}`)
	checkOrderPaymentConsumption(t, ctx, db, s, in, cartID, channel)
}
