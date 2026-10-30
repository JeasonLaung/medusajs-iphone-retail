package identity_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Only the owned fixture can read its artificial verification-mail intent.
// Registration, verification, login and order attachment use production HTTP.
func mountCheckoutAccounts(t *testing.T, db *gorm.DB, sessions *sessionstore.Store, ring *protected.Ring, router *gin.Engine, fixture *gin.RouterGroup, key, origin string, client *redis.Client) {
	t.Helper()
	h, e := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: origin, BridgeKey: key, StoreID: "shop"})
	if e != nil {
		t.Fatal("checkout customer handler unavailable")
	}
	h.Register(router.Group("/api/store/customer"))
	// Four browser cases are independent tests. Reset only this owned namespace's
	// limiter between cases; keep the production eight-attempt policy per case.
	fixture.POST("/case/:index", func(c *gin.Context) {
		index := c.Param("index")
		if index != "1" && index != "2" && index != "3" && index != "4" {
			c.Status(404)
			return
		}
		keys, e := client.Keys(c.Request.Context(), "eurorefurb:checkout-browser:v1:customer:login-attempt:*").Result()
		if e != nil || len(keys) > 100 {
			c.Status(503)
			return
		}
		if len(keys) > 0 && client.Del(c.Request.Context(), keys...).Err() != nil {
			c.Status(503)
			return
		}
		c.Status(204)
	})
	fixture.GET("/verification/:index", func(c *gin.Context) {
		index := c.Param("index")
		if index != "1" && index != "2" && index != "3" && index != "4" {
			c.Status(404)
			return
		}
		var job struct {
			ID      string
			Payload []byte
		}
		if db.WithContext(c.Request.Context()).Raw("SELECT m.id,m.payload FROM customer_verification_mail m JOIN customer_accounts a ON a.id=m.customer_id WHERE a.email=? AND m.status='pending'", fmt.Sprintf("checkout-%s@example.invalid", index)).Scan(&job).Error != nil || job.ID == "" {
			c.Status(404)
			return
		}
		plain, e := ring.Open(protected.Context{Purpose: "customer-verification", Table: "customer_verification_mail", Column: "payload", Row: job.ID}, job.Payload)
		var message struct{ URL string }
		if e != nil || json.Unmarshal(plain, &message) != nil {
			c.Status(503)
			return
		}
		u, e := url.Parse(message.URL)
		if e != nil || u.Scheme+"://"+u.Host != origin || len(u.Query().Get("token")) != 64 {
			c.Status(503)
			return
		}
		c.JSON(200, gin.H{"url": message.URL})
	})
}

func checkCheckoutAccountReceipts(t *testing.T, db *gorm.DB, orderIDs []string) {
	t.Helper()
	for _, id := range orderIDs {
		var count int64
		if db.Table("customer_order_ownership own").Joins("JOIN customer_accounts a ON a.id=own.customer_id AND a.verified_at IS NOT NULL").Joins("JOIN orders o ON o.id=own.order_id AND o.email=a.email").Where("own.order_id=?", id).Count(&count).Error != nil || count != 1 {
			t.Fatal("checkout account ownership differs")
		}
		if db.Table("audit_events").Where("resource_id=? AND action='customer.order.attach'", id).Count(&count).Error != nil || count != 1 {
			t.Fatal("checkout attachment audit differs")
		}
	}
	t.Log(`CHECKOUT_ACCOUNT_JOINT_SQL {"four_verified_new_owners":true,"one_attachment_audit_per_order":true,"old_accounts_used":false}`)
}
