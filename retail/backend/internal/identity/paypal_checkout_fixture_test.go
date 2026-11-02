package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type paypalCheckoutProvider struct {
	mu        sync.Mutex
	calls     map[string]int
	uncertain bool
}

func (p *paypalCheckoutProvider) CreateSession(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls[r.SessionID]++
	if p.uncertain {
		return payments.CreateUncertain, payments.CreatedSession{}, payments.ErrCreateExecution
	}
	var data map[string]string
	_ = json.Unmarshal(r.Data, &data)
	raw, _ := json.Marshal(map[string]any{"cart_id": data["cart_id"], "session_id": r.SessionID, "paypal": map[string]any{"order_id": "5O190127TN364715T", "private": "not-for-browser"}})
	return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: raw}, nil
}

func checkPayPalCheckout(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	t.Helper()
	const wallet = "pp_paypal_paypal"
	const card = "pp_paypal_card_paypal_card"
	const origin = "http://127.0.0.1:19200"
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("PayPal fixed fixture SQL failed")
		}
	}
	for _, id := range []string{wallet, card} {
		exec("INSERT INTO payment_providers(id,is_enabled,created_at,updated_at) VALUES(?,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id)
		exec("INSERT INTO commerce_region_payment_providers(id,region_id,payment_provider_id,created_at,updated_at) VALUES(?,'region',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "binding-"+id, id)
	}
	protected := func() [32]byte {
		var rows []any
		for _, table := range []string{"inventory_levels", "inventory_reservations", "orders", "payment_captures", "order_notification_outbox"} {
			var part []map[string]any
			if db.Table(table).Find(&part).Error != nil {
				t.Fatal("protected facts unavailable")
			}
			rows = append(rows, part)
		}
		raw, _ := json.Marshal(rows)
		return sha256.Sum256(raw)
	}
	before := protected()
	for _, provider := range []string{wallet, card} {
		for _, unknown := range []bool{false, true} {
			t.Run(provider+map[bool]string{false: "/success", true: "/uncertain"}[unknown], func(t *testing.T) {
				adapter := &paypalCheckoutProvider{calls: map[string]int{}, uncertain: unknown}
				configured := options
				configured.ReturnOrigin = origin
				var e error
				configured.Adapters, e = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{wallet: adapter, card: adapter, "pp_mollie-hosted-checkout_mollie": adapter})
				if e != nil {
					t.Fatal(e)
				}
				handler, e := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: options.ManualProviders, InitialPayment: &configured})
				if e != nil {
					t.Fatal(e)
				}
				router := gin.New()
				handler.Register(router.Group("/api/store"))
				server := httptest.NewServer(router)
				defer server.Close()
				ref, in := makeCart()
				in.ProviderID = provider
				issued, e := sessions.IssueCart(ctx, ref)
				if e != nil {
					t.Fatal(e)
				}
				call := func(body, key, token, csrf, requestOrigin string) (int, []byte) {
					req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/store/carts/"+ref.CartID+"/payment-attempts", strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Idempotency-Key", key)
					req.Header.Set("Origin", requestOrigin)
					req.Header.Set("X-CSRF-Token", csrf)
					req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: token})
					response, err := server.Client().Do(req)
					if err != nil {
						t.Error("private HTTP call failed")
						return 0, nil
					}
					defer response.Body.Close()
					raw, _ := io.ReadAll(response.Body)
					return response.StatusCode, raw
				}
				body := `{"version":"4","provider_id":"` + provider + `"}`
				// Security failures must occur before an attempt/effect exists.
				for _, bad := range []struct{ token, csrf, origin string }{{issued.Token, issued.CSRF, "http://foreign.invalid"}, {issued.Token, strings.Repeat("a", 64), origin}, {strings.Repeat("b", 64), issued.CSRF, origin}} {
					code, _ := call(body, in.OperationID, bad.token, bad.csrf, bad.origin)
					if code < 400 {
						t.Fatal("private HTTP guard accepted invalid capability")
					}
				}
				var empty int64
				db.Table("cart_checkout_attempts").Where("cart_id=?", ref.CartID).Count(&empty)
				if empty != 0 {
					t.Fatal("guard failure staged payment")
				}
				code, raw := call(body, in.OperationID, issued.Token, issued.CSRF, origin)
				if code != 200 {
					t.Fatalf("create HTTP %d %s", code, raw)
				}
				var result cart.InitialPaymentResult
				if json.Unmarshal(raw, &result) != nil {
					t.Fatal("invalid payment result")
				}
				if result.Version != "5" || result.CheckoutURL != nil || strings.Contains(string(raw), "not-for-browser") {
					t.Fatal("wrong private projection", string(raw))
				}
				if unknown {
					if result.State != "requires_review" || result.ProviderOrderID != nil {
						t.Fatal("uncertain exposed an order")
					}
				} else if result.State != "sdk_ready" || result.ProviderID != provider || result.ProviderOrderID == nil || *result.ProviderOrderID != "5O190127TN364715T" {
					t.Fatal("SDK order projection missing")
				}
				// Six concurrent replays use the original command and durable result.
				var wg sync.WaitGroup
				for i := 0; i < 6; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						status, again := call(body, in.OperationID, issued.Token, issued.CSRF, origin)
						if status != 200 || string(again) != string(raw) {
							t.Errorf("concurrent replay changed %d %s", status, again)
						}
					}()
				}
				wg.Wait()
				for _, other := range []string{wallet, card, "pp_mollie-hosted-checkout_mollie"} {
					if other == provider {
						continue
					}
					status, _ := call(`{"version":"4","provider_id":"`+other+`"}`, in.OperationID, issued.Token, issued.CSRF, origin)
					if status < 400 {
						t.Fatal("same UUID switched provider")
					}
				}
				status, _ := call(`{"version":"4"}`, in.OperationID, issued.Token, issued.CSRF, origin)
				if status < 400 {
					t.Fatal("legacy Mollie command consumed PayPal receipt")
				}
				status, _ = call(body, uuid.NewString(), issued.Token, issued.CSRF, origin)
				if status < 400 {
					t.Fatal("new UUID bypassed frozen checkout")
				}
				var row struct {
					SessionID, CollectionID   string
					InputVersion, CartVersion uint64
					Activated                 bool
					RequestSHA256             []byte
				}
				if db.Table("cart_checkout_attempts").Where("cart_id=?", ref.CartID).Take(&row).Error != nil || row.InputVersion != 4 || row.CartVersion != 5 || row.Activated == unknown {
					t.Fatal("immutable attempt mismatch")
				}
				adapter.mu.Lock()
				calls := adapter.calls[row.SessionID]
				adapter.mu.Unlock()
				if calls != 1 {
					t.Fatal("provider CREATE repeated", calls)
				}
				var session struct{ ProviderID, Status, Data string }
				if db.Table("payment_sessions").Where("id=?", row.SessionID).Take(&session).Error != nil || session.ProviderID != provider || session.Status != "pending" || strings.Contains(session.Data, "not-for-browser") {
					t.Fatal("session facts not protected")
				}
				key := payments.EffectKey{ScopeID: ref.CartID, OperationID: in.OperationID, Kind: "create_session", ResourceID: row.SessionID}
				copy(key.Digest[:], row.RequestSHA256)
				if e = db.Transaction(func(tx *gorm.DB) error {
					request, err := payments.ReadSessionRequest(ctx, tx, key, options.Ring)
					if err != nil {
						return err
					}
					if request.ProviderID != provider || request.Amount != "158.75" || request.Currency != "eur" || strings.Contains(string(request.Data), "mollie-return") {
						t.Error("persisted request changed")
					}
					receipt, err := payments.InspectEffect(ctx, tx, key)
					if err != nil {
						return err
					}
					want := "succeeded"
					if unknown {
						want = "uncertain"
					}
					if receipt.State != want {
						t.Error("wrong durable effect state")
					}
					return nil
				}); e != nil {
					t.Fatal(e)
				}
				// Creation does not authorize mutations or money/order facts.
				if _, e = cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "5", VariantID: "add-v", Quantity: "1"}); e == nil {
					t.Fatal("frozen PayPal cart editable")
				}
			})
		}
	}
	if protected() != before {
		t.Fatal("checkout creation changed stock/order/capture/notification facts")
	}
	t.Log("PAYPAL_CHECKOUT_SQL: wallet/card success/unknown, HTTP guards, six replays, provider binding, frozen cart, encrypted durable requests; CREATE once per cart; protected facts unchanged")
}
