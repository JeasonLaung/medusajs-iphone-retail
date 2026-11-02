package identity_test

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/productreviews"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The local runner calls these helpers only in its owned source snapshot. It
// changes fixture wiring, never production handlers, order/payment facts or
// authentication. The ordinary checkout runner remains unchanged.
func mountCheckoutReviewJoint(t *testing.T, ctx context.Context, db *gorm.DB, router *gin.Engine) func() {
	t.Helper()
	var database string
	if os.Getenv("CHECKOUT_REVIEWS") != "1" || db.Raw("SELECT DATABASE()").Scan(&database).Error != nil || !strings.HasPrefix(database, "eurorefurb_review_joint_") {
		t.Fatal("owned local review joint database required")
	}
	if db.WithContext(ctx).Exec("UPDATE product_review_runtime SET reads_enabled=TRUE,writes_enabled=TRUE WHERE id=1").Error != nil {
		t.Fatal("review runtime fixture")
	}
	router.GET("/api/store/product-reviews", (productreviews.Service{DB: db, StoreID: "shop"}).PublicHTTP)
	router.POST("/__fixture/review-facts", func(c *gin.Context) {
		key := os.Getenv("CHECKOUT_FIXTURE_KEY")
		if len(key) != 64 || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Fixture-Key")), []byte(key)) != 1 {
			c.Status(404)
			return
		}
		pool, e := db.DB()
		if e != nil {
			c.Status(503)
			return
		}
		h := sha256.New()
		for _, table := range []string{"inventory_levels", "inventory_reservations", "pricing_prices", "device_assignments", "payments", "payment_captures"} {
			rows, err := pool.QueryContext(c.Request.Context(), "SELECT * FROM "+table+" ORDER BY id")
			if err != nil {
				c.Status(503)
				return
			}
			columns, err := rows.Columns()
			if err != nil {
				rows.Close()
				c.Status(503)
				return
			}
			fmt.Fprintf(h, "%d:%s", len(table), table)
			for rows.Next() {
				values := make([]sql.RawBytes, len(columns))
				args := make([]any, len(columns))
				for i := range values {
					args[i] = &values[i]
				}
				if rows.Scan(args...) != nil {
					rows.Close()
					c.Status(503)
					return
				}
				for i, value := range values {
					fmt.Fprintf(h, "%d:%s", len(columns[i]), columns[i])
					if value == nil {
						fmt.Fprint(h, "N;")
					} else {
						fmt.Fprintf(h, "V%d:%s;", len(value), value)
					}
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				c.Status(503)
				return
			}
		}
		c.Header("Cache-Control", "private, no-store")
		c.JSON(200, gin.H{"sha256": fmt.Sprintf("%x", h.Sum(nil))})
	})
	return func() {
		if db.Exec("UPDATE product_review_runtime SET writes_enabled=FALSE,reads_enabled=FALSE WHERE id=1").Error != nil {
			t.Error("review fixture gate cleanup")
		}
	}
}

func checkCheckoutReviewJoint(t *testing.T, db *gorm.DB, orders []string) {
	t.Helper()
	if len(orders) != 4 {
		t.Fatal("four original checkout orders required")
	}
	checks := []struct {
		query string
		want  int64
	}{
		{"SELECT COUNT(*) FROM product_reviews WHERE product_id='add-product' AND status='approved' AND account_id IS NOT NULL AND deleted_at IS NULL", 4},
		{"SELECT COUNT(DISTINCT account_id) FROM product_reviews", 4},
		{"SELECT COUNT(*) FROM product_reviews WHERE verified_purchase=TRUE AND order_id IS NOT NULL", 2},
		{"SELECT COUNT(*) FROM product_reviews WHERE verified_purchase=FALSE AND order_id IS NULL", 2},
		{"SELECT COUNT(*) FROM product_reviews r JOIN customer_order_ownership ow ON ow.order_id=r.order_id AND ow.customer_id=r.account_id AND ow.store_id='shop' WHERE r.verified_purchase=TRUE", 2},
		{"SELECT COUNT(*) FROM product_review_commands WHERE command='submit' AND result_status='pending'", 4},
		{"SELECT COUNT(*) FROM product_review_commands WHERE command='moderate'", 7},
		{"SELECT COUNT(*) FROM product_reviews WHERE author='New' AND customer_id=account_id", 4},
	}
	for _, check := range checks {
		var n int64
		if db.Raw(check.query).Scan(&n).Error != nil || n != check.want {
			t.Fatal("native review SQL evidence differs", check.query, n)
		}
	}
	for _, order := range orders {
		var n int64
		if db.Raw("SELECT COUNT(*) FROM product_reviews r JOIN customer_order_ownership ow ON ow.customer_id=r.account_id AND ow.order_id=? WHERE r.product_id='add-product'", order).Scan(&n).Error != nil || n != 1 {
			t.Fatal("same new checkout customer review missing")
		}
	}
	t.Log(`PRODUCT_REVIEWS_JOINT_SQL {"same_four_new_checkout_customers":true,"reviews":4,"approved":4,"verified_purchase":2,"ordinary_without_order":2,"submit_receipts":4,"moderation_receipts":7,"original_checkout_fulfillment_checks_retained":true}`)
}
