package catalog

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStoreProductQueryBoundary(t *testing.T) {
	for _, q := range []string{"", "limit=100&offset=100000", "q=%25_%21", "handle=phones%2F13", "category_id=cat&collection_id=col", "order=-created_at", "option_value_id=one&option_value_id=two&option_value_id=one"} {
		if _, err := parseStoreProducts(q, false); err != nil {
			t.Fatal("valid public product query rejected")
		}
	}
	for _, q := range []string{"limit=0", "limit=101", "limit=01", "offset=-1", "offset=100001", "q=", "q=%00", "q=%FF", "bad%zz", "limit=1&limit=2", "fields=*", "sales_channel_id=other", "quantity=1", "quantity=0&region_id=r", "quantity=01&region_id=r", "quantity=1000000001&region_id=r", "currency_code=eur", "order=id", "order=-created_at&order=-created_at", "order=-created_at%3BDROP", "option_value_id=", "option_value_id=%FF", "option_value_id=%00"} {
		if _, err := parseStoreProducts(q, false); err == nil {
			t.Fatal("invalid public product query accepted")
		}
	}
	if _, err := parseStoreProducts("handle=a", true); err == nil {
		t.Fatal("detail accepted a handle override")
	}
	for _, query := range []string{"order=-created_at", "option_value_id=one"} {
		if _, err := parseStoreProducts(query, true); err == nil {
			t.Fatal("detail accepted list selector")
		}
	}
	parsed, err := parseStoreProducts("order=-created_at&option_value_id=one&option_value_id=two&option_value_id=one", false)
	if err != nil || parsed.order != "p.created_at DESC,p.id ASC" || len(parsed.optionValues) != 2 {
		t.Fatal("collection selectors not normalized")
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := StoreHandler{}
	r.GET("/products", h.Products)
	r.GET("/products/:id", h.Product)
	for _, path := range []string{"/products", "/products/id"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 503 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unconfigured product read accepted")
		}
	}
}
