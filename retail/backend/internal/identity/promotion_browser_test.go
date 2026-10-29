package identity_test

import (
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
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Opt-in bridge for the owned SQL rehearsal only. Production handlers serve all
// application requests; the two fixture endpoints exist only in this test binary.
func checkPromotionBrowser(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, baseline string) {
	t.Helper()
	if os.Getenv("PROMOTION_BROWSER_FIXTURE") != "1" {
		return
	}
	key := os.Getenv("PROMOTION_FIXTURE_KEY")
	if len(key) != 64 || strings.Trim(key, "0123456789abcdef") != "" {
		t.Fatal("owned browser fixture key required")
	}
	const origin = "http://127.0.0.1:19378"
	var initialReceipts int64
	if db.Table("cart_promotion_command_operations").Where("cart_id=?", ref.CartID).Count(&initialReceipts).Error != nil {
		t.Fatal("browser initial promotion receipts unavailable")
	}
	shop, err := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: []string{"delivery-provider"}})
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
	if db.Exec("UPDATE carts SET locale='en-GB' WHERE id=?", ref.CartID).Error != nil {
		t.Fatal("browser cart locale fixture failed")
	}
	defer db.Exec("UPDATE carts SET locale='en' WHERE id=?", ref.CartID)
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
	fixture.GET("/ready", func(c *gin.Context) {
		c.JSON(200, gin.H{"cart_id": ref.CartID, "cart_token": possession.Token, "cart_csrf": possession.CSRF, "baseline": baseline})
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
	if db.Table("cart_promotion_command_operations").Where("cart_id=?", ref.CartID).Count(&count).Error != nil || count != initialReceipts+8 {
		t.Fatal("browser promotion receipts differ", count)
	}
	detail, err := cart.ReadAppliedPromotions(ctx, db, "shop", ref)
	amounts, e := cart.ReadAmounts(ctx, db, "shop", ref)
	if err != nil || e != nil || len(detail.Promotions) != 1 || !detail.Promotions[0].Automatic || amounts.Totals["total"] != baseline {
		t.Fatal("browser did not restore automatic-only discount", err, e)
	}
	t.Log(`PROMOTION_JOINT_SQL {"real_handlers_mysql_redis":true,"eight_browser_writes_persisted":true,"automatic_preserved":true,"exact_total_restored":true}`)
}
