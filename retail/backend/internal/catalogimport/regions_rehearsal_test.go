package catalogimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"

	cataloghttp "github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
)

func checkActualRegions(t *testing.T, ctx context.Context, db *sql.DB, router *gin.Engine, counts map[string]int64) {
	t.Helper()
	reply := httptest.NewRecorder()
	router.ServeHTTP(reply, httptest.NewRequest("GET", "/regions", nil))
	var body struct {
		Regions []cataloghttp.StoreRegion
		Count   int64
	}
	var expected int64
	if reply.Code != 200 || json.Unmarshal(reply.Body.Bytes(), &body) != nil || db.QueryRowContext(ctx, "SELECT COUNT(*) FROM commerce_regions WHERE deleted_at IS NULL").Scan(&expected) != nil || expected != body.Count || expected != int64(len(body.Regions)) {
		t.Fatal("actual public region list differs")
	}
	for _, r := range body.Regions {
		var name, currency string
		var taxes bool
		if db.QueryRowContext(ctx, "SELECT name,currency_code,automatic_taxes FROM commerce_regions WHERE id=? AND deleted_at IS NULL", r.ID).Scan(&name, &currency, &taxes) != nil || name != r.Name || currency != r.CurrencyCode || taxes != r.AutomaticTaxes {
			t.Fatal("actual public region fields differ")
		}
		rows, err := db.QueryContext(ctx, "SELECT iso_2,iso_3,num_code,name,display_name,region_id FROM commerce_countries WHERE region_id=? AND deleted_at IS NULL ORDER BY iso_2 ASC", r.ID)
		if err != nil {
			t.Fatal("actual country query failed")
		}
		countries := []cataloghttp.StoreCountry{}
		for rows.Next() {
			var country cataloghttp.StoreCountry
			if rows.Scan(&country.ISO2, &country.ISO3, &country.NumCode, &country.Name, &country.DisplayName, &country.RegionID) != nil {
				t.Fatal("actual country scan failed")
			}
			countries = append(countries, country)
		}
		if rows.Err() != nil || rows.Close() != nil {
			t.Fatal("actual country iteration failed")
		}
		actual, _ := json.Marshal(r.Countries)
		want, _ := json.Marshal(countries)
		if !bytes.Equal(actual, want) {
			t.Fatal("actual country fields or membership differ")
		}
		detail := httptest.NewRecorder()
		router.ServeHTTP(detail, httptest.NewRequest("GET", "/regions/"+url.PathEscape(r.ID), nil))
		var d struct{ Region cataloghttp.StoreRegion }
		if detail.Code != 200 || json.Unmarshal(detail.Body.Bytes(), &d) != nil {
			t.Fatal("actual region detail unavailable")
		}
		a, _ := json.Marshal(r)
		b, _ := json.Marshal(d.Region)
		if !bytes.Equal(a, b) {
			t.Fatal("actual region detail differs from list")
		}
		counts["http_public_actual_regions"]++
		counts["http_public_actual_countries"] += int64(len(countries))
	}
}
