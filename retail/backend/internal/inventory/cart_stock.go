package inventory

import (
	"context"
	"errors"
	"math/big"
	"sort"

	"gorm.io/gorm"
)

var ErrCartStockInput = errors.New("invalid cart stock observation")
var ErrCartStockLimit = errors.New("cart stock observation search limit")

// CartStockLine describes the complete prospective cart, not only the line
// being added. Variant flags must be loaded by the owning catalog/cart domain.
type CartStockLine struct {
	ID, VariantID, Quantity string
}

// CartStockPlan is an internal observation. It neither locks nor reserves stock;
// checkout must revalidate using current SQL locks before applying reservations.
// Assignments are never a public API payload or a durable allocation promise.
type CartStockPlan struct {
	State       string                `json:"state"`
	CanFulfill  bool                  `json:"can_fulfill"`
	Assignments []CartStockAssignment `json:"-"`
}

type CartStockAssignment struct {
	LineItemID, InventoryItemID, LocationID, Quantity string
	AllowBackorder                                    bool
}

type cartStockDemand struct {
	id        string
	component string
	backorder bool
	items     map[string]*big.Int
	locations []string
}

type stockSearch struct {
	ctx       context.Context
	remaining int
}

func (s *stockSearch) step() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	s.remaining--
	if s.remaining < 0 {
		return ErrCartStockLimit
	}
	return nil
}

func stockLines(variants []AvailableVariant, lines []CartStockLine) ([]AvailableVariant, error) {
	if len(lines) > 1000 || len(variants) > 1000 {
		return nil, ErrCartStockInput
	}
	byID := map[string]AvailableVariant{}
	for _, v := range variants {
		if !validID(v.ID) {
			return nil, ErrCartStockInput
		}
		if _, ok := byID[v.ID]; ok {
			return nil, ErrCartStockInput
		}
		byID[v.ID] = v
	}
	seen, needed := map[string]bool{}, map[string]bool{}
	for _, line := range lines {
		if !validID(line.ID) || !validID(line.VariantID) || seen[line.ID] {
			return nil, ErrCartStockInput
		}
		if _, err := purchaseQuantity(line.Quantity); err != nil {
			return nil, ErrCartStockInput
		}
		if _, ok := byID[line.VariantID]; !ok {
			return nil, ErrCartStockInput
		}
		seen[line.ID] = true
		needed[line.VariantID] = true
	}
	selected := make([]AvailableVariant, 0, len(needed))
	for id := range needed {
		selected = append(selected, byID[id])
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	return selected, nil
}

// ReadCartStock uses ONE caller-owned repeatable-read snapshot. Each inventory
// component must fit at one healthy channel-linked location, as in Medusa's
// per-inventory-item preparation/reservation. Components may use different
// locations. Shared item/location capacity is counted across all cart lines.
// No serial-device count, incoming stock or foreign-channel stock is included.
func ReadCartStock(tx *gorm.DB, variants []AvailableVariant, lines []CartStockLine, channel string) (CartStockPlan, error) {
	selected, err := stockLines(variants, lines)
	if err != nil {
		return CartStockPlan{}, err
	}
	if tx == nil || tx.Statement == nil {
		return CartStockPlan{}, ErrCartStockInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return CartStockPlan{}, ErrCartStockInput
	}
	facts, err := readAvailabilityFacts(tx, selected, channel)
	if err != nil {
		return CartStockPlan{}, err
	}
	return planCartStock(tx.Statement.Context, selected, lines, facts, 200000)
}

func planCartStock(ctx context.Context, variants []AvailableVariant, lines []CartStockLine, facts *availabilityFacts, budget int) (CartStockPlan, error) {
	denied := CartStockPlan{State: "unavailable", Assignments: []CartStockAssignment{}}
	search := &stockSearch{ctx: ctx, remaining: budget}
	byID := map[string]AvailableVariant{}
	for _, v := range variants {
		byID[v.ID] = v
	}
	demands := make([]cartStockDemand, 0, len(lines))
	for _, line := range lines {
		if err := search.step(); err != nil {
			return CartStockPlan{}, err
		}
		v := byID[line.VariantID]
		if !v.Complete {
			denied.State = "incomplete"
			return denied, nil
		}
		if !v.ManageInventory {
			continue
		}
		seenItems := map[string]bool{}
		if len(facts.requirements[v.ID]) == 0 {
			return denied, nil
		}
		for _, r := range facts.requirements[v.ID] {
			d := cartStockDemand{id: line.ID, component: r.InventoryItemID, backorder: v.AllowBackorder, items: map[string]*big.Int{}}
			if !r.ItemAvailable || !validID(r.InventoryItemID) {
				return denied, nil
			}
			if seenItems[r.InventoryItemID] {
				return CartStockPlan{}, ErrSchema
			}
			amount, err := RequiredInventoryQuantity(line.Quantity, r.RequiredQuantity)
			if err != nil {
				return CartStockPlan{}, ErrQuantity
			}
			seenItems[r.InventoryItemID] = true
			d.items[r.InventoryItemID], _ = scaled(amount)
			for _, location := range facts.locations {
				healthy, fits := true, true
				for item, need := range d.items {
					if err := search.step(); err != nil {
						return CartStockPlan{}, err
					}
					available, ok := facts.safe[availabilityPair{item, location}]
					if !ok {
						healthy = false
						break
					}
					if available.Cmp(need) < 0 {
						fits = false
					}
				}
				if healthy && (d.backorder || fits) {
					d.locations = append(d.locations, location)
				}
			}
			if len(d.locations) == 0 {
				// Missing/held/mismatched levels are unavailable, even for backorders.
				// A healthy but insufficient level is a normal stock rejection.
				if !d.backorder {
					for _, location := range facts.locations {
						healthy := true
						for item := range d.items {
							if _, ok := facts.safe[availabilityPair{item, location}]; !ok {
								healthy = false
								break
							}
						}
						if healthy {
							denied.State = "out_of_stock"
							break
						}
					}
				}
				return denied, nil
			}
			sort.Strings(d.locations)
			demands = append(demands, d)
		}
	}
	// Backorders do not consume the finite-stock entitlement of normal lines.
	// This matches ApplyReservations' sum of non-backorder demand per level.
	sort.Slice(demands, func(i, j int) bool {
		if demands[i].backorder != demands[j].backorder {
			return !demands[i].backorder
		}
		if len(demands[i].locations) != len(demands[j].locations) {
			return len(demands[i].locations) < len(demands[j].locations)
		}
		if demands[i].id != demands[j].id {
			return demands[i].id < demands[j].id
		}
		return demands[i].component < demands[j].component
	})
	used := map[availabilityPair]*big.Int{}
	chosen := make([]string, len(demands))
	fits := func(d cartStockDemand, location string) (bool, error) {
		for item, need := range d.items {
			if err := search.step(); err != nil {
				return false, err
			}
			p := availabilityPair{item, location}
			total := new(big.Int).Set(need)
			if used[p] != nil {
				total.Add(total, used[p])
			}
			if total.Cmp(facts.safe[p]) > 0 {
				return false, nil
			}
		}
		return true, nil
	}
	change := func(d cartStockDemand, location string, add bool) {
		for item, need := range d.items {
			p := availabilityPair{item, location}
			if used[p] == nil {
				used[p] = new(big.Int)
			}
			if add {
				used[p].Add(used[p], need)
			} else {
				used[p].Sub(used[p], need)
			}
		}
	}
	var place func(int) (bool, error)
	place = func(index int) (bool, error) {
		if err := search.step(); err != nil {
			return false, err
		}
		if index == len(demands) || demands[index].backorder {
			return true, nil
		}
		d := demands[index]
		for _, location := range d.locations {
			ok, err := fits(d, location)
			if err != nil {
				return false, err
			}
			if !ok {
				continue
			}
			change(d, location, true)
			chosen[index] = location
			found, err := place(index + 1)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
			change(d, location, false)
			chosen[index] = ""
		}
		return false, nil
	}
	found, err := place(0)
	if err != nil {
		return CartStockPlan{}, err
	}
	if !found {
		denied.State = "out_of_stock"
		return denied, nil
	}
	result := CartStockPlan{State: "available", CanFulfill: true, Assignments: []CartStockAssignment{}}
	for i, d := range demands {
		if d.backorder {
			chosen[i] = d.locations[0]
			covered := false
			for _, location := range d.locations {
				ok, err := fits(d, location)
				if err != nil {
					return CartStockPlan{}, err
				}
				if ok {
					chosen[i] = location
					covered = true
					break
				}
			}
			if !covered {
				result.State = "backorder"
			}
			change(d, chosen[i], true)
		}
		for item, need := range d.items {
			result.Assignments = append(result.Assignments, CartStockAssignment{d.id, item, chosen[i], fixed(need), d.backorder})
		}
	}
	sort.Slice(result.Assignments, func(i, j int) bool {
		a, b := result.Assignments[i], result.Assignments[j]
		if a.LineItemID != b.LineItemID {
			return a.LineItemID < b.LineItemID
		}
		return a.InventoryItemID < b.InventoryItemID
	})
	return result, nil
}
