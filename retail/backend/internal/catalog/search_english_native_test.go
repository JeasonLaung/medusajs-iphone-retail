package catalog

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Called only by the fresh private-socket native suite. This owns its channel
// and catalog, and never modifies a running development database.
func verifySearchEnglishNative(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("search fixture", e)
		}
	}
	exec("INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('search-english-channel','Search',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES('search-english-store','Search','search-english-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO commerce_store_currencies(id,currency_code,is_default,store_id,created_at,updated_at) VALUES('search-english-cad','cad',TRUE,'search-english-store',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_categories(id,name,description,handle,mpath,is_active,is_internal,`rank`,created_at,updated_at) VALUES('search-english-category','iPhone','','search-english-category','search-english-category',TRUE,FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("search-english-%02d", i)
		exec("INSERT INTO catalog_products(id,title,handle,description,status,is_giftcard,discountable,created_at,updated_at) VALUES(?,?,?,?,'published',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, "iPhone 13 Pro Max", id, "Stock quantity has not been verified. Supplied catalog snapshot. Space-gray 6.1 a@example.test phones128 verified-quantities 1.2e3 8.3.0. https://catalog.example.test/phones /usr/local/phones.txt <a href=\"hiddenkeyword\">visibleword</a> &amp; <script>scriptsecret</script> ")
		if i < 4 {
			extra := []string{"rankneedle filler filler close", "rankneedle close", "rankneedle the close", "rankneedle close rankneedle close"}[i]
			exec("UPDATE catalog_products SET description=CONCAT(description,?) WHERE id=?", extra, id)
		}
		exec("INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES(?,?,'search-english-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, id)
		exec("INSERT INTO catalog_category_products(product_id,category_id) VALUES(?,'search-english-category')", id)
	}
	for _, option := range []struct{ id, title string }{
		{"search-english-material", " NativeMaterial "}, {"search-english-finish", " NativeFinish "},
	} {
		exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES(?,?,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", option.id, option.title)
		for _, value := range []struct{ id, text string }{{"shared", " ChoiceShared "}, {"other", "ChoiceOther"}} {
			exec("INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", option.id+"-"+value.id, option.id, value.text)
		}
	}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("search-english-%02d", i)
		option := []string{"search-english-material", "search-english-finish"}[i%2]
		value := []string{"shared", "other"}[i/2]
		exec("INSERT INTO catalog_product_options(id,product_id,product_option_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id+"-option", id, option)
		exec("INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id+"-value", id+"-option", option+"-"+value)
	}
	exec("INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at,deleted_at) VALUES('search-english-deleted','search-english-material','ChoiceDeleted',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('search-english-unselected','search-english-material','ChoiceUnselected',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)")
	exec("INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES('search-english-deleted-link','search-english-00-option','search-english-deleted',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	longTitle, longValue := strings.Repeat("型", 100), strings.Repeat("值", 500)
	exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('search-long-option',?,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "\ufeff"+longTitle+"\ufeff")
	exec("INSERT INTO catalog_option_values(id,option_id,value,created_at,updated_at) VALUES('search-long-value','search-long-option',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "\ufeff"+longValue+"\ufeff")
	exec("INSERT INTO catalog_product_options(id,product_id,product_option_id,created_at,updated_at) VALUES('search-long-link','search-english-04','search-long-option',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES('search-long-value-link','search-long-link','search-long-value',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	router := gin.New()
	router.POST("/search", StoreHandler{DB: db, StoreID: "search-english-store"}.Search)
	for _, tc := range []struct {
		query string
		count int
	}{{"quantities", 30}, {"verifying", 30}, {"the", 0}, {"the iphone", 30}, {"iphone the", 30}, {"iphone pr", 30}, {"i13pm", 30}, {"i13 unfindable", 0}, {"iphone unfindable", 0}, {"苹果", 0}, {"", 30}, {"space-gray", 30}, {"space gray", 30}, {"6.1", 30}, {"6 1", 0}, {"a@example.test", 30}, {"a example test", 0}, {"phone128", 0}, {"phones128", 30}, {"1.2e3", 30}, {"8.3.0", 30}, {"catalog.example.test", 30}, {"https://catalog.example.test/phones", 30}, {"/phones", 30}, {"catalog example test", 0}, {"/usr/local/phones.txt", 30}, {"usr local phones", 0}, {"hiddenkeyword", 0}, {"visibleword", 30}, {"amp", 0}, {"scriptsecret", 0}, {"<b>verifying</b>", 30}, {"&amp;", 0}, {"<script>verifying</script>", 0}, {"rankneedle close", 4}, {"nativematerial", 2}, {"nativefinish", 2}, {"choiceshared", 2}, {"nativematerial choiceshared", 1}, {"nativematerial choiceother", 1}, {"choicedeleted", 0}, {"choiceunselected", 0}} {
		body, _ := json.Marshal(StoreSearchRequest{RegionID: "tier-region", Queries: []StoreSearchQuery{{Key: "main", Query: tc.query, Page: 1, Limit: 50, Sort: "relevance"}}})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("POST", "/search", strings.NewReader(string(body))))
		var result struct{ Results []StoreSearchResult }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Results) != 1 {
			t.Fatalf("native search %q HTTP %d: %s", tc.query, w.Code, w.Body.String())
		}
		got := result.Results[0]
		if tc.query == "rankneedle close" {
			want := []string{"search-english-03", "search-english-01", "search-english-02", "search-english-00"}
			if len(got.Hits) != len(want) {
				t.Fatal("rank fixture count", got.Count)
			}
			for i, id := range want {
				if got.Hits[i].ID != id {
					t.Fatalf("cover rank position %d: %s want %s", i, got.Hits[i].ID, id)
				}
			}
		}
		if got.Count != tc.count || len(got.Hits) != tc.count || got.Query != tc.query {
			t.Fatalf("native search %q: count %d/hits %d want %d", tc.query, got.Count, len(got.Hits), tc.count)
		}
	}
	for _, tc := range []struct {
		values []string
		ids    []string
	}{
		{[]string{"NativeMaterial:ChoiceShared"}, []string{"search-english-00"}},
		{[]string{"NativeFinish:ChoiceShared"}, []string{"search-english-01"}},
		{[]string{"NativeMaterial:ChoiceShared", "NativeFinish:ChoiceShared"}, []string{"search-english-00", "search-english-01"}},
		{[]string{"NativeMaterial:ChoiceOther"}, []string{"search-english-02"}},
		{[]string{"ChoiceShared"}, []string{}},
		{[]string{"nativematerial:choiceshared"}, []string{}},
		{[]string{"NativeMaterial:ChoiceDeleted"}, []string{}},
		{[]string{longTitle + ":" + longValue}, []string{"search-english-04"}},
	} {
		body, _ := json.Marshal(StoreSearchRequest{RegionID: "tier-region", Queries: []StoreSearchQuery{{Key: "options", Filters: map[string][]string{"option_values": tc.values}, Sort: "relevance", Page: 1, Limit: 50}}})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("POST", "/search", strings.NewReader(string(body))))
		var result struct{ Results []StoreSearchResult }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Results) != 1 {
			t.Fatal("option filter response", w.Code)
		}
		got := result.Results[0]
		if got.Count != len(tc.ids) || len(got.Hits) != len(tc.ids) {
			t.Fatalf("qualified option %v count %d want %d", tc.values, got.Count, len(tc.ids))
		}
		for i, id := range tc.ids {
			if got.Hits[i].ID != id {
				t.Fatalf("qualified option %v ID %s want %s", tc.values, got.Hits[i].ID, id)
			}
		}
		facets := got.CatalogFacets["option_values"]
		if len(facets) != 5 {
			t.Fatalf("qualified catalog facets %v", facets)
		}
		for _, facet := range facets {
			if facet.Count != 1 || (!strings.Contains(facet.Value, ":Choice") && facet.Value != longTitle+":"+longValue) || strings.Contains(facet.Value, " ") {
				t.Fatalf("source trim/identity %v", facet)
			}
		}
	}
	t.Log(`SEARCH_ENGLISH_NATIVE {"owned_mysql":true,"products":30,"http_queries":50,"stemming":true,"stopwords":true,"last_prefix":true,"alias_all_terms":true,"complex_lexemes":true,"links_and_markup":true,"cover_density_order":true,"qualified_option_values":true,"option_filter_queries":8,"qualified_option_601_chars":true,"source_bom_trim":true,"full_search_parity":false}`)
}
