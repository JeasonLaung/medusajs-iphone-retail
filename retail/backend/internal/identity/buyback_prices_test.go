package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkBuybackPrices(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("buyback artificial setup", e)
		}
	}
	rules := `{"version":1,"enabled":true,"bases":{"LIKE_NEW":294,"ALMOST_NEW":270,"GOOD":248,"MODERATE":236},"fault_costs":{"BROKEN":43,"BROKEN_SCREEN":105,"BATTERY_ISSUE":23,"BROKEN_FRAME":99,"BROKEN_BACK_CAMERA":58},"damage_fee":13.98,"floor":25,"battery_threshold":80,"unknown_battery_issue":true,"source":{"name":"private-provenance","country":"dk","fetched_at":"private-time"}}`
	insert := func(id, capacity, amount, raw string, configuration any) {
		t.Helper()
		exec(`INSERT INTO buyback_prices(id,product_id,capacity,amount,raw_amount,pricing_rules,currency_code,created_at,updated_at) VALUES(?,'public-a',?,?,?,?, 'eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, capacity, amount, raw, configuration)
	}
	insert("bb-price", "256GB", "294", `{"value":"294","precision":100}`, rules)
	insert("bb-legacy", "512GB", "10.123456789012345679", `{"value":"10.1234567890123456789","precision":100}`, nil)
	insert("bb-tb", "1TB", "20", `{"value":"20","precision":100}`, "null")
	insert("bb-deleted", "128GB", "50", `{"value":"50","precision":100}`, nil)
	exec("UPDATE buyback_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='bb-deleted'")
	call := func(method, path, body string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if method == "POST" {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Cookie", "retail_customer_session=not-an-identity")
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("buyback %s %s got %d want %d", method, path, w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
			t.Fatal("public buyback cache/cookie boundary")
		}
		for _, private := range []string{"private-provenance", "private-time", "raw_amount", "pricing_rules", "bb-price", "customer_id"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("buyback leaked internal field")
			}
		}
		return w.Body.Bytes()
	}
	var prices struct{ Prices []buyback.Price }
	if json.Unmarshal(call("GET", "/api/store/buyback/prices?product_id=public-a", "", 200), &prices) != nil || len(prices.Prices) != 3 {
		t.Fatal("live buyback prices")
	}
	for _, p := range prices.Prices {
		if p.Capacity == "512GB" && p.Amount != "10.1234567890123456789" {
			t.Fatal("legacy raw precision lost")
		}
	}
	call("GET", "/api/store/buyback/prices", "", 200)
	call("GET", "/api/store/buyback/prices?product_id=missing", "", 200)
	for _, path := range []string{"?product_id=a&product_id=b", "?store=other", "?product_id=" + strings.Repeat("x", 101), "?product_id=%xx", "?"} {
		call("GET", "/api/store/buyback/prices"+path, "", 400)
	}
	body := `{"product_id":"public-a","capacity":"256GB","assessment":{"battery":"79","checks":[true,true,true,true,true],"faults":["bright","pixels"],"glass":["cracked"],"screen":"visible","frame":"none","back":"none"}}`
	var result struct {
		Amount       *json.Number
		Status       string
		Visual       *string
		Faults       []string
		Provisional  bool
		CurrencyCode string `json:"currency_code"`
	}
	estimate := func(input string) []byte { return call("POST", "/api/store/buyback/estimate", input, 200) }
	if json.Unmarshal(estimate(body), &result) != nil || result.Amount == nil || *result.Amount != "152.02" || result.Status != "estimated" || result.Visual == nil || *result.Visual != "LIKE_NEW" || result.Provisional || strings.Join(result.Faults, ",") != "BROKEN_SCREEN,BATTERY_ISSUE" || result.CurrencyCode != "eur" {
		t.Fatal("real rules to estimate differs")
	}
	for _, tc := range []struct{ capacity, status, amount string }{{"128GB", "unconfigured", ""}, {"512GB", "legacy", "10.1234567890123456789"}, {"1024GB", "legacy", "20"}} {
		if json.Unmarshal(estimate(strings.Replace(body, `"256GB"`, `"`+tc.capacity+`"`, 1)), &result) != nil || result.Status != tc.status || tc.amount == "" && result.Amount != nil || tc.amount != "" && (result.Amount == nil || string(*result.Amount) != tc.amount) {
			t.Fatal("missing/legacy/TB semantics")
		}
	}
	exec("UPDATE buyback_prices SET pricing_rules=? WHERE id='bb-price'", strings.Replace(rules, `"enabled":true`, `"enabled":false`, 1))
	if json.Unmarshal(estimate(body), &result) != nil || result.Status != "disabled" || result.Amount != nil {
		t.Fatal("disabled rule ignored")
	}
	if json.Unmarshal(estimate(strings.Replace(body, `[true,true,true,true,true]`, `[false,true,true,true,true]`, 1)), &result) != nil || result.Status != "ineligible" {
		t.Fatal("eligibility precedence")
	}
	exec("UPDATE buyback_prices SET pricing_rules=? WHERE id='bb-price'", rules)
	for _, input := range []string{strings.Replace(body, `"battery":"79"`, `"battery":"101"`, 1), strings.TrimSuffix(body, "}") + `,"amount":999}`, strings.Replace(body, `"product_id":"public-a"`, `"product_id":"public-a","product_id":"other"`, 1)} {
		call("POST", "/api/store/buyback/estimate", input, 400)
	}
	call("POST", "/api/store/buyback/estimate?store=other", body, 400)
	if db.Exec(`INSERT INTO buyback_prices(id,product_id,capacity,amount,raw_amount,currency_code,created_at,updated_at) VALUES('bb-duplicate','public-a','256GB',1,'{"value":"1","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`).Error == nil {
		t.Fatal("active product/capacity duplicate accepted")
	}
	// Source soft-deleted rows do not block a replacement active configuration.
	insert("bb-replacement", "128GB", "15", `{"value":"15","precision":100}`, nil)
	if e := db.Exec(`UPDATE buyback_prices SET raw_amount='not-json' WHERE id='bb-price'`).Error; e == nil {
		t.Fatal("invalid raw JSON stored")
	}
	exec("UPDATE buyback_prices SET raw_amount='{}' WHERE id='bb-price'")
	call("POST", "/api/store/buyback/estimate", body, 503)
	exec(`UPDATE buyback_prices SET raw_amount='{"value":"294","precision":100}',pricing_rules='{}' WHERE id='bb-price'`)
	call("POST", "/api/store/buyback/estimate", body, 503)
	exec("UPDATE buyback_prices SET pricing_rules=? WHERE id='bb-price'", rules)
	// Reads never update source timestamps, identity bindings, or price records.
	var before, after string
	fingerprint := `SELECT GROUP_CONCAT(CONCAT(id,':',amount,':',raw_amount,':',COALESCE(pricing_rules,'SQLNULL'),':',updated_at) ORDER BY id SEPARATOR '|') FROM buyback_prices`
	if db.Raw(fingerprint).Scan(&before).Error != nil {
		t.Fatal("fingerprint")
	}
	estimate(body)
	call("GET", "/api/store/buyback/prices?product_id=public-a", "", 200)
	if db.Raw(fingerprint).Scan(&after).Error != nil || before != after {
		t.Fatal("buyback read mutated source")
	}
	missing := gin.New()
	(buyback.Store{DB: db, StoreID: "missing-shop"}).Register(missing.Group(""))
	w := httptest.NewRecorder()
	missing.ServeHTTP(w, httptest.NewRequest("GET", "/buyback/prices?product_id=public-a", nil))
	if w.Code != 503 {
		t.Fatal("unbound shop exposed price")
	}
	t.Log(`BUYBACK_PRICE_REHEARSAL {"live_prices":3,"real_rules_estimate":"152.02","legacy_raw_precision":true,"missing_null":true,"disabled_and_ineligible":true,"tb_normalization":true,"partial_unique":true,"strict_public_http":true,"read_only_fingerprint":true}`)
}
