package identity_test

import (
	"bytes"
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Opt-in bridge for the owned SQL rehearsal only. Production handlers serve all
// application requests; the two fixture endpoints exist only in this test binary.
func checkCustomerOrdersBrowser(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, orderID, email string) {
	t.Helper()
	if os.Getenv("CUSTOMER_ORDERS_BROWSER_FIXTURE") != "1" {
		return
	}
	key := os.Getenv("CUSTOMER_ORDERS_FIXTURE_KEY")
	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		t.Fatal("owned browser fixture key required")
	}
	const origin = "http://127.0.0.1:19377"
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{83}, 32)})
	account, err := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: origin, BridgeKey: key})
	if err != nil {
		t.Fatal("browser customer handler unavailable")
	}
	shop, err := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop"})
	if err != nil {
		t.Fatal("browser cart handler unavailable")
	}
	possession, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal("browser cart capability unavailable")
	}
	defer sessions.RevokeCart(context.Background(), possession.Token)
	// The cart unit fixture uses the short code "en". The public directory
	// requires a registered locale with its existing display name.
	if db.Exec("UPDATE commerce_store_locales SET locale_code='en-GB' WHERE store_id='shop' AND locale_code='en'").Error != nil {
		t.Fatal("browser public locale fixture failed")
	}
	defer db.Exec("UPDATE commerce_store_locales SET locale_code='en' WHERE store_id='shop' AND locale_code='en-GB'")
	// Re-run attachment from the browser, then retain the same account ownership
	// and single audit receipt expected by the surrounding SQL regression.
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM customer_order_ownership WHERE order_id=?", orderID).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM audit_events WHERE action='customer.order.attach' AND resource_id=?", orderID).Error
	}); err != nil {
		t.Fatal("browser attachment fixture preparation failed")
	}
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	account.Register(router.Group("/api/store/customer"))
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
	fixture.GET("/ready", func(c *gin.Context) {
		c.JSON(200, gin.H{"order_id": orderID, "email": email, "password": "fixture-customer-order-password", "cart_token": possession.Token})
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
		t.Fatal("owned browser listener unavailable")
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
		t.Fatal("owned browser did not finish before fixture deadline")
	case <-done:
		t.Fatal("owned browser listener stopped early")
	}
	var count int64
	if db.Table("customer_order_ownership").Where("order_id=?", orderID).Count(&count).Error != nil || count != 1 {
		t.Fatal("browser attachment not persisted")
	}
	if db.Table("audit_events").Where("action='customer.order.attach' AND resource_id=?", orderID).Count(&count).Error != nil || count != 1 {
		t.Fatal("browser attachment audit not unique")
	}
	t.Log(`CUSTOMER_ORDERS_JOINT_SQL {"real_handlers_mysql_redis":true,"browser_attachment_persisted":true,"single_attachment_audit":true,"external_payment_provider":false}`)
}
