package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPriceQuantityEditWire(t *testing.T) {
	five := "5"
	in := PriceQuantityInput{uuid.NewString(), strings.Repeat("a", 64), &five, nil}
	b, _ := json.Marshal(in)
	if _, e := decodePriceQuantityEdit(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	in.MinQuantity = nil
	empty, _ := json.Marshal(in)
	if _, e := decodePriceQuantityEdit(strings.NewReader(string(empty))); e != nil {
		t.Fatal("clear both", e)
	}
	for _, raw := range []string{string(b) + `{}`, strings.Replace(string(b), `"min_quantity":"5"`, `"min_quantity":5`, 1), strings.Replace(string(b), `"min_quantity":"5"`, `"min_quantity":""`, 1), strings.Replace(string(b), `"min_quantity":"5"`, `"min_quantity":"5","min_quantity":null`, 1), strings.Replace(string(b), `"max_quantity":null`, `"max_quantity":"4"`, 1), strings.Replace(string(b), `,"max_quantity":null`, "", 1), strings.TrimSuffix(string(b), "}") + `,"amount":"1"}`} {
		if _, e := decodePriceQuantityEdit(strings.NewReader(raw)); e == nil {
			t.Fatal("invalid quantity edit", raw)
		}
	}
}
