package totals

import "sort"

// ShippingAllocationInput contains already qualified methods for ONE promotion.
// This is arithmetic only: it does not qualify a campaign, check a budget, or
// authorize persisting a discount. Subtotals and previous discounts are untaxed.
type ShippingAllocationInput struct {
	Allocation  string
	Type        string
	Value       string
	MaxQuantity *int
	Methods     []ShippingAllocationMethod
	Previous    map[string]string
}

type ShippingAllocationMethod struct{ ID, Subtotal string }
type ShippingAllocationAmount struct{ ID, Amount string }
type ShippingAllocationResult struct {
	Amounts []ShippingAllocationAmount
	// Fresh shared-discount state; the input map is never mutated.
	Discounts map[string]string
}

// AllocateShipping uses the reviewed legacy decimal division (20 places,
// half away from zero), without provider currency rounding. once consumes one
// quota per positively discounted method, ordered by subtotal, stably for ties.
func AllocateShipping(input ShippingAllocationInput) (ShippingAllocationResult, error) {
	empty := ShippingAllocationResult{}
	if (input.Allocation != "each" && input.Allocation != "once" && input.Allocation != "across") ||
		(input.Type != "fixed" && input.Type != "percentage") || len(input.Methods) > 1000 || len(input.Previous) > 1000 {
		return empty, ErrInput
	}
	quota := 0
	if input.MaxQuantity != nil {
		quota = *input.MaxQuantity
		if quota < 0 || quota > 1000000 {
			return empty, ErrInput
		}
	}
	value, err := parse(input.Value)
	if err != nil || value.r.Sign() < 0 {
		return empty, ErrInput
	}
	previous := map[string]decimal{}
	result := ShippingAllocationResult{Amounts: []ShippingAllocationAmount{}, Discounts: map[string]string{}}
	for id, raw := range input.Previous {
		if id == "" || len(id) > 191 {
			return empty, ErrInput
		}
		d, err := parse(raw)
		if err != nil || d.r.Sign() < 0 {
			return empty, ErrInput
		}
		previous[id], result.Discounts[id] = d, d.text()
	}
	type method struct {
		id       string
		subtotal decimal
	}
	methods := make([]method, 0, len(input.Methods))
	seen := map[string]bool{}
	total := integer(0)
	for _, row := range input.Methods {
		if row.ID == "" || len(row.ID) > 191 || seen[row.ID] {
			return empty, ErrInput
		}
		seen[row.ID] = true
		raw := row.Subtotal
		if raw == "" {
			raw = "0"
		}
		d, err := parse(raw)
		if err != nil || d.r.Sign() < 0 {
			return empty, ErrInput
		}
		methods = append(methods, method{row.ID, d})
		applied, ok := previous[row.ID]
		if !ok {
			applied = integer(0)
		}
		// Do not clamp individual negative remainders before summing: the
		// legacy across denominator includes them, including zero-price rows.
		total = total.add(d.sub(applied))
	}
	if input.Allocation == "once" {
		sort.SliceStable(methods, func(i, j int) bool { return methods[i].subtotal.r.Cmp(methods[j].subtotal.r) < 0 })
	}
	if input.Allocation == "across" && total.r.Sign() <= 0 {
		return result, nil
	}
	for _, row := range methods {
		if input.Allocation == "once" && quota <= 0 {
			break
		}
		if row.subtotal.zero() {
			continue
		}
		applied, ok := previous[row.id]
		if !ok {
			applied = integer(0)
		}
		available := row.subtotal.sub(applied)
		amount := value
		if input.Type == "percentage" {
			ratio, err := value.div(integer(100))
			if err != nil {
				return empty, err
			}
			amount = ratio.mul(available)
		} else if input.Allocation == "across" {
			ratio, err := available.div(total)
			if err != nil {
				return empty, err
			}
			amount = ratio.mul(value)
		}
		if amount.r.Cmp(available.r) > 0 {
			amount = available
		}
		if amount.r.Sign() <= 0 {
			continue
		}
		result.Amounts = append(result.Amounts, ShippingAllocationAmount{row.id, amount.text()})
		previous[row.id] = applied.add(amount)
		result.Discounts[row.id] = previous[row.id].text()
		if input.Allocation == "once" {
			quota--
		}
	}
	return result, nil
}
