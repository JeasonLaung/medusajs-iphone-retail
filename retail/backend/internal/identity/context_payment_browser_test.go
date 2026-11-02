package identity_test

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Only external provider calls are artificial. Astro uses production cart HTTP
// handlers and real Redis possession/CSRF plus native MySQL transactions.
type contextBrowserProvider struct {
	mu                                    sync.Mutex
	mode                                  string
	creates, deletes, observes, discovers int
	status                                map[string]string
	drift                                 func()
}

func (p *contextBrowserProvider) CreateSession(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++
	p.status[r.SessionID] = "pending"
	if p.mode == "create-unknown" {
		return payments.CreateUncertain, payments.CreatedSession{}, nil
	}
	return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(r, "tr_"+strings.ReplaceAll(r.SessionID, "-", ""), "https://www.mollie.com/checkout/contextjoint")}, nil
}
func (p *contextBrowserProvider) DeleteSession(_ context.Context, r payments.SessionRequest) (payments.DeleteOutcome, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deletes++
	if p.mode == "delete-unknown" {
		return payments.DeleteUncertain, nil
	}
	p.status[r.SessionID] = "canceled"
	if (p.mode == "restore" || p.mode == "create-unknown") && p.drift != nil {
		p.drift()
	}
	return payments.DeleteSucceeded, nil
}
func (p *contextBrowserProvider) observation(r payments.SessionRequest, id string) payments.SessionObservation {
	status := p.status[r.SessionID]
	if status == "" {
		status = "pending"
	}
	action := map[string]string{"pending": "pending", "canceled": "canceled", "paid": "successful"}[status]
	local := map[string]string{"pending": "pending", "canceled": "canceled", "paid": "authorized"}[status]
	raw, _ := json.Marshal(map[string]any{"id": id, "status": status, "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}, "_links": map[string]any{"checkout": map[string]string{"href": "https://www.mollie.com/checkout/contextjoint"}}})
	return payments.SessionObservation{PaymentID: id, ProviderStatus: status, Action: action, Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: local, Data: raw}}
}
func (p *contextBrowserProvider) ObserveSession(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.observes++
	return p.observation(r, id), nil
}
func (p *contextBrowserProvider) DiscoverSession(_ context.Context, r payments.SessionRequest) (payments.SessionObservation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.discovers++
	if p.mode == "create-unknown" {
		return payments.SessionObservation{}, payments.ErrCreateExecution
	}
	return p.observation(r, "tr_"+strings.ReplaceAll(r.SessionID, "-", "")), nil
}

func checkContextPaymentBrowser(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	t.Helper()
	if os.Getenv("CONTEXT_PAYMENT_BROWSER") != "1" {
		return
	}
	origin, key := os.Getenv("CONTEXT_PAYMENT_ORIGIN"), os.Getenv("CONTEXT_PAYMENT_KEY")
	if !strings.HasPrefix(origin, "http://127.0.0.1:") || len(key) != 64 {
		t.Fatal("owned context browser origin/key required")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context browser SQL", err)
		}
	}
	exec("UPDATE cart_context_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_context_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE commerce_store_locales SET locale_code='en-GB' WHERE store_id='shop' AND locale_code='en'")
	defer exec("UPDATE commerce_store_locales SET locale_code='en' WHERE store_id='shop' AND locale_code='en-GB'")
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
		t.Fatal("browser price")
	}
	defer exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
	adapter := &contextBrowserProvider{status: map[string]string{}}
	adapter.drift = func() {
		exec(`UPDATE pricing_prices SET amount=122,raw_amount='{"value":"122","precision":100}' WHERE id='add-single'`)
	}
	configured := options
	configured.ReturnOrigin = origin
	var err error
	configured.Adapters, err = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter})
	if err != nil {
		t.Fatal(err)
	}
	deletes, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter})
	if err != nil {
		t.Fatal(err)
	}
	shop, err := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: options.ManualProviders, InitialPayment: &configured, ContextPaymentDeletes: deletes})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	shop.Register(router.Group("/api/store"))
	public := router.Group("/api/store")
	catalogue := catalog.StoreHandler{DB: db, StoreID: "shop"}
	public.GET("/regions", (catalog.Handler{DB: db}).Regions)
	public.GET("/locales", catalogue.Locales)
	public.GET("/collections", catalogue.Collections)
	public.GET("/product-categories", catalogue.Categories)
	public.GET("/buy-policies", catalogue.Policies)
	public.GET("/customer-service", catalogue.Contacts)
	fixture := router.Group("/__fixture")
	fixture.Use(func(c *gin.Context) {
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Fixture-Key")), []byte(key)) != 1 {
			c.AbortWithStatus(404)
		}
	})
	var active sessionstore.CartReference
	protectedFacts := func() string {
		var all []string
		for _, table := range []string{"inventory_levels", "inventory_reservations", "orders", "payment_captures", "order_notification_outbox"} {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Error("protected context facts unavailable")
				return "invalid"
			}
			var values []string
			for _, row := range rows {
				raw, _ := json.Marshal(row)
				values = append(values, string(raw))
			}
			sort.Strings(values)
			all = append(all, table+strings.Join(values, "\n"))
		}
		raw, _ := json.Marshal(all)
		return fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	baseline := protectedFacts()
	completed := 0
	finish := make(chan struct{}, 1)
	fixture.POST("/new", func(c *gin.Context) {
		exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
		ref, initial := makeCart()
		active = ref
		issued, e := sessions.IssueCart(ctx, ref)
		if e != nil {
			c.Status(503)
			return
		}
		adapter.mu.Lock()
		adapter.mode = ""
		adapter.creates = 0
		adapter.deletes = 0
		adapter.observes = 0
		adapter.discovers = 0
		adapter.mu.Unlock()
		c.JSON(200, gin.H{"cart_id": ref.CartID, "token": issued.Token, "csrf": issued.CSRF, "version": initial.Version})
	})
	fixture.POST("/mode/:mode", func(c *gin.Context) {
		mode := c.Param("mode")
		if mode != "normal" && mode != "restore" && mode != "delete-unknown" && mode != "create-unknown" && mode != "canceled" && mode != "paid" {
			c.Status(400)
			return
		}
		adapter.mu.Lock()
		adapter.mode = mode
		if mode == "canceled" || mode == "paid" {
			for id := range adapter.status {
				adapter.status[id] = mode
			}
		}
		adapter.mu.Unlock()
		c.Status(200)
	})
	fixture.POST("/price", func(c *gin.Context) {
		exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
		c.Status(200)
	})
	fixture.POST("/stage", func(c *gin.Context) {
		var command cart.ContextCommand
		if c.ShouldBindJSON(&command) != nil {
			c.Status(400)
			return
		}
		staged, e := cart.StageContextPayment(ctx, db, "shop", active, command, configured)
		if e != nil || len(staged.Effects) != 1 {
			t.Error("stage fixture", e)
			c.Status(503)
			return
		}
		_, e = cart.ExecuteContextPaymentSession(ctx, db, "shop", active, command, staged.Effects[0].ResourceID, configured, deletes)
		if e != nil {
			t.Error(e)
			c.Status(503)
			return
		}
		c.Status(200)
	})
	fixture.GET("/facts", func(c *gin.Context) {
		var owner struct {
			Version                                      uint64
			CheckoutAttemptID, PaymentRefreshOperationID *string
		}
		if db.Table("cart_ownership").Where("cart_id=?", active.CartID).Take(&owner).Error != nil {
			c.Status(503)
			return
		}
		var receipts int64
		db.Table("cart_context_operations").Where("cart_id=?", active.CartID).Count(&receipts)
		var attempts []struct {
			InputVersion, CartVersion uint64
			EffectiveCartVersion      *uint64
		}
		db.Table("cart_checkout_attempts").Where("cart_id=?", active.CartID).Order("created_at").Find(&attempts)
		adapter.mu.Lock()
		defer adapter.mu.Unlock()
		c.JSON(200, gin.H{"version": owner.Version, "checkout": owner.CheckoutAttemptID, "refresh": owner.PaymentRefreshOperationID, "receipts": receipts, "attempts": attempts, "creates": adapter.creates, "deletes": adapter.deletes, "observes": adapter.observes, "discovers": adapter.discovers, "protected_unchanged": protectedFacts() == baseline})
	})
	fixture.POST("/case-done", func(c *gin.Context) { completed++; c.Status(200) })
	fixture.POST("/finish", func(c *gin.Context) { finish <- struct{}{}; c.Status(200) })
	listener, err := net.Listen("tcp", "0.0.0.0:9100")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	select {
	case <-finish:
	case <-ctx.Done():
		t.Fatal("context browser deadline")
	}
	expected, err := strconv.Atoi(os.Getenv("CONTEXT_PAYMENT_CASE_COUNT"))
	if err != nil || expected < 1 || completed != expected {
		t.Fatal("context browser cases", completed)
	}
	if protectedFacts() != baseline {
		t.Fatal("context flow changed inventory, orders, captures or mail outbox")
	}
	t.Logf(`CONTEXT_PAYMENT_BROWSER_SQL {"cases":%d,"real_mysql_redis":true,"production_http":true,"artificial_provider_only":true}`, completed)
}
