package identity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/catalog"
	"gorm.io/gorm"
)

// Shares only the artificial two-shop catalog seeded by checkStoreTaxonomy.
func checkStoreProducts(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if db.Exec(sql, args...).Error != nil {
			t.Fatal("public product fixture mutation failed")
		}
	}
	exec(`UPDATE catalog_products SET subtitle='Subtitle',description='Description',metadata='{"private":"product-secret"}' WHERE id='public-a'`)
	exec(`INSERT INTO catalog_variants(id,product_id,title,sku,allow_backorder,manage_inventory,variant_rank,metadata,created_at,updated_at) VALUES('public-v','public-a','128 GB','000123',FALSE,TRUE,1,'{"private":"variant-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-v-incomplete','public-a','Invalid',NULL,FALSE,TRUE,2,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-v-deleted','public-a','Deleted',NULL,FALSE,TRUE,0,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE catalog_variants SET deleted_at=UTC_TIMESTAMP(6) WHERE id='public-v-deleted'`)
	exec(`INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('public-storage','Storage',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES('public-128','128 GB','public-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('private-256','256 GB','public-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_options(id,product_id,product_option_id,created_at,updated_at) VALUES('public-membership','public-a','public-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES('public-value','public-membership','public-128',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES('public-v','public-128'),('public-v-incomplete','private-256')`)
	exec("INSERT INTO catalog_images(id,url,`rank`,product_id,created_at,updated_at) VALUES('public-image','/phone.svg',1,'public-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('private-image','/private.svg',0,'public-b',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-image','/deleted.svg',2,'public-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec(`UPDATE catalog_images SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-image'`)
	exec(`INSERT INTO catalog_variant_images(id,variant_id,image_id,created_at,updated_at) VALUES('public-link','public-v','public-image',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('foreign-link','public-v','private-image',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-link','public-v','deleted-image',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	call := func(path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Cookie", "retail_admin_session=invalid")
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("public product status %d expected %d on %s", w.Code, status, path)
		}
		for _, s := range []string{"product-secret", "variant-secret", "metadata", "payload", "status", "revision", "public-main", "private-256", "private-image", "deleted-image"} {
			if strings.Contains(w.Body.String(), s) {
				t.Fatal("public product private or unimplemented field escaped")
			}
		}
		if w.Header().Get("Set-Cookie") != "" || status == 200 && w.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" || status != 200 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("public product cache/identity boundary failed")
		}
		return w.Body.Bytes()
	}
	fingerprint := func() []byte {
		t.Helper()
		out := []json.RawMessage{}
		for _, table := range []string{"catalog_products", "catalog_variants", "catalog_options", "catalog_option_values", "catalog_product_options", "catalog_product_option_values", "catalog_variant_options", "catalog_images", "catalog_variant_images", "catalog_product_channels", "catalog_categories", "catalog_category_products", "catalog_collections", "commerce_stores", "commerce_sales_channels"} {
			var rows []map[string]any
			if db.Table(table).Order("1,2").Find(&rows).Error != nil {
				t.Fatal("public product fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			out = append(out, b)
		}
		b, _ := json.Marshal(out)
		return b
	}
	before := fingerprint()
	var list struct {
		Products     []catalog.StoreProduct
		Count        int
		Capabilities map[string]bool
	}
	if json.Unmarshal(call("/api/store/products", 200), &list) != nil || list.Count != 1 || len(list.Products) != 1 || list.Products[0].ID != "public-a" || len(list.Capabilities) != 3 || list.Capabilities["checkout"] || list.Capabilities["pricing"] || !list.Capabilities["availability"] {
		t.Fatal("public product scope or capability projection incorrect")
	}
	p := list.Products[0]
	if p.Subtitle == nil || *p.Subtitle != "Subtitle" || p.CollectionID == nil || *p.CollectionID != "col-public" || len(p.Options) != 1 || len(p.Options[0].Values) != 1 || len(p.Images) != 1 || len(p.Variants) != 2 || p.Variants[0].ID != "public-v" || !p.Variants[0].ConfigurationComplete || len(p.Variants[0].Options) != 1 || len(p.Variants[0].Images) != 1 || p.Variants[1].ConfigurationComplete || len(p.Variants[1].Options) != 0 || p.Variants[1].Options == nil || p.Variants[1].Images == nil {
		t.Fatal("public product relations incorrect")
	}
	var detail struct{ Product catalog.StoreProduct }
	if json.Unmarshal(call("/api/store/products/public-a", 200), &detail) != nil {
		t.Fatal("public product detail decode failed")
	}
	a, _ := json.Marshal(p)
	b, _ := json.Marshal(detail.Product)
	if !bytes.Equal(a, b) {
		t.Fatal("public product detail differs from list")
	}
	for _, query := range []string{"handle=visible", "q=Visible", "collection_id=col-public&category_id=cat-root", "limit=1&offset=0"} {
		if json.Unmarshal(call("/api/store/products?"+query, 200), &list) != nil || list.Count != 1 {
			t.Fatal("public product filter failed")
		}
	}
	for _, query := range []string{"handle=missing", "q=%25", "q=_", "collection_id=col-foreign", "category_id=cat-hidden", "category_id=cat-inactive", "category_id=cat-deleted"} {
		if json.Unmarshal(call("/api/store/products?"+query, 200), &list) != nil || list.Count != 0 || len(list.Products) != 0 || list.Products == nil {
			t.Fatal("literal search or visibility filter incorrect")
		}
	}
	if json.Unmarshal(call("/api/store/products?offset=1", 200), &list) != nil || list.Count != 1 || len(list.Products) != 0 {
		t.Fatal("product count was paginated")
	}
	for _, id := range []string{"visible", "public-b", "public-draft", "public-deleted", "missing"} {
		call("/api/store/products/"+id, 404)
	}
	call("/api/store/products?region_id=unknown", 404)
	for _, query := range []string{"fields=*", "sales_channel_id=public-other", "quantity=1", "limit=0", "q=a&q=b"} {
		call("/api/store/products?"+query, 400)
	}
	call("/api/store/products/public-a?handle=visible", 400)
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("public product reads mutated history")
	}
	exec(`UPDATE catalog_product_channels SET deleted_at=UTC_TIMESTAMP(6) WHERE product_id='public-a' AND sales_channel_id='public-main'`)
	call("/api/store/products/public-a", 404)
	exec(`UPDATE catalog_product_channels SET deleted_at=NULL WHERE product_id='public-a' AND sales_channel_id='public-main'`)
	exec(`UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='public-main'`)
	call("/api/store/products", 503)
	exec(`UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='public-main'`)
	// Relation overflow must fail as a whole, including a single-item page.
	rows := []map[string]any{}
	for i := 0; i < 1001; i++ {
		rows = append(rows, map[string]any{"id": fmt.Sprintf("public-overflow-%04d", i), "product_id": "public-a", "title": "Overflow", "manage_inventory": true, "allow_backorder": false, "created_at": "2026-10-28 00:00:00", "updated_at": "2026-10-28 00:00:00"})
	}
	if db.Table("catalog_variants").Create(rows).Error != nil {
		t.Fatal("public product overflow fixture failed")
	}
	call("/api/store/products?limit=1", 503)
	exec("DELETE FROM catalog_variants WHERE id LIKE 'public-overflow-%'")
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("public product failure checks changed evidence")
	}
	t.Log(`STORE_PRODUCT_REHEARSAL {"current_channel_only":true,"published_live_only":true,"strict_query":true,"literal_search":true,"exact_detail_id":true,"configuration_membership":true,"foreign_options_and_images_hidden":true,"stable_pagination":true,"empty_arrays":true,"relation_overflow_503":true,"no_legacy_payload_or_prices":true,"unimplemented_capabilities_explicit":true,"read_fingerprint_unchanged":true}`)
	checkStorePricing(t, db, router)
	checkStoreDisplay(t, db, router)
}
