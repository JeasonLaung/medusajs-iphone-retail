package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/checkoutconfig"
	"github.com/eurorefurb/retail/internal/mollieworker"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkMollieCheckoutConsumer(t *testing.T, ctx context.Context, db *gorm.DB, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("notification checkout fixture", err)
		}
	}
	const provider = "pp_mollie-hosted-checkout_mollie"
	base, err := o.Adapters.Lookup(provider)
	if err != nil {
		t.Fatal(err)
	}
	var status atomic.Value
	status.Store("pending")
	var gets atomic.Int32
	adapters, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: executionObserver{
		create: base.CreateSession,
		observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			gets.Add(1)
			s := status.Load().(string)
			actions := map[string]string{"pending": "pending", "open": "not_supported", "authorized": "authorized", "paid": "successful", "failed": "failed"}
			local := map[string]string{"pending": "pending", "open": "pending", "authorized": "authorized", "paid": "authorized", "failed": "error"}
			raw, _ := json.Marshal(map[string]any{"id": id, "status": s, "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"cart_id": inScope(r), "session_id": r.SessionID}})
			return payments.SessionObservation{PaymentID: id, ProviderStatus: s, Action: actions[s], Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: local[s], Data: raw}}, nil
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	o.Adapters = adapters
	// These two additional artificial orders each reserve 2.5 units. Supply
	// their own stock so pre-existing promotion/stock fixtures keep their budget.
	exec("UPDATE inventory_levels SET stocked_quantity=stocked_quantity+5 WHERE id='add-level'")
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE mollie_notification_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	start := func() (sessionstore.CartReference, cart.InitialPaymentCommand) {
		t.Helper()
		ref, in := makeCart()
		if r, e := cart.StartInitialPayment(ctx, db, "shop", ref, in, o); e != nil || r.State != "ready" {
			t.Fatal("notification initial checkout", r.State, e)
		}
		return ref, in
	}
	notify := func(ref sessionstore.CartReference, s string) string {
		t.Helper()
		status.Store(s)
		var remote string
		if db.Table("mollie_payment_references").Select("remote_id").Where("scope_id=?", ref.CartID).Scan(&remote).Error != nil || remote == "" {
			t.Fatal("notification reference missing")
		}
		if e := db.Transaction(func(tx *gorm.DB) error {
			_, e := payments.EnqueueMollieNotification(ctx, tx, provider, remote)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		r, e := payments.ProcessMollieNotification(ctx, db, provider, remote, o.Ring, o.Adapters)
		if e != nil || !r.Called || r.Receipt.AttemptID == nil {
			t.Fatal("notification query", e)
		}
		return *r.Receipt.AttemptID
	}
	consume := func(attempt string) cart.MollieCheckoutConsumption {
		t.Helper()
		r, e := cart.ConsumeMollieCheckoutObservation(ctx, db, "shop", attempt, o)
		if e != nil {
			t.Fatal("notification consumption", e)
		}
		return r
	}
	var trackedCart string
	fingerprint := func() string {
		t.Helper()
		out := ""
		for _, table := range []string{"orders", "order_carts", "payments", "payment_captures", "order_transactions", "order_summaries", "inventory_reservations", "inventory_reservation_operations", "promotion_usage_operations", "cart_completion_receipts", "order_notification_outbox", "cart_checkout_observations"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("notification effect count")
			}
			out += fmt.Sprint(table, n)
		}
		var reserved string
		db.Table("inventory_levels").Select("reserved_quantity").Where("id='add-level'").Scan(&reserved)
		var state string
		if db.Raw("SELECT CONCAT(o.version,':',COALESCE(a.observed_status,''),':',a.needs_review,':',COALESCE(a.paid_observation_id,'')) FROM cart_ownership o JOIN cart_checkout_attempts a ON a.cart_id=o.cart_id WHERE o.cart_id=?", trackedCart).Scan(&state).Error != nil {
			t.Fatal("notification current state fingerprint")
		}
		var allocator struct{ LastDisplayID, LastTick int64 }
		if db.Table("order_number_allocator").Where("id=1").Take(&allocator).Error != nil {
			t.Fatal("notification order number baseline")
		}
		return out + reserved + state + fmt.Sprint(allocator)
	}
	ref, in := start()
	checkContextPaymentComplete(t, ctx, db, ref, in, o)
	trackedCart = ref.CartID
	for _, s := range []string{"pending", "authorized"} {
		attempt := notify(ref, s)
		r := consume(attempt)
		if r.Completion.OrderID != "" || r.State != s {
			t.Fatal("unpaid notification produced order", s, r)
		}
		if r = consume(attempt); !r.Replayed || r.Completion.OrderID != "" {
			t.Fatal("unpaid consumption replay", r)
		}
	}
	attempt := notify(ref, "paid")
	before := fingerprint()
	unchanged := func() {
		t.Helper()
		var n int64
		if db.Table("mollie_notification_observations").Where("attempt_id=? AND consumed_at IS NULL", attempt).Count(&n).Error != nil || n != 1 || fingerprint() != before {
			t.Fatal("failed consumer leaked effects or acknowledged evidence")
		}
	}
	reject := func(opts cart.InitialPaymentOptions, store string) {
		t.Helper()
		if _, e := cart.ConsumeMollieCheckoutObservation(ctx, db, store, attempt, opts); e == nil {
			t.Fatal("invalid notification authority accepted")
		}
		unchanged()
	}
	wrong := o
	wrong.Ring, _ = protected.New("wrong", map[string][]byte{"wrong": bytes.Repeat([]byte{1}, 32)})
	reject(wrong, "shop")
	reject(o, "another-shop")
	var proof struct{ ResultEnvelope []byte }
	if db.Table("mollie_notification_observations").Select("result_envelope").Where("attempt_id=?", attempt).Take(&proof).Error != nil || len(proof.ResultEnvelope) < 34 {
		t.Fatal("notification encrypted evidence unavailable")
	}
	envelope := proof.ResultEnvelope
	corrupt := append([]byte(nil), envelope...)
	corrupt[len(corrupt)-1] ^= 1
	exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", corrupt, attempt)
	reject(o, "shop")
	exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", envelope, attempt)
	// Even authenticated ciphertext must match the original request. Model
	// mismatched evidence without relying solely on AEAD corruption checks.
	plain, err := o.Ring.Open(protected.Context{Purpose: "mollie-observation-v1", Table: "mollie_notification_observations", Column: "result_envelope", Row: attempt}, envelope)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plain)
	for _, field := range []string{"amount", "currency", "session", "cart"} {
		var observed payments.SessionObservation
		if json.Unmarshal(plain, &observed) != nil {
			t.Fatal("owned observation decode")
		}
		var data map[string]any
		if json.Unmarshal(observed.Session.Data, &data) != nil {
			t.Fatal("owned provider data decode")
		}
		switch field {
		case "amount":
			observed.Amount = "1"
			data["amount"].(map[string]any)["value"] = "1"
		case "currency":
			observed.Currency = "usd"
			data["amount"].(map[string]any)["currency"] = "USD"
		case "session":
			data["metadata"].(map[string]any)["session_id"] = "payses_foreign"
		case "cart":
			data["metadata"].(map[string]any)["cart_id"] = "cart_foreign"
		}
		observed.Session.Data, _ = json.Marshal(data)
		raw, _ := json.Marshal(observed)
		sealed, e := o.Ring.Seal(protected.Context{Purpose: "mollie-observation-v1", Table: "mollie_notification_observations", Column: "result_envelope", Row: attempt}, raw)
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", sealed, attempt)
		reject(o, "shop")
	}
	exec("UPDATE mollie_notification_observations SET result_envelope=? WHERE attempt_id=?", envelope, attempt)
	exec("UPDATE mollie_payment_references SET request_sha256=UNHEX(REPEAT('00',32)) WHERE scope_id=?", ref.CartID)
	reject(o, "shop")
	exec("UPDATE mollie_payment_references r JOIN cart_checkout_attempts a ON a.cart_id=r.scope_id AND a.operation_id=r.operation_id SET r.request_sha256=a.request_sha256 WHERE r.scope_id=?", ref.CartID)
	for _, gate := range []string{"mollie_notification_runtime", "cart_checkout_runtime", "cart_completion_runtime"} {
		exec("UPDATE " + gate + " SET enabled=FALSE WHERE id=1")
		reject(o, "shop")
		exec("UPDATE " + gate + " SET enabled=TRUE WHERE id=1")
	}
	exec("UPDATE cart_ownership SET active=FALSE WHERE cart_id=?", ref.CartID)
	reject(o, "shop")
	exec("UPDATE cart_ownership SET active=TRUE,generation=REPEAT('0',64) WHERE cart_id=?", ref.CartID)
	reject(o, "shop")
	exec("UPDATE cart_ownership SET generation=?,checkout_attempt_id=? WHERE cart_id=?", ref.Generation, uuid.NewString(), ref.CartID)
	reject(o, "shop")
	exec("UPDATE cart_ownership SET checkout_attempt_id=? WHERE cart_id=?", in.OperationID, ref.CartID)
	var session string
	db.Table("cart_checkout_attempts").Select("session_id").Where("cart_id=?", ref.CartID).Scan(&session)
	for _, change := range []struct{ set, restore string }{{"currency_code='usd'", "currency_code='eur'"}, {"amount=1,raw_amount='{\"value\":\"1\",\"precision\":38}'", "amount=158.75,raw_amount='{\"value\":\"158.75\",\"precision\":38}'"}} {
		exec("UPDATE payment_sessions SET "+change.set+" WHERE id=?", session)
		reject(o, "shop")
		exec("UPDATE payment_sessions SET "+change.restore+" WHERE id=?", session)
	}
	// Fail each local stage, especially the final consumed_at write, proving
	// that order/stock/cash/promotions/mail/evidence commit as one transaction.
	for _, stage := range []struct{ table, event string }{{"orders", "INSERT"}, {"inventory_reservations", "INSERT"}, {"promotion_usage_operations", "INSERT"}, {"payments", "INSERT"}, {"order_notification_outbox", "INSERT"}, {"cart_completion_receipts", "INSERT"}, {"mollie_notification_observations", "UPDATE"}} {
		exec("CREATE TRIGGER notification_checkout_fault BEFORE " + stage.event + " ON " + stage.table + " FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned notification checkout failure'")
		reject(o, "shop")
		exec("DROP TRIGGER notification_checkout_fault")
	}
	// Browsers and workers race the same current payment. Prime the ordinary
	// browser proof without completing, then race both transaction owners.
	if r, e := cart.ObserveInitialPayment(ctx, db, "shop", ref, o); e != nil || r.State != "paid_pending_order" {
		t.Fatal("browser competing observation", e)
	}
	var wg sync.WaitGroup
	results := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(browser bool) {
			defer wg.Done()
			if browser {
				r, e := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o)
				results <- r.OrderID
				errs <- e
			} else {
				r, e := cart.ConsumeMollieCheckoutObservation(ctx, db, "shop", attempt, o)
				results <- r.Completion.OrderID
				errs <- e
			}
		}(i%2 == 0)
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("browser/worker completion race", e)
		}
	}
	order := ""
	for id := range results {
		if id == "" || order != "" && order != id {
			t.Fatal("duplicate or missing notification order")
		}
		order = id
	}
	after := fingerprint()
	if r := consume(attempt); !r.Replayed || r.Completion.OrderID != order || fingerprint() != after {
		t.Fatal("consumed paid replay changed effects")
	}
	exec("UPDATE cart_ownership SET active=FALSE WHERE cart_id=?", ref.CartID)
	if _, e := cart.ConsumeMollieCheckoutObservation(ctx, db, "shop", attempt, o); e == nil {
		t.Fatal("revoked completed notification replay accepted")
	}
	exec("UPDATE cart_ownership SET active=TRUE WHERE cart_id=?", ref.CartID)
	exec("UPDATE payment_providers SET is_enabled=FALSE WHERE id=?", provider)
	if _, e := cart.ConsumeMollieCheckoutObservation(ctx, db, "shop", attempt, o); e == nil {
		t.Fatal("disabled provider completed notification replay accepted")
	}
	exec("UPDATE payment_providers SET is_enabled=TRUE WHERE id=?", provider)
	if r := consume(notify(ref, "open")); r.State != "completed" || r.Completion.OrderID != order {
		t.Fatal("delayed open downgraded paid order")
	}
	if r := consume(notify(ref, "failed")); r.State != "requires_review" || r.Completion.OrderID != order {
		t.Fatal("contradictory terminal notification not retained for review")
	}
	// A second checkout never polls or invokes browser completion: a durable
	// notification alone creates the order, available through the normal receipt.
	fresh, _ := start()
	status.Store("paid")
	webhooks, e := checkoutconfig.Webhooks(func(k string) string {
		if k == "MOLLIE_NOTIFICATIONS_ENABLED" {
			return "true"
		}
		return ""
	}, &o)
	if e != nil {
		t.Fatal(e)
	}
	h, e := payments.NewMollieWebhook(db, webhooks)
	if e != nil {
		t.Fatal(e)
	}
	router := gin.New()
	h.Register(router)
	var remote string
	if db.Table("mollie_payment_references").Select("remote_id").Where("scope_id=?", fresh.CartID).Scan(&remote).Error != nil || remote == "" {
		t.Fatal("worker remote reference")
	}
	req := httptest.NewRequest("POST", "/hooks/payment/mollie-hosted-checkout_mollie", strings.NewReader("id="+remote))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != 200 {
		t.Fatal("configured notification HTTP did not commit queue")
	}
	getCount := gets.Load()
	worker := mollieworker.Worker{DB: db, StoreID: "shop", Payment: o}
	exec("UPDATE mollie_notification_runtime SET enabled=FALSE WHERE id=1")
	if _, e := worker.RunOnce(ctx); e == nil || gets.Load() != getCount {
		t.Fatal("disabled worker queried provider")
	}
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	foreign := mollieworker.Worker{DB: db, StoreID: "another-shop", Payment: o}
	if result, e := foreign.RunOnce(ctx); e != nil || result.Queried != 0 || result.Consumed != 0 {
		t.Fatal("worker crossed configured store", e)
	}
	// Query ACK survives a later business failure. A new worker must discover
	// the saved observation without another provider call or browser visit.
	exec("CREATE TRIGGER notification_worker_fault BEFORE UPDATE ON mollie_notification_observations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned worker consume failure'")
	if result, e := worker.RunOnce(ctx); e != nil || result.Queried != 1 || result.Consumed != 0 || result.Failed != 1 {
		t.Fatal("worker did not retain failed business consumption", result, e)
	}
	exec("DROP TRIGGER notification_worker_fault")
	worker = mollieworker.Worker{DB: db, StoreID: "shop", Payment: o}
	if result, e := worker.RunOnce(ctx); e != nil || result.Queried != 0 || result.Consumed != 1 || result.Failed != 0 || gets.Load() != getCount+1 {
		t.Fatal("restarted worker did not recover durable observation", result, e)
	}
	var receipt struct{ OrderID string }
	if db.Table("cart_completion_receipts").Select("order_id").Where("cart_id=?", fresh.CartID).Take(&receipt).Error != nil || receipt.OrderID == "" {
		t.Fatal("notification-only worker completion failed")
	}
	if confirmed, e := cart.CompleteCurrentPaidOrder(ctx, db, "shop", fresh, o); e != nil || !confirmed.Replayed || confirmed.OrderID != receipt.OrderID {
		t.Fatal("returning browser did not recover same order", e)
	}
	if result, e := worker.RunOnce(ctx); e != nil || result.Queried != 0 || result.Consumed != 0 {
		t.Fatal("idle worker repeated effects", result, e)
	}
	t.Log(`MOLLIE_CHECKOUT_CONSUMER_REHEARSAL {"real_mysql_redis":true,"configured_http_and_worker":true,"worker_restart_without_second_get":true,"worker_store_and_gate_checked":true,"idle_no_repeat":true,"notification_only_order":true,"pending_authorized_no_order":true,"same_order_on_return":true,"browser_worker_race":8,"atomic_failure_stages":7,"corrupt_and_wrong_key_denied":true,"disabled_and_revoked_denied":true,"session_currency_amount_denied":true,"late_open_no_downgrade":true,"terminal_conflict_review":true,"provider_mocked":true}`)
}
