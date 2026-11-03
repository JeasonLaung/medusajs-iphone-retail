package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Independently callable paid-order subflow. Only the external channel is fake;
// account creation/login, cart transfer, pricing and local completion are real.
func checkCustomerGroupCompletion(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("group completion fixture", err)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return orders.SeedOrderNumbers(ctx, tx, orders.OrderNumberSeed{LastDisplayID: 1000, LastTick: 200000000000000, EvidenceSHA256: sha256.Sum256([]byte("owned group completion baseline"))})
	}); err != nil {
		t.Fatal(err)
	}
	options.ReturnOrigin = "http://127.0.0.1:19200"
	// Preserve the existing anonymous coupon/serialized email budget regression.
	checkPaidOrderPromotionCompletion(t, ctx, db, makeCart, options)
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	exec("UPDATE customer_cart_runtime SET enabled=TRUE WHERE id=1")
	router := gin.New()
	contextGroupFixture(t, db, router.Group("/__fixture"))
	exec("INSERT INTO promotion_campaigns(id,name,campaign_identifier,created_at,updated_at) VALUES('group-completion-campaign','Owned group completion','GROUP_COMPLETION',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE promotions SET campaign_id='group-completion-campaign',`limit`=10 WHERE id='context-member-coupon'")
	exec(`INSERT INTO promotion_campaign_budgets(id,type,campaign_id,attribute,currency_code,` + "`limit`" + `,raw_limit,used,raw_used,created_at,updated_at) VALUES('group-completion-budget','use_by_attribute','group-completion-campaign','customer_id','eur',1,'{"value":"1","precision":100}',0,'{"value":"0","precision":100}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	const origin = "http://127.0.0.1:19200"
	accounts, err := customer.New(db, options.Ring, sessions, origin)
	if err != nil {
		t.Fatal(err)
	}
	const password = "owned-group-completion-password"
	account, err := accounts.Register(ctx, customer.Registration{Email: "context-buyer@example.invalid", Password: password, FirstName: "Owned", LastName: "Member", Country: "dk", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE customer_accounts SET verified_at=UTC_TIMESTAMP(6) WHERE id=?", account.ID)
	login, err := accounts.Login(ctx, "context-buyer@example.invalid", password)
	if err != nil {
		t.Fatal(err)
	}
	member := func(enabled bool) {
		t.Helper()
		if err := db.Transaction(func(tx *gorm.DB) error {
			if e := tx.Exec("DELETE FROM customer_group_members WHERE group_id='context-frozen-group'").Error; e != nil {
				return e
			}
			if enabled {
				if e := tx.Exec("INSERT INTO customer_group_members(group_id,customer_id,created_at) VALUES('context-frozen-group',?,UTC_TIMESTAMP(6))", account.ID).Error; e != nil {
					return e
				}
			}
			return tx.Exec("UPDATE customer_group_runtime SET revision=revision+1 WHERE id=1").Error
		}); err != nil {
			t.Fatal(err)
		}
	}
	member(true)
	adapter := &contextBrowserProvider{status: map[string]string{}}
	o := options
	o.ReturnOrigin = origin
	o.Adapters, err = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: o.ManualProviders, InitialPayment: &o})
	if err != nil {
		t.Fatal(err)
	}
	handler.Register(router.Group("/api/store"))
	server := httptest.NewServer(router)
	defer server.Close()
	type prepared struct {
		ref         sessionstore.CartReference
		token, csrf string
	}
	prepare := func() prepared {
		t.Helper()
		ref, _ := makeCart()
		issued, e := sessions.IssueCart(ctx, ref)
		if e != nil {
			t.Fatal(e)
		}
		if result, e := accounts.TransferCart(ctx, login.Session.Token, issued.Token, "shop", cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: "4"}, o.ManualProviders, o); e != nil || result.Version != "5" {
			t.Fatal("member transfer", result, e)
		}
		if result, e := cart.SavePromotions(ctx, db, "shop", ref, cart.PromotionCommand{OperationID: uuid.NewString(), Version: "5", Codes: []string{"MEMBER10"}}, o.ManualProviders); e != nil || result.Version != "6" {
			t.Fatal("member coupon", result, e)
		}
		in := cart.InitialPaymentCommand{OperationID: uuid.NewString(), Version: "6"}
		if result, e := cart.StartInitialPayment(ctx, db, "shop", ref, in, o); e != nil || result.State != "ready" {
			t.Fatal("member payment", result, e)
		}
		adapter.mu.Lock()
		for id := range adapter.status {
			adapter.status[id] = "paid"
		}
		adapter.mu.Unlock()
		if result, e := cart.ObserveInitialPayment(ctx, db, "shop", ref, o); e != nil || result.State != "paid_pending_order" {
			t.Fatal("member paid observation", result, e)
		}
		return prepared{ref, issued.Token, issued.CSRF}
	}
	first, second := prepare(), prepare()
	fingerprint := func() string {
		t.Helper()
		var all []any
		for _, table := range []string{"orders", "order_carts", "order_items", "order_line_items", "order_line_adjustments", "payments", "payment_captures", "order_transactions", "order_summaries", "inventory_levels", "inventory_reservations", "inventory_reservation_operations", "cart_completion_receipts", "order_notification_outbox", "order_number_allocator", "cart_ownership", "customer_order_ownership", "promotions", "promotion_campaign_budgets", "promotion_budget_usages", "promotion_usage_operations"} {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("completion fingerprint")
			}
			sort.Slice(rows, func(i, j int) bool {
				a, _ := json.Marshal(rows[i])
				b, _ := json.Marshal(rows[j])
				return string(a) < string(b)
			})
			all = append(all, rows)
		}
		raw, _ := json.Marshal(all)
		return string(raw)
	}
	complete := func(p prepared) (int, []byte, error) {
		req, e := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/store/carts/"+p.ref.CartID+"/complete", strings.NewReader("{}"))
		if e != nil {
			return 0, nil, e
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-CSRF-Token", p.csrf)
		req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: p.token})
		response, e := server.Client().Do(req)
		if e != nil {
			return 0, nil, e
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(response.Body)
		return response.StatusCode, raw, e
	}
	before := fingerprint()
	member(false)
	if code, _, e := complete(first); e != nil || code != 409 || fingerprint() != before {
		t.Fatal("removed member must reject paid completion atomically", code, e)
	}
	member(true)
	exec("CREATE TRIGGER group_completion_late_failure BEFORE INSERT ON cart_completion_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned group completion late failure'")
	if code, _, e := complete(first); e != nil || code < 400 || fingerprint() != before {
		t.Fatal("late completion failure leaked usage/order/stock", code, e)
	}
	exec("DROP TRIGGER group_completion_late_failure")
	var wg sync.WaitGroup
	type result struct {
		code int
		raw  []byte
		err  error
	}
	results := make(chan result, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); code, raw, e := complete(first); results <- result{code, raw, e} }()
	}
	wg.Wait()
	close(results)
	orderID := ""
	for r := range results {
		var body struct{ OrderID, Version string }
		// Public response uses snake_case order_id.
		var decoded map[string]string
		if r.err != nil || r.code != 200 || json.Unmarshal(r.raw, &decoded) != nil {
			t.Fatal("concurrent group completion", r.code, r.err, string(r.raw))
		}
		body.OrderID, body.Version = decoded["order_id"], decoded["version"]
		if body.OrderID == "" || body.Version != "8" || orderID != "" && orderID != body.OrderID {
			t.Fatal("duplicate member order", body)
		}
		orderID = body.OrderID
	}
	var used int64
	if db.Table("promotions").Select("used").Where("id='context-member-coupon'").Scan(&used).Error != nil || used != 1 {
		t.Fatal("group usage must be once", used)
	}
	var budget struct{ AttributeValue, Used string }
	if db.Table("promotion_budget_usages").Where("budget_id='group-completion-budget' AND deleted_at IS NULL").Take(&budget).Error != nil || budget.AttributeValue != account.ID || budget.Used != "1.000000000000000000" {
		t.Fatal("trusted customer budget not consumed once", budget)
	}
	var owners int64
	if db.Table("customer_order_ownership").Where("order_id=? AND customer_id=? AND cart_id=?", orderID, account.ID, first.ref.CartID).Count(&owners).Error != nil || owners != 1 {
		t.Fatal("member order ownership")
	}
	counts := map[string]int64{}
	for key, query := range map[string]*gorm.DB{
		"usage_journal":          db.Table("promotion_usage_operations").Where("scope_id=? AND state='registered'", orderID),
		"completion_receipt":     db.Table("cart_completion_receipts").Where("order_id=? AND cart_id=?", orderID, first.ref.CartID),
		"paid_capture":           db.Table("payment_captures c").Joins("JOIN cart_completion_receipts r ON r.capture_id=c.id AND r.payment_id=c.payment_id").Where("r.order_id=? AND c.deleted_at IS NULL AND c.amount=121.25 AND JSON_UNQUOTE(JSON_EXTRACT(c.raw_amount,'$.value'))='121.25'", orderID),
		"order_transaction":      db.Table("order_transactions").Where("order_id=? AND amount=121.25 AND currency_code='eur'", orderID),
		"frozen_coupon_snapshot": db.Table("order_line_adjustments a").Joins("JOIN order_items i ON i.item_id=a.item_id AND i.version=a.version").Where("i.order_id=? AND a.promotion_id='context-member-coupon' AND a.code='MEMBER10' AND a.amount=10 AND a.deleted_at IS NULL", orderID),
		"notification":           db.Table("order_notification_outbox").Where("order_id=? AND event='order.placed'", orderID),
	} {
		var n int64
		if query.Count(&n).Error != nil || n != 1 {
			t.Fatal("missing/duplicate group completion fact", key, n)
		}
		counts[key] = n
	}
	after := fingerprint()
	if code, _, e := complete(second); e != nil || code != 409 || fingerprint() != after {
		t.Fatal("second paid member cart bypassed budget", code, e)
	}
	member(false)
	if code, raw, e := complete(first); e != nil || code != 200 || !strings.Contains(string(raw), orderID) || fingerprint() != after {
		t.Fatal("lost completion response replay changed facts", code, e)
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if adapter.creates != 2 || adapter.deletes != 0 || len(adapter.events) != 2 {
		t.Fatal("completion recharged member", adapter.events)
	}
	for _, event := range adapter.events {
		if event.Amount != "121.25" || event.Currency != "eur" {
			t.Fatal("member quote mismatch", event)
		}
	}
	rawFacts, err := json.Marshal(map[string]any{"order_id": orderID, "cart_id": first.ref.CartID, "customer_id": account.ID, "promotion_used": used, "customer_budget_used": budget.Used, "order_owner_count": owners, "counts": counts, "provider_events": adapter.events})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("CUSTOMER_GROUP_COMPLETION_FACTS " + string(rawFacts))
	t.Log(`CUSTOMER_GROUP_COMPLETION_SQL {"real_mysql_redis":true,"production_completion_http":true,"artificial_provider_only":true,"new_account_group_coupon":true,"settlement":"121.25","concurrent_retries":6,"usage_once":true,"customer_budget_once":true,"removed_member_rejected_atomically":true,"late_failure_rolls_back":true,"second_paid_cart_budget_rejected_atomically":true,"lost_response_replay_after_member_removal":true,"order_owner_once":true,"no_external_recharge":true}`)
}
