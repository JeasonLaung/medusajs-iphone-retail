package catalog

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func editVariantFixture() VariantEditInput {
	return VariantEditInput{VariantFields: VariantFields{Title: "自然色 / 128 GB", ManageInventory: true}, OperationID: "d89473cd-86ce-4240-a1c7-d3c9842ea421", ExpectedRevision: strings.Repeat("a", 64), OptionValueIDs: []string{"color-natural", "size-128"}}
}
func TestVariantEditWire(t *testing.T) {
	in := editVariantFixture()
	raw, _ := json.Marshal(in)
	body := string(raw)
	if _, e := decodeVariantEdit(strings.NewReader(body)); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{
		strings.Replace(body, `"sku":null`, `"sku":null,"sku":"other"`, 1),
		strings.Replace(body, `"weight":null`, `"weight":"1.25"`, 1),
		strings.Replace(body, `"weight":null`, `"weight":1e9999`, 1),
		strings.Replace(body, `"manage_inventory":true`, `"manage_inventory":null`, 1),
		strings.Replace(body, `"option_value_ids":["color-natural","size-128"]`, `"option_value_ids":["color-natural","color-natural"]`, 1),
		strings.Replace(body, `"sku":null`, `"sku":null,"metadata":{"private":true}`, 1),
		strings.Replace(body, `"title":"自然色 / 128 GB"`, `"title":""`, 1),
		body + `{}`, body + string([]byte{255}), strings.Repeat(" ", 262145),
	} {
		if _, e := decodeVariantEdit(strings.NewReader(bad)); e == nil {
			t.Fatal("ambiguous or invalid SKU accepted")
		}
	}
	n := math.Inf(1)
	in.Width = &n
	if validVariantEdit(in) {
		t.Fatal("infinite dimension accepted")
	}
}

func TestSourceVariantOptionCollision(t *testing.T) {
	if variantOptionCollision([]string{}, map[string][]string{"other": {}}) {
		t.Fatal("source permits zero-dimensional SKUs")
	}
	if !variantOptionCollision([]string{"black", "128"}, map[string][]string{"other": {"black", "128", "stale-legacy-value"}}) {
		t.Fatal("stale extra value bypassed collision")
	}
	if variantOptionCollision([]string{"black", "256"}, map[string][]string{"other": {"black", "128"}}) {
		t.Fatal("different full configuration rejected")
	}
}
