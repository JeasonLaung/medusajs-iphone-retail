package catalogimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"

	cataloghttp "github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkActualTaxonomy(t *testing.T, ctx context.Context, db *sql.DB, gdb *gorm.DB, router *gin.Engine, counts map[string]int64) {
	t.Helper()
	var storeID, channel string
	var stores int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commerce_stores WHERE deleted_at IS NULL").Scan(&stores) != nil || stores != 1 || db.QueryRowContext(ctx, "SELECT id,default_sales_channel_id FROM commerce_stores WHERE deleted_at IS NULL").Scan(&storeID, &channel) != nil {
		t.Fatal("actual isolated store fixture unavailable")
	}
	// Binding comes from the verified fixture owner, not from request parameters.
	h := cataloghttp.StoreHandler{DB: gdb, StoreID: storeID}
	checkActualPublicProducts(t, ctx, db, router, h, channel, counts)
	router.GET("/categories", h.Categories)
	router.GET("/categories/:id", h.Category)
	router.GET("/collections", h.Collections)
	router.GET("/collections/:id", h.Collection)
	reply := httptest.NewRecorder()
	router.ServeHTTP(reply, httptest.NewRequest("GET", "/categories", nil))
	var list struct {
		Categories []cataloghttp.StoreCategory `json:"product_categories"`
		Count      int64
	}
	var expected int64
	if reply.Code != 200 || json.Unmarshal(reply.Body.Bytes(), &list) != nil || db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_categories WHERE deleted_at IS NULL AND is_active=TRUE AND is_internal=FALSE").Scan(&expected) != nil || expected != list.Count || expected != int64(len(list.Categories)) {
		t.Fatal("actual public category count differs")
	}
	for _, c := range list.Categories {
		var name, description, handle string
		var rank int
		var count int64
		if db.QueryRowContext(ctx, "SELECT name,description,handle,`rank` FROM catalog_categories WHERE id=?", c.ID).Scan(&name, &description, &handle, &rank) != nil || name != c.Name || description != c.Description || handle != c.Handle || rank != c.Rank {
			t.Fatal("actual public category fields differ")
		}
		if db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT p.id) FROM catalog_category_products cp JOIN catalog_products p ON p.id=cp.product_id AND p.deleted_at IS NULL AND p.status='published' JOIN catalog_product_channels pc ON pc.product_id=p.id AND pc.sales_channel_id=? AND pc.deleted_at IS NULL WHERE cp.category_id=?", channel, c.ID).Scan(&count) != nil || count != c.ProductCount {
			t.Fatal("actual category published-channel membership differs")
		}
		counts["http_public_actual_categories"]++
		counts["http_public_category_product_memberships"] += count
	}
	collections := httptest.NewRecorder()
	router.ServeHTTP(collections, httptest.NewRequest("GET", "/collections", nil))
	var result struct {
		Collections []cataloghttp.StoreCollection
		Count       int64
	}
	if collections.Code != 200 || json.Unmarshal(collections.Body.Bytes(), &result) != nil || db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_collections c WHERE c.deleted_at IS NULL AND EXISTS(SELECT 1 FROM catalog_products p JOIN catalog_product_channels pc ON pc.product_id=p.id AND pc.deleted_at IS NULL AND pc.sales_channel_id=? WHERE p.collection_id=c.id AND p.deleted_at IS NULL AND p.status='published')", channel).Scan(&expected) != nil || result.Count != expected {
		t.Fatal("actual public collection count differs")
	}
	counts["http_public_actual_collections"] = result.Count
}
