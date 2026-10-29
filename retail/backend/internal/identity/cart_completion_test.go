package identity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkPaidOrderCompletion(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, in cart.InitialPaymentCommand, o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("completion fixture", err)
		}
	}
	if _, err := cart.CompletePaidOrder(ctx, db, "shop", ref, in, o); err != cart.ErrSchema {
		t.Fatal("completion default gate open", err)
	}
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	if _, err := cart.CompletePaidOrder(ctx, db, "shop", ref, in, o); err == nil {
		t.Fatal("unpaid order completed")
	}
	registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			t.Error("completion created external payment")
			return payments.CreateUncertain, payments.CreatedSession{}, nil
		},
		observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			raw, _ := json.Marshal(map[string]any{"id": id, "status": "paid", "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}})
			return payments.SessionObservation{PaymentID: id, ProviderStatus: "paid", Action: "successful", Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: "authorized", Data: raw}}, nil
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	o.Adapters = registry
	if got, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, o); err != nil || got.State != "paid_pending_order" {
		t.Fatal("completion paid observation", got.State, err)
	}
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	fingerprint := func() string {
		t.Helper()
		out := ""
		for _, table := range []string{"orders", "order_carts", "payments", "payment_captures", "order_transactions", "order_summaries", "inventory_reservations", "inventory_reservation_operations", "cart_completion_receipts", "order_notification_outbox"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("completion count")
			}
			out += fmt.Sprint(table, n)
		}
		var owner struct{ Version uint64 }
		db.Table("cart_ownership").Where("cart_id=?", ref.CartID).Take(&owner)
		var allocator struct{ LastDisplayID, LastTick int64 }
		db.Table("order_number_allocator").Where("id=1").Take(&allocator)
		var stock string
		db.Table("inventory_levels").Select("reserved_quantity").Where("id='add-level'").Scan(&stock)
		return out + fmt.Sprint(owner, allocator, stock)
	}
	before := fingerprint()
	exec("CREATE TRIGGER completion_late_failure BEFORE INSERT ON cart_completion_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned completion failure'")
	if _, err := cart.CompletePaidOrder(ctx, db, "shop", ref, in, o); err == nil {
		t.Fatal("late completion failure accepted")
	}
	exec("DROP TRIGGER completion_late_failure")
	if fingerprint() != before {
		t.Fatal("late completion leaked order/stock/cash/outbox/number/version")
	}
	// Recovery from a rejected current stock allocation uses the SAME paid attempt.
	exec("UPDATE inventory_levels SET stocked_quantity=0 WHERE id='add-level'")
	if _, err := cart.CompletePaidOrder(ctx, db, "shop", ref, in, o); err == nil {
		t.Fatal("out of stock completed")
	}
	exec("UPDATE inventory_levels SET stocked_quantity=10 WHERE id='add-level'")
	if fingerprint() != before {
		t.Fatal("stock failure leaked completion")
	}
	var wg sync.WaitGroup
	results := make(chan cart.CompletionResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := cart.CompletePaidOrder(ctx, db, "shop", ref, in, o)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent completion", err)
		}
	}
	orderID := ""
	fresh := 0
	for r := range results {
		if r.OrderID == "" || r.Version != "6" {
			t.Fatal("completion result missing")
		}
		if orderID != "" && orderID != r.OrderID {
			t.Fatal("duplicate orders")
		}
		orderID = r.OrderID
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("completion fresh count", fresh)
	}
	after := fingerprint()
	if r, e := cart.CompletePaidOrder(ctx, db, "shop", ref, in, o); e != nil || !r.Replayed || r.OrderID != orderID || fingerprint() != after {
		t.Fatal("lost-response retry changed completion", e)
	}
	var completed struct{ CompletedAt *time.Time }
	if db.Table("carts").Select("completed_at").Where("id=?", ref.CartID).Take(&completed).Error != nil || completed.CompletedAt == nil {
		t.Fatal("cart not closed")
	}
	if _, e := cart.Inspect(ctx, db, "shop", ref); e == nil {
		t.Fatal("completed cart still writable/readable as draft")
	}
	var notification struct {
		ID, Status      string
		PayloadEnvelope []byte
		Attempts        int
	}
	if db.Table("order_notification_outbox").Where("order_id=?", orderID).Take(&notification).Error != nil || notification.Status != "pending" || notification.Attempts != 0 {
		t.Fatal("notification intent absent")
	}
	plain, e := o.Ring.Open(protected.Context{Purpose: "order-notification-v1", Table: "order_notification_outbox", Column: "payload_envelope", Row: notification.ID}, notification.PayloadEnvelope)
	if e != nil || !strings.Contains(string(plain), orderID) || !strings.Contains(string(plain), "Email") {
		t.Fatal("notification envelope invalid")
	}
	clear(plain)
	if _, e = o.Ring.Open(protected.Context{Purpose: "order-notification-v1", Table: "order_notification_outbox", Column: "payload_envelope", Row: uuid.NewString()}, notification.PayloadEnvelope); e == nil {
		t.Fatal("notification row swap accepted")
	}
	foreign := ref
	foreign.Generation = strings.Repeat("0", 64)
	if _, e := cart.CompletePaidOrder(ctx, db, "shop", foreign, in, o); e != cart.ErrDenied {
		t.Fatal("foreign possession replay", e)
	}
	changed := in
	changed.Version = "3"
	if _, e := cart.CompletePaidOrder(ctx, db, "shop", ref, changed, o); e != cart.ErrConflict {
		t.Fatal("changed replay command", e)
	}
	exec("UPDATE cart_ownership SET active=FALSE WHERE cart_id=?", ref.CartID)
	if _, e := cart.CompletePaidOrder(ctx, db, "shop", ref, in, o); e != cart.ErrDenied {
		t.Fatal("revoked completion replay", e)
	}
	exec("UPDATE cart_ownership SET active=TRUE WHERE cart_id=?", ref.CartID)
	if fingerprint() != after {
		t.Fatal("rejected replay changed completion")
	}
	t.Log(`CART_COMPLETION_REHEARSAL {"real_mysql_redis":true,"concurrent_requests":8,"one_committed_order":true,"same_attempt_after_stock_recovery":true,"late_receipt_failure_rolls_back_all":true,"lost_response_replay":true,"closed_cart":true,"revoked_replay_denied":true,"sealed_notification_intent":true,"notification_delivery":false,"coupon_completion":false}`)
}
