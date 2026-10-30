package identity_test

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Opt-in owned browser rehearsal. Only the payment provider is replaced; cart,
// checkout, observation, completion and confirmation use production handlers.
func checkCheckoutBrowser(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, options cart.InitialPaymentOptions) {
	t.Helper()
	if os.Getenv("CHECKOUT_BROWSER_FIXTURE") != "1" {
		return
	}
	key := os.Getenv("CHECKOUT_FIXTURE_KEY")
	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		t.Fatal("owned checkout fixture key required")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if db.WithContext(ctx).Exec(query, args...).Error != nil {
			t.Fatal("owned checkout fixture setup failed")
		}
	}
	const origin = "http://127.0.0.1:19379"
	exec("UPDATE commerce_store_locales SET locale_code='en-GB' WHERE store_id='shop' AND locale_code='en'")
	defer exec("UPDATE commerce_store_locales SET locale_code='en' WHERE store_id='shop' AND locale_code='en-GB'")
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE cart_promotion_command_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_promotion_command_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	// Preserve and check the actual stock increment caused by these four orders.
	var reservedBefore string
	if db.Table("inventory_levels").Select("CAST(reserved_quantity AS CHAR)").Where("id='add-level'").Scan(&reservedBefore).Error != nil {
		t.Fatal("owned checkout stock baseline unavailable")
	}
	// Each fixture SKU consumes 2.5 inventory units. Add only the ten units
	// needed by these four carts to this disposable database, retaining holds.
	exec("UPDATE inventory_levels SET stocked_quantity=stocked_quantity+10 WHERE id='add-level'")
	type fixtureCart struct {
		ID    string `json:"id"`
		Token string `json:"token"`
		CSRF  string `json:"csrf"`
	}
	var fixtures []fixtureCart
	known := map[string]bool{}
	paid := map[string]bool{}
	var mu sync.Mutex
	for i := 0; i < 4; i++ {
		id := uuid.NewString()
		locale := "en-GB"
		if i >= 2 {
			locale = "zh-CN"
		}
		created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: id, ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: locale})
		if err != nil {
			t.Fatal("owned checkout cart unavailable", err)
		}
		ref := created.Reference
		if _, err = cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"}); err != nil {
			t.Fatal("owned checkout item unavailable", err)
		}
		possession, err := sessions.IssueCart(ctx, ref)
		if err != nil {
			t.Fatal("owned checkout capability unavailable")
		}
		defer sessions.RevokeCart(context.Background(), possession.Token)
		fixtures = append(fixtures, fixtureCart{ref.CartID, possession.Token, possession.CSRF})
		known[ref.CartID] = true
	}
	var creates, observations atomic.Int32
	registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(callCtx context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			creates.Add(1)
			if !known[inScope(r)] || r.Amount != "158.75" || r.Currency != "eur" {
				t.Error("browser payment request differs")
				return payments.CreateUncertain, payments.CreatedSession{}, nil
			}
			var freeze string
			if db.WithContext(callCtx).Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", inScope(r)).Scan(&freeze).Error != nil || freeze == "" {
				t.Error("browser provider dispatched before committed freeze")
			}
			raw, _ := json.Marshal(map[string]any{"id": "tr_" + strings.ReplaceAll(r.SessionID, "-", ""), "_links": map[string]any{"checkout": map[string]string{"href": "https://www.mollie.com/checkout/joint/" + r.SessionID}}})
			return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: raw}, nil
		},
		observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			observations.Add(1)
			mu.Lock()
			settled := paid[inScope(r)]
			mu.Unlock()
			status, action, local := "pending", "pending", "pending"
			if settled {
				status, action, local = "paid", "successful", "authorized"
			}
			raw, _ := json.Marshal(map[string]any{"id": id, "status": status, "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}})
			return payments.SessionObservation{PaymentID: id, ProviderStatus: status, Action: action, Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: local, Data: raw}}, nil
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	options.Adapters, options.ReturnOrigin = registry, origin
	shop, err := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: options.ManualProviders, InitialPayment: &options})
	if err != nil {
		t.Fatal("owned checkout handler unavailable", err)
	}
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	public := router.Group("/api/store")
	shop.Register(public)
	catalogue := catalog.StoreHandler{DB: db, StoreID: "shop"}
	public.GET("/regions", (catalog.Handler{DB: db}).Regions)
	public.GET("/locales", catalogue.Locales)
	public.GET("/collections", catalogue.Collections)
	public.GET("/product-categories", catalogue.Categories)
	finish := make(chan struct{}, 1)
	fixture := router.Group("/__fixture")
	fixture.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Fixture-Key")), []byte(key)) != 1 {
			c.AbortWithStatus(404)
		}
	})
	fixture.GET("/ready", func(c *gin.Context) { c.JSON(200, gin.H{"carts": fixtures}) })
	fixture.POST("/paid/:id", func(c *gin.Context) {
		id := c.Param("id")
		if !known[id] {
			c.Status(404)
			return
		}
		// Change only the isolated provider's observation, never SQL order/payment state.
		mu.Lock()
		paid[id] = true
		mu.Unlock()
		c.Status(204)
	})
	fixture.POST("/finish", func(c *gin.Context) {
		select {
		case finish <- struct{}{}:
		default:
		}
		c.Status(204)
	})
	listener, err := net.Listen("tcp", "0.0.0.0:9100")
	if err != nil {
		t.Fatal("owned checkout listener unavailable")
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	select {
	case <-finish:
	case <-ctx.Done():
		t.Fatal("owned checkout browser deadline")
	case <-done:
		t.Fatal("owned checkout listener stopped")
	}
	if creates.Load() != 4 || observations.Load() < 8 {
		t.Fatal("browser provider calls differ", creates.Load(), observations.Load())
	}
	var orderIDs []string
	for _, f := range fixtures {
		var receipt struct {
			OrderID          string
			CompletedVersion uint64
		}
		if db.Table("cart_completion_receipts").Where("cart_id=?", f.ID).Take(&receipt).Error != nil || receipt.CompletedVersion != 6 {
			t.Fatal("browser completion receipt missing")
		}
		orderIDs = append(orderIDs, receipt.OrderID)
		for _, table := range []string{"order_carts", "order_notification_outbox"} {
			var count int64
			if db.Table(table).Where("order_id=?", receipt.OrderID).Count(&count).Error != nil || count != 1 {
				t.Fatal("browser order/outbox differs", table)
			}
		}
		var count int64
		if db.Table("inventory_reservation_operations").Where("owner_id=?", receipt.OrderID).Count(&count).Error != nil || count != 1 {
			t.Fatal("browser reservation operation missing")
		}
		var capture string
		if db.Table("payment_captures").Select("CAST(amount AS CHAR)").Where("payment_id=(SELECT payment_id FROM cart_completion_receipts WHERE cart_id=?)", f.ID).Scan(&capture).Error != nil {
			t.Fatal("browser capture amount differs", capture)
		}
		value, valid := new(big.Rat).SetString(capture)
		if !valid || value.Cmp(big.NewRat(635, 4)) != 0 {
			t.Fatal("browser capture amount differs")
		}
	}
	var reservedAfter string
	if db.Table("inventory_levels").Select("CAST(reserved_quantity AS CHAR)").Where("id='add-level'").Scan(&reservedAfter).Error != nil {
		t.Fatal("browser final stock unavailable")
	}
	before, ok := new(big.Rat).SetString(reservedBefore)
	after, valid := new(big.Rat).SetString(reservedAfter)
	if !ok || !valid || new(big.Rat).Sub(after, before).Cmp(big.NewRat(10, 1)) != 0 {
		t.Fatal("browser stock reservation delta differs")
	}
	checkCheckoutMailJoint(t, ctx, db, options.Ring, orderIDs)
	t.Log(`CHECKOUT_JOINT_SQL {"four_unique_orders":true,"four_provider_creations":true,"notification_outbox_per_order":true,"reservation_operation_per_order":true,"captured_amount":"158.75","reserved_inventory_delta":"10"}`)
}
