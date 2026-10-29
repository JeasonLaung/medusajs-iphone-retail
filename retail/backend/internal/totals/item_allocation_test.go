package totals

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestOriginalItemAllocation(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/item-allocation.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name  string
			Input struct {
				Allocation, Type, Value, Epsilon string
				IsTaxInclusive                   bool   `json:"is_tax_inclusive"`
				MaxQuantity                      *int64 `json:"max_quantity"`
				Items                            []struct {
					ID, Subtotal   string
					OriginalTotal  string `json:"original_total"`
					Quantity       int64
					IsDiscountable *bool `json:"is_discountable"`
				}
				Previous map[string]string
			}
			Expected struct {
				Amounts []struct {
					ID, Amount     string
					IsTaxInclusive bool `json:"is_tax_inclusive"`
				}
				Discounts map[string]string
			}
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 33 {
		t.Fatal("item fixture coverage changed")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			i := row.Input
			in := ItemAllocationInput{Allocation: i.Allocation, Type: i.Type, Value: i.Value, Epsilon: i.Epsilon, IsTaxInclusive: i.IsTaxInclusive, MaxQuantity: i.MaxQuantity, Previous: i.Previous}
			for _, line := range i.Items {
				in.Items = append(in.Items, ItemAllocationLine{line.ID, line.Subtotal, line.OriginalTotal, line.Quantity, line.IsDiscountable})
			}
			want := ItemAllocationResult{Amounts: []ItemAllocationAmount{}, Discounts: row.Expected.Discounts}
			for _, amount := range row.Expected.Amounts {
				want.Amounts = append(want.Amounts, ItemAllocationAmount{amount.ID, amount.Amount, amount.IsTaxInclusive})
			}
			before, _ := json.Marshal(in)
			got, err := AllocateItems(in)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v %v want %+v", got, err, want)
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("input mutated")
			}
			got.Discounts["new"] = "1"
			if _, ok := in.Previous["new"]; ok {
				t.Fatal("output aliases input")
			}
		})
	}
}

func TestItemAllocationValidation(t *testing.T) {
	base := func() ItemAllocationInput {
		return ItemAllocationInput{Allocation: "each", Type: "fixed", Value: "5", Epsilon: "0.0001", Items: []ItemAllocationLine{{ID: "a", Subtotal: "10", Quantity: 1}}}
	}
	for _, change := range []func(*ItemAllocationInput){
		func(i *ItemAllocationInput) { i.Allocation = "all" },
		func(i *ItemAllocationInput) { i.Type = "money" },
		func(i *ItemAllocationInput) { i.Value = "-1" },
		func(i *ItemAllocationInput) { i.Value = "1e1000000" },
		func(i *ItemAllocationInput) { i.Epsilon = "" },
		func(i *ItemAllocationInput) { i.Epsilon = "-0.0001" },
		func(i *ItemAllocationInput) { i.Items[0].Quantity = 0 },
		func(i *ItemAllocationInput) { i.Items[0].Quantity = 2147483648 },
		func(i *ItemAllocationInput) { i.Items[0].Subtotal = "-1" },
		func(i *ItemAllocationInput) { i.IsTaxInclusive = true },
		func(i *ItemAllocationInput) {
			i.Items = append(i.Items, ItemAllocationLine{ID: "a", Subtotal: "1", Quantity: 1})
		},
		func(i *ItemAllocationInput) {
			i.Items = append(i.Items, ItemAllocationLine{ID: "b", Subtotal: "bad", Quantity: 1})
		},
		func(i *ItemAllocationInput) { i.Previous = map[string]string{"a": "-1"} },
		func(i *ItemAllocationInput) { n := int64(-1); i.MaxQuantity = &n },
		func(i *ItemAllocationInput) { i.Items = make([]ItemAllocationLine, 1001) },
	} {
		in := base()
		change(&in)
		got, err := AllocateItems(in)
		if err != ErrInput || got.Amounts != nil || got.Discounts != nil {
			t.Fatalf("invalid item produced partial discounts: %+v %v", got, err)
		}
	}
}

func TestItemAllocationMixedTaxBasesAndSharedBalance(t *testing.T) {
	in := ItemAllocationInput{Allocation: "each", Type: "fixed", Value: "25", Epsilon: "0.0001", IsTaxInclusive: true, Items: []ItemAllocationLine{{ID: "a", Subtotal: "100", OriginalTotal: "125", Quantity: 1}}}
	first, err := AllocateItems(in)
	if err != nil || first.Discounts["a"] != "20" {
		t.Fatalf("inclusive balance: %+v %v", first, err)
	}
	in.Previous = first.Discounts
	in.Value = "95"
	in.IsTaxInclusive = false
	second, err := AllocateItems(in)
	if err != nil || len(second.Amounts) != 1 || second.Amounts[0].Amount != "80" || second.Discounts["a"] != "100" || first.Discounts["a"] != "20" {
		t.Fatalf("untaxed remainder: %+v %v", second, err)
	}
	in.Previous = second.Discounts
	in.IsTaxInclusive = true
	third, err := AllocateItems(in)
	if err != nil || len(third.Amounts) != 0 || third.Discounts["a"] != "100" {
		t.Fatal("exhausted mixed-base item discounted again")
	}
}
