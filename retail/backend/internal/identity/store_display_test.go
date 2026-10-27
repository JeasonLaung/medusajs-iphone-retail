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

func checkStoreDisplay(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("store display fixture failed")
		}
	}
	fingerprint := func() []byte {
		t.Helper()
		var values []any
		for _, table := range []string{"catalog_products", "catalog_variants"} {
			var rows []map[string]any
			if db.Table(table).Order("id").Find(&rows).Error != nil {
				t.Fatal("store display fingerprint failed")
			}
			values = append(values, rows)
		}
		b, _ := json.Marshal(values)
		return b
	}
	call := func(path string, status int) catalog.StoreProduct {
		t.Helper()
		before := fingerprint()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Cookie", "retail_admin_session=invalid")
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatal("store display response status differs")
		}
		if !bytes.Equal(before, fingerprint()) {
			t.Fatal("store display read changed source evidence")
		}
		if w.Header().Get("Set-Cookie") != "" || status != 200 && w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("store display cache or identity boundary failed")
		}
		for _, secret := range []string{"private-display-secret", "source_file_sha256", "source_url", "metadata", "demo_stock_seed"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("private display source escaped")
			}
		}
		if status != 200 {
			return catalog.StoreProduct{}
		}
		var body struct{ Product catalog.StoreProduct }
		if json.Unmarshal(w.Body.Bytes(), &body) != nil {
			t.Fatal("store display decode failed")
		}
		return body.Product
	}
	exec(`UPDATE catalog_products SET metadata=? WHERE id='public-a'`, `{"source":"swappie-html-snapshot","source_url":"private-display-secret","source_file_sha256":"private-display-secret","card_colors":[{"name":"Blue","hex":"#0000FF","private":"private-display-secret"}],"apple_model_specs":{"release_year":2021,"display_size_inches":6.1,"tech_specs_url":"https://support.apple.com/specs","private":"private-display-secret"},"apple_launch_prices":{"128 GB":{"amount":9007199254740993.005,"currency_code":"eur","private":"private-display-secret"}}}`)
	exec(`UPDATE catalog_variants SET metadata=? WHERE id='public-v'`, `{"slug":"iphone-13-128gb-blue-S85","demo_price_basis":"model-starting-price","apple_launch_price":{"amount":869.125000000000000001,"currency_code":"eur"},"demo_stock_seed":"private-display-secret"}`)
	p := call("/api/store/products/public-a", 200)
	if len(p.Display.Colors) != 1 || p.Display.Specs == nil || p.Display.Specs.ReleaseYear != "2021" || p.Display.Specs.DisplaySize != "6.1" || p.Display.Source == nil || len(p.Display.LaunchPrices) != 1 || p.Display.LaunchPrices[0].Amount != "9007199254740993.005" {
		t.Fatal("relational product display projection differs")
	}
	found := false
	for _, v := range p.Variants {
		if v.ID == "public-v" {
			found = v.Display.Slug != nil && *v.Display.Slug == "iphone-13-128gb-blue-S85" && v.Display.DemoPriceBasis != nil && v.Display.LaunchPrice != nil && v.Display.LaunchPrice.Amount == "869.125000000000000001"
		}
	}
	if !found {
		t.Fatal("relational variant display projection differs")
	}
	exec(`UPDATE catalog_products SET metadata=? WHERE id='public-a'`, `{"apple_model_specs":null,"release_year":2020,"card_colors":[{"name":"bad","hex":"url(private-display-secret)"}],"source":"private-display-secret"}`)
	p = call("/api/store/products/public-a", 200)
	if p.Display.Specs == nil || p.Display.Specs.ReleaseYear != "2020" || p.Display.Source != nil || len(p.Display.Colors) != 0 {
		t.Fatal("legacy optional display fallback differs")
	}
	exec(`UPDATE catalog_products SET metadata=? WHERE id='public-a'`, `{"private":"`+strings.Repeat("x", 65536)+`"}`)
	call("/api/store/products/public-a", 503)
	exec(`UPDATE catalog_products SET metadata=NULL WHERE id='public-a'`)
	exec(`UPDATE catalog_variants SET metadata=? WHERE id='public-v'`, `{"private":"`+strings.Repeat("x", 65536)+`"}`)
	call("/api/store/products/public-a", 503)
	exec(`UPDATE catalog_variants SET metadata=NULL WHERE id='public-v'`)
	exec(`UPDATE catalog_products SET metadata=? WHERE id='public-b'`, `{"private":"`+strings.Repeat("x", 65536)+`"}`)
	call("/api/store/products/public-a", 200)
	call("/api/store/products/public-b", 404)
	rows := []map[string]any{}
	for i := 0; i < 66; i++ {
		rows = append(rows, map[string]any{"id": fmt.Sprintf("display-overflow-%d", i), "product_id": "public-a", "title": "Oversized combined display", "manage_inventory": true, "allow_backorder": false, "metadata": `{"private":"` + strings.Repeat("x", 64000) + `"}`, "created_at": "2026-10-28 00:00:00", "updated_at": "2026-10-28 00:00:00"})
	}
	if db.Table("catalog_variants").Create(rows).Error != nil {
		t.Fatal("display total budget fixture failed")
	}
	call("/api/store/products/public-a", 503)
	exec(`DELETE FROM catalog_variants WHERE id LIKE 'display-overflow-%'`)
	call("/api/store/products/public-a", 200)
	t.Log(`STORE_DISPLAY_REHEARSAL {"reviewed_fields_only":true,"exact_launch_prices":true,"original_slug_and_demo_note":true,"nested_specs_and_root_fallback":true,"invalid_optional_values_omitted":true,"visible_scope_only":true,"per_record_and_total_bounds":true,"read_fingerprint_unchanged":true}`)
}
