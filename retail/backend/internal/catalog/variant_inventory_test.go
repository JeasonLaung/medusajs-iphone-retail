package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInventoryLinksWirePreservesExactQuantity(t *testing.T) {
	in := VariantInventoryInput{OperationID: "d89473cd-86ce-4240-a1c7-d3c9842ea421", ExpectedRevision: strings.Repeat("a", 64), Items: []InventoryRequirement{{"iitem-fixture", "0.123456789012345678"}}}
	raw, _ := json.Marshal(in)
	body := string(raw)
	out, e := decodeInventoryLinks(strings.NewReader(body))
	if e != nil || out.Items[0].RequiredQuantity != in.Items[0].RequiredQuantity {
		t.Fatal("exact quantity lost", e)
	}
	for _, bad := range []string{
		strings.Replace(body, `"required_quantity":"0.123456789012345678"`, `"required_quantity":0.123456789012345678`, 1),
		strings.Replace(body, `"required_quantity":"0.123456789012345678"`, `"required_quantity":"0"`, 1),
		strings.Replace(body, `"required_quantity":"0.123456789012345678"`, `"required_quantity":"1.0000000000000000001"`, 1),
		strings.Replace(body, `"required_quantity":"0.123456789012345678"`, `"required_quantity":"100000000000000000000"`, 1),
		strings.Replace(body, `"inventory_item_id":"iitem-fixture"`, `"inventory_item_id":"iitem-fixture","inventory_item_id":"other"`, 1),
		strings.Replace(body, `"items":[`, `"stocked_quantity":"999","items":[`, 1),
		strings.Replace(body, `"items":[{"inventory_item_id":"iitem-fixture","required_quantity":"0.123456789012345678"}]`, `"items":null`, 1),
		body + `{}`,
	} {
		if _, e := decodeInventoryLinks(strings.NewReader(bad)); e == nil {
			t.Fatal("ambiguous or lossy inventory command accepted")
		}
	}
	in.Items = []InventoryRequirement{}
	raw, _ = json.Marshal(in)
	if _, e := decodeInventoryLinks(strings.NewReader(string(raw))); e != nil {
		t.Fatal("explicit unlink rejected")
	}
	in.Items = []InventoryRequirement{{"iitem-fixture", "1"}, {"iitem-fixture", "2"}}
	if validInventoryLinks(in) {
		t.Fatal("duplicate identity accepted")
	}
}
