package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Fixed cart subflow, not a complete shopping/fulfillment run. All channel
// responses are artificial; the owning order/stock/cash transaction is real.
func checkPayPalCompletion(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("PayPal completion SQL", err)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return orders.SeedOrderNumbers(ctx, tx, orders.OrderNumberSeed{LastDisplayID: 1000, LastTick: 200000000000000, EvidenceSHA256: sha256.Sum256([]byte("owned PayPal checkout fixed source snapshot"))})
	}); err != nil {
		t.Fatal("owned numbering seed", err)
	}
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	fingerprint := func() string {
		t.Helper()
		var parts []any
		for _, table := range []string{"orders", "order_carts", "payments", "payment_captures", "order_transactions", "order_summaries", "inventory_levels", "inventory_reservations", "inventory_reservation_operations", "cart_completion_receipts", "order_notification_outbox", "order_number_allocator", "cart_ownership"} {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("completion fingerprint")
			}
			sort.Slice(rows, func(i, j int) bool {
				a, _ := json.Marshal(rows[i])
				b, _ := json.Marshal(rows[j])
				return string(a) < string(b)
			})
			parts = append(parts, rows)
		}
		raw, _ := json.Marshal(parts)
		return string(raw)
	}
	for _, provider := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
		for _, intent := range []string{"AUTHORIZE", "CAPTURE"} {
			t.Run(provider+"/complete-"+intent, func(t *testing.T) {
				adapter := &paypalCheckoutProvider{calls: map[string]int{}, intent: intent, orderID: strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:17]}
				o := options
				o.ReturnOrigin = "http://127.0.0.1:19200"
				o.Adapters, _ = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: adapter, "pp_mollie-hosted-checkout_mollie": adapter})
				ref, in := makeCart()
				in.ProviderID = provider
				if _, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, o); err != nil {
					t.Fatal(err)
				}
				if _, err := cart.SettlePayPal(ctx, db, "shop", ref, o); err != nil {
					t.Fatal(err)
				}
				handler, e := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: o.ReturnOrigin, StoreID: "shop", ManualProviderIDs: o.ManualProviders, InitialPayment: &o})
				if e != nil {
					t.Fatal(e)
				}
				router := gin.New()
				handler.Register(router.Group("/api/store"))
				server := httptest.NewServer(router)
				defer server.Close()
				issued, e := sessions.IssueCart(ctx, ref)
				if e != nil {
					t.Fatal(e)
				}
				completeHTTP := func(token, csrf, origin, body string) (int, []byte) {
					req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/store/carts/"+ref.CartID+"/complete", strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Origin", origin)
					req.Header.Set("X-CSRF-Token", csrf)
					req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: token})
					response, e := server.Client().Do(req)
					if e != nil {
						t.Fatal(e)
					}
					defer response.Body.Close()
					raw, _ := io.ReadAll(response.Body)
					return response.StatusCode, raw
				}
				before := fingerprint()
				for _, bad := range []struct{ token, csrf, origin, body string }{
					{issued.Token, issued.CSRF, "http://foreign.invalid", "{}"},
					{issued.Token, strings.Repeat("a", 64), o.ReturnOrigin, "{}"},
					{strings.Repeat("b", 64), issued.CSRF, o.ReturnOrigin, "{}"},
					{issued.Token, issued.CSRF, o.ReturnOrigin, `{"provider_id":"pp_paypal_paypal"}`},
				} {
					if code, _ := completeHTTP(bad.token, bad.csrf, bad.origin, bad.body); code < 400 {
						t.Fatal("invalid completion HTTP guard")
					}
				}
				if fingerprint() != before {
					t.Fatal("guard changed completion facts")
				}
				if intent == "AUTHORIZE" {
					if _, err := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o); err == nil {
						t.Fatal("authorization counted as cash")
					}
					if fingerprint() != before {
						t.Fatal("authorization rejection changed facts")
					}
					return
				}
				wrong := in
				wrong.ProviderID = "pp_mollie-hosted-checkout_mollie"
				if _, err := cart.CompletePaidOrder(ctx, db, "shop", ref, wrong, o); err == nil {
					t.Fatal("cross-provider completion accepted")
				}
				if _, err := cart.CompleteCurrentPaidOrder(ctx, db, "shop", sessionstore.CartReference{CartID: ref.CartID, Generation: uuid.NewString()}, o); err == nil {
					t.Fatal("foreign possession accepted")
				}
				exec("CREATE TRIGGER paypal_completion_fault BEFORE INSERT ON cart_completion_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned paypal completion rollback'")
				_, err := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o)
				exec("DROP TRIGGER paypal_completion_fault")
				if err == nil || fingerprint() != before {
					t.Fatal("late failure leaked order/stock/cash/outbox/number/version")
				}
				var wg sync.WaitGroup
				results := make(chan cart.CompletionResult, 6)
				errs := make(chan error, 6)
				for i := 0; i < 6; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						r, e := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o)
						results <- r
						errs <- e
					}()
				}
				wg.Wait()
				close(results)
				close(errs)
				for err := range errs {
					if err != nil {
						t.Fatal("parallel completion", err)
					}
				}
				orderID := ""
				fresh := 0
				for r := range results {
					if r.OrderID == "" || r.Version != "6" || (orderID != "" && orderID != r.OrderID) {
						t.Fatal("duplicate/missing order", r)
					}
					orderID = r.OrderID
					if !r.Replayed {
						fresh++
					}
				}
				if fresh != 1 {
					t.Fatal("fresh completion count", fresh)
				}
				after := fingerprint()
				// A return-page retry may have lost its previous success response.
				for i := 0; i < 2; i++ {
					code, raw := completeHTTP(issued.Token, issued.CSRF, o.ReturnOrigin, "{}")
					var reply struct {
						CartID  string `json:"cart_id"`
						OrderID string `json:"order_id"`
						Version string `json:"version"`
					}
					if code != 200 || json.Unmarshal(raw, &reply) != nil || reply.CartID != ref.CartID || reply.OrderID != orderID || reply.Version != "6" {
						t.Fatalf("completion HTTP replay: %d %s", code, raw)
					}
					if fingerprint() != after {
						t.Fatal("HTTP receipt replay changed facts")
					}
				}
				if r, e := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o); e != nil || !r.Replayed || r.OrderID != orderID || fingerprint() != after {
					t.Fatal("closed cart replay", e)
				}
				var reservations int64
				if db.Table("inventory_reservations r").Joins("JOIN order_items i ON i.item_id=r.line_item_id AND i.deleted_at IS NULL").Where("i.order_id=? AND r.deleted_at IS NULL AND r.quantity=2.5", orderID).Count(&reservations).Error != nil || reservations != 1 {
					t.Fatal("missing unique order reservation")
				}
				var payment struct{ ID, ProviderID, Data, CurrencyCode, Amount string }
				if db.Table("payments p").Select("p.*").Joins("JOIN order_payment_collections c ON c.payment_collection_id=p.payment_collection_id").Where("c.order_id=?", orderID).Take(&payment).Error != nil || payment.ProviderID != provider || payment.CurrencyCode != "eur" || payment.Amount != "158.750000000000000000" {
					t.Fatal("wrong payment provider")
				}
				if !strings.HasPrefix(payment.Data, "ers1.") || strings.Contains(payment.Data, adapter.orderID) {
					t.Fatal("unprotected provider payload")
				}
				envelope, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(payment.Data, "ers1."))
				if e != nil {
					t.Fatal(e)
				}
				raw, e := o.Ring.Open(protected.Context{Purpose: "checkout-payment-v1", Table: "payments", Column: "data", Row: payment.ID}, envelope)
				if e != nil || !strings.Contains(string(raw), adapter.orderID) {
					t.Fatal("cash proof not bound to payment")
				}
				clear(raw)
				for _, table := range []string{"payment_captures", "order_transactions", "order_notification_outbox"} {
					var count int64
					q := db.Table(table)
					if table == "payment_captures" {
						q = q.Where("payment_id=?", payment.ID)
					} else {
						q = q.Where("order_id=?", orderID)
					}
					if q.Count(&count).Error != nil || count != 1 {
						t.Fatal("missing/duplicate local cash/outbox", table, count)
					}
				}
				adapter.mu.Lock()
				dispatches := adapter.settlements
				adapter.mu.Unlock()
				if dispatches != 1 {
					t.Fatal("completion called external channel")
				}
			})
		}
	}
	t.Log("PAYPAL_COMPLETION_SQL: wallet/card full capture, authorization not cash, wrong-provider/possession rejected, late whole-transaction rollback, six parallel completions, closed cart receipt, provider-bound encrypted cash, one capture/outbox, actual HTTP guards and closed-cart replay")
}
