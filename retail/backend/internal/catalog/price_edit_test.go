package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPriceAmountWire(t *testing.T) {
	in := PriceAmountInput{uuid.NewString(), strings.Repeat("a", 64), "12345678901234567890.123456789012345678"}
	raw, _ := json.Marshal(in)
	if _, e := decodePriceAmount(strings.NewReader(string(raw))); e != nil {
		t.Fatal(e)
	}
	for _, a := range []string{"1", "01.000000000000000000", "-0.000000000000000000", "-1.000000000000000000", "1e2", " 1.000000000000000000", "100000000000000000000.000000000000000000", "0.0000000000000000001"} {
		in.Amount = a
		b, _ := json.Marshal(in)
		if _, e := decodePriceAmount(strings.NewReader(string(b))); e == nil {
			t.Fatal("coercible amount accepted")
		}
	}
	for _, body := range []string{string(raw) + `{}`, strings.Replace(string(raw), `"amount":`, `"amount":null,"extra":`, 1), strings.Replace(string(raw), `"amount":`, `"amount":"0.000000000000000000","amount":`, 1), strings.Replace(string(raw), `"amount":"12345678901234567890.123456789012345678"`, `"amount":1`, 1)} {
		if _, e := decodePriceAmount(strings.NewReader(body)); e == nil {
			t.Fatal("invalid wire accepted")
		}
	}
}
