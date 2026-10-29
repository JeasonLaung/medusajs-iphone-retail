package totals

import "sort"

// BuyGetInput contains server-qualified buy and target membership. Rules,
// campaigns and budgets must be coordinated before persisting these results.
// Buy/target consumption maps are separate, as in the original engine.
type BuyGetInput struct {
	Code, Type, Value           string
	MinimumBuy, ApplyQuantity   int64
	MaxQuantity                 *int64
	Items                       []BuyGetLine
	Previous                    map[string]string
	BuyConsumed, TargetConsumed map[string][]BuyGetQuantity
}
type BuyGetLine struct {
	ID, Subtotal                string
	Quantity                    int64
	BuyEligible, TargetEligible bool
}
type BuyGetQuantity struct {
	ID       string
	Quantity int64
}
type BuyGetAmount struct{ ID, Amount string }
type BuyGetResult struct {
	Amounts                     []BuyGetAmount
	Discounts                   map[string]string
	BuyConsumed, TargetConsumed map[string][]BuyGetQuantity
	// The legacy engine stops after 1000 applications. Expose this explicitly
	// so orchestration can reject rather than silently persist partial work.
	IterationLimit bool
}

// AllocateBuyGet preserves reservation order and quantity sharing. Amounts use
// subtotal/quantity, irrespective of a tax-inclusive flag, matching this legacy
// branch. Existing discounts accumulate but do not reduce the target price.
func AllocateBuyGet(in BuyGetInput) (BuyGetResult, error) {
	return AllocateBuyGetFiltered(in, nil)
}

func AllocateBuyGetFiltered(in BuyGetInput, filter AllocationFilter) (BuyGetResult, error) {
	empty := BuyGetResult{}
	if in.Code == "" || len(in.Code) > 191 || (in.Type != "fixed" && in.Type != "percentage") || in.MinimumBuy <= 0 || in.MinimumBuy > 2147483647 || in.ApplyQuantity < 0 || in.ApplyQuantity > 2147483647 || len(in.Items) > 1000 || len(in.Previous) > 1000 {
		return empty, ErrInput
	}
	maximum := int64(1)
	if in.MaxQuantity != nil {
		maximum = *in.MaxQuantity
	}
	if maximum < 0 || maximum > 2147483647 {
		return empty, ErrInput
	}
	value, err := parse(in.Value)
	if err != nil || value.r.Sign() < 0 {
		return empty, ErrInput
	}
	result := BuyGetResult{Amounts: []BuyGetAmount{}, Discounts: map[string]string{}}
	previous := map[string]decimal{}
	for id, raw := range in.Previous {
		if id == "" || len(id) > 191 {
			return empty, ErrInput
		}
		d, err := parse(raw)
		if err != nil || d.r.Sign() < 0 {
			return empty, ErrInput
		}
		previous[id], result.Discounts[id] = d, d.text()
	}
	clone := func(source map[string][]BuyGetQuantity) (map[string][]BuyGetQuantity, error) {
		if len(source) > 1000 {
			return nil, ErrInput
		}
		out := map[string][]BuyGetQuantity{}
		count := 0
		for code, rows := range source {
			if code == "" || len(code) > 191 {
				return nil, ErrInput
			}
			count += len(rows)
			if count > 10000 {
				return nil, ErrInput
			}
			out[code] = make([]BuyGetQuantity, len(rows))
			for i, row := range rows {
				if row.ID == "" || len(row.ID) > 191 || row.Quantity < 0 || row.Quantity > 2147483647 {
					return nil, ErrInput
				}
				out[code][i] = row
			}
		}
		return out, nil
	}
	result.BuyConsumed, err = clone(in.BuyConsumed)
	if err != nil {
		return empty, err
	}
	result.TargetConsumed, err = clone(in.TargetConsumed)
	if err != nil {
		return empty, err
	}
	type item struct {
		row      BuyGetLine
		subtotal decimal
	}
	items := make([]item, 0, len(in.Items))
	byID := map[string]item{}
	for _, row := range in.Items {
		if row.ID == "" || len(row.ID) > 191 || row.Quantity <= 0 || row.Quantity > 2147483647 {
			return empty, ErrInput
		}
		if _, ok := byID[row.ID]; ok {
			return empty, ErrInput
		}
		d, err := parse(row.Subtotal)
		if err != nil || d.r.Sign() < 0 {
			return empty, ErrInput
		}
		v := item{row, d}
		items = append(items, v)
		byID[row.ID] = v
	}
	// Original invalid/empty context returns before replacing current-code maps.
	if len(items) == 0 {
		return result, nil
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].subtotal.r.Cmp(items[j].subtotal.r) > 0 })
	buy, target := []item{}, []item{}
	for _, v := range items {
		if v.row.BuyEligible {
			buy = append(buy, v)
		}
		if v.row.TargetEligible {
			target = append(target, v)
		}
	}
	// Preserve price order inside both groups, preferring buy-only items.
	reservation := []item{}
	for _, v := range buy {
		if !v.row.TargetEligible {
			reservation = append(reservation, v)
		}
	}
	for _, v := range buy {
		if v.row.TargetEligible {
			reservation = append(reservation, v)
		}
	}
	remaining := func(rows []item, consumed map[string][]BuyGetQuantity) map[string]int64 {
		out := map[string]int64{}
		for _, v := range rows {
			qty := v.row.Quantity
			for code, used := range consumed {
				if code != in.Code {
					for _, u := range used {
						if u.ID == v.row.ID {
							qty -= u.Quantity
						}
					}
				}
			}
			out[v.row.ID] = max(qty, 0)
		}
		return out
	}
	buyRemaining, targetRemaining := remaining(buy, result.BuyConsumed), remaining(target, result.TargetConsumed)
	totalBuy, totalTarget := []BuyGetQuantity{}, []BuyGetQuantity{}
	accumulate := func(rows *[]BuyGetQuantity, addition []BuyGetQuantity) {
		for _, a := range addition {
			found := false
			for i := range *rows {
				if (*rows)[i].ID == a.ID {
					(*rows)[i].Quantity += a.Quantity
					found = true
					break
				}
			}
			if !found {
				*rows = append(*rows, a)
			}
		}
	}
	amounts := map[string]decimal{}
	amountOrder := []string{}
	appliedQuantity := int64(0)
	for iteration := 0; ; iteration++ {
		if iteration == 1000 {
			result.IterationLimit = true
			break
		}
		buyTotal := int64(0)
		for _, n := range buyRemaining {
			buyTotal += n
		}
		if buyTotal < in.MinimumBuy {
			break
		}
		buyRows, targetRows := []BuyGetQuantity{}, []BuyGetQuantity{}
		need := in.MinimumBuy
		reserved := map[string]int64{}
		for _, v := range reservation {
			qty := min(max(buyRemaining[v.row.ID], 0), need)
			if qty > 0 {
				buyRows = append(buyRows, BuyGetQuantity{v.row.ID, qty})
				reserved[v.row.ID] += qty
				need -= qty
			}
			if need == 0 {
				break
			}
		}
		if need > 0 {
			break
		}
		need = in.ApplyQuantity
		for _, v := range target {
			available := targetRemaining[v.row.ID] - reserved[v.row.ID]
			// Original allowance uses previously applied quantity; it is not
			// decremented while collecting multiple target lines in this batch.
			qty := min(need, available, maximum-appliedQuantity)
			if qty > 0 {
				targetRows = append(targetRows, BuyGetQuantity{v.row.ID, qty})
				need -= qty
			}
			if need <= 0 {
				break
			}
		}
		if need > 0 {
			break
		}
		remainingToApply := in.ApplyQuantity
		for _, row := range targetRows {
			if remainingToApply <= 0 {
				break
			}
			v := byID[row.ID]
			qty := min(row.Quantity, remainingToApply)
			unit, err := v.subtotal.div(integer(v.row.Quantity))
			if err != nil {
				return empty, err
			}
			base := unit.mul(integer(qty))
			amount := value.mul(integer(qty))
			if in.Type == "fixed" {
				if amount.r.Cmp(base.r) > 0 {
					amount = base
				}
			} else {
				amount, err = base.mul(value).div(integer(100))
				if err != nil {
					return empty, err
				}
			}
			if amount.r.Sign() <= 0 {
				continue
			}
			remainingToApply -= qty
			accepted, err := acceptAllocation(filter, row.ID, amount.text())
			if err != nil {
				return empty, err
			}
			if !accepted {
				continue
			}
			appliedQuantity += qty
			old, ok := previous[row.ID]
			if !ok {
				old = integer(0)
			}
			previous[row.ID] = old.add(amount)
			result.Discounts[row.ID] = previous[row.ID].text()
			current, ok := amounts[row.ID]
			if !ok {
				current = integer(0)
				amountOrder = append(amountOrder, row.ID)
			}
			amounts[row.ID] = current.add(amount)
		}
		consumed := map[string]int64{}
		for _, row := range buyRows {
			consumed[row.ID] += row.Quantity
		}
		for _, row := range targetRows {
			consumed[row.ID] += row.Quantity
		}
		for id, n := range consumed {
			if _, ok := buyRemaining[id]; ok {
				buyRemaining[id] -= n
			}
			if _, ok := targetRemaining[id]; ok {
				targetRemaining[id] -= n
			}
		}
		accumulate(&totalBuy, buyRows)
		accumulate(&totalTarget, targetRows)
	}
	for _, id := range amountOrder {
		result.Amounts = append(result.Amounts, BuyGetAmount{id, amounts[id].text()})
	}
	result.BuyConsumed[in.Code] = totalBuy
	result.TargetConsumed[in.Code] = totalTarget
	return result, nil
}
