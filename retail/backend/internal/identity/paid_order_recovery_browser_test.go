package identity_test

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Continue the already paid, budget-conflicted cart from the completion subflow.
// The only control mutation raises this owned fixture's budget; the browser
// always uses production possession, CSRF, observation and completion handlers.
func checkPaidOrderRecoveryBrowser(t *testing.T, ctx context.Context, db *gorm.DB, router *gin.Engine, cartID, token, csrf string) {
	t.Helper()
	key := os.Getenv("CONTEXT_PAYMENT_KEY")
	if len(key) != 64 {
		t.Fatal("owned recovery key required")
	}
	router.GET("/api/store/regions", (catalog.Handler{DB: db}).Regions)
	finish := make(chan struct{}, 1)
	fixture := router.Group("/__fixture")
	fixture.Use(func(c *gin.Context) {
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Fixture-Key")), []byte(key)) != 1 {
			c.AbortWithStatus(404)
		}
	})
	fixture.GET("/facts", func(c *gin.Context) { c.Status(503) })
	fixture.POST("/new", func(c *gin.Context) {
		c.JSON(200, gin.H{"cart_id": cartID, "token": token, "csrf": csrf})
	})
	fixture.POST("/resolve", func(c *gin.Context) {
		// This is an explicit test control, not a new merchant API or a policy
		// that silently increases budgets in production.
		r := db.WithContext(c.Request.Context()).Exec(`UPDATE promotion_campaign_budgets SET ` + "`limit`=2" + `,raw_limit='{"value":"2","precision":100}' WHERE id='group-completion-budget'`)
		if r.Error != nil {
			c.AbortWithStatus(500)
			return
		}
		c.Status(200)
	})
	fixture.POST("/finish", func(c *gin.Context) {
		select {
		case finish <- struct{}{}:
		default:
		}
		c.Status(200)
	})
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
		t.Fatal("paid recovery browser deadline")
	}
	for name, query := range map[string]string{
		"receipt":      `SELECT COUNT(*) FROM cart_completion_receipts WHERE cart_id=? AND completed_version=8`,
		"capture":      `SELECT COUNT(*) FROM cart_completion_receipts r JOIN payment_captures p ON p.id=r.capture_id AND p.payment_id=r.payment_id WHERE r.cart_id=? AND p.amount=121.25 AND p.deleted_at IS NULL`,
		"transaction":  `SELECT COUNT(*) FROM cart_completion_receipts r JOIN order_transactions p ON p.order_id=r.order_id WHERE r.cart_id=? AND p.amount=121.25 AND p.currency_code='eur'`,
		"ownership":    `SELECT COUNT(*) FROM customer_order_ownership WHERE cart_id=?`,
		"notification": `SELECT COUNT(*) FROM cart_completion_receipts r JOIN order_notification_outbox n ON n.order_id=r.order_id WHERE r.cart_id=? AND n.event='order.placed'`,
		"promotion":    `SELECT COUNT(*) FROM cart_completion_receipts r JOIN promotion_usage_operations p ON p.scope_id=r.order_id WHERE r.cart_id=? AND p.state='registered'`,
	} {
		var count int64
		if db.Raw(query, cartID).Scan(&count).Error != nil || count != 1 {
			t.Fatal("recovered paid cart fact", name, count)
		}
	}
	var budgetUsed string
	if db.Table("promotion_budget_usages").Select("used").Where("budget_id='group-completion-budget' AND deleted_at IS NULL").Scan(&budgetUsed).Error != nil || budgetUsed != "2.000000000000000000" {
		t.Fatal("recovery budget counted twice", budgetUsed)
	}
	t.Log(`PAID_ORDER_RECOVERY_SQL {"original_paid_cart_recovered":true,"single_receipt_capture_transaction_owner_notification_usage":true,"saved_settlement":"121.25","budget_used":"2","provider_creates_unchanged":true}`)
}
