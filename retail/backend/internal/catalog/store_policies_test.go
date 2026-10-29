package catalog

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuyPolicyProjection(t *testing.T) {
	decode := func(raw string) BuyPolicies {
		t.Helper()
		out, e := buyPolicies(&raw)
		if e != nil {
			t.Fatal("policy projection failed")
		}
		return out
	}
	expected := BuyPolicies{12, 30, PolicyNote{"Twelve months", "十二个月"}, PolicyNote{"Return note", "退货说明"}}
	valid := `{"private":"not-public","buy_policies":{"warranty_months":12,"return_days":30,"warranty_note":{"en":"  Twelve months  ","zh":"十二个月","private":"hidden"},"return_note":{"en":"Return note","zh":"退货说明"}}}`
	if got := decode(valid); got != expected {
		t.Fatalf("configured policies differ: %+v", got)
	}
	if got := decode(strings.Replace(valid, `  Twelve months  `, `\ufeffTwelve months\ufeff`, 1)); got != expected {
		t.Fatal("source JavaScript trim whitespace differs")
	}
	if got := decode(strings.Replace(valid, `"warranty_months":12`, `"warranty_months":12.0`, 1)); got != expected {
		t.Fatal("integral decimal must match source z.number.int")
	}
	for _, raw := range []string{`{}`, `{"buy_policies":null}`, strings.Replace(valid, `"warranty_months":12`, `"warranty_months":-1`, 1), strings.Replace(valid, `"return_days":30`, `"return_days":366`, 1), strings.Replace(valid, `"return_days":30`, `"return_days":0.5`, 1), strings.Replace(valid, `"warranty_months":12`, `"warranty_months":"12"`, 1), strings.Replace(valid, `"warranty_months":12`, `"extra":true,"warranty_months":12`, 1), strings.Replace(valid, `"zh":"退货说明"`, `"zh":"`+strings.Repeat("😀", 101)+`"`, 1)} {
		if got := decode(raw); got != defaultBuyPolicies() {
			t.Fatal("malformed whole settings did not fall back")
		}
	}
	zero := strings.ReplaceAll(strings.ReplaceAll(valid, `"warranty_months":12`, `"warranty_months":0`), `"return_days":30`, `"return_days":0`)
	if got := decode(zero); got.WarrantyMonths != 0 || got.ReturnDays != 0 {
		t.Fatal("explicit zero policy replaced by defaults")
	}
	b, _ := json.Marshal(decode(valid))
	if strings.Contains(string(b), "private") || strings.Contains(string(b), "hidden") {
		t.Fatal("private metadata escaped")
	}
	for _, bad := range []string{`{"buy_policies":{},"buy_policies":{}}`, `{"private":"` + strings.Repeat("x", 65536) + `"}`} {
		if _, e := buyPolicies(&bad); e == nil {
			t.Fatal("unsafe metadata accepted")
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/policies", StoreHandler{}.Policies)
	for _, tc := range []struct {
		path string
		code int
	}{{"/policies", 503}, {"/policies?store=other", 400}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.code || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("policy failure boundary")
		}
	}
}
