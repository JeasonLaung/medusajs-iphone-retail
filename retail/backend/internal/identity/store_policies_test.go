package identity_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkStorePolicies(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	var before struct{ Metadata *string }
	if db.Table("commerce_stores").Select("metadata").Where("id=?", "public-shop").Take(&before).Error != nil {
		t.Fatal("policy store unavailable")
	}
	defer func() {
		if db.Exec("UPDATE commerce_stores SET metadata=? WHERE id=?", before.Metadata, "public-shop").Error != nil {
			t.Error("policy restore failed")
		}
	}()
	configured := `{"private":"policy-secret","buy_policies":{"warranty_months":12,"return_days":30,"warranty_note":{"en":" Twelve months ","zh":"十二个月","private":"note-secret"},"return_note":{"en":"Return note","zh":"退货说明"}}}`
	for _, tc := range []struct {
		raw          string
		months, days int
	}{{configured, 12, 30}, {`{}`, 24, 14}, {strings.Replace(configured, `"return_days":30`, `"return_days":366`, 1), 24, 14}, {strings.ReplaceAll(strings.ReplaceAll(configured, `"warranty_months":12`, `"warranty_months":0`), `"return_days":30`, `"return_days":0`), 0, 0}} {
		if db.Exec("UPDATE commerce_stores SET metadata=? WHERE id=?", tc.raw, "public-shop").Error != nil {
			t.Fatal("policy fixture failed")
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/store/buy-policies", nil)
		req.Header.Set("Cookie", "untrusted=secret")
		router.ServeHTTP(w, req)
		var out struct{ Policies catalog.BuyPolicies }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Policies.WarrantyMonths != tc.months || out.Policies.ReturnDays != tc.days {
			t.Fatal("policy stored/default values differ")
		}
		if strings.Contains(w.Body.String(), "secret") || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("policy public boundary failed")
		}
		var after string
		if db.Table("commerce_stores").Select("metadata").Where("id=?", "public-shop").Scan(&after).Error != nil || after != tc.raw {
			t.Fatal("policy read changed stored metadata")
		}
	}
	other := gin.New()
	other.GET("/policy", catalog.StoreHandler{DB: db, StoreID: "missing-shop"}.Policies)
	w := httptest.NewRecorder()
	other.ServeHTTP(w, httptest.NewRequest("GET", "/policy", nil))
	if w.Code != 503 {
		t.Fatal("missing shop policy exposed")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/store/buy-policies?store=other", nil))
	if w.Code != 400 {
		t.Fatal("caller selected policy shop")
	}
	t.Log(`STORE_POLICIES_REHEARSAL {"stored_values":true,"whole_default":true,"explicit_zero":true,"private_projection":true,"no_read_writes":true,"store_binding":true}`)
}
