package catalog

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func definitionInputFixture() OptionDefinitionInput {
	return OptionDefinitionInput{OperationID: "d89473cd-86ce-4240-a1c7-d3c9842ea421", ExpectedRevision: strings.Repeat("a", 64), Title: "容量", Values: []string{"128 GB", "256 GB"}}
}

func TestOptionDefinitionRejectsAmbiguousWire(t *testing.T) {
	in := definitionInputFixture()
	in.Ranks = map[string]int{"128 GB": 0, "256 GB": -1}
	raw, _ := json.Marshal(in)
	body := string(raw)
	if _, err := decodeOptionDefinition(strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	for _, ranks := range []map[string]int{nil, {}, {"128 GB": 2}} {
		optional := in
		optional.Ranks = ranks
		raw, _ := json.Marshal(optional)
		if _, err := decodeOptionDefinition(strings.NewReader(string(raw))); err != nil {
			t.Fatalf("optional or partial ranks rejected: %s %v", raw, err)
		}
	}
	for _, invalid := range []string{
		strings.Replace(body, `"title":"容量"`, `"title":"容量","title":"other"`, 1),
		strings.Replace(body, `"128 GB":0`, `"128 GB":0,"128 GB":1`, 1),
		strings.Replace(body, `"128 GB":0`, `"missing":0`, 1),
		strings.Replace(body, `"128 GB":0`, `"128 GB":null`, 1),
		strings.Replace(body, `"128 GB":0`, `"128 GB":1.5`, 1),
		strings.Replace(body, `"128 GB":0`, `"128 GB":2147483648`, 1),
		strings.Replace(body, `"values":["128 GB","256 GB"]`, `"values":["128 GB","128 GB"]`, 1),
		strings.Replace(body, `"values":["128 GB","256 GB"]`, `"values":[]`, 1),
		strings.Replace(body, `"title":"容量"`, `"title":null`, 1),
		strings.Replace(body, `"title":"容量"`, `"title":"容量","is_exclusive":true`, 1),
		body + `{}`, strings.Repeat(" ", 262145), string([]byte{0xff}),
	} {
		if _, err := decodeOptionDefinition(strings.NewReader(invalid)); err == nil {
			t.Fatalf("ambiguous command accepted: %.180s", invalid)
		}
	}
}

func TestOptionDefinitionRetainsIdentityAndUnspecifiedRanks(t *testing.T) {
	oldRank := 7
	current := []definitionValue{{ID: "value-a", Value: "128 GB", Rank: &oldRank, Memberships: 3}, {ID: "value-b", Value: "256 GB"}, {ID: "value-c", Value: "512 GB"}}
	before, _ := json.Marshal(current)
	in := definitionInputFixture()
	in.Values = []string{"256 GB", "128 GB", "1 TB"}
	in.Ranks = map[string]int{"256 GB": -1, "1 TB": 0}
	plan, err := planDefinitionValues(current, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Keep) != 3 || plan.Keep[0].ID != "value-b" || *plan.Keep[0].Rank != -1 || plan.Keep[1].ID != "value-a" || *plan.Keep[1].Rank != 7 || plan.Keep[2].ID != "" || *plan.Keep[2].Rank != 0 || !reflect.DeepEqual(plan.Remove, []string{"value-c"}) {
		t.Fatalf("identity/rank reconciliation changed: %+v", plan)
	}
	after, _ := json.Marshal(current)
	if string(before) != string(after) {
		t.Fatal("planning mutated current facts")
	}
}

func TestOptionDefinitionCannotRenameAnAssociatedValue(t *testing.T) {
	// No SKU use is needed to block a removal: product membership suffices.
	current := []definitionValue{{ID: "value-a", Value: "128 GB", Memberships: 1}, {ID: "value-b", Value: "256 GB"}}
	in := definitionInputFixture()
	in.Values = []string{"128GB", "256 GB", "512 GB"}
	plan, err := planDefinitionValues(current, in)
	if !errors.Is(err, ErrOptionDefinitionAssociated) || len(plan.Keep) != 0 || len(plan.Remove) != 0 {
		t.Fatalf("associated value rename returned a partial plan: %+v %v", plan, err)
	}
	current[0].Memberships = 0
	plan, err = planDefinitionValues(current, in)
	if err != nil || plan.Keep[0].ID != "" || !reflect.DeepEqual(plan.Remove, []string{"value-a"}) {
		t.Fatalf("unassociated rename should create a new identity: %+v %v", plan, err)
	}
}

func TestOptionDefinitionRejectsCorruptExistingIdentity(t *testing.T) {
	for _, current := range [][]definitionValue{
		{{ID: "a", Value: "128 GB"}, {ID: "b", Value: "128 GB"}},
		{{ID: "a", Value: "128 GB"}, {ID: "a", Value: "256 GB"}},
		{{ID: "", Value: "128 GB"}},
		{{ID: "a", Value: "128 GB", Memberships: -1}},
	} {
		if _, err := planDefinitionValues(current, definitionInputFixture()); !errors.Is(err, ErrSchema) {
			t.Fatalf("corrupt identity accepted: %+v %v", current, err)
		}
	}
}
