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
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Only external provider calls are artificial. Astro uses production cart HTTP
// handlers and real Redis possession/CSRF plus native MySQL transactions.
type contextBrowserProvider struct {
	mu                                    sync.Mutex
	mode                                  string
	creates, deletes, observes, discovers int
	status                                map[string]string
	events                                []contextProviderEvent
	drift                                 func()
}

type contextProviderEvent struct {
	Kind, Amount, Currency string
}

type contextPayPalRetirer struct {
	provider *paypalCheckoutProvider
	mode     *contextBrowserProvider
	calls    int
}

func (p *contextPayPalRetirer) DeleteSession(ctx context.Context, r payments.SessionRequest) (payments.DeleteOutcome, error) {
	p.calls++
	id, err := payments.PayPalCreatedOrderID(r, payments.CreatedSession{Status: "pending", Data: r.Data})
	if err != nil {
		return payments.DeleteUncertain, err
	}
	observed, err := p.provider.ObserveSession(ctx, r, id)
	if err != nil {
		return payments.DeleteUncertain, err
	}
	out, err := payments.PayPalRetirementOutcome(r, id, observed)
	p.mode.mu.Lock()
	mode := p.mode.mode
	p.mode.mu.Unlock()
	if mode == "restore" && out == payments.DeleteSucceeded {
		p.provider.mu.Lock()
		p.provider.orderID = strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:17]
		p.provider.mu.Unlock()
		p.mode.drift()
	}
	if mode == "delete-unknown" {
		return payments.DeleteUncertain, payments.ErrDeleteExecution
	}
	return out, err
}

// Independent browser cases receive independent rate-limit keys while keeping
// the real Redis limiter, its eight-attempt limit and TTL. Session/CSRF keys
// still use the original store. Reusing one loopback IP across the whole matrix
// must not consume a later case's fresh fixture budget.
type contextBrowserCustomerSessions struct {
	*sessionstore.Store
	rateScope atomic.Uint64
}

func (s *contextBrowserCustomerSessions) AllowLogin(ctx context.Context, realm sessionstore.Realm, identity string) (bool, error) {
	return s.Store.AllowLogin(ctx, realm, strconv.FormatUint(s.rateScope.Load(), 10)+":"+identity)
}

func (p *contextBrowserProvider) CreateSession(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++
	p.events = append(p.events, contextProviderEvent{"create", r.Amount, r.Currency})
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
	p.events = append(p.events, contextProviderEvent{"delete", r.Amount, r.Currency})
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
	action := map[string]string{"pending": "pending", "canceled": "canceled", "paid": "successful", "authorized": "authorized"}[status]
	local := map[string]string{"pending": "pending", "canceled": "canceled", "paid": "authorized", "authorized": "authorized"}[status]
	raw, _ := json.Marshal(map[string]any{"id": id, "status": status, "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}, "_links": map[string]any{"checkout": map[string]string{"href": "https://www.mollie.com/checkout/contextjoint"}}})
	if status == "paid" || status == "authorized" {
		// Completed provider sessions need not retain a checkout link.
		var data map[string]any
		_ = json.Unmarshal(raw, &data)
		delete(data, "_links")
		raw, _ = json.Marshal(data)
	}
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
		exec(`UPDATE pricing_prices SET amount=159.75,raw_amount='{"value":"159.75","precision":100}' WHERE id='add-cad'`)
	}
	paypalContext := os.Getenv("PAYPAL_CONTEXT_BROWSER") == "1"
	paypal := &paypalCheckoutProvider{calls: map[string]int{}}
	retirer := &contextPayPalRetirer{provider: paypal, mode: adapter}
	if paypalContext {
		for _, id := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
			exec("INSERT INTO payment_providers(id,is_enabled,created_at,updated_at) VALUES(?,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id)
			exec("INSERT INTO commerce_region_payment_providers(id,region_id,payment_provider_id,created_at,updated_at) VALUES(?,'region',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "context-"+id, id)
		}
	}
	configured := options
	configured.ReturnOrigin = origin
	var err error
	entries := map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter}
	if paypalContext {
		entries["pp_paypal_paypal"] = paypal
		entries["pp_paypal_card_paypal_card"] = paypal
	}
	configured.Adapters, err = payments.NewCreateAdapters(entries)
	if err != nil {
		t.Fatal(err)
	}
	deleteEntries := map[string]payments.DeleteSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter}
	if paypalContext {
		deleteEntries["pp_paypal_paypal"] = retirer
		deleteEntries["pp_paypal_card_paypal_card"] = retirer
	}
	deletes, err := payments.NewDeleteAdapters(deleteEntries)
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
	accounts, err := customer.New(db, options.Ring, sessions, origin)
	if err != nil {
		t.Fatal(err)
	}
	customerSessions := &contextBrowserCustomerSessions{Store: sessions}
	customerHTTP, err := customer.NewHTTP(db, customer.HTTPOptions{ContextPaymentDeletes: deletes, InitialPayment: &configured, Sessions: customerSessions, Ring: options.Ring, Origin: origin, StoreID: "shop", BridgeKey: key, ManualProviderIDs: options.ManualProviders})
	if err != nil {
		t.Fatal(err)
	}
	customerHTTP.Register(router.Group("/api/store/customer"))
	exec("UPDATE customer_cart_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE customer_cart_runtime SET enabled=FALSE WHERE id=1")
	const password = "context-owned-customer-password"
	for _, email := range []string{"context-buyer@example.invalid", "context-other@example.invalid"} {
		a, e := accounts.Register(ctx, customer.Registration{Email: email, Password: password, FirstName: "Owned", LastName: "Buyer", Country: "dk", Locale: "en"})
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE customer_accounts SET verified_at=UTC_TIMESTAMP(6) WHERE id=?", a.ID)
	}
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
	resetGroups := contextGroupFixture(t, db, fixture)
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
		customerSessions.rateScope.Add(1)
		resetGroups()
		exec(`UPDATE pricing_prices SET amount=158.75,raw_amount='{"value":"158.75","precision":100}' WHERE id='add-cad'`)
		exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
		ref, initial := makeCart()
		active = ref
		if paypalContext {
			retirer.calls = 0
			paypal.mu.Lock()
			paypal.calls = map[string]int{}
			paypal.status = ""
			paypal.settlements = 0
			paypal.orderID = strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:17]
			paypal.mu.Unlock()
		}
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
		adapter.events = nil
		adapter.mu.Unlock()
		c.JSON(200, gin.H{"cart_id": ref.CartID, "token": issued.Token, "csrf": issued.CSRF, "version": initial.Version, "account_email": "context-buyer@example.invalid", "other_email": "context-other@example.invalid", "password": password})
	})
	fixture.POST("/mode/:mode", func(c *gin.Context) {
		mode := c.Param("mode")
		if mode != "normal" && mode != "restore" && mode != "delete-unknown" && mode != "create-unknown" && mode != "canceled" && mode != "paid" && mode != "replacement-paid" && mode != "replacement-authorized" {
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
		if mode == "replacement-paid" || mode == "replacement-authorized" {
			for id, status := range adapter.status {
				if status == "pending" {
					adapter.status[id] = strings.TrimPrefix(mode, "replacement-")
				}
			}
		}
		adapter.mu.Unlock()
		c.Status(200)
	})
	fixture.POST("/price", func(c *gin.Context) {
		exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
		c.Status(200)
	})
	fixture.POST("/original-price", func(c *gin.Context) {
		exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
		c.Status(200)
	})
	fixture.POST("/precision-price", func(c *gin.Context) {
		exec(`UPDATE pricing_prices SET amount=120,raw_amount='{"value":"120.000000000000000000001","precision":100}' WHERE id='add-single'`)
		c.Status(200)
	})
	fixture.POST("/edited-email", func(c *gin.Context) {
		address := cart.ContactAddress{FirstName: "Owned", LastName: "Buyer", Address1: "Test street 1", City: "Copenhagen", PostalCode: "0012", Country: "dk", Phone: "+45 111"}
		_, e := cart.SaveContacts(ctx, db, "shop", active, cart.ContactCommand{OperationID: uuid.NewString(), Version: "5", Email: "edited-checkout@example.invalid", Shipping: address, Billing: address}, options.ManualProviders)
		if e != nil {
			t.Error("owned contact edit", e)
			c.Status(503)
			return
		}
		c.Status(200)
	})
	fixture.POST("/frozen-transfer-audit", func(c *gin.Context) {
		if err := auditFrozenCustomerTransfer(ctx, db, accounts, sessions, active, options.ManualProviders); err != nil {
			t.Error("frozen transfer audit", err)
			c.Status(503)
			return
		}
		c.Status(200)
	})
	fixture.POST("/first-attachment-audit", func(c *gin.Context) {
		if err := auditFirstPaymentAttachment(ctx, db, accounts, sessions, active, configured); err != nil {
			t.Error("first attachment audit", err)
			c.Status(503)
			return
		}
		c.Status(200)
	})
	fixture.POST("/transfer-receipt-fault/:enabled", func(c *gin.Context) {
		if c.Param("enabled") == "yes" {
			exec("CREATE TRIGGER transfer_receipt_fault BEFORE INSERT ON customer_cart_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned transfer rollback'")
		} else {
			exec("DROP TRIGGER transfer_receipt_fault")
		}
		c.Status(200)
	})
	fixture.POST("/transfer-scope-audit", func(c *gin.Context) {
		var in cart.CustomerTransferCommand
		if c.ShouldBindJSON(&in) != nil {
			c.Status(400)
			return
		}
		login, e := accounts.Login(ctx, "context-other@example.invalid", password)
		if e != nil {
			t.Error(e)
			c.Status(503)
			return
		}
		defer sessions.Revoke(context.WithoutCancel(ctx), sessionstore.Customer, login.Session.Token)
		possession, e := sessions.IssueCart(ctx, active)
		if e != nil {
			t.Error(e)
			c.Status(503)
			return
		}
		defer sessions.RevokeCart(context.WithoutCancel(ctx), possession.Token)
		before, e := transferTableFingerprint(db)
		if e != nil {
			t.Error(e)
			c.Status(503)
			return
		}
		_, foreign := accounts.ContinuePaymentCart(ctx, login.Session.Token, possession.Token, "shop", in, configured, deletes, false)
		var current struct{ RegionID, Locale string }
		if db.Table("carts").Where("id=?", active.CartID).Take(&current).Error != nil {
			c.Status(503)
			return
		}
		_, plain := cart.ContinueContextPayment(ctx, db, "shop", active, cart.ContextCommand{OperationID: in.OperationID, Version: in.Version, RegionID: current.RegionID, Locale: current.Locale}, configured, deletes)
		after, e := transferTableFingerprint(db)
		if foreign == nil || plain == nil || e != nil || before != after {
			t.Error("transfer scope bypass or mutation", foreign, plain, e)
			c.Status(503)
			return
		}
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
		var transferReceipts, accountOwners int64
		db.Table("customer_cart_operations").Where("cart_id=?", active.CartID).Count(&transferReceipts)
		db.Table("customer_cart_ownership").Where("cart_id=?", active.CartID).Count(&accountOwners)
		var email string
		db.Table("carts").Select("email").Where("id=?", active.CartID).Scan(&email)
		var attempts []struct {
			InputVersion, CartVersion uint64
			EffectiveCartVersion      *uint64
		}
		db.Table("cart_checkout_attempts").Where("cart_id=?", active.CartID).Order("created_at").Find(&attempts)
		var collections []struct{ CurrencyCode, RawAmount string }
		if db.Table("payment_collections p").Select("p.currency_code,p.raw_amount").Joins("JOIN cart_payment_collections l ON l.payment_collection_id=p.id").Where("l.cart_id=? AND l.deleted_at IS NULL AND p.deleted_at IS NULL", active.CartID).Scan(&collections).Error != nil {
			c.Status(503)
			return
		}
		var liveSessions int64
		if db.Table("payment_sessions s").Joins("JOIN cart_payment_collections l ON l.payment_collection_id=s.payment_collection_id").Where("l.cart_id=? AND l.deleted_at IS NULL AND s.deleted_at IS NULL", active.CartID).Count(&liveSessions).Error != nil {
			c.Status(503)
			return
		}
		adapter.mu.Lock()
		defer adapter.mu.Unlock()
		c.JSON(200, gin.H{"version": owner.Version, "checkout": owner.CheckoutAttemptID, "refresh": owner.PaymentRefreshOperationID, "receipts": receipts, "attempts": attempts, "paypal_creates": func() int {
			paypal.mu.Lock()
			defer paypal.mu.Unlock()
			n := 0
			for _, v := range paypal.calls {
				n += v
			}
			return n
		}(), "paypal_retirements": retirer.calls, "paypal_settlements": paypal.settlements, "creates": adapter.creates, "deletes": adapter.deletes, "observes": adapter.observes, "discovers": adapter.discovers, "protected_unchanged": protectedFacts() == baseline, "transfer_receipts": transferReceipts, "account_owners": accountOwners, "email": email, "collections": collections, "live_sessions": liveSessions, "provider_events": adapter.events})
	})
	if paypalContext {
		fixture.POST("/paypal-dispatch-audit", func(c *gin.Context) {
			for _, provider := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
				ref, in := makeCart()
				in.ProviderID = provider
				a := &paypalCheckoutProvider{calls: map[string]int{}, lostSettlement: true}
				cfg := configured
				cfg.Adapters, _ = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter, provider: a})
				if _, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, cfg); err != nil {
					t.Error("claim audit start", err)
					c.Status(503)
					return
				}
				// Hold only this caller just before its first owner lock:
				// metadata reads have already established an RR snapshot.
				// A real settlement commits in the gap, then the staged
				// command must observe that new claim using a current read.
				marker := struct{ Label string }{"owned-retirement-race"}
				reached, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				callback := "owned_paypal_retirement_barrier"
				if err := db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Context.Value(marker) == true && tx.Statement.Table == "cart_ownership" {
						once.Do(func() {
							close(reached)
							select {
							case <-release:
							case <-ctx.Done():
							}
						})
					}
				}); err != nil {
					t.Error(err)
					c.Status(503)
					return
				}
				cmd := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "region", Locale: "zh-CN"}
				resultCh := make(chan error, 1)
				go func() {
					_, err := cart.StageContextPayment(context.WithValue(ctx, marker, true), db, "shop", ref, cmd, cfg)
					resultCh <- err
				}()
				select {
				case <-reached:
				case <-time.After(10 * time.Second):
					close(release)
					db.Callback().Query().Remove(callback)
					t.Error("owner barrier not reached")
					c.Status(503)
					return
				}
				result, settleErr := cart.SettlePayPal(ctx, db, "shop", ref, cfg)
				close(release)
				stageErr := <-resultCh
				db.Callback().Query().Remove(callback)
				if settleErr != nil || result.State != "requires_review" {
					t.Error("claim audit expected unknown settlement", result, settleErr)
					c.Status(503)
					return
				}
				if stageErr != cart.ErrConflict {
					t.Error("claim audit missed concurrently committed settlement", stageErr)
					c.Status(503)
					return
				}
				var count int64
				if db.Table("cart_payment_refresh_operations").Where("cart_id=?", ref.CartID).Count(&count).Error != nil || count != 0 {
					t.Error("claim audit staged effects")
					c.Status(503)
					return
				}
			}
			c.JSON(200, gin.H{"blocked": 2, "settlement_claims_preserved": true})
		})
	}
	if paypalContext {
		fixture.POST("/stage-paypal-context", func(c *gin.Context) {
			var in struct {
				OperationID string `json:"operation_id"`
				Version     string `json:"version"`
				RegionID    string `json:"region_id"`
				Locale      string `json:"locale"`
			}
			if c.ShouldBindJSON(&in) != nil {
				c.Status(400)
				return
			}
			op, e := cart.StageContextPayment(ctx, db, "shop", active, cart.ContextCommand{OperationID: in.OperationID, Version: in.Version, RegionID: in.RegionID, Locale: in.Locale}, configured)
			if e != nil {
				t.Error("owned PayPal stage", e)
				c.Status(503)
				return
			}
			c.JSON(200, gin.H{"effects": len(op.Effects), "state": op.State})
		})
	}
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
