package totals

import (
	"errors"
	"reflect"
	"testing"
)

func TestAllocationFiltersRejectBeforeQuotaAndSharedState(t *testing.T) {
	q := int64(1)
	var checked []string
	filter := func(id, amount string) (bool, error) {
		checked = append(checked, id+":"+amount)
		return id != "first", nil
	}
	in := ItemAllocationInput{Allocation: "once", Type: "fixed", Value: "2", Epsilon: "0.0001", MaxQuantity: &q, Items: []ItemAllocationLine{{ID: "first", Subtotal: "5", Quantity: 1}, {ID: "second", Subtotal: "10", Quantity: 1}}}
	r, err := AllocateItemsFiltered(in, filter)
	if err != nil || len(r.Amounts) != 1 || r.Amounts[0].ID != "second" || !reflect.DeepEqual(checked, []string{"first:2", "second:2"}) || !reflect.DeepEqual(r.Discounts, map[string]string{"second": "2"}) {
		t.Fatalf("item rejection consumed quota/state: %+v %v", r, err)
	}
	sq := 1
	s, err := AllocateShippingFiltered(ShippingAllocationInput{Allocation: "once", Type: "fixed", Value: "2", MaxQuantity: &sq, Methods: []ShippingAllocationMethod{{"first", "5"}, {"second", "10"}}}, filter)
	if err != nil || len(s.Amounts) != 1 || s.Amounts[0].ID != "second" || !reflect.DeepEqual(s.Discounts, map[string]string{"second": "2"}) {
		t.Fatalf("shipping rejection consumed quota/state: %+v %v", s, err)
	}
}

func TestAllocationFilterErrorReturnsNoPartialResults(t *testing.T) {
	failure := errors.New("fixture failure")
	filter := func(string, string) (bool, error) { return false, failure }
	items, err := AllocateItemsFiltered(ItemAllocationInput{Allocation: "each", Type: "fixed", Value: "2", Epsilon: "0.0001", Items: []ItemAllocationLine{{ID: "a", Subtotal: "5", Quantity: 1}}}, filter)
	if err != failure || !reflect.DeepEqual(items, ItemAllocationResult{}) {
		t.Fatal("item callback failure leaked partial result")
	}
	shipping, err := AllocateShippingFiltered(ShippingAllocationInput{Allocation: "each", Type: "fixed", Value: "2", Methods: []ShippingAllocationMethod{{"a", "5"}}}, filter)
	if err != failure || !reflect.DeepEqual(shipping, ShippingAllocationResult{}) {
		t.Fatal("shipping callback failure leaked partial result")
	}
	buy, err := AllocateBuyGetFiltered(BuyGetInput{Code: "p", Type: "fixed", Value: "2", MinimumBuy: 1, ApplyQuantity: 1, Items: []BuyGetLine{{ID: "buy", Subtotal: "10", Quantity: 1, BuyEligible: true}, {ID: "gift", Subtotal: "5", Quantity: 1, TargetEligible: true}}}, filter)
	if err != failure || !reflect.DeepEqual(buy, BuyGetResult{}) {
		t.Fatal("buy-get callback failure leaked partial result")
	}
}
