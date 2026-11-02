package catalog

import (
	"strings"
	"testing"
)

func TestPriceListWire(t *testing.T) {
	good := `{"operation_id":"12345678-1234-4234-8234-123456789012","expected_revision":"` + strings.Repeat("a", 64) + `","title":"Summer","description":"Existing sale","status":"active","type":"sale","starts_at":"2020-01-01T00:00:00.123456Z","ends_at":null}`
	if _, e := decodePriceListInput(strings.NewReader(good)); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{strings.Replace(good, `"active"`, `"paused"`, 1), strings.Replace(good, `"sale"`, `"discount"`, 1), strings.Replace(good, `"Summer"`, `null`, 1), strings.Replace(good, `"ends_at":null`, `"ends_at":42`, 1), strings.Replace(good, `"ends_at":null`, `"ends_at":""`, 1), strings.Replace(good, `.123456Z`, `.1234567Z`, 1), strings.Replace(good, `"ends_at":null`, `"ends_at":null,"ends_at":null`, 1), strings.Replace(good, `,"ends_at":null`, "", 1), strings.Replace(good, `"Summer"`, `" "`, 1), strings.Replace(good, `"Existing sale"`, `""`, 1), good + `{}`, strings.Replace(good, `"ends_at":null`, `"metadata":null`, 1)} {
		if _, e := decodePriceListInput(strings.NewReader(s)); e == nil {
			t.Fatal("accepted invalid price-list command", s)
		}
	}
}
