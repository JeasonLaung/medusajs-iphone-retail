package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/catalog"
	"gorm.io/gorm"
)

func checkStoreSearch(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	if db.Exec("UPDATE catalog_categories SET name='iPhone' WHERE id='cat-root'").Error != nil {
		t.Fatal("search fixture failed")
	}
	defer func() {
		if db.Exec("UPDATE catalog_categories SET name='Root' WHERE id='cat-root'").Error != nil {
			t.Error("search restore failed")
		}
	}()
	call := func(body string, status int) []catalog.StoreSearchResult {
		t.Helper()
		w := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/api/store/search", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Cookie", "retail_admin_session=invalid")
		router.ServeHTTP(w, request)
		if w.Code != status {
			t.Fatalf("search status %d expected %d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
			t.Fatal("search public identity boundary")
		}
		for _, secret := range []string{"product-secret", "pricing-secret", "metadata", "payload", "public-other", "public-b", "public-draft", "public-deleted", "private-256"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("search private projection leaked")
			}
		}
		var out struct{ Results []catalog.StoreSearchResult }
		if status == 200 && json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("search response invalid")
		}
		return out.Results
	}
	results := call(`{"region_id":"price-region","queries":[{"key":"first"},{"key":"empty","filters":{"storage_options":["missing"]}}]}`, 200)
	if len(results) != 2 || results[0].Key != "first" || results[0].Count != 1 || results[1].Key != "empty" || results[1].Count != 0 || results[1].Hits == nil {
		t.Fatal("batch count/order/empty hits")
	}
	hit := results[0].Hits[0]
	if hit.ID != "public-a" || results[0].Currency != "eur" {
		t.Fatal("search channel/currency")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/store/products/public-a?region_id=price-region", nil))
	var detail struct{ Product catalog.StoreProduct }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil {
		t.Fatal("search detail comparison unavailable")
	}
	found := false
	for _, v := range detail.Product.Variants {
		if v.CalculatedPrice != nil {
			found = true
			if hit.Price == nil || hit.Price.Min != v.CalculatedPrice.Amount {
				t.Fatal("list/detail exact price mismatch")
			}
		}
	}
	if !found {
		t.Fatal("missing priced fixture")
	}
	// Real inventory changes must affect the next search without rebuilding an
	// index. Existing unmanaged/backorder variants cannot invent physical stock.
	func() {
		var levels []struct {
			ID                                string
			StockedQuantity, ReservedQuantity string
		}
		if db.Table("inventory_levels").Select("id,stocked_quantity,reserved_quantity").Where("inventory_item_id IN ?", []string{"availability-item", "availability-second"}).Scan(&levels).Error != nil || len(levels) == 0 {
			t.Fatal("search inventory fixture unavailable")
		}
		defer func() {
			for _, level := range levels {
				if db.Exec("UPDATE inventory_levels SET stocked_quantity=?,reserved_quantity=? WHERE id=?", level.StockedQuantity, level.ReservedQuantity, level.ID).Error != nil {
					t.Error("search inventory restore failed")
				}
			}
		}()
		if len(hit.StockedStorages) != 1 || hit.StockedStorages[0] != "128 GB" {
			t.Fatal("real stock label missing")
		}
		if db.Exec("UPDATE inventory_levels SET stocked_quantity=reserved_quantity WHERE inventory_item_id IN ?", []string{"availability-item", "availability-second"}).Error != nil {
			t.Fatal("search inventory depletion failed")
		}
		depleted := call(`{"region_id":"price-region","queries":[{"key":"depleted"}]}`, 200)
		if depleted[0].Count != 1 || len(depleted[0].Hits[0].StockedStorages) != 0 {
			t.Fatal("backorder or unmanaged SKU advertised as physical stock")
		}
	}()
	call(`{"region_id":"price-region","queries":[{"key":"unsafe","filters":{"sales_channel_ids":["public-other"]}}]}`, 400)
	call(`{"region_id":"missing","queries":[{"key":"unknown"}]}`, 404)
	if db.Exec("UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='public-main'").Error != nil {
		t.Fatal("search disable failed")
	}
	defer db.Exec("UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='public-main'")
	call(`{"region_id":"price-region","queries":[{"key":"closed"}]}`, 503)
	t.Log(`STORE_SEARCH_REHEARSAL {"batch_order":true,"current_channel_only":true,"live_published_only":true,"strict_filters":true,"list_detail_price_equal":true,"live_stock_without_reindex":true,"backorder_and_unmanaged_not_physical_stock":true,"anonymous_cookie_independent":true,"closed_channel_503":true,"keyword_parity_accepted":false,"formal_store_page_accepted":false}`)
}
