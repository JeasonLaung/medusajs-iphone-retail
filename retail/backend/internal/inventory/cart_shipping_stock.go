package inventory

import "gorm.io/gorm"

// ReadCartStockAtLocations observes the complete prospective cart with explicit
// server-resolved shipping warehouses for EVERY line. Empty lists mean no
// eligible warehouse, never an unrestricted fallback. Shared inventory remains
// shared across profiles/lines; each component fits one allowed warehouse.
// The coordinator must derive these IDs from current channel/provider/option
// relationships, not browser input. This does not reserve or authorize payment.
func ReadCartStockAtLocations(tx *gorm.DB, variants []AvailableVariant, lines []CartStockLine, channel string, locationsByLine map[string][]string) (CartStockPlan, error) {
	selected, err := stockLines(variants, lines)
	if err != nil {
		return CartStockPlan{}, err
	}
	allowed, err := shippingLocations(lines, locationsByLine)
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
	// Unmanaged-only carts do not load inventory facts, but still need a real
	// channel warehouse for shipping. Tracked carts already loaded this list.
	if len(facts.locations) == 0 {
		if err := tx.Table("inventory_locations l").Select("l.id").Joins("JOIN commerce_channel_locations cl ON cl.stock_location_id=l.id AND cl.deleted_at IS NULL").Where("cl.sales_channel_id=? AND l.deleted_at IS NULL", channel).Order("l.id").Limit(1001).Scan(&facts.locations).Error; err != nil {
			return CartStockPlan{}, err
		}
		if len(facts.locations) > 1000 {
			return CartStockPlan{}, errLimit
		}
	}
	return planCartStockAtLocations(tx.Statement.Context, selected, lines, facts, allowed, 200000)
}

func shippingLocations(lines []CartStockLine, byLine map[string][]string) (map[string]map[string]bool, error) {
	if byLine == nil || len(byLine) != len(lines) {
		return nil, ErrCartStockInput
	}
	out := map[string]map[string]bool{}
	budget := 10000
	for _, line := range lines {
		locations, ok := byLine[line.ID]
		if !ok || len(locations) > 1000 {
			return nil, ErrCartStockInput
		}
		budget -= len(locations)
		if budget < 0 {
			return nil, ErrCartStockInput
		}
		seen := map[string]bool{}
		for _, location := range locations {
			if !validID(location) || seen[location] {
				return nil, ErrCartStockInput
			}
			seen[location] = true
		}
		out[line.ID] = seen
	}
	return out, nil
}
