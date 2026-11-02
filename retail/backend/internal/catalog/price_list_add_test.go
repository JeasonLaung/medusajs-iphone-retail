package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPriceListAddWire(t *testing.T) {
	in := PriceListAddInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), ProductIDs: []string{"p"}, Prices: []InitialListPrice{{ProductID: "p", VariantID: "v", CurrencyCode: "eur", Amount: "12.123456789012345678"}}}
	b, _ := json.Marshal(in)
	raw := string(b)
	if got, e := decodePriceListAdd(strings.NewReader(raw)); e != nil || len(got.Prices) != 1 || got.Prices[0] != in.Prices[0] {
		t.Fatal("batch add wire", e)
	}
	for _, bad := range []string{raw + `{}`, strings.Replace(raw, `"product_ids":["p"]`, `"product_ids":[]`, 1), strings.Replace(raw, `"product_ids":["p"]`, `"product_ids":["p","p"]`, 1), strings.Replace(raw, `"variant_id":"v"`, `"variant_id":"v","variant_id":"other"`, 1), strings.Replace(raw, `"region_id":""`, `"region_id":null`, 1), strings.Replace(raw, `"amount":"12.123456789012345678"`, `"amount":12`, 1), strings.Replace(raw, `"prices":[`, `"unknown":null,"prices":[`, 1)} {
		if _, e := decodePriceListAdd(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid batch accepted", bad)
		}
	}
}
