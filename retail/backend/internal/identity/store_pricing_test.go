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

func checkStorePricing(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if db.Exec(sql, args...).Error != nil {
			t.Fatal("store pricing fixture mutation failed")
		}
	}
	exec(`INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('eur','EUR','EUR',2,0,'{"value":"0","precision":20}','Euro',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('price-region','Europe','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_store_currencies(id,store_id,currency_code,is_default,created_at,updated_at) VALUES('public-currency','public-shop','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('public-price-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variants(id,product_id,title,sku,allow_backorder,manage_inventory,variant_rank,created_at,updated_at) VALUES('public-shared','public-a','Shared','0000',FALSE,TRUE,3,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES('public-shared','public-128')`)
	exec(`INSERT INTO catalog_variant_price_sets(variant_id,price_set_id,id,created_at,updated_at) VALUES('public-v','public-price-set','public-price-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-v-incomplete','public-price-set','public-incomplete-price-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-shared','public-price-set','public-shared-price-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_price_lists(id,title,description,type,status,rules_count,created_at,updated_at,metadata) VALUES('public-sale-list','Private sale title','Private sale description','sale','active',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"pricing-secret"}'),('public-override-list','Private override title','Private override description','override','draft',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`)
	for _, r := range []struct {
		id, amount string
		count      int
		list       any
	}{{"public-base", "100", 0, nil}, {"public-regional", "120", 1, nil}, {"public-tier", "80", 1, nil}, {"public-sale", "70", 0, "public-sale-list"}, {"public-override", "150", 0, "public-override-list"}, {"public-unknown", "0.01", 1, nil}, {"public-deleted-price", "0", 0, nil}} {
		raw := fmt.Sprintf(`{"value":"%s","precision":20}`, r.amount)
		exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,price_list_id,created_at,updated_at) VALUES(?,'public-price-set','eur',?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, r.id, r.amount, raw, r.count, r.list)
	}
	exec(`UPDATE pricing_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='public-deleted-price'`)
	exec(`UPDATE pricing_prices SET min_quantity=2,max_quantity=5,raw_min_quantity='{"value":"2","precision":20}',raw_max_quantity='{"value":"5","precision":20}' WHERE id='public-tier'`)
	exec(`INSERT INTO pricing_price_rules(id,price_id,attribute,operator,value,priority,created_at,updated_at) VALUES('public-region-rule','public-regional','region_id','eq','price-region',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-tier-rule','public-tier','region_id','eq','price-region',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-unknown-rule','public-unknown','internal.condition','eq','pricing-secret',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_preferences(id,attribute,value,is_tax_inclusive,created_at,updated_at) VALUES('public-currency-pref','currency_code','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('public-region-pref','region_id','price-region',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	fingerprint := func() []byte {
		t.Helper()
		out := []json.RawMessage{}
		for _, table := range []string{"pricing_prices", "pricing_price_rules", "pricing_price_sets", "pricing_price_lists", "pricing_price_list_rules", "pricing_preferences", "catalog_variant_price_sets", "commerce_regions", "commerce_currencies", "commerce_store_currencies"} {
			var rows []map[string]any
			if db.Table(table).Order("1,2").Find(&rows).Error != nil {
				t.Fatal("pricing fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			out = append(out, b)
		}
		b, _ := json.Marshal(out)
		return b
	}
	call := func(query string, status int, amount, original string, tax bool) {
		t.Helper()
		before := fingerprint()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/store/products/public-a?"+query, nil)
		req.Header.Set("Cookie", "retail_admin_session=invalid")
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("store pricing status %d expected %d", w.Code, status)
		}
		if w.Header().Get("Set-Cookie") != "" || !bytes.Equal(before, fingerprint()) {
			t.Fatal("pricing read changed identity or data")
		}
		for _, secret := range []string{"pricing-secret", "raw_amount", "public-price-set", "public-sale-list", "public-override-list", "Private sale", "Private override", "rules_count", "price_id"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("private pricing field escaped")
			}
		}
		if status != 200 {
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("pricing failure was cacheable")
			}
			return
		}
		var out struct {
			Product      catalog.StoreProduct
			Capabilities map[string]bool
		}
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || !out.Capabilities["pricing"] || out.Capabilities["checkout"] || out.Capabilities["availability"] {
			t.Fatal("regional pricing capability incorrect")
		}
		count := 0
		for _, v := range out.Product.Variants {
			if v.ID == "public-v-incomplete" {
				if v.CalculatedPrice != nil {
					t.Fatal("incomplete variant quoted")
				}
				continue
			}
			q := v.CalculatedPrice
			if amount == "" {
				if q != nil {
					t.Fatal("missing price was fabricated")
				}
				continue
			}
			if q == nil || q.Amount != amount || q.OriginalAmount == nil || *q.OriginalAmount != original || q.Currency != "eur" || q.CalculatedTaxInclusive != tax || q.PriceID != "" || q.SetID != "" {
				t.Fatal("regional quote projection differs")
			}
			count++
		}
		if amount != "" && count != 2 {
			t.Fatal("shared price set did not quote both complete variants")
		}
	}
	call("region_id=price-region", 200, "70", "120", true)
	exec(`UPDATE pricing_price_lists SET status='draft' WHERE id='public-sale-list'`)
	call("region_id=price-region", 200, "120", "120", false)
	call("region_id=price-region&quantity=2", 200, "80", "80", false)
	call("region_id=price-region&quantity=5", 200, "80", "80", false)
	call("region_id=price-region&quantity=6", 200, "120", "120", false)
	exec(`UPDATE pricing_price_lists SET status='active' WHERE id='public-override-list'`)
	call("region_id=price-region", 200, "150", "150", true)
	exec(`UPDATE pricing_price_lists SET status='active' WHERE id='public-sale-list'`)
	call("region_id=price-region", 200, "70", "150", true)
	exec(`UPDATE pricing_prices SET raw_amount='{}' WHERE id='public-base'`)
	call("region_id=price-region", 503, "", "", false)
	exec(`UPDATE pricing_prices SET raw_amount='{"value":"100","precision":20}' WHERE id='public-base'`)
	exec(`UPDATE commerce_store_currencies SET deleted_at=UTC_TIMESTAMP(6) WHERE id='public-currency'`)
	call("region_id=price-region", 503, "", "", false)
	exec(`UPDATE commerce_store_currencies SET deleted_at=NULL WHERE id='public-currency'`)
	call("region_id=missing", 404, "", "", false)
	call("region_id=price-region&currency_code=usd", 400, "", "", false)
	call("region_id=price-region&customer.groups.id=vip", 400, "", "", false)
	exec(`UPDATE pricing_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE price_set_id='public-price-set' AND id!='public-unknown'`)
	call("region_id=price-region", 200, "", "", false)
	exec(`UPDATE pricing_prices SET deleted_at=NULL WHERE price_set_id='public-price-set' AND id!='public-deleted-price'`)
	for start := 0; start < 20000; start += 500 {
		rows := []map[string]any{}
		for i := start; i < start+500; i++ {
			rows = append(rows, map[string]any{"id": fmt.Sprintf("quote-overflow-%05d", i), "price_id": "public-base", "attribute": "region_id", "operator": "eq", "value": "price-region", "priority": 0, "created_at": "2026-10-28 00:00:00", "updated_at": "2026-10-28 00:00:00"})
		}
		if db.Table("pricing_price_rules").Create(rows).Error != nil {
			t.Fatal("quote rule overflow fixture failed")
		}
	}
	call("region_id=price-region", 503, "", "", false)
	exec("DELETE FROM pricing_price_rules WHERE id LIKE 'quote-overflow-%'")
	t.Log(`STORE_PRICING_REHEARSAL {"regional_currency":true,"quantity_bounds":true,"specificity":true,"sale_override_reference":true,"shared_sets":true,"incomplete_variant_unquoted":true,"unknown_condition_unquoted":true,"missing_price_null":true,"exact_raw_validation":true,"tax_preferences":true,"private_fields_excluded":true,"rule_overflow_503":true,"read_fingerprint_unchanged":true}`)
}
