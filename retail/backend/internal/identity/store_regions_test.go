package identity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"gorm.io/gorm"
)

// Uses only the owned ProductPrices fixture, after all formal migrations. The
// real server router proves this endpoint needs no admin/customer session.
func checkStoreRegions(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if db.Exec(sql, args...).Error != nil {
			t.Fatal("public region fixture mutation failed")
		}
	}
	for _, table := range []string{"commerce_regions", "commerce_countries"} {
		var count int64
		if db.Table(table).Count(&count).Error != nil || count != 0 {
			t.Fatal("region fixture must be empty")
		}
	}
	call := func(path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Cookie", "retail_admin_session=invalid; retail_customer_session=invalid")
		req.Header.Set("Origin", "https://unrelated.example")
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("public region status %d, expected %d", w.Code, status)
		}
		if w.Header().Get("Set-Cookie") != "" || w.Header().Get("Vary") != "" {
			t.Fatal("public regions touched identity state")
		}
		cache := w.Header().Get("Cache-Control")
		if status == 200 && cache != "public, max-age=0, must-revalidate" || status != 200 && cache != "no-store" {
			t.Fatal("public region cache policy incorrect")
		}
		for _, value := range []string{"must-not-escape", "metadata", "payload", "payment_provider", "created_at", "deleted_at", "auth_version"} {
			if strings.Contains(w.Body.String(), value) {
				t.Fatal("private region fields escaped")
			}
		}
		return w.Body.Bytes()
	}
	empty := call("/api/store/regions", 200)
	if !bytes.Contains(empty, []byte(`"regions":[]`)) {
		t.Fatal("empty region list is not an array")
	}
	call("/api/store/regions/missing", 404)
	stamp := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		id, currency string
		deleted      any
	}{{"public-a", "eur", nil}, {"public-b", "usd", nil}, {"public-deleted", "eur", stamp}} {
		exec("INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,metadata,created_at,updated_at,deleted_at) VALUES(?,?,?,TRUE,?,?,?,?)", r.id, r.id, r.currency, `{"private":"must-not-escape"}`, stamp, stamp, r.deleted)
	}
	for _, c := range []struct {
		iso, num        string
		region, deleted any
	}{{"dk", "208", "public-a", nil}, {"us", "084", "public-a", nil}, {"zz", "999", "public-a", stamp}, {"aa", "000", nil, nil}, {"bb", "001", "public-deleted", nil}} {
		exec("INSERT INTO commerce_countries(iso_2,iso_3,num_code,name,display_name,region_id,metadata,created_at,updated_at,deleted_at) VALUES(?,?,?,'Country','Country label',?,?,?,?,?)", c.iso, "XXX", c.num, c.region, `{"private":"must-not-escape"}`, stamp, stamp, c.deleted)
	}
	fingerprint := func() []byte {
		t.Helper()
		out := []json.RawMessage{}
		for _, table := range []string{"commerce_regions", "commerce_countries"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("region fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			out = append(out, b)
		}
		b, _ := json.Marshal(out)
		return b
	}
	before := fingerprint()
	var list struct {
		Regions []catalog.StoreRegion
		Count   int
	}
	body := call("/api/store/regions", 200)
	if json.Unmarshal(body, &list) != nil || list.Count != 2 || len(list.Regions) != 2 || list.Regions[0].ID != "public-a" || list.Regions[1].CurrencyCode != "usd" || len(list.Regions[1].Countries) != 0 || !list.Regions[0].AutomaticTaxes {
		t.Fatal("region routing fields incorrect")
	}
	countries := list.Regions[0].Countries
	if len(countries) != 2 || countries[0].ISO2 != "dk" || countries[1].ISO2 != "us" || countries[1].NumCode != "084" || countries[0].RegionID != "public-a" {
		t.Fatal("country membership, stable order or code precision changed")
	}
	var detail struct{ Region catalog.StoreRegion }
	if json.Unmarshal(call("/api/store/regions/public-a", 200), &detail) != nil {
		t.Fatal("region detail invalid")
	}
	a, _ := json.Marshal(detail.Region)
	b, _ := json.Marshal(list.Regions[0])
	if !bytes.Equal(a, b) {
		t.Fatal("region detail differs from list")
	}
	call("/api/store/regions/public-deleted", 404)
	for _, path := range []string{"/api/store/regions?fields=*", "/api/store/regions?limit=1", "/api/store/regions?x=1&x=2", "/api/store/regions?bad%zz", "/api/store/regions/public-a?country=dk"} {
		call(path, 400)
	}
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("public reads changed relational evidence")
	}
	exec("UPDATE commerce_regions SET currency_code='EUR' WHERE id='public-a'")
	call("/api/store/regions", 503)
	exec("UPDATE commerce_regions SET currency_code='eur' WHERE id='public-a'")
	exec("UPDATE commerce_countries SET iso_2='DK' WHERE iso_2='dk'")
	call("/api/store/regions/public-a", 503)
	exec("UPDATE commerce_countries SET iso_2='dk' WHERE iso_2='DK'")
	exec("UPDATE catalog_runtime_schema SET reads_enabled=FALSE WHERE id=1")
	call("/api/store/regions", 503)
	exec("UPDATE catalog_runtime_schema SET reads_enabled=TRUE WHERE id=1")
	roles := []map[string]any{}
	for i := 0; i < 99; i++ {
		roles = append(roles, map[string]any{"id": fmt.Sprintf("overflow-r-%03d", i), "name": "Overflow", "currency_code": "eur", "automatic_taxes": false, "created_at": stamp, "updated_at": stamp})
	}
	if db.Table("commerce_regions").Create(roles).Error != nil {
		t.Fatal("region overflow seed failed")
	}
	call("/api/store/regions", 503)
	// A bounded detail remains available even if the complete directory is large.
	call("/api/store/regions/public-b", 200)
	exec("DELETE FROM commerce_regions WHERE id LIKE 'overflow-r-%'")
	rows := []map[string]any{}
	for i := 0; i < 999; i++ {
		rows = append(rows, map[string]any{"iso_2": fmt.Sprintf("z%04d", i), "iso_3": "XXX", "num_code": "000", "name": "Overflow", "display_name": "Overflow", "region_id": "public-a", "created_at": stamp, "updated_at": stamp})
	}
	if db.Table("commerce_countries").Create(rows).Error != nil {
		t.Fatal("country overflow seed failed")
	}
	call("/api/store/regions", 503)
	call("/api/store/regions/public-a", 503)
	exec("DELETE FROM commerce_countries WHERE iso_2 LIKE 'z0%'")
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("failure checks changed original region evidence")
	}
	exec("DELETE FROM commerce_countries")
	exec("DELETE FROM commerce_regions")
	t.Log(`STORE_REGION_REHEARSAL {"migrations":41,"anonymous_cookie_independent":true,"explicit_projection":true,"list_detail_equal":true,"soft_deleted_and_unassigned_excluded":true,"empty_arrays":true,"string_country_codes":true,"strict_query":true,"invalid_routing_values_rejected":true,"directory_limits":true,"schema_failure_closed":true,"public_revalidation":true,"read_fingerprint_unchanged":true}`)
}
