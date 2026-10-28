package inventory

import (
	"math/big"
	"testing"
)

func TestAvailabilityObservations(t *testing.T) {
	variant := AvailableVariant{ID: "v", ManageInventory: true, Complete: true}
	requirements := []availabilityRequirement{{VariantID: "v", InventoryItemID: "item", RequiredQuantity: "2.5", ItemAvailable: true}}
	locations := []string{"a", "b"}
	stock := func(s string) *big.Int {
		n, e := scaled(s)
		if e != nil {
			t.Fatal("bad quantity fixture")
		}
		return n
	}
	safe := map[availabilityPair]*big.Int{{"item", "a"}: stock("10"), {"item", "b"}: stock("15")}
	assert := func(v AvailableVariant, rs []availabilityRequirement, ls []string, levels map[availabilityPair]*big.Int, quantity int64, state, total, maximum string, allowed bool) {
		t.Helper()
		a := observedAvailability(v, rs, ls, levels, big.NewInt(quantity))
		if a.State != state || a.CanFulfill != allowed || total == "" && a.Quantity != nil || total != "" && (a.Quantity == nil || *a.Quantity != total) || maximum == "" && a.MaxFulfillableQuantity != nil || maximum != "" && (a.MaxFulfillableQuantity == nil || *a.MaxFulfillableQuantity != maximum) {
			t.Fatal("channel stock observation differs")
		}
	}
	assert(variant, requirements, locations, safe, 6, "available", "10", "6", true)
	assert(variant, requirements, locations, safe, 7, "out_of_stock", "10", "6", false)
	variant.AllowBackorder = true
	assert(variant, requirements, locations, safe, 7, "backorder", "10", "6", true)
	assert(variant, requirements, locations, nil, 1, "unavailable", "0", "0", false)
	assert(variant, requirements, nil, safe, 1, "unavailable", "", "", false)
	assert(variant, nil, locations, safe, 1, "unavailable", "", "", false)
	variant.ManageInventory = false
	assert(variant, nil, nil, nil, 100, "untracked", "", "", true)
	variant.Complete = false
	assert(variant, nil, locations, safe, 1, "incomplete", "", "", false)
	variant.ManageInventory = true
	variant.Complete = true
	variant.AllowBackorder = false
	precise := map[availabilityPair]*big.Int{{"item", "a"}: stock("9007199254740993.000000000000000001")}
	r := []availabilityRequirement{{InventoryItemID: "item", RequiredQuantity: "0.000000000000000001", ItemAvailable: true}}
	assert(variant, r, []string{"a"}, precise, 1, "available", "9007199254740993000000000000000001", "9007199254740993000000000000000001", true)
	for _, value := range []string{"0", "-1", "NaN", "1/2"} {
		r[0].RequiredQuantity = value
		assert(variant, r, locations, safe, 1, "unavailable", "", "", false)
	}
	r[0].RequiredQuantity = "1"
	r[0].ItemAvailable = false
	assert(variant, r, locations, safe, 1, "unavailable", "", "", false)
}
func TestBundleComponentsMayUseSeparateWarehouses(t *testing.T) {
	v := AvailableVariant{ID: "v", ManageInventory: true, Complete: true}
	r := []availabilityRequirement{{InventoryItemID: "one", RequiredQuantity: "1", ItemAvailable: true}, {InventoryItemID: "two", RequiredQuantity: "1", ItemAvailable: true}}
	a, _ := scaled("10")
	zero := new(big.Int)
	levels := map[availabilityPair]*big.Int{{"one", "a"}: a, {"two", "b"}: a}
	result := observedAvailability(v, r, []string{"a", "b"}, levels, big.NewInt(1))
	if result.Quantity == nil || *result.Quantity != "10" || !result.CanFulfill || result.State != "available" || result.MaxFulfillableQuantity == nil || *result.MaxFulfillableQuantity != "10" {
		t.Fatal("independently stocked components rejected")
	}
	levels[availabilityPair{"two", "a"}] = zero
	levels[availabilityPair{"one", "b"}] = zero
	result = observedAvailability(v, r, []string{"a", "b"}, levels, big.NewInt(1))
	if result.State != "available" || !result.CanFulfill || result.MaxFulfillableQuantity == nil || *result.MaxFulfillableQuantity != "10" {
		t.Fatal("zero local component hid a different stocked warehouse")
	}
	v.AllowBackorder = true
	result = observedAvailability(v, r, []string{"a", "b"}, levels, big.NewInt(11))
	if !result.CanFulfill || result.State != "backorder" {
		t.Fatal("valid common backorder levels rejected")
	}
}
func TestAvailabilityInputBoundary(t *testing.T) {
	for _, s := range []string{"0", "-1", "01", "1.5", "1e2", "1000000001", "NaN"} {
		if _, e := purchaseQuantity(s); e == nil {
			t.Fatal("invalid purchase quantity accepted")
		}
	}
	if _, e := ReadAvailability(nil, nil, "channel", "1"); e == nil {
		t.Fatal("nil storage availability accepted")
	}
}
