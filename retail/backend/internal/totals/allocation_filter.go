package totals

// AllocationFilter checks each positive candidate before shared discounts or
// quotas are updated. It runs at the original budget-check position. A rejected
// candidate is skipped, not capped. Callers must keep callback state private and
// discard it on error; this hook does not reserve or persist a campaign budget.
type AllocationFilter func(id, amount string) (bool, error)

func acceptAllocation(filter AllocationFilter, id, amount string) (bool, error) {
	if filter == nil {
		return true, nil
	}
	return filter(id, amount)
}
