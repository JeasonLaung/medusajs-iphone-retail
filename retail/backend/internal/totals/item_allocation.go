package totals

import "sort"

// ItemAllocationInput is the arithmetic phase for one qualified promotion.
// Epsilon is trusted server configuration, not currency settlement precision.
// Previous always contains UNTAXED amounts, even for a tax-inclusive promotion.
// Campaign eligibility, target rules, budget checks and persistence are separate.
type ItemAllocationInput struct {
	Allocation, Type, Value, Epsilon string
	IsTaxInclusive                   bool
	MaxQuantity                      *int64
	Items                            []ItemAllocationLine
	Previous                         map[string]string
}

type ItemAllocationLine struct {
	ID, Subtotal, OriginalTotal string
	Quantity                    int64
	IsDiscountable              *bool
}
type ItemAllocationAmount struct {
	ID, Amount     string
	IsTaxInclusive bool
}
type ItemAllocationResult struct {
	Amounts   []ItemAllocationAmount
	Discounts map[string]string
}

// AllocateItems preserves the legacy arithmetic operation order, including
// rounded divisions before multiplication and inclusive/exclusive conversion.
// Invalid input returns no partial allocation and never mutates caller state.
func AllocateItems(input ItemAllocationInput) (ItemAllocationResult, error) {
	return AllocateItemsFiltered(input, nil)
}

func AllocateItemsFiltered(input ItemAllocationInput, filter AllocationFilter) (ItemAllocationResult, error) {
	empty := ItemAllocationResult{}
	if (input.Allocation != "each" && input.Allocation != "once" && input.Allocation != "across") ||
		(input.Type != "fixed" && input.Type != "percentage") || len(input.Items) > 1000 || len(input.Previous) > 1000 {
		return empty, ErrInput
	}
	value, err := parse(input.Value)
	if err != nil || value.r.Sign() < 0 {
		return empty, ErrInput
	}
	epsilon, err := parse(input.Epsilon)
	if err != nil || epsilon.r.Sign() < 0 {
		return empty, ErrInput
	}
	quota := int64(0)
	if input.MaxQuantity != nil {
		quota = *input.MaxQuantity
		if quota < 0 || quota > 2147483647 {
			return empty, ErrInput
		}
	}
	previous := map[string]decimal{}
	result := ItemAllocationResult{Amounts: []ItemAllocationAmount{}, Discounts: map[string]string{}}
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
	type line struct {
		id                                                  string
		quantity                                            int64
		subtotal, base, applied, available, unit, sortPrice decimal
	}
	lines := make([]line, 0, len(input.Items))
	seen := map[string]bool{}
	total := integer(0)
	for _, row := range input.Items {
		if row.ID == "" || len(row.ID) > 191 || seen[row.ID] || row.Quantity <= 0 || row.Quantity > 2147483647 {
			return empty, ErrInput
		}
		seen[row.ID] = true
		subtotal, err := parse(row.Subtotal)
		if err != nil || subtotal.r.Sign() < 0 {
			return empty, ErrInput
		}
		base := subtotal
		if input.IsTaxInclusive {
			base, err = parse(row.OriginalTotal)
			if err != nil || base.r.Sign() < 0 {
				return empty, ErrInput
			}
		}
		if subtotal.zero() || row.IsDiscountable != nil && !*row.IsDiscountable {
			continue
		}
		applied, ok := previous[row.ID]
		if !ok {
			applied = integer(0)
		}
		appliedInBase := applied
		if input.IsTaxInclusive {
			ratio, err := base.div(subtotal)
			if err != nil {
				return empty, err
			}
			appliedInBase = applied.mul(ratio)
		}
		unit, err := base.div(integer(row.Quantity))
		if err != nil {
			return empty, err
		}
		sortPrice, err := subtotal.div(integer(row.Quantity))
		if err != nil {
			return empty, err
		}
		available := base.sub(appliedInBase)
		lines = append(lines, line{row.ID, row.Quantity, subtotal, base, applied, available, unit, sortPrice})
		total = total.add(available)
	}
	if input.Allocation == "once" {
		sort.SliceStable(lines, func(i, j int) bool { return lines[i].sortPrice.r.Cmp(lines[j].sortPrice.r) < 0 })
	}
	if input.Allocation == "across" && total.r.Sign() <= 0 {
		return result, nil
	}
	for _, row := range lines {
		if input.Allocation == "once" && quota <= 0 {
			break
		}
		if row.base.r.Sign() <= 0 {
			continue
		}
		maximum := int64(1)
		if input.MaxQuantity != nil {
			maximum = *input.MaxQuantity
		}
		if input.Allocation == "once" {
			maximum = min(quota, row.quantity)
		}
		applicable := row.available
		if input.Allocation == "across" {
			quantity := row.quantity
			// The original across helper treats max_quantity=0 as absent.
			if input.MaxQuantity != nil && *input.MaxQuantity != 0 {
				quantity = min(quantity, *input.MaxQuantity)
			}
			appliedInBase := row.base.sub(row.available)
			applicable = row.unit.mul(integer(quantity)).sub(appliedInBase)
		} else {
			cap := row.unit.mul(integer(maximum))
			if cap.r.Cmp(applicable.r) < 0 {
				applicable = cap
			}
		}
		if applicable.r.Cmp(epsilon.r) <= 0 {
			continue
		}
		amount := value
		if input.Type == "percentage" {
			ratio, err := value.div(integer(100))
			if err != nil {
				return empty, err
			}
			amount = ratio.mul(applicable)
		} else if input.Allocation == "across" {
			ratio, err := applicable.div(total)
			if err != nil {
				return empty, err
			}
			amount = ratio.mul(value)
			if amount.r.Cmp(applicable.r) > 0 {
				// Preserve the old two-division correction before final min;
				// replacing it with min alone changes low decimal places.
				percent, err := applicable.mul(integer(100)).div(amount)
				if err != nil {
					return empty, err
				}
				fraction, err := percent.div(integer(100))
				if err != nil {
					return empty, err
				}
				amount = amount.mul(fraction)
			}
		} else {
			amount = value.mul(integer(min(row.quantity, maximum)))
		}
		if amount.r.Cmp(applicable.r) > 0 {
			amount = applicable
		}
		if amount.r.Cmp(epsilon.r) <= 0 {
			continue
		}
		accepted, err := acceptAllocation(filter, row.id, amount.text())
		if err != nil {
			return empty, err
		}
		if !accepted {
			continue
		}
		appliedExclusive := amount
		if input.IsTaxInclusive {
			ratio, err := row.subtotal.div(row.base)
			if err != nil {
				return empty, err
			}
			appliedExclusive = amount.mul(ratio)
		}
		result.Amounts = append(result.Amounts, ItemAllocationAmount{row.id, amount.text(), input.IsTaxInclusive})
		result.Discounts[row.id] = row.applied.add(appliedExclusive).text()
		if input.Allocation == "once" {
			quota -= min(maximum, row.quantity)
		}
	}
	return result, nil
}
