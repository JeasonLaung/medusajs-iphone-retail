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
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkPaidOrderPromotionCompletion(t *testing.T, ctx context.Context, db *gorm.DB, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("promotion completion fixture", e)
		}
	}
	exec("INSERT INTO promotion_campaigns(id,name,campaign_identifier,created_at,updated_at) VALUES('checkout-coupon-campaign','Checkout coupon','CHECKOUT_COUPON_CAMPAIGN',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO promotions(id,code,campaign_id,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('checkout-coupon','CHECKOUT_COUPON','checkout-coupon-campaign',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE promotions SET `limit`=10 WHERE id='checkout-coupon'")
	exec(`INSERT INTO promotion_application_methods(id,promotion_id,type,target_type,allocation,value,raw_value,currency_code,created_at,updated_at) VALUES('checkout-coupon-method','checkout-coupon','fixed','items','across',1.25,'{"value":"1.25","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO promotion_campaign_budgets(id,type,campaign_id,attribute,currency_code,` + "`limit`" + `,raw_limit,used,raw_used,created_at,updated_at) VALUES('checkout-coupon-budget','use_by_attribute','checkout-coupon-campaign','customer_email','eur',1,'{"value":"1","precision":100}',0,'{"value":"0","precision":100}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	var externalCreates int
	registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			externalCreates++
			if r.Amount != "157.19" {
				t.Error("coupon settlement", r.Amount)
			}
			return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(r, "tr_coupon", "https://www.mollie.com/checkout/coupon")}, nil
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
	prepare := func() (sessionstore.CartReference, cart.InitialPaymentCommand) {
		t.Helper()
		ref, in := makeCart()
		e := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, e := cart.RefreshPromotions(ctx, tx, "shop", ref, in.Version, cart.PromotionRefresh{Codes: []string{"CHECKOUT_COUPON"}, At: time.Now().UTC()})
			return e
		})
		if e != nil {
			t.Fatal("coupon application", e)
		}
		if r, e := cart.StartInitialPayment(ctx, db, "shop", ref, in, o); e != nil || r.State != "ready" {
			t.Fatal("coupon payment", e, r.State)
		}
		if r, e := cart.ObserveInitialPayment(ctx, db, "shop", ref, o); e != nil || r.State != "paid_pending_order" {
			t.Fatal("coupon observation", e, r.State)
		}
		return ref, in
	}
	// Freeze both paid attempts before usage is consumed. Completing one must
	// prevent the other from bypassing its serialized per-email budget.
	first, firstIn := prepare()
	second, secondIn := prepare()
	fingerprint := func() string {
		t.Helper()
		s := ""
		for _, table := range []string{"orders", "payments", "payment_captures", "inventory_reservations", "promotion_usage_operations", "promotion_budget_usages", "cart_completion_receipts", "order_notification_outbox"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("coupon fingerprint")
			}
			s += fmt.Sprint(table, n)
		}
		var used int64
		if db.Table("promotions").Select("used").Where("id='checkout-coupon'").Scan(&used).Error != nil {
			t.Fatal("coupon usage")
		}
		return s + fmt.Sprint(used)
	}
	before := fingerprint()
	exec("UPDATE cart_line_adjustments a JOIN cart_line_items i ON i.id=a.item_id SET a.code='FOREIGN_CODE' WHERE i.cart_id=? AND a.promotion_id='checkout-coupon' AND a.deleted_at IS NULL", first.CartID)
	if _, e := cart.CompletePaidOrder(ctx, db, "shop", first, firstIn, o); e != cart.ErrCompletionPromotion {
		t.Fatal("mismatched frozen code accepted", e)
	}
	exec("UPDATE cart_line_adjustments a JOIN cart_line_items i ON i.id=a.item_id SET a.code='CHECKOUT_COUPON' WHERE i.cart_id=? AND a.promotion_id='checkout-coupon' AND a.deleted_at IS NULL", first.CartID)
	if fingerprint() != before {
		t.Fatal("coupon code rejection leaked order")
	}

	exec("CREATE TRIGGER coupon_completion_late_failure BEFORE INSERT ON cart_completion_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned coupon completion failure'")
	if _, e := cart.CompletePaidOrder(ctx, db, "shop", first, firstIn, o); e == nil {
		t.Fatal("late coupon failure accepted")
	}
	exec("DROP TRIGGER coupon_completion_late_failure")
	if fingerprint() != before {
		t.Fatal("coupon usage escaped completion rollback")
	}
	// Changed configuration cannot silently reprice an already paid cart.
	exec("UPDATE promotion_application_methods SET value=2,raw_value='" + `{"value":"2","precision":100}` + "' WHERE id='checkout-coupon-method'")
	if _, e := cart.CompletePaidOrder(ctx, db, "shop", first, firstIn, o); e != cart.ErrCompletionPromotion {
		t.Fatal("changed coupon accepted", e)
	}
	exec("UPDATE promotion_application_methods SET value=1.25,raw_value='" + `{"value":"1.25","precision":100}` + "' WHERE id='checkout-coupon-method'")
	if fingerprint() != before {
		t.Fatal("coupon configuration rejection leaked rows")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	results := make(chan cart.CompletionResult, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := cart.CompletePaidOrder(ctx, db, "shop", first, firstIn, o)
			results <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for e := range errs {
		if e != nil {
			t.Fatal("coupon completion", e)
		}
	}
	var orderID string
	for r := range results {
		if orderID != "" && r.OrderID != orderID {
			t.Fatal("coupon duplicate order")
		}
		orderID = r.OrderID
	}
	var used int64
	if db.Table("promotions").Select("used").Where("id='checkout-coupon'").Scan(&used).Error != nil || used != 1 {
		t.Fatal("coupon usage repeated", used)
	}
	var attr struct{ Used, RawUsed string }
	if db.Table("promotion_budget_usages").Where("budget_id='checkout-coupon-budget' AND deleted_at IS NULL").Take(&attr).Error != nil || attr.Used != "1.000000000000000000" {
		t.Fatal("coupon email budget missing")
	}
	var journals int64
	if db.Table("promotion_usage_operations").Where("scope_id=? AND operation_id=? AND state='registered'", orderID, firstIn.OperationID).Count(&journals).Error != nil || journals != 1 {
		t.Fatal("coupon reversal journal absent")
	}
	after := fingerprint()
	if _, e := cart.CompletePaidOrder(ctx, db, "shop", second, secondIn, o); e != cart.ErrCompletionPromotion {
		t.Fatal("paid second cart bypassed exhausted email budget", e)
	}
	if fingerprint() != after {
		t.Fatal("exhausted budget leaked order/stock/payment")
	}
	if r, e := cart.CompletePaidOrder(ctx, db, "shop", first, firstIn, o); e != nil || !r.Replayed || fingerprint() != after {
		t.Fatal("completed coupon replay recomputed exhausted configuration", e)
	}
	if externalCreates != 2 {
		t.Fatal("completion called external creation")
	}
	t.Log(`CART_PROMOTION_COMPLETION_REHEARSAL {"real_mysql_redis":true,"real_applied_coupon":true,"settlement":"157.19","concurrent_retries":4,"usage_once":true,"email_budget_serialized":true,"late_failure_rolls_back_usage_and_order":true,"changed_configuration_rejected":true,"paid_other_cart_budget_rejected_atomically":true,"reversal_journal":true,"no_external_recharge":true}`)
}
