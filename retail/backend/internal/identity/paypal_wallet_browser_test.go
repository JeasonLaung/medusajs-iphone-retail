package identity_test

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Starts at the saved address/shipping checkpoint. Only the external SDK and
// provider are artificial; cart HTTP, completion, stock and cash use real SQL.
func checkPayPalWalletBrowser(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	origin, key := os.Getenv("CONTEXT_PAYMENT_ORIGIN"), os.Getenv("CONTEXT_PAYMENT_KEY")
	if !strings.HasPrefix(origin, "http://127.0.0.1:") || len(key) != 64 {
		t.Fatal("owned wallet origin/key")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("wallet fixture SQL", err)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		return orders.SeedOrderNumbers(ctx, tx, orders.OrderNumberSeed{LastDisplayID: 1000, LastTick: 200000000000000, EvidenceSHA256: sha256.Sum256([]byte("owned wallet source"))})
	}); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	exec("UPDATE commerce_store_locales SET locale_code='en-GB' WHERE store_id='shop' AND locale_code='en'")
	exec("INSERT INTO payment_providers(id,is_enabled,created_at,updated_at) VALUES('pp_paypal_paypal',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO commerce_region_payment_providers(id,region_id,payment_provider_id,created_at,updated_at) VALUES('wallet-binding','region','pp_paypal_paypal',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	wallet := &paypalCheckoutProvider{calls: map[string]int{}}
	mollie := &contextBrowserProvider{status: map[string]string{}}
	configured := options
	configured.ReturnOrigin = origin
	var err error
	configured.Adapters, err = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": mollie, "pp_paypal_paypal": wallet})
	if err != nil {
		t.Fatal(err)
	}
	shop, err := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: options.ManualProviders, InitialPayment: &configured})
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
	completed := 0
	finish := make(chan struct{}, 1)
	fixture.POST("/new", func(c *gin.Context) {
		active, _ = makeCart()
		issued, e := sessions.IssueCart(ctx, active)
		if e != nil {
			c.Status(503)
			return
		}
		wallet.mu.Lock()
		wallet.calls = map[string]int{}
		wallet.settlements = 0
		wallet.status = ""
		wallet.orderID = strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:17]
		wallet.mu.Unlock()
		c.JSON(200, gin.H{"cart_id": active.CartID, "token": issued.Token, "csrf": issued.CSRF})
	})
	fixture.GET("/facts", func(c *gin.Context) {
		if active.CartID == "" {
			c.Status(503)
			return
		}
		count := func(table, where string, args ...any) int64 {
			var n int64
			if db.Table(table).Where(where, args...).Count(&n).Error != nil {
				t.Error("wallet facts")
				return -1
			}
			return n
		}
		var order struct{ OrderID string }
		db.Table("order_carts").Select("order_id").Where("cart_id=?", active.CartID).Scan(&order)
		var cash struct{ ProviderID, CurrencyCode, Amount string }
		if order.OrderID != "" {
			db.Table("payments p").Select("p.provider_id,p.currency_code,CAST(p.amount AS CHAR) AS amount").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id").Where("l.order_id=?", order.OrderID).Scan(&cash)
		}
		wallet.mu.Lock()
		defer wallet.mu.Unlock()
		creates := 0
		for _, n := range wallet.calls {
			creates += n
		}
		c.JSON(200, gin.H{"creates": creates, "settlements": wallet.settlements, "order_id": order.OrderID, "orders": count("order_carts", "cart_id=?", active.CartID), "receipts": count("cart_completion_receipts", "cart_id=?", active.CartID), "captures": count("payment_captures c", "c.payment_id IN (SELECT p.id FROM payments p JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id WHERE l.order_id=?)", order.OrderID), "cash": cash})
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
		t.Fatal("wallet browser deadline")
	}
	expected, e := strconv.Atoi(os.Getenv("CONTEXT_PAYMENT_CASE_COUNT"))
	if e != nil || expected < 1 || completed != expected {
		t.Fatal("wallet cases", completed)
	}
	for _, table := range []string{"orders", "payment_captures", "cart_completion_receipts", "order_notification_outbox"} {
		var n int64
		if db.Table(table).Count(&n).Error != nil || n != int64(expected) {
			t.Fatal("wallet unique final rows", table, n, expected)
		}
	}
	t.Logf("CONTEXT_PAYMENT_BROWSER_SQL wallet cases=%d actual_sql_redis=true artificial_sdk_provider=true", completed)
}
