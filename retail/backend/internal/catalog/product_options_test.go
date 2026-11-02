package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func optionInputFixture() ProductOptionsInput {
	return ProductOptionsInput{OperationID: "d89473cd-86ce-4240-a1c7-d3c9842ea421", ExpectedRevision: strings.Repeat("a", 64), Options: []OptionSelection{{Title: "容量", Values: []OptionSelectionValue{{Value: "128 GB"}, {Value: "256 GB"}}}}}
}
func TestProductOptionsWireRejectsAmbiguity(t *testing.T) {
	in := optionInputFixture()
	raw, _ := json.Marshal(in)
	body := string(raw)
	if _, e := decodeProductOptions(strings.NewReader(body)); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{
		strings.Replace(body, `"title":"容量"`, `"title":"容量","title":"other"`, 1),
		strings.Replace(body, `"value":"128 GB"`, `"value":"128 GB","value":"256 GB"`, 1),
		strings.Replace(body, `"id":null`, `"id":null,"extra":true`, 1),
		strings.Replace(body, `"value":"256 GB"`, `"value":"128 GB"`, 1),
		strings.Replace(body, `"options":[`, `"options":null,"unused":[`, 1),
		strings.Replace(body, `"id":null,"value":"128 GB"`, `"id":"foreign-id","value":"128 GB"`, 1),
		strings.Replace(body, `"title":"容量"`, `"title":" "`, 1),
		body + `{}`, strings.Repeat(" ", 262145),
	} {
		if _, e := decodeProductOptions(strings.NewReader(s)); e == nil {
			t.Fatal("invalid option command accepted", s[:min(len(s), 180)])
		}
	}
	empty := in
	empty.Options = []OptionSelection{}
	if !validProductOptions(empty) {
		t.Fatal("explicit remove-all rejected")
	}
	in.Options[0].Values = nil
	if validProductOptions(in) {
		t.Fatal("empty selected group accepted")
	}
}
