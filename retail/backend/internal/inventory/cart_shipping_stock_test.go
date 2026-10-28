package inventory

import (
	"math/big"
	"testing"
)

func TestShippingStockRestrictedWarehouses(t *testing.T) {
	variants := []AvailableVariant{{ID: "v", ManageInventory: true, Complete: true}}
	lines := []CartStockLine{{ID: "one", VariantID: "v", Quantity: "4"}, {ID: "two", VariantID: "v", Quantity: "4"}}
	facts := &availabilityFacts{locations: []string{"a", "b"}, requirements: map[string][]availabilityRequirement{"v": {{InventoryItemID: "item", RequiredQuantity: "2.5", ItemAvailable: true}}}, safe: map[availabilityPair]*big.Int{}}
	for _, location := range facts.locations {
		facts.safe[availabilityPair{"item", location}], _ = scaled("10")
	}
	run := func(scopes map[string][]string, want bool) CartStockPlan {
		t.Helper()
		allowed, err := shippingLocations(lines, scopes)
		if err != nil {
			t.Fatal(err)
		}
		out, err := planCartStockAtLocations(t.Context(), variants, lines, facts, allowed, 200000)
		if err != nil || out.CanFulfill != want {
			t.Fatalf("warehouse restriction differs: %s %v", out.State, err)
		}
		for _, assignment := range out.Assignments {
			if !allowed[assignment.LineItemID][assignment.LocationID] {
				t.Fatal("assignment escaped shipping warehouses")
			}
		}
		return out
	}
	run(map[string][]string{"one": {"a"}, "two": {"b"}}, true)
	run(map[string][]string{"one": {"a"}, "two": {"a"}}, false)
	run(map[string][]string{"one": {}, "two": {"b"}}, false)
	run(map[string][]string{"one": {"foreign"}, "two": {"b"}}, false)
	variants[0].AllowBackorder = true
	run(map[string][]string{"one": {"a"}, "two": {"a"}}, true)
	delete(facts.safe, availabilityPair{"item", "a"})
	run(map[string][]string{"one": {"a"}, "two": {"b"}}, false)
	variants[0].ManageInventory = false
	run(map[string][]string{"one": {"foreign"}, "two": {"b"}}, false)
	run(map[string][]string{"one": {"a"}, "two": {"b"}}, true)
	for _, scopes := range []map[string][]string{nil, {}, {"one": {"a"}, "unknown": {"b"}}, {"one": {"a", "a"}, "two": {"b"}}, {"one": {""}, "two": {"b"}}} {
		if _, err := shippingLocations(lines, scopes); err != ErrCartStockInput {
			t.Fatal("invalid warehouse scope accepted")
		}
	}
	if _, err := ReadCartStockAtLocations(nil, variants, lines, "channel", map[string][]string{"one": {"a"}, "two": {"b"}}); err != ErrCartStockInput {
		t.Fatal("nil stock transaction accepted")
	}
}
