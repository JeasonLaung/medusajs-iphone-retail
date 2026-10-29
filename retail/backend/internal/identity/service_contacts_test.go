package identity_test

import (
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func checkServiceContacts(t *testing.T, db *gorm.DB, router http.Handler) {
	t.Helper()
	var before struct{ Metadata *string }
	if db.Table("commerce_stores").Select("metadata").Where("id=?", "public-shop").Take(&before).Error != nil {
		t.Fatal("contact fixture store unavailable")
	}
	defer func() {
		if db.Exec("UPDATE commerce_stores SET metadata=? WHERE id=?", before.Metadata, "public-shop").Error != nil {
			t.Error("contact fixture restore failed")
		}
	}()
	raw := `{"private":"must-not-escape","customer_service_contacts":[{"id":"enabled","name":"Support","department":"pre_sales","whatsapp":"+4512345678","email":"help@example.test","timezone":"Europe/Copenhagen","days":[1,2,3,4,5],"start":"09:00","end":"17:00","availability":"schedule","enabled":true},{"id":"disabled","name":"Hidden","department":"after_sales","whatsapp":"","email":"hidden@example.test","timezone":"UTC","days":[1],"start":"09:00","end":"17:00","availability":"offline","enabled":false}]}`
	if db.Exec("UPDATE commerce_stores SET metadata=? WHERE id=?", raw, "public-shop").Error != nil {
		t.Fatal("contact fixture write failed")
	}
	w := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/store/customer-service", nil)
	request.Header.Set("Cookie", "untrusted=secret")
	router.ServeHTTP(w, request)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "help@example.test") || strings.Contains(w.Body.String(), "hidden@example.test") || strings.Contains(w.Body.String(), "must-not-escape") || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("scoped public contacts differ")
	}
	var after string
	if db.Table("commerce_stores").Select("metadata").Where("id=?", "public-shop").Scan(&after).Error != nil || after != raw {
		t.Fatal("contact read changed settings")
	}
	other := gin.New()
	other.GET("/contacts", catalog.StoreHandler{DB: db, StoreID: "missing-shop"}.Contacts)
	w = httptest.NewRecorder()
	other.ServeHTTP(w, httptest.NewRequest("GET", "/contacts", nil))
	if w.Code != 503 {
		t.Fatal("missing store exposed contacts")
	}
}
