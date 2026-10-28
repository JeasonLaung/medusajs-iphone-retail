package identity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"
)

func checkStoreLocales(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("locale fixture mutation failed")
		}
	}
	call := func(r http.Handler, path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Cookie", "retail_admin_session=invalid; retail_customer_session=invalid")
		req.Header.Set("Origin", "https://unrelated.example")
		r.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("locale response %d expected %d", w.Code, status)
		}
		expected := "no-store"
		if status == 200 {
			expected = "public, max-age=0, must-revalidate"
		}
		if w.Header().Get("Set-Cookie") != "" || w.Header().Get("Vary") != "" || w.Header().Get("Cache-Control") != expected {
			t.Fatal("locale identity/cache boundary changed")
		}
		return w.Body.Bytes()
	}
	var empty struct {
		Locales []catalog.StoreLocale
		Count   int
	}
	if json.Unmarshal(call(router, "/api/store/locales", 200), &empty) != nil || empty.Locales == nil || len(empty.Locales) != 0 || empty.Count != 0 {
		t.Fatal("empty locale directory differs")
	}
	exec(`INSERT INTO commerce_store_locales(id,locale_code,store_id,created_at,updated_at,deleted_at) VALUES('locale-a','en-GB','public-shop',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('locale-b','zh-CN','public-shop',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('locale-other','fr-FR','other-shop',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('locale-deleted','de-DE','public-shop',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	before := call(router, "/api/store/locales", 200)
	var result struct {
		Locales []catalog.StoreLocale
		Count   int
	}
	if json.Unmarshal(before, &result) != nil || result.Count != 2 || len(result.Locales) != 2 || result.Locales[0].Code != "en-GB" || result.Locales[0].Name != "English (United Kingdom)" || result.Locales[1].Name != "Chinese Simplified (China)" {
		t.Fatal("scoped original locale names differ")
	}
	for _, query := range []string{"?fields=*", "?store_id=other-shop", "?locale=en-GB", "?bad%zz"} {
		call(router, "/api/store/locales"+query, 400)
	}
	for _, code := range []string{"en-GB", "EN-gb", "bad\nvalue", "unknown", "en-XX", string(bytes.Repeat([]byte("a"), 4096))} {
		exec(`INSERT INTO commerce_store_locales(id,locale_code,store_id,created_at,updated_at) VALUES('locale-bad',?,'public-shop',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, code)
		call(router, "/api/store/locales", 503)
		exec("DELETE FROM commerce_store_locales WHERE id='locale-bad'")
	}
	// Check the bound before duplicate/name validation and restore the fixture.
	for i := 0; i < 99; i++ {
		exec(`INSERT INTO commerce_store_locales(id,locale_code,store_id,created_at,updated_at) VALUES(?,'en-GB','public-shop',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, fmt.Sprintf("locale-overflow-%03d", i))
	}
	call(router, "/api/store/locales", 503)
	exec("DELETE FROM commerce_store_locales WHERE id LIKE 'locale-overflow-%'")
	exec("UPDATE commerce_stores SET deleted_at=UTC_TIMESTAMP(6) WHERE id='public-shop'")
	call(router, "/api/store/locales", 503)
	exec("UPDATE commerce_stores SET deleted_at=NULL WHERE id='public-shop'")
	for _, h := range []catalog.StoreHandler{{DB: db}, {DB: db, StoreID: "missing"}, {StoreID: "public-shop"}} {
		r := gin.New()
		r.GET("/locales", h.Locales)
		call(r, "/locales", 503)
	}
	if !bytes.Equal(before, call(router, "/api/store/locales", 200)) {
		t.Fatal("locale reads changed directory")
	}
	exec("DELETE FROM commerce_store_locales WHERE id IN ('locale-a','locale-b','locale-other','locale-deleted')")
	t.Log(`STORE_LOCALE_REHEARSAL {"scoped_store":true,"original_default_names":true,"empty_directory":true,"deleted_and_foreign_excluded":true,"unknown_custom_name_503":true,"duplicate_or_oversized_503":true,"identity_untouched":true}`)
}
