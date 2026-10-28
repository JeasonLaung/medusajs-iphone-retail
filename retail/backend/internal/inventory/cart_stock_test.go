package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"math/rand/v2"
	"testing"
)

func TestWholeCartSharedCapacity(t *testing.T) {
	variants := []AvailableVariant{{ID: "v", ManageInventory: true, Complete: true}, {ID: "other", ManageInventory: true, Complete: true}}
	facts := &availabilityFacts{locations: []string{"a", "b"}, requirements: map[string][]availabilityRequirement{}, safe: map[availabilityPair]*big.Int{}}
	for _, v := range variants {
		facts.requirements[v.ID] = []availabilityRequirement{{InventoryItemID: "shared", RequiredQuantity: "2.5", ItemAvailable: true}}
	}
	for _, location := range facts.locations {
		facts.safe[availabilityPair{"shared", location}], _ = scaled("10")
	}
	lines := []CartStockLine{{"one", "v", "4"}, {"two", "other", "4"}}
	before, _ := json.Marshal(struct {
		V []AvailableVariant
		L []CartStockLine
	}{variants, lines})
	run := func(want string, allowed bool) CartStockPlan {
		t.Helper()
		result, err := planCartStock(t.Context(), variants, lines, facts, 200000)
		if err != nil || result.State != want || result.CanFulfill != allowed {
			t.Fatalf("whole cart state differs: %s %v", result.State, err)
		}
		return result
	}
	result := run("available", true)
	if len(result.Assignments) != 2 || result.Assignments[0].LocationID == result.Assignments[1].LocationID || result.Assignments[0].Quantity != "10.000000000000000000" {
		t.Fatal("shared demand not assigned across warehouses")
	}
	lines[1].Quantity = "5"
	run("out_of_stock", false)
	lines[0].Quantity = "3"
	lines[1].Quantity = "3"
	delete(facts.safe, availabilityPair{"shared", "b"})
	run("out_of_stock", false)
	variants[1].AllowBackorder = true
	result = run("backorder", true)
	if !result.Assignments[1].AllowBackorder {
		t.Fatal("backorder binding lost")
	}
	delete(facts.safe, availabilityPair{"shared", "a"})
	run("unavailable", false)
	variants[0].ManageInventory = false
	variants[1].ManageInventory = false
	run("available", true)
	variants[1].Complete = false
	run("incomplete", false)
	variants[0].ManageInventory = true
	variants[1].ManageInventory = true
	variants[1].AllowBackorder = false
	variants[1].Complete = true
	lines[0].Quantity = "4"
	lines[1].Quantity = "4"
	after, _ := json.Marshal(struct {
		V []AvailableVariant
		L []CartStockLine
	}{variants, lines})
	if string(before) != string(after) {
		t.Fatal("fixture restoration differs")
	}
	if _, err := planCartStock(t.Context(), variants, lines, facts, 0); err != ErrCartStockLimit {
		t.Fatal("search limit mistaken for out of stock")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := planCartStock(ctx, variants, lines, facts, 200000); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
}

func TestCartStockBacktracksAndMatchesExhaustiveAssignments(t *testing.T) {
	// Greedy a-first places 1+2 at a, then cannot fit both 3s.
	// A feasible assignment is (1+3) at a and (2+3) at b.
	variants := []AvailableVariant{{ID: "v", ManageInventory: true, Complete: true}}
	facts := &availabilityFacts{locations: []string{"a", "b"}, requirements: map[string][]availabilityRequirement{"v": {{InventoryItemID: "item", RequiredQuantity: "1", ItemAvailable: true}}}, safe: map[availabilityPair]*big.Int{}}
	for loc, amount := range map[string]string{"a": "4", "b": "5"} {
		facts.safe[availabilityPair{"item", loc}], _ = scaled(amount)
	}
	lines := []CartStockLine{{"a", "v", "1"}, {"b", "v", "2"}, {"c", "v", "3"}, {"d", "v", "3"}}
	result, err := planCartStock(t.Context(), variants, lines, facts, 200000)
	if err != nil || !result.CanFulfill {
		t.Fatal("feasible non-greedy cart rejected")
	}
	// Independent exhaustive integer oracle enumerates all component/warehouse assignments,
	// with shared component demands and indivisible component/warehouse placement.
	rng := rand.New(rand.NewPCG(17, 41))
	for sample := 0; sample < 250; sample++ {
		capacities := [3][2]int{}
		for l := range capacities {
			for item := range capacities[l] {
				capacities[l][item] = rng.IntN(8)
			}
		}
		needs := make([][2]int, 5)
		vs := make([]AvailableVariant, 5)
		ls := make([]CartStockLine, 5)
		f := &availabilityFacts{locations: []string{"a", "b", "c"}, requirements: map[string][]availabilityRequirement{}, safe: map[availabilityPair]*big.Int{}}
		for l, location := range f.locations {
			for item, id := range []string{"x", "y"} {
				f.safe[availabilityPair{id, location}], _ = scaled(big.NewInt(int64(capacities[l][item])).String())
			}
		}
		for i := range needs {
			id := string(rune('v' + i))
			vs[i] = AvailableVariant{ID: id, ManageInventory: true, Complete: true}
			ls[i] = CartStockLine{ID: id, VariantID: id, Quantity: "1"}
			for item, stockID := range []string{"x", "y"} {
				needs[i][item] = 1 + rng.IntN(4)
				f.requirements[id] = append(f.requirements[id], availabilityRequirement{InventoryItemID: stockID, RequiredQuantity: big.NewInt(int64(needs[i][item])).String(), ItemAvailable: true})
			}
		}
		var exhaustive func(int, [3][2]int) bool
		exhaustive = func(index int, left [3][2]int) bool {
			if index == len(needs)*2 {
				return true
			}
			line, item := index/2, index%2
			for l := range left {
				if left[l][item] >= needs[line][item] {
					next := left
					next[l][item] -= needs[line][item]
					if exhaustive(index+1, next) {
						return true
					}
				}
			}
			return false
		}
		want := exhaustive(0, capacities)
		got, err := planCartStock(t.Context(), vs, ls, f, 200000)
		if err != nil || got.CanFulfill != want {
			t.Fatalf("cart differs from exhaustive assignment sample %d", sample)
		}
		used := map[availabilityPair]*big.Int{}
		for _, a := range got.Assignments {
			p := availabilityPair{a.InventoryItemID, a.LocationID}
			if used[p] == nil {
				used[p] = new(big.Int)
			}
			n, _ := scaled(a.Quantity)
			used[p].Add(used[p], n)
			if used[p].Cmp(f.safe[p]) > 0 {
				t.Fatal("assignment exceeds shared capacity")
			}
		}
		if !want && len(got.Assignments) != 0 {
			t.Fatal("partial assignment escaped rejection")
		}
	}
}

func TestCartStockInputBoundaries(t *testing.T) {
	variants := []AvailableVariant{{ID: "v", Complete: true}}
	for _, lines := range [][]CartStockLine{{{"one", "missing", "1"}}, {{"one", "v", "0"}}, {{"one", "v", "01"}}, {{"one", "v", "1"}, {"one", "v", "1"}}, make([]CartStockLine, 1001)} {
		if _, err := stockLines(variants, lines); err != ErrCartStockInput {
			t.Fatal("bad cart stock input accepted")
		}
	}
	if _, err := ReadCartStock(nil, variants, nil, "channel"); err == nil {
		t.Fatal("nil stock transaction accepted")
	}
}
