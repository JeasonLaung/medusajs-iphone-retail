package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPriceListCreateWire(t *testing.T) {
	in := PriceListCreateInput{PriceListInput: PriceListInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), Title: "New list", Description: "Initial prices", Status: "active", Type: "sale"}, ProductIDs: []string{"product-1"}, CustomerGroupIDs: []string{}, Prices: []InitialListPrice{{ProductID: "product-1", VariantID: "variant-1", CurrencyCode: "eur", Amount: "12.123456789012345678", MinQuantity: "2", MaxQuantity: "3"}}}
	b, _ := json.Marshal(in)
	raw := string(b)
	if got, e := decodePriceListCreate(strings.NewReader(raw)); e != nil || len(got.Prices) != 1 || got.Prices[0] != in.Prices[0] {
		t.Fatal("valid list creation", e)
	}
	for _, bad := range []string{raw + `{}`, strings.Replace(raw, `"prices":[`, `"prices":null,"ignored":[`, 1), strings.Replace(raw, `"product_ids":["product-1"]`, `"product_ids":[]`, 1), strings.Replace(raw, `"product_ids":["product-1"]`, `"product_ids":["product-1","product-1"]`, 1), strings.Replace(raw, `"customer_group_ids":[]`, `"customer_group_ids":null`, 1), strings.Replace(raw, `"customer_group_ids":[]`, `"customer_group_ids":["vip","vip"]`, 1), strings.Replace(raw, `"variant_id":"variant-1"`, `"variant_id":"variant-1","variant_id":"forged"`, 1), strings.Replace(raw, `"variant_id":"variant-1"`, `"variant_id":null`, 1), strings.Replace(raw, `"product_id":"product-1"`, `"product_id":"outside"`, 1), strings.Replace(raw, `"min_quantity":"2"`, `"min_quantity":"0"`, 1), strings.Replace(raw, `"amount":"12.123456789012345678"`, `"amount":12`, 1), strings.Replace(raw, `"ends_at":null`, `"ends_at":null,"title":"duplicate"`, 1)} {
		if _, e := decodePriceListCreate(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid list creation accepted", bad)
		}
	}
	in.Prices = []InitialListPrice{}
	b, _ = json.Marshal(in)
	if _, e := decodePriceListCreate(strings.NewReader(string(b))); e != nil {
		t.Fatal("source allows no priced rows", e)
	}
}
