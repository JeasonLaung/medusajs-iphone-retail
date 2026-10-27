package catalogimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	cataloghttp "github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
)

func checkActualPublicProducts(t *testing.T, ctx context.Context, db *sql.DB, router *gin.Engine, h cataloghttp.StoreHandler, channel string, counts map[string]int64) {
	t.Helper()
	router.GET("/public-products", h.Products)
	router.GET("/public-products/:id", h.Product)
	var expected int64
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_products p WHERE p.deleted_at IS NULL AND p.status='published' AND EXISTS(SELECT 1 FROM catalog_product_channels pc WHERE pc.product_id=p.id AND pc.sales_channel_id=? AND pc.deleted_at IS NULL)", channel).Scan(&expected) != nil {
		t.Fatal("actual public product count failed")
	}
	seen := map[string]bool{}
	previous := ""
	for offset := 0; int64(offset) < expected; offset += 5 {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", fmt.Sprintf("/public-products?limit=5&offset=%d", offset), nil))
		var page struct {
			Products []cataloghttp.StoreProduct
			Count    int64
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Count != expected || len(page.Products) == 0 {
			t.Fatal("actual public product page failed")
		}
		for _, p := range page.Products {
			if seen[p.ID] || p.ID <= previous {
				t.Fatal("actual public product pagination unstable")
			}
			seen[p.ID] = true
			previous = p.ID
			var title, handle string
			var subtitle, description, thumbnail sql.NullString
			if db.QueryRowContext(ctx, "SELECT title,handle,subtitle,description,thumbnail FROM catalog_products WHERE id=?", p.ID).Scan(&title, &handle, &subtitle, &description, &thumbnail) != nil {
				t.Fatal("actual public product lookup failed")
			}
			matches := func(value *string, expected sql.NullString) bool {
				return value == nil && !expected.Valid || value != nil && expected.Valid && *value == expected.String
			}
			if p.Title != title || p.Handle != handle || !matches(p.Subtitle, subtitle) || !matches(p.Description, description) || !matches(p.Thumbnail, thumbnail) {
				t.Fatal("actual public product fields differ")
			}
			var variants, images int64
			if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_variants WHERE product_id=? AND deleted_at IS NULL", p.ID).Scan(&variants) != nil || variants != int64(len(p.Variants)) || db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_images WHERE product_id=? AND deleted_at IS NULL", p.ID).Scan(&images) != nil || images != int64(len(p.Images)) {
				t.Fatal("actual public product relation counts differ")
			}
			for _, v := range p.Variants {
				var vt string
				var sku, thumb sql.NullString
				var rank sql.NullInt64
				if db.QueryRowContext(ctx, "SELECT title,sku,thumbnail,variant_rank FROM catalog_variants WHERE id=? AND product_id=? AND deleted_at IS NULL", v.ID, p.ID).Scan(&vt, &sku, &thumb, &rank) != nil || v.Title != vt || !matches(v.SKU, sku) || !matches(v.Thumbnail, thumb) || v.VariantRank == nil && rank.Valid || v.VariantRank != nil && (!rank.Valid || int64(*v.VariantRank) != rank.Int64) {
					t.Fatal("actual public variant fields differ")
				}
				for _, choice := range v.Options {
					var n int
					if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM catalog_variant_options vo JOIN catalog_option_values ov ON ov.id=vo.option_value_id AND ov.deleted_at IS NULL JOIN catalog_product_option_values pv ON pv.product_option_value_id=ov.id AND pv.deleted_at IS NULL JOIN catalog_product_options po ON po.id=pv.product_product_option_id AND po.deleted_at IS NULL AND po.product_option_id=ov.option_id JOIN catalog_options o ON o.id=ov.option_id AND o.deleted_at IS NULL WHERE vo.variant_id=? AND ov.id=? AND ov.option_id=? AND ov.value=? AND po.product_id=?", v.ID, choice.ID, choice.OptionID, choice.Value, p.ID).Scan(&n) != nil || n != 1 {
						t.Fatal("actual public variant choice membership differs")
					}
					counts["http_public_actual_variant_choices"]++
				}
				counts["http_public_actual_variants"]++
			}
			for _, im := range p.Images {
				var url string
				var rank int
				if db.QueryRowContext(ctx, "SELECT url,`rank` FROM catalog_images WHERE id=? AND product_id=? AND deleted_at IS NULL", im.ID, p.ID).Scan(&url, &rank) != nil || url != im.URL || rank != im.Rank {
					t.Fatal("actual public image fields differ")
				}
				counts["http_public_actual_images"]++
			}
			w = httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/public-products/"+p.ID, nil))
			var detail struct{ Product cataloghttp.StoreProduct }
			a, _ := json.Marshal(p)
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil {
				t.Fatal("actual public product detail failed")
			}
			b, _ := json.Marshal(detail.Product)
			if string(a) != string(b) {
				t.Fatal("actual public product detail differs from page")
			}
			counts["http_public_actual_products"]++
		}
	}
	if int64(len(seen)) != expected {
		t.Fatal("actual public product page omitted resources")
	}
	checkActualStorePrices(t, router, counts)
}
