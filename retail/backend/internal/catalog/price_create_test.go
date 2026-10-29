package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPriceCreateWire(t *testing.T) {
	in := PriceCreateInput{uuid.NewString(), strings.Repeat("a", 64), "eur", "0.000000000000000000"}
	b, _ := json.Marshal(in)
	if _, e := decodePriceCreate(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{string(b) + `{}`, strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":null`, 1), strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":"EUR"`, 1), strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":"eur","currency_code":"usd"`, 1), strings.Replace(string(b), `"amount":"0.000000000000000000"`, `"amount":0`, 1), strings.Replace(string(b), `"currency_code":"eur"`, `"currency_code":"eur","price_set_id":"forged"`, 1)} {
		if _, e := decodePriceCreate(strings.NewReader(body)); e == nil {
			t.Fatal("invalid create wire accepted")
		}
	}
}
