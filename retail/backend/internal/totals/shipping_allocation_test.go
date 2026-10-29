package totals

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestOriginalShippingAllocation(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/shipping-allocation.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name  string
			Input struct {
				Allocation, Type, Value string
				MaxQuantity             *int `json:"max_quantity"`
				Methods                 []ShippingAllocationMethod
				Previous                map[string]string
			}
			Expected ShippingAllocationResult
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 20 {
		t.Fatal("shipping fixture coverage changed")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			in := ShippingAllocationInput{row.Input.Allocation, row.Input.Type, row.Input.Value, row.Input.MaxQuantity, row.Input.Methods, row.Input.Previous}
			before, _ := json.Marshal(in)
			got, err := AllocateShipping(in)
			if err != nil || !reflect.DeepEqual(got, row.Expected) {
				t.Fatalf("got %+v %v want %+v", got, err, row.Expected)
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("input mutated")
			}
			got.Discounts["new"] = "10"
			if _, ok := in.Previous["new"]; ok {
				t.Fatal("returned state aliases input")
			}
		})
	}
}

func TestShippingAllocationRejectsInvalidInput(t *testing.T) {
	base := func() ShippingAllocationInput {
		return ShippingAllocationInput{Allocation: "each", Type: "fixed", Value: "5", Methods: []ShippingAllocationMethod{{"a", "10"}}}
	}
	for _, change := range []func(*ShippingAllocationInput){
		func(i *ShippingAllocationInput) { i.Allocation = "all" },
		func(i *ShippingAllocationInput) { i.Type = "money" },
		func(i *ShippingAllocationInput) { i.Value = "-1" },
		func(i *ShippingAllocationInput) { i.Value = "1e1000000" },
		func(i *ShippingAllocationInput) { i.Value = "NaN" },
		func(i *ShippingAllocationInput) { n := -1; i.MaxQuantity = &n },
		func(i *ShippingAllocationInput) { i.Methods = append(i.Methods, ShippingAllocationMethod{"a", "1"}) },
		func(i *ShippingAllocationInput) { i.Methods = append(i.Methods, ShippingAllocationMethod{"b", "bad"}) },
		func(i *ShippingAllocationInput) { i.Methods[0].Subtotal = "-1" },
		func(i *ShippingAllocationInput) { i.Previous = map[string]string{"a": "-1"} },
		func(i *ShippingAllocationInput) { i.Methods = make([]ShippingAllocationMethod, 1001) },
	} {
		in := base()
		change(&in)
		got, err := AllocateShipping(in)
		if err != ErrInput || got.Amounts != nil || got.Discounts != nil {
			t.Fatalf("invalid input yielded partial discount: %+v %v", got, err)
		}
	}
}

func TestShippingAllocationsShareRemainingAmount(t *testing.T) {
	in := ShippingAllocationInput{Allocation: "each", Type: "fixed", Value: "7", Methods: []ShippingAllocationMethod{{"a", "10"}}}
	first, err := AllocateShipping(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Previous = first.Discounts
	second, err := AllocateShipping(in)
	if err != nil || len(second.Amounts) != 1 || second.Amounts[0].Amount != "3" || second.Discounts["a"] != "10" || first.Discounts["a"] != "7" {
		t.Fatalf("shared remainder failure: %+v %v", second, err)
	}
	in.Previous = second.Discounts
	third, err := AllocateShipping(in)
	if err != nil || len(third.Amounts) != 0 || third.Discounts["a"] != "10" {
		t.Fatal("exhausted method discounted again")
	}
}
