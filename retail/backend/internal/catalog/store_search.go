package catalog

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Native read-only search. No arbitrary index names, SQL fields, channels or
// currencies can be selected by callers. All requests share one catalog snapshot.
type StoreSearchQuery struct {
	Key      string              `json:"key"`
	Query    string              `json:"query"`
	Filters  map[string][]string `json:"filters"`
	Sort     string              `json:"sort"`
	Page     int                 `json:"page"`
	Limit    int                 `json:"limit"`
	MinPrice *string             `json:"min_price"`
	MaxPrice *string             `json:"max_price"`
}
type StoreSearchRequest struct {
	RegionID string             `json:"region_id"`
	Queries  []StoreSearchQuery `json:"queries"`
}
type SearchPrice struct {
	Min      string  `json:"min"`
	Max      string  `json:"max"`
	Original *string `json:"original"`
	OnSale   bool    `json:"on_sale"`
}
type StoreSearchHit struct {
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	Handle          string              `json:"handle"`
	Thumbnail       *string             `json:"thumbnail"`
	Values          map[string][]string `json:"values"`
	Colors          []ModelColor        `json:"colors"`
	ModelStorages   []string            `json:"model_storage_options"`
	StockedStorages []string            `json:"stocked_storage_options"`
	StockedColors   []string            `json:"stocked_color_options"`
	Price           *SearchPrice        `json:"price"`
	created         time.Time
	description     string
}
type SearchFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}
type StoreSearchResult struct {
	Key               string                   `json:"key"`
	Query             string                   `json:"query"`
	Currency          string                   `json:"currency_code"`
	Hits              []StoreSearchHit         `json:"hits"`
	Count             int                      `json:"count"`
	Page              int                      `json:"page"`
	Limit             int                      `json:"limit"`
	Facets            map[string][]SearchFacet `json:"facets"`
	CatalogFacets     map[string][]SearchFacet `json:"catalog_facets"`
	PriceDistribution []string                 `json:"price_distribution"`
}

var searchAttributes = []string{"category", "model", "storage_options", "battery_options", "display_size", "color_families", "rear_cameras", "condition_options", "release_year", "labels", "option_values"}
var searchSorts = map[string]bool{"created_at:desc": true, "relevance": true, "price:asc": true, "price:desc": true, "title:asc": true, "title:desc": true}

func searchTextOK(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !containsControl(s)
}
func searchMoney(s string) (*big.Rat, bool) {
	if len(s) > 100 {
		return nil, false
	}
	value, err := totals.Resolve(totals.Number{Decimal: s})
	if err != nil || strings.HasPrefix(value, "-") {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(value)
	return n, ok
}
func parseSearch(raw []byte) (StoreSearchRequest, error) {
	var r StoreSearchRequest
	// displayJSON also rejects duplicate keys and excessive nesting. It is never
	// used to construct public responses or to pass through arbitrary metadata.
	s := string(raw)
	if _, e := displayJSON(&s); e != nil {
		return r, errListQuery
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&r); e != nil {
		return r, errListQuery
	}
	if d.Decode(new(any)) != io.EOF {
		return r, errListQuery
	}
	if !validProductID(r.RegionID) || len(r.Queries) < 1 || len(r.Queries) > 8 {
		return r, errListQuery
	}
	keys := map[string]bool{}
	attrs := map[string]bool{}
	for _, a := range searchAttributes {
		attrs[a] = true
	}
	for i := range r.Queries {
		q := &r.Queries[i]
		if q.Key == "" || !searchTextOK(q.Key, 64) || keys[q.Key] || !searchTextOK(q.Query, 256) {
			return r, errListQuery
		}
		keys[q.Key] = true
		if q.Sort == "" {
			q.Sort = "created_at:desc"
		}
		if !searchSorts[q.Sort] {
			return r, errListQuery
		}
		if q.Page == 0 {
			q.Page = 1
		}
		if q.Limit == 0 {
			q.Limit = 50
		}
		if q.Page < 1 || q.Page > 2000 || q.Limit < 1 || q.Limit > 100 {
			return r, errListQuery
		}
		for attr, values := range q.Filters {
			if !attrs[attr] || len(values) < 1 || len(values) > 32 {
				return r, errListQuery
			}
			seen := map[string]bool{}
			for _, v := range values {
				if v == "" || !searchTextOK(v, 191) || seen[v] {
					return r, errListQuery
				}
				seen[v] = true
			}
		}
		for _, v := range []*string{q.MinPrice, q.MaxPrice} {
			if v != nil {
				if _, ok := searchMoney(*v); !ok {
					return r, errListQuery
				}
			}
		}
		if q.MinPrice != nil && q.MaxPrice != nil {
			lo, _ := searchMoney(*q.MinPrice)
			hi, _ := searchMoney(*q.MaxPrice)
			if lo.Cmp(hi) > 0 {
				return r, errListQuery
			}
		}
	}
	return r, nil
}

func (h StoreHandler) Search(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 65536))
	q, err := parseSearch(raw)
	if e != nil || err != nil || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid search request")
		return
	}
	if h.DB == nil {
		taxonomyFailure(c, errStoreScope)
		return
	}
	results := []StoreSearchResult{}
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		channel, e := h.channel(tx)
		if e != nil {
			return e
		}
		context, e := h.publicPriceContext(tx, channel, q.RegionID, "1")
		if e != nil {
			return e
		}
		products := []StoreProduct{}
		// Never return partial facets when a resource bound is reached.
		if e = publicProducts(tx, channel).Select("p.id,p.title,p.handle,p.subtitle,p.description,p.thumbnail").Order("p.id").Limit(1001).Scan(&products).Error; e != nil {
			return e
		}
		if len(products) > 1000 {
			return errDetailLimit
		}
		budget := 100000
		for i := range products {
			if e = publicProductRelations(tx, &products[i], &budget); e != nil {
				return e
			}
		}
		if e = loadStoreDisplay(tx, products); e != nil {
			return e
		}
		if e = projectPublicOffers(tx, products, channel, "1", context, time.Now().UTC()); e != nil {
			return e
		}
		hits, e := searchProjection(tx, products)
		if e != nil {
			return e
		}
		for _, query := range q.Queries {
			results = append(results, evaluateSearch(hits, query, context.Currency))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		taxonomyFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"results": results})
}

func addSearchValue(m map[string][]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, old := range m[key] {
		if old == value {
			return
		}
	}
	m[key] = append(m[key], value)
}
func searchStrings(m map[string]any, key string) []string {
	out := []string{}
	if a, ok := m[key].([]any); ok {
		for _, v := range a {
			if s, ok := v.(string); ok {
				add := strings.TrimSpace(s)
				if add != "" {
					out = append(out, add)
				}
			}
		}
	}
	return out
}
func searchProjection(tx *gorm.DB, products []StoreProduct) ([]StoreSearchHit, error) {
	out := []StoreSearchHit{}
	if len(products) == 0 {
		return out, nil
	}
	ids := []string{}
	for _, p := range products {
		ids = append(ids, p.ID)
	}
	var rows []struct {
		ID        string
		Metadata  *string
		CreatedAt time.Time
		Size      int
	}
	if e := tx.Table("catalog_products").Select("id,LEFT(metadata,65537) AS metadata,OCTET_LENGTH(metadata) AS size,created_at").Where("id IN ?", ids).Scan(&rows).Error; e != nil {
		return nil, e
	}
	meta := map[string]map[string]any{}
	dates := map[string]time.Time{}
	for _, row := range rows {
		if row.Size > 65536 {
			return nil, errDetailLimit
		}
		m, e := displayJSON(row.Metadata)
		if e != nil {
			return nil, e
		}
		meta[row.ID] = m
		dates[row.ID] = row.CreatedAt
	}
	var links []struct{ ProductID, Name string }
	if e := tx.Table("catalog_category_products cp").Select("cp.product_id,c.name").Joins("JOIN catalog_categories c ON c.id=cp.category_id AND c.deleted_at IS NULL AND c.is_active=TRUE AND c.is_internal=FALSE").Where("cp.product_id IN ?", ids).Order("cp.product_id,c.id").Limit(10001).Scan(&links).Error; e != nil {
		return nil, e
	}
	if len(links) > 10000 {
		return nil, errDetailLimit
	}
	categories := map[string][]string{}
	for _, l := range links {
		categories[l.ProductID] = append(categories[l.ProductID], l.Name)
	}
	links = nil
	if e := tx.Table("catalog_product_tags pt").Select("pt.product_id,t.value AS name").Joins("JOIN catalog_tags t ON t.id=pt.tag_id AND t.deleted_at IS NULL").Where("pt.product_id IN ?", ids).Order("pt.product_id,t.id").Limit(10001).Scan(&links).Error; e != nil {
		return nil, e
	}
	if len(links) > 10000 {
		return nil, errDetailLimit
	}
	tags := map[string][]string{}
	for _, l := range links {
		tags[l.ProductID] = append(tags[l.ProductID], l.Name)
	}
	for _, p := range products {
		m := meta[p.ID]
		hit := StoreSearchHit{ID: p.ID, Title: p.Title, Handle: p.Handle, Thumbnail: p.Thumbnail, Values: map[string][]string{}, Colors: p.Display.Colors, ModelStorages: []string{}, StockedStorages: []string{}, StockedColors: []string{}, created: dates[p.ID]}
		if p.Description != nil {
			hit.description = *p.Description
		}
		for _, attr := range searchAttributes {
			hit.Values[attr] = []string{}
		}
		for _, v := range categories[p.ID] {
			addSearchValue(hit.Values, "category", v)
		}
		for _, v := range tags[p.ID] {
			addSearchValue(hit.Values, "labels", v)
		}
		if m["source"] == "swappie-html-snapshot" {
			addSearchValue(hit.Values, "model", p.Title)
		}
		for _, pair := range [][2]string{{"available_batteries", "battery_options"}, {"available_conditions", "condition_options"}} {
			for _, v := range searchStrings(m, pair[0]) {
				addSearchValue(hit.Values, pair[1], v)
			}
		}
		for _, v := range searchStrings(m, "available_storages") {
			addSearchValue(hit.Values, "model_storage_options", v)
		}
		for _, pair := range [][2]string{{"release_year", "release_year"}, {"display_size_inches", "display_size"}, {"rear_camera_count", "rear_cameras"}} {
			if n, ok := m[pair[0]].(json.Number); ok {
				v := displayScalar(n)
				if v != "" {
					if pair[1] == "display_size" {
						v += " inches"
					}
					if pair[1] == "rear_cameras" {
						if v == "1" {
							v += " rear camera"
						} else {
							v += " rear cameras"
						}
					}
					addSearchValue(hit.Values, pair[1], v)
				}
			}
		}
		titles := map[string]string{}
		for _, o := range p.Options {
			titles[o.ID] = strings.ToLower(strings.TrimSpace(o.Title))
			for _, v := range o.Values {
				addSearchValue(hit.Values, "option_values", v.Value)
			}
		}
		for _, v := range p.Variants {
			stock := false
			if v.ConfigurationComplete && v.InventoryQuantity != nil {
				n, ok := searchMoney(*v.InventoryQuantity)
				stock = ok && n.Sign() > 0
			}
			for _, choice := range v.Options {
				kind := titles[choice.OptionID]
				storage := kind == "storage" || kind == "capacity" || kind == "容量"
				color := kind == "color" || kind == "colour" || kind == "颜色"
				if storage {
					addSearchValue(hit.Values, "storage_options", choice.Value)
					if v.SKU != nil && strings.TrimSpace(*v.SKU) != "" {
						addSearchValue(hit.Values, "model_storage_options", choice.Value)
					}
					if stock {
						addSearchValue(hit.Values, "stocked_storage_options", choice.Value)
					}
				}
				if color && stock {
					addSearchValue(hit.Values, "stocked_color_options", choice.Value)
				}
			}
			if quote := v.CalculatedPrice; quote != nil {
				n, ok := searchMoney(quote.Amount)
				if !ok {
					return nil, errDetailLimit
				}
				if hit.Price == nil {
					hit.Price = &SearchPrice{Min: quote.Amount, Max: quote.Amount, Original: quote.OriginalAmount}
				}
				lo, _ := searchMoney(hit.Price.Min)
				hi, _ := searchMoney(hit.Price.Max)
				if n.Cmp(lo) < 0 {
					hit.Price.Min = quote.Amount
					hit.Price.Original = quote.OriginalAmount
				}
				if n.Cmp(hi) > 0 {
					hit.Price.Max = quote.Amount
				}
			}
		}
		if hit.Price != nil && hit.Price.Original != nil {
			n, ok := searchMoney(*hit.Price.Original)
			lo, _ := searchMoney(hit.Price.Min)
			hit.Price.OnSale = ok && n.Cmp(lo) > 0
		}
		for _, color := range p.Display.Colors {
			if family := searchColorFamily(color.Name); family != "" {
				addSearchValue(hit.Values, "color_families", family)
			}
		}
		for _, key := range []string{"model_storage_options", "stocked_storage_options", "stocked_color_options"} {
			switch key {
			case "model_storage_options":
				hit.ModelStorages = append(hit.ModelStorages, hit.Values[key]...)
			case "stocked_storage_options":
				hit.StockedStorages = append(hit.StockedStorages, hit.Values[key]...)
			default:
				hit.StockedColors = append(hit.StockedColors, hit.Values[key]...)
			}
			delete(hit.Values, key)
		}
		out = append(out, hit)
	}
	return out, nil
}

func searchMatches(hit StoreSearchHit, q StoreSearchQuery, skip string, price bool) bool {
	// The shop's permanent category constraint is independent of refinements.
	iphone := false
	for _, v := range hit.Values["category"] {
		if v == "iPhone" {
			iphone = true
		}
	}
	if !iphone {
		return false
	}
	for attr, selected := range q.Filters {
		if attr == skip {
			continue
		}
		found := false
		for _, v := range hit.Values[attr] {
			for _, want := range selected {
				if v == want {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	if price && (q.MinPrice != nil || q.MaxPrice != nil) {
		if hit.Price == nil {
			return false
		}
		n, _ := searchMoney(hit.Price.Min)
		if q.MinPrice != nil {
			lo, _ := searchMoney(*q.MinPrice)
			if n.Cmp(lo) < 0 {
				return false
			}
		}
		if q.MaxPrice != nil {
			hi, _ := searchMoney(*q.MaxPrice)
			if n.Cmp(hi) > 0 {
				return false
			}
		}
	}
	return true
}
func searchTerms(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// Keyword prototype: all complete words plus a last-word prefix. PostgreSQL
// English stemming and ts_rank_cd parity remain a separate acceptance gate.
func searchRank(h StoreSearchHit, query string) (int, bool) {
	expanded := normalizeSearchQuery(query)
	terms := searchTerms(expanded)
	if len(terms) == 0 {
		return 0, strings.TrimSpace(query) == ""
	}
	fields := []string{h.Title, strings.Join(h.Values["option_values"], " "), h.description}
	weights := []int{3, 2, 1}
	rank := 0
	for i, term := range terms {
		found := false
		for f, text := range fields {
			for _, word := range searchTerms(text) {
				if word == term || expanded == query && i == len(terms)-1 && strings.HasPrefix(word, term) {
					found = true
					rank += weights[f]
				}
			}
		}
		if !found {
			return 0, false
		}
	}
	return rank, true
}

var searchModelQuery = regexp.MustCompile(`(?i)\bi(?:phone)?\s*(\d{1,2})(?:\s*(pro\s*max|pm|pro|plus|mini|p|e))?\b`)

// Same shorthand expansion as the source storefront; the response retains the
// customer's original query. Expanded model queries require complete words.
func normalizeSearchQuery(query string) string {
	return searchModelQuery.ReplaceAllStringFunc(query, func(match string) string {
		parts := searchModelQuery.FindStringSubmatch(match)
		suffix := strings.Join(strings.Fields(strings.ToLower(parts[2])), "")
		return "iPhone " + parts[1] + map[string]string{"p": " Pro", "pro": " Pro", "pm": " Pro Max", "promax": " Pro Max", "plus": " Plus", "mini": " mini", "e": "e"}[suffix]
	})
}
func evaluateSearch(hits []StoreSearchHit, q StoreSearchQuery, currency string) StoreSearchResult {
	r := StoreSearchResult{Key: q.Key, Query: q.Query, Currency: currency, Hits: []StoreSearchHit{}, Page: q.Page, Limit: q.Limit, Facets: map[string][]SearchFacet{}, CatalogFacets: map[string][]SearchFacet{}, PriceDistribution: []string{}}
	candidates := []StoreSearchHit{}
	rank := map[string]int{}
	facets := map[string]map[string]int{}
	catalog := map[string]map[string]int{}
	for _, a := range searchAttributes {
		facets[a] = map[string]int{}
		catalog[a] = map[string]int{}
	}
	for _, h := range hits {
		if searchMatches(h, StoreSearchQuery{}, "", false) {
			for _, a := range searchAttributes {
				for _, v := range h.Values[a] {
					catalog[a][v]++
				}
			}
		}
		score, match := searchRank(h, q.Query)
		if !match {
			continue
		}
		rank[h.ID] = score
		for _, a := range searchAttributes {
			if searchMatches(h, q, a, true) {
				for _, v := range h.Values[a] {
					facets[a][v]++
				}
			}
		}
		if searchMatches(h, q, "", false) && h.Price != nil {
			r.PriceDistribution = append(r.PriceDistribution, h.Price.Min)
		}
		if searchMatches(h, q, "", true) {
			candidates = append(candidates, h)
		}
	}
	convert := func(m map[string]int) []SearchFacet {
		out := []SearchFacet{}
		for v, count := range m {
			out = append(out, SearchFacet{v, count})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
		return out
	}
	for _, a := range searchAttributes {
		r.Facets[a] = convert(facets[a])
		r.CatalogFacets[a] = convert(catalog[a])
	}
	sort.Slice(r.PriceDistribution, func(i, j int) bool {
		a, _ := searchMoney(r.PriceDistribution[i])
		b, _ := searchMoney(r.PriceDistribution[j])
		return a.Cmp(b) < 0
	})
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		cmp := 0
		switch q.Sort {
		case "price:asc", "price:desc":
			if a.Price == nil || b.Price == nil {
				if a.Price == nil && b.Price != nil {
					return false
				}
				if a.Price != nil && b.Price == nil {
					return true
				}
			} else {
				x, _ := searchMoney(a.Price.Min)
				y, _ := searchMoney(b.Price.Min)
				cmp = x.Cmp(y)
				if q.Sort == "price:desc" {
					cmp = -cmp
				}
			}
		case "title:asc", "title:desc":
			cmp = strings.Compare(a.Title, b.Title)
			if q.Sort == "title:desc" {
				cmp = -cmp
			}
		case "relevance":
			cmp = rank[b.ID] - rank[a.ID]
		default:
			cmp = b.created.Compare(a.created)
		}
		if cmp == 0 {
			return a.ID < b.ID
		}
		return cmp < 0
	})
	r.Count = len(candidates)
	start := (q.Page - 1) * q.Limit
	if start < len(candidates) {
		end := start + q.Limit
		if end > len(candidates) {
			end = len(candidates)
		}
		r.Hits = append(r.Hits, candidates[start:end]...)
	}
	return r
}
