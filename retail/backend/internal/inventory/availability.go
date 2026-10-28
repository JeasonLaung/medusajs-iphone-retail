package inventory

import (
	"math/big"
	"strconv"

	"gorm.io/gorm"
)

// Availability is a channel-scoped observation, not a reservation or checkout
// authorization. Internal item/location/hold details never enter this DTO.
type Availability struct {
	State                  string  `json:"state"`
	CanFulfill             bool    `json:"can_fulfill"`
	MaxFulfillableQuantity *string `json:"max_fulfillable_quantity"`
	Quantity               *string `json:"-"`
}
type AvailableVariant struct {
	ID                                        string
	ManageInventory, AllowBackorder, Complete bool
}
type availabilityRequirement struct {
	VariantID, InventoryItemID, RequiredQuantity string
	ItemAvailable                                bool
}
type availabilityPair struct{ item, location string }

func purchaseQuantity(s string) (*big.Int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || strconv.Itoa(n) != s || n < 1 || n > 1000000000 {
		return nil, ErrQuantity
	}
	return big.NewInt(int64(n)), nil
}
func observedAvailability(v AvailableVariant, requirements []availabilityRequirement, locations []string, safe map[availabilityPair]*big.Int, requested *big.Int) Availability {
	out := Availability{State: "unavailable"}
	if !v.Complete {
		out.State = "incomplete"
		return out
	}
	if !v.ManageInventory {
		out.State = "untracked"
		out.CanFulfill = true
		return out
	}
	if len(requirements) == 0 || len(locations) == 0 {
		return out
	}
	var pooled, maximum *big.Int
	allComponents := true
	for _, r := range requirements {
		required, err := scaled(r.RequiredQuantity)
		if err != nil || required.Sign() <= 0 || !r.ItemAvailable {
			return out
		}
		total, best := new(big.Int), new(big.Int)
		hasLevel := false
		for _, location := range locations {
			amount, ok := safe[availabilityPair{r.InventoryItemID, location}]
			if !ok {
				continue
			}
			hasLevel = true
			total.Add(total, amount)
			units := new(big.Int).Quo(amount, required)
			if units.Cmp(best) > 0 {
				best.Set(units)
			}
		}
		allComponents = allComponents && hasLevel
		units := new(big.Int).Quo(total, required)
		if pooled == nil || units.Cmp(pooled) < 0 {
			pooled = units
		}
		if maximum == nil || best.Cmp(maximum) < 0 {
			maximum = best
		}
	}
	quantity, limit := pooled.String(), maximum.String()
	out.Quantity = &quantity
	out.MaxFulfillableQuantity = &limit
	if !allComponents {
		return out
	}
	max := maximum
	out.CanFulfill = max.Cmp(requested) >= 0
	if out.CanFulfill {
		out.State = "available"
	} else if v.AllowBackorder {
		out.State = "backorder"
		out.CanFulfill = true
	} else {
		out.State = "out_of_stock"
	}
	return out
}

// ReadAvailability uses the caller's read-only snapshot. Persistent holds and
// current reservation mismatches remove the affected item/location, including
// for backorders. Healthy locations remain usable without erasing history.
func ReadAvailability(tx *gorm.DB, variants []AvailableVariant, channel, quantity string) (map[string]Availability, error) {
	requested, err := purchaseQuantity(quantity)
	if err != nil {
		return nil, err
	}
	facts, err := readAvailabilityFacts(tx, variants, channel)
	if err != nil {
		return nil, err
	}
	out := map[string]Availability{}
	for _, v := range variants {
		out[v.ID] = observedAvailability(v, facts.requirements[v.ID], facts.locations, facts.safe, requested)
	}
	return out, nil
}

type availabilityFacts struct {
	locations    []string
	requirements map[string][]availabilityRequirement
	safe         map[availabilityPair]*big.Int
}

func readAvailabilityFacts(tx *gorm.DB, variants []AvailableVariant, channel string) (*availabilityFacts, error) {
	var err error
	if tx == nil || tx.Statement == nil || !validID(channel) || len(variants) > 10000 || Ready(tx.Statement.Context, tx) != nil {
		return nil, ErrSchema
	}
	facts := &availabilityFacts{requirements: map[string][]availabilityRequirement{}, safe: map[availabilityPair]*big.Int{}}
	tracked := []string{}
	seenVariants := map[string]bool{}
	for _, v := range variants {
		if !validID(v.ID) {
			return nil, ErrSchema
		}
		if seenVariants[v.ID] {
			return nil, ErrSchema
		}
		seenVariants[v.ID] = true
		if v.Complete && v.ManageInventory {
			tracked = append(tracked, v.ID)
		}
	}
	if len(tracked) == 0 {
		return facts, nil
	}
	locations := []string{}
	if err = tx.Table("inventory_locations l").Select("l.id").Joins("JOIN commerce_channel_locations cl ON cl.stock_location_id=l.id AND cl.deleted_at IS NULL").Where("cl.sales_channel_id=? AND l.deleted_at IS NULL", channel).Order("l.id").Limit(1001).Scan(&locations).Error; err != nil {
		return nil, err
	}
	if len(locations) > 1000 {
		return nil, errLimit
	}
	facts.locations = locations
	if len(locations) == 0 {
		return facts, nil
	}
	budget := 20000 - len(locations)
	var requirements []availabilityRequirement
	if err = tx.Table("catalog_variant_inventory_items vi").Select("vi.variant_id,vi.inventory_item_id,vi.required_quantity,(i.id IS NOT NULL) AS item_available").Joins("LEFT JOIN inventory_items i ON i.id=vi.inventory_item_id AND i.deleted_at IS NULL").Where("vi.variant_id IN ? AND vi.deleted_at IS NULL", tracked).Order("vi.variant_id,vi.inventory_item_id").Limit(budget + 1).Scan(&requirements).Error; err != nil {
		return nil, err
	}
	budget -= len(requirements)
	if budget < 0 {
		return nil, errLimit
	}
	byVariant := map[string][]availabilityRequirement{}
	items := []string{}
	seen := map[string]bool{}
	for _, r := range requirements {
		byVariant[r.VariantID] = append(byVariant[r.VariantID], r)
		if r.ItemAvailable && !seen[r.InventoryItemID] {
			items = append(items, r.InventoryItemID)
			seen[r.InventoryItemID] = true
		}
	}
	facts.requirements = byVariant
	if len(items) == 0 {
		return facts, nil
	}
	var levels []struct{ InventoryItemID, LocationID, StockedQuantity, ReservedQuantity string }
	if err = tx.Table("inventory_levels").Select("inventory_item_id,location_id,stocked_quantity,reserved_quantity").Where("inventory_item_id IN ? AND location_id IN ? AND deleted_at IS NULL", items, locations).Order("inventory_item_id,location_id,id").Limit(budget + 1).Scan(&levels).Error; err != nil {
		return nil, err
	}
	budget -= len(levels)
	if budget < 0 {
		return nil, errLimit
	}
	var reservations []struct{ InventoryItemID, LocationID, Quantity string }
	if err = tx.Table("inventory_reservations").Select("inventory_item_id,location_id,quantity").Where("inventory_item_id IN ? AND location_id IN ? AND deleted_at IS NULL", items, locations).Order("inventory_item_id,location_id,id").Limit(budget + 1).Scan(&reservations).Error; err != nil {
		return nil, err
	}
	budget -= len(reservations)
	if budget < 0 {
		return nil, errLimit
	}
	var holds []struct{ InventoryItemID, LocationID string }
	if err = tx.Table("inventory_migration_holds").Distinct("inventory_item_id,location_id").Where("inventory_item_id IN ? AND location_id IN ?", items, locations).Order("inventory_item_id,location_id").Limit(budget + 1).Scan(&holds).Error; err != nil {
		return nil, err
	}
	budget -= len(holds)
	if budget < 0 {
		return nil, errLimit
	}
	blocked := map[availabilityPair]bool{}
	counts := map[availabilityPair]int{}
	sums := map[availabilityPair]*big.Int{}
	for _, h := range holds {
		blocked[availabilityPair{h.InventoryItemID, h.LocationID}] = true
	}
	for _, r := range reservations {
		key := availabilityPair{r.InventoryItemID, r.LocationID}
		counts[key]++
		if counts[key] > 10000 {
			blocked[key] = true
		}
		n, e := scaled(r.Quantity)
		if e != nil || n.Sign() < 0 {
			blocked[key] = true
			continue
		}
		if sums[key] == nil {
			sums[key] = new(big.Int)
		}
		sums[key].Add(sums[key], n)
	}
	safe := map[availabilityPair]*big.Int{}
	for _, l := range levels {
		key := availabilityPair{l.InventoryItemID, l.LocationID}
		if blocked[key] {
			continue
		}
		stocked, e := scaled(l.StockedQuantity)
		reserved, f := scaled(l.ReservedQuantity)
		if e != nil || f != nil || stocked.Sign() < 0 || reserved.Sign() < 0 {
			continue
		}
		sum := sums[key]
		if sum == nil {
			sum = new(big.Int)
		}
		if sum.Cmp(reserved) != 0 {
			continue
		}
		available := new(big.Int).Sub(stocked, reserved)
		if available.Sign() < 0 {
			available.SetInt64(0)
		}
		safe[key] = available
	}
	facts.safe = safe
	return facts, nil
}
