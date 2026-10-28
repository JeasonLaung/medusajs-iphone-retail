package tax

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../../docs/migration/fixtures/legacy-tax-selection.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeItem(t *testing.T, data json.RawMessage) Item {
	t.Helper()
	var item Item
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	_, item.Shipping = fields["shipping_option_id"]
	return item
}

func TestOriginalTaxSelectionAndSystemProvider(t *testing.T) {
	var f struct {
		Cases []struct {
			Name     string          `json:"name"`
			Item     json.RawMessage `json:"item"`
			Rates    []Rate          `json:"rates"`
			Regions  []string        `json:"region_ids"`
			Expected []string        `json:"expected"`
		} `json:"cases"`
		Providers []struct {
			Name   string `json:"name"`
			Groups []struct {
				Item  json.RawMessage `json:"item"`
				Rates []Rate          `json:"rates"`
			} `json:"groups"`
			Expected []Line `json:"expected"`
		} `json:"provider_cases"`
	}
	if err := json.Unmarshal(fixture(t), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) != 24 || len(f.Providers) != 6 {
		t.Fatal("original tax evidence coverage changed")
	}
	for _, test := range f.Cases {
		t.Run("select/"+test.Name, func(t *testing.T) {
			item := decodeItem(t, test.Item)
			before, _ := json.Marshal(test.Rates)
			selected, err := Select(item, test.Rates, test.Regions)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, rate := range selected {
				ids = append(ids, rate.ID)
			}
			if !reflect.DeepEqual(ids, test.Expected) {
				t.Fatalf("got %v want %v", ids, test.Expected)
			}
			after, _ := json.Marshal(test.Rates)
			if string(before) != string(after) {
				t.Fatal("selection mutated configuration")
			}
		})
	}
	for _, test := range f.Providers {
		t.Run("system/"+test.Name, func(t *testing.T) {
			groups := []Group{}
			for _, group := range test.Groups {
				groups = append(groups, Group{Item: decodeItem(t, group.Item), Rates: group.Rates})
			}
			before, _ := json.Marshal(groups)
			lines, err := SystemLines(groups)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(lines, test.Expected) {
				t.Fatalf("got %+v want %+v", lines, test.Expected)
			}
			after, _ := json.Marshal(groups)
			if string(before) != string(after) {
				t.Fatal("provider projection mutated configuration")
			}
		})
	}
}

func ptr(s string) *string { return &s }
func sample() Rate {
	return Rate{ID: "rate", RegionID: "country", Region: Region{ID: "country"}, Rate: ptr("25"), IsDefault: true, Rules: []Rule{{Reference: "product", ReferenceID: "product"}}}
}

func TestTaxSelectionBoundariesAndCopies(t *testing.T) {
	item := Item{ID: "item", ProductID: ptr("product")}
	for _, edit := range []func(*Rate){
		func(r *Rate) { r.ID = "" }, func(r *Rate) { r.Region.ID = "foreign" }, func(r *Rate) { r.Rate = ptr("NaN") }, func(r *Rate) { r.Rate = ptr("1/2") }, func(r *Rate) { r.Rate = ptr(strings.Repeat("9", 257)) }, func(r *Rate) { r.Rules[0].ReferenceID = "" }, func(r *Rate) { r.Region.ParentID = ptr("") },
	} {
		rate := sample()
		edit(&rate)
		if result, err := Select(item, []Rate{rate}, []string{"country"}); err != ErrInput || result != nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	for _, rates := range [][]Rate{{sample(), sample()}, make([]Rate, 10001)} {
		if result, err := Select(item, rates, []string{"country"}); err != ErrInput || result != nil {
			t.Fatal("duplicate/oversize rates accepted")
		}
	}
	if _, err := Select(item, []Rate{sample()}, []string{"country", "country"}); err != ErrInput {
		t.Fatal("duplicate regions accepted")
	}
	if _, err := Select(Item{ID: "item", ShippingOptionID: ptr("option")}, nil, nil); err != ErrInput {
		t.Fatal("ambiguous item kind accepted")
	}
	rate := sample()
	rate.Region.ProvinceCode = ptr("province")
	rate.Region.ParentID = ptr("parent")
	result, err := Select(item, []Rate{rate}, []string{"country"})
	if err != nil || len(result) != 1 {
		t.Fatal(err)
	}
	*result[0].Rate = "0"
	*result[0].Region.ProvinceCode = "changed"
	*result[0].Region.ParentID = "changed"
	result[0].Rules[0].ReferenceID = "changed"
	if *rate.Rate != "25" || *rate.Region.ProvinceCode != "province" || *rate.Region.ParentID != "parent" || rate.Rules[0].ReferenceID != "product" {
		t.Fatal("selection aliases input")
	}
	if result, err := SystemLines([]Group{{Item: item, Rates: []Rate{sample()}}, {Item: item, Rates: []Rate{sample()}}}); err != ErrInput || result != nil {
		t.Fatal("partial projection accepted after duplicate line")
	}
	if result, err := SystemLines([]Group{{Item: item, Rates: []Rate{sample()}}, {Item: Item{ID: ""}, Rates: []Rate{sample()}}}); err != ErrInput || result != nil {
		t.Fatal("partial projection accepted after bad line")
	}
}

func TestTaxSelectionConcurrentInputUnchanged(t *testing.T) {
	item, rates, regions := Item{ID: "item", ProductID: ptr("product")}, []Rate{sample()}, []string{"country"}
	before, _ := json.Marshal(rates)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 20 {
				selected, err := Select(item, rates, regions)
				if err != nil || len(selected) != 1 {
					t.Error("concurrent selection failed")
					return
				}
				lines, err := SystemLines([]Group{{Item: item, Rates: selected}})
				if err != nil || len(lines) != 1 || lines[0].Rate != "25" {
					t.Error("concurrent projection failed")
					return
				}
			}
		})
	}
	wg.Wait()
	after, _ := json.Marshal(rates)
	if string(before) != string(after) {
		t.Fatal("concurrent execution changed configuration")
	}
}
