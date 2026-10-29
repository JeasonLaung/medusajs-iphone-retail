package totals

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type buyGetWireQuantity struct {
	ID       string
	Quantity int64
}

func buyGetFromWire(m map[string][]buyGetWireQuantity) map[string][]BuyGetQuantity {
	out := map[string][]BuyGetQuantity{}
	for code, rows := range m {
		out[code] = []BuyGetQuantity{}
		for _, row := range rows {
			out[code] = append(out[code], BuyGetQuantity{row.ID, row.Quantity})
		}
	}
	return out
}

func TestOriginalBuyGet(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/buy-get.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name  string
			Input struct {
				Code, Type, Value string
				MinimumBuy        int64  `json:"minimum_buy"`
				ApplyQuantity     int64  `json:"apply_quantity"`
				MaxQuantity       *int64 `json:"max_quantity"`
				Items             []struct {
					ID, Subtotal   string
					Quantity       int64
					BuyEligible    bool `json:"buy_eligible"`
					TargetEligible bool `json:"target_eligible"`
				}
				Previous       map[string]string
				BuyConsumed    map[string][]buyGetWireQuantity `json:"buy_consumed"`
				TargetConsumed map[string][]buyGetWireQuantity `json:"target_consumed"`
			}
			Expected struct {
				Amounts        []BuyGetAmount
				Discounts      map[string]string
				BuyConsumed    map[string][]buyGetWireQuantity `json:"buy_consumed"`
				TargetConsumed map[string][]buyGetWireQuantity `json:"target_consumed"`
			}
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 20 {
		t.Fatal("buy-get fixture coverage changed")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			i := row.Input
			in := BuyGetInput{Code: i.Code, Type: i.Type, Value: i.Value, MinimumBuy: i.MinimumBuy, ApplyQuantity: i.ApplyQuantity, MaxQuantity: i.MaxQuantity, Previous: i.Previous, BuyConsumed: buyGetFromWire(i.BuyConsumed), TargetConsumed: buyGetFromWire(i.TargetConsumed)}
			for _, v := range i.Items {
				in.Items = append(in.Items, BuyGetLine{v.ID, v.Subtotal, v.Quantity, v.BuyEligible, v.TargetEligible})
			}
			want := BuyGetResult{Amounts: row.Expected.Amounts, Discounts: row.Expected.Discounts, BuyConsumed: buyGetFromWire(row.Expected.BuyConsumed), TargetConsumed: buyGetFromWire(row.Expected.TargetConsumed)}
			before, _ := json.Marshal(in)
			got, err := AllocateBuyGet(in)
			want.IterationLimit = row.Name == "original-iteration-limit"
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v %v want %+v", got, err, want)
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("input mutated")
			}
			got.Discounts["new"] = "1"
			got.BuyConsumed["NEW"] = []BuyGetQuantity{{"a", 1}}
			for code, rows := range got.TargetConsumed {
				if len(rows) > 0 {
					rows[0].Quantity++
					got.TargetConsumed[code] = rows
				}
			}
			after, _ = json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("result aliases caller maps or slices")
			}
		})
	}
}

func TestBuyGetPrecisionAndIterationLimit(t *testing.T) {
	in := BuyGetInput{Code: "PRECISION", Type: "fixed", Value: "9007199254740993", MinimumBuy: 2, ApplyQuantity: 1, Items: []BuyGetLine{{"a", "27021597764222979", 3, true, true}}}
	got, err := AllocateBuyGet(in)
	if err != nil || len(got.Amounts) != 1 || got.Amounts[0].Amount != "9007199254740993" || got.Discounts["a"] != "9007199254740993" {
		t.Fatalf("exact shared-map migration: %+v %v", got, err)
	}
	maximum := int64(2000)
	in = BuyGetInput{Code: "LIMIT", Type: "fixed", Value: "1", MinimumBuy: 2, ApplyQuantity: 1, MaxQuantity: &maximum, Items: []BuyGetLine{{"a", "3003", 3003, true, true}}}
	got, err = AllocateBuyGet(in)
	if err != nil || !got.IterationLimit || len(got.Amounts) != 1 || got.Amounts[0].Amount != "1000" || got.BuyConsumed["LIMIT"][0].Quantity != 2000 || got.TargetConsumed["LIMIT"][0].Quantity != 1000 {
		t.Fatalf("bounded legacy iteration: %+v %v", got, err)
	}
}

func TestBuyGetRejectsInvalidInputWithoutPartialResults(t *testing.T) {
	base := func() BuyGetInput {
		return BuyGetInput{Code: "C", Type: "fixed", Value: "1", MinimumBuy: 2, ApplyQuantity: 1, Items: []BuyGetLine{{"a", "30", 3, true, true}}}
	}
	for _, change := range []func(*BuyGetInput){
		func(i *BuyGetInput) { i.Code = "" }, func(i *BuyGetInput) { i.Type = "money" },
		func(i *BuyGetInput) { i.Value = "-1" }, func(i *BuyGetInput) { i.Value = "1e1000000" },
		func(i *BuyGetInput) { i.MinimumBuy = 0 }, func(i *BuyGetInput) { i.ApplyQuantity = -1 },
		func(i *BuyGetInput) { n := int64(-1); i.MaxQuantity = &n },
		func(i *BuyGetInput) { i.Items[0].Quantity = 0 },
		func(i *BuyGetInput) { i.Items = append(i.Items, BuyGetLine{"a", "1", 1, true, true}) },
		func(i *BuyGetInput) { i.Items = append(i.Items, BuyGetLine{"b", "bad", 1, true, true}) },
		func(i *BuyGetInput) { i.Items = make([]BuyGetLine, 1001) },
		func(i *BuyGetInput) { i.Previous = map[string]string{"a": "-1"} },
		func(i *BuyGetInput) { i.BuyConsumed = map[string][]BuyGetQuantity{"C": {{"a", -1}}} },
		func(i *BuyGetInput) { i.TargetConsumed = map[string][]BuyGetQuantity{"C": {{"a", 2147483648}}} },
	} {
		in := base()
		change(&in)
		got, err := AllocateBuyGet(in)
		if err != ErrInput || !reflect.DeepEqual(got, BuyGetResult{}) {
			t.Fatalf("invalid input yielded partial allocations: %+v %v", got, err)
		}
	}
}
