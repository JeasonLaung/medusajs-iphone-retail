package catalog

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTaxonomyQueries(t *testing.T) {
	for _, raw := range []string{"", "limit=1&offset=0", "handle=iphone%2F13", "parent_category_id=null", "parent_category_id=cat-1"} {
		if _, err := parseTaxonomy(raw, true, false); err != nil {
			t.Fatalf("valid taxonomy query rejected: %s", raw)
		}
	}
	for _, raw := range []string{"limit=01", "offset=-1", "offset=100001", "limit=101", "limit=", "limit=1&limit=2", "fields=*", "sales_channel_id=elsewhere", "store_id=other", "handle=%00", "handle=%FF", "bad%zz", "parent_category_id="} {
		if _, err := parseTaxonomy(raw, true, false); err == nil {
			t.Fatal("invalid taxonomy query accepted")
		}
	}
	if _, err := parseTaxonomy("parent_category_id=null", false, false); err == nil {
		t.Fatal("collection accepted category selector")
	}
	if _, err := parseTaxonomy("limit=1", true, true); err == nil {
		t.Fatal("detail accepted list query")
	}
}

func TestStoreTaxonomyWithoutDatabaseFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := StoreHandler{}
	r.GET("/categories", h.Categories)
	r.GET("/collections", h.Collections)
	for _, path := range []string{"/categories", "/collections"} {
		reply := httptest.NewRecorder()
		r.ServeHTTP(reply, httptest.NewRequest("GET", path, nil))
		if reply.Code != 503 || reply.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unconfigured taxonomy did not fail closed")
		}
	}
}
