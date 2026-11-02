package catalog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPriceListBatchWire(t *testing.T) {
	in := PriceListBatchInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), ProductIDs: []string{"p"}, Create: []InitialListPrice{}, Update: []PriceListPriceUpdate{{PriceListPriceRef{"price", "p", "v"}, "12.123456789012345678", "2", "3"}}, Delete: []PriceListPriceRef{{"removed", "p", "v"}}}
	b, _ := json.Marshal(in)
	raw := string(b)
	got, e := decodePriceListBatch(strings.NewReader(raw))
	if e != nil || len(got.Update) != 1 || got.Update[0] != in.Update[0] || got.Delete[0] != in.Delete[0] {
		t.Fatal("batch wire", got, e)
	}
	for _, bad := range []string{raw + `{}`, strings.Replace(raw, `"create":[]`, `"create":null`, 1), strings.Replace(raw, `"update":[`, `"x":0,"update":[`, 1), strings.Replace(raw, `"id":"price"`, `"id":"price","id":"other"`, 1), strings.Replace(raw, `"amount":"12.123456789012345678"`, `"amount":12`, 1), strings.Replace(raw, `"min_quantity":"2"`, `"min_quantity":null`, 1), strings.Replace(raw, `"id":"removed"`, `"id":"price"`, 1), strings.Replace(raw, `"product_ids":["p"]`, `"product_ids":["other"]`, 1), strings.Replace(raw, `"product_id":"p"`, `"product_id":"p","currency_code":"eur"`, 1), strings.Replace(raw, `"max_quantity":"3"`, `"max_quantity":"1"`, 1)} {
		if _, e := decodePriceListBatch(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid batch accepted", bad)
		}
	}
	empty := in
	empty.Update = []PriceListPriceUpdate{}
	empty.Delete = []PriceListPriceRef{}
	if validPriceListBatch(empty) {
		t.Fatal("empty mutation accepted")
	}
	// A full replacement is an explicit delete+create, not a new ID assigned to an update.
	in.Update = []PriceListPriceUpdate{}
	in.Create = []InitialListPrice{{ProductID: "p", VariantID: "v", CurrencyCode: "eur", Amount: "14.000000000000000000"}}
	if !validPriceListBatch(in) {
		t.Fatal("delete/create replacement rejected")
	}
}
