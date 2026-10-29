package catalog

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceContactProjection(t *testing.T) {
	contact := map[string]any{"id": "staff-1", "name": " Support ", "department": "after_sales", "whatsapp": "+4512345678", "email": "support@example.test", "timezone": "Europe/Copenhagen", "days": []int{1, 2, 3, 4, 5}, "start": "09:00", "end": "17:00", "availability": "schedule", "enabled": true, "private": "hidden"}
	project := func(rows any) []ServiceContact {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"customer_service_contacts": rows, "secret": "hidden"})
		raw := string(b)
		result, e := serviceContacts(&raw)
		if e != nil {
			t.Fatal(e)
		}
		return result
	}
	result := project([]any{contact})
	if len(result) != 1 || result[0].Name != "Support" {
		t.Fatal("contact projection differs")
	}
	b, _ := json.Marshal(result)
	if strings.Contains(string(b), "hidden") || strings.Contains(string(b), "enabled") {
		t.Fatal("private settings escaped")
	}
	for key, value := range map[string]any{"timezone": "../secret", "start": "25:00", "end": "08:00", "whatsapp": "https://evil.test", "email": "Name <mail@example.test>", "days": []int{7}, "enabled": "true", "department": "private"} {
		changed := map[string]any{}
		for k, v := range contact {
			changed[k] = v
		}
		changed[key] = value
		if len(project([]any{contact, changed})) != 0 {
			t.Fatalf("malformed %s accepted as partial contacts", key)
		}
	}
	if len(project([]any{contact, contact})) != 0 {
		t.Fatal("duplicate ID accepted")
	}
	contact["enabled"] = false
	if len(project([]any{contact})) != 0 {
		t.Fatal("disabled contact published")
	}
	if contacts, e := serviceContacts(nil); e != nil || len(contacts) != 0 {
		t.Fatal("absent settings differ")
	}
	raw := `{"customer_service_contacts":[],"customer_service_contacts":[]}`
	if _, e := serviceContacts(&raw); e == nil {
		t.Fatal("ambiguous metadata accepted")
	}
}
func TestServiceContactsHTTPUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/contacts", StoreHandler{}.Contacts)
	for path, status := range map[string]int{"/contacts": 503, "/contacts?store_id=other": 400} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("public contact boundary differs")
		}
	}
}
