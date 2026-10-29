package catalog

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStoreSearchValidation(t *testing.T) {
	valid := `{"region_id":"europe","queries":[{"key":"main"}]}`
	q, e := parseSearch([]byte(valid))
	if e != nil || q.Queries[0].Limit != 50 || q.Queries[0].Page != 1 || q.Queries[0].Sort != "created_at:desc" {
		t.Fatal("search defaults")
	}
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"region_id":"europe","queries":[]}`,
		`{"region_id":"europe","queries":[{"key":"a","key":"b"}]}`,
		`{"region_id":"europe","queries":[{"key":"a"},{"key":"a"}]}`,
		`{"region_id":"europe","queries":[{"key":"a","filters":{"status":["draft"]}}]}`,
		`{"region_id":"europe","queries":[{"key":"a","sort":"price; DROP TABLE products"}]}`,
		`{"region_id":"europe","queries":[{"key":"a","min_price":"-1"}]}`,
		`{"region_id":"europe","queries":[{"key":"a","min_price":"1/2"}]}`,
		`{"region_id":"europe","queries":[{"key":"a","min_price":"3","max_price":"2"}]}`,
		`{"region_id":"europe","queries":[{"key":"a","filters":{"category":["iPhone","iPhone"]}}]}`,
		`{"region_id":"europe","queries":[{"key":"a","limit":101}]}`,
		`{"region_id":"europe","queries":[{"key":"a","page":-1}]}`,
		strings.Replace(valid, `"region_id"`, `"channel_id"`, 1), valid + ` {}`,
	} {
		if _, err := parseSearch([]byte(raw)); err == nil {
			t.Fatalf("invalid search accepted: %s", raw)
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/search", StoreHandler{}.Search)
	for _, tc := range []struct {
		path, body string
		status     int
	}{{"/search", valid, 503}, {"/search?channel=other", valid, 400}, {"/search", strings.Repeat("x", 65537), 400}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("search HTTP failure boundary")
		}
	}
}

func TestSearchFacetsPaginationAndExactPrice(t *testing.T) {
	makeHit := func(id, storage, color, price string) StoreSearchHit {
		return StoreSearchHit{ID: id, Title: "iPhone 13 Pro", Values: map[string][]string{"category": {"iPhone"}, "storage_options": {storage}, "color_families": {color}}, Price: &SearchPrice{Min: price, Max: price}}
	}
	hits := []StoreSearchHit{makeHit("a", "128 GB", "Black", "9007199254740992.01"), makeHit("b", "256 GB", "Black", "9007199254740992.02"), makeHit("c", "128 GB", "Blue", "0")}
	missing := makeHit("d", "512 GB", "Black", "0")
	missing.Price = nil
	hits = append(hits, missing)
	hidden := makeHit("hidden", "Secret", "Red", "1")
	hidden.Values["category"] = []string{"Other"}
	hits = append(hits, hidden)
	q := StoreSearchQuery{Key: "main", Filters: map[string][]string{"storage_options": {"128 GB", "256 GB"}, "color_families": {"Black"}}, Sort: "price:asc", Page: 1, Limit: 1}
	r := evaluateSearch(hits, q, "eur")
	if r.Count != 2 || len(r.Hits) != 1 || r.Hits[0].ID != "a" || r.Currency != "eur" {
		t.Fatal("exact decimal ordering or OR/AND semantics")
	}
	if !reflect.DeepEqual(r.Facets["storage_options"], []SearchFacet{{"128 GB", 1}, {"256 GB", 1}, {"512 GB", 1}}) {
		t.Fatalf("disjunctive storage facets: %+v", r.Facets)
	}
	if !reflect.DeepEqual(r.Facets["color_families"], []SearchFacet{{"Black", 2}, {"Blue", 1}}) {
		t.Fatal("disjunctive color facets")
	}
	q.Page = 2
	r = evaluateSearch(hits, q, "eur")
	if r.Hits[0].ID != "b" || len(r.PriceDistribution) != 2 {
		t.Fatal("page independent distribution")
	}
	min := "9007199254740992.02"
	q.MinPrice = &min
	q.Page = 1
	r = evaluateSearch(hits, q, "eur")
	if r.Count != 1 || r.Hits[0].ID != "b" || len(r.PriceDistribution) != 2 {
		t.Fatal("range must not filter its own distribution")
	}
	q = StoreSearchQuery{Key: "all", Sort: "price:desc", Page: 1, Limit: 100}
	r = evaluateSearch(hits, q, "eur")
	if r.Count != 4 || r.Hits[0].ID != "b" || r.Hits[2].ID != "c" || r.Hits[3].ID != "d" {
		t.Fatal("zero price vs missing price")
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "Secret") || strings.Contains(string(raw), "hidden") {
		t.Fatal("fixed category scope leaked")
	}
}

func TestSearchKeywordAndColor(t *testing.T) {
	h := StoreSearchHit{Title: "iPhone 13 Pro", description: "Refurbished phone", Values: map[string][]string{"option_values": {"128 GB"}}}
	for _, query := range []string{"iphone pr", "IPHONE 128", "refurbished phone"} {
		if _, ok := searchRank(h, query); !ok {
			t.Fatal("keyword prefix/all fields")
		}
	}
	for _, query := range []string{"phone secret", "%", "不存在"} {
		if _, ok := searchRank(h, query); ok {
			t.Fatal("nonmatching terms")
		}
	}
	if normalizeSearchQuery("i13pm") != "iPhone 13 Pro Max" || normalizeSearchQuery("i16e") != "iPhone 16e" {
		t.Fatal("model shorthand expansion")
	}
	for input, want := range map[string]string{"  Space   Gray ": "Gray", "(PRODUCT)RED": "Red", "White Titanium": "White", "unknown": ""} {
		if searchColorFamily(input) != want {
			t.Fatal("source color alias")
		}
	}
}
