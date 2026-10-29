package cart

import (
	"math/big"
	"strings"

	"github.com/eurorefurb/retail/internal/totals"
)

type PaymentRefreshCollection struct {
	ID, Amount, Currency string
	SessionIDs           []string
}

type PaymentRefreshFacts struct {
	InputCartProvided, InputCartIDPresent bool
	Amount, Currency                      string
	Collection                            *PaymentRefreshCollection
}

// PaymentRefreshPlan is private preparation, not evidence that a provider was
// called or a session invalidated. A changed collection requires successful
// provider/session coordination before the enclosing cart command completes.
type PaymentRefreshPlan struct {
	ShouldExecute, Changed, UpdateCollection bool
	CollectionID, Amount, Currency           string
	DeleteSessionIDs                         []string
}

// PlanPaymentRefresh follows the original refresh-payment-collection workflow's
// shouldExecute, exact raw amount comparison and session/update preparation.
// Facts must come from trusted storage; this pure function grants no authority.
func PlanPaymentRefresh(in PaymentRefreshFacts) (PaymentRefreshPlan, error) {
	failed := PaymentRefreshPlan{}
	amount, err := totals.Resolve(totals.Number{Decimal: in.Amount})
	if err != nil || strings.HasPrefix(amount, "-") || !currencyPattern.MatchString(in.Currency) {
		return failed, ErrInput
	}
	out := PaymentRefreshPlan{Amount: amount, Currency: in.Currency, DeleteSessionIDs: []string{}}
	if in.InputCartProvided {
		out.ShouldExecute = in.Collection != nil
	} else {
		out.ShouldExecute = in.InputCartIDPresent
	}
	equal := false
	if c := in.Collection; c != nil {
		if !idPattern.MatchString(c.ID) || !currencyPattern.MatchString(c.Currency) || len(c.SessionIDs) > 100 {
			return failed, ErrInput
		}
		old, err := totals.Resolve(totals.Number{Decimal: c.Amount})
		if err != nil || strings.HasPrefix(old, "-") {
			return failed, ErrInput
		}
		a, ok := new(big.Rat).SetString(old)
		if !ok {
			return failed, ErrInput
		}
		b, ok := new(big.Rat).SetString(amount)
		if !ok {
			return failed, ErrInput
		}
		equal = a.Cmp(b) == 0
		seen := map[string]bool{}
		for _, id := range c.SessionIDs {
			if !idPattern.MatchString(id) || seen[id] {
				return failed, ErrInput
			}
			seen[id] = true
		}
		out.CollectionID = c.ID
	}
	if equal {
		out.Changed = in.Collection.Currency != in.Currency
	} else {
		out.Changed = out.ShouldExecute
	}
	if out.Changed && in.Collection != nil {
		out.UpdateCollection = true
		out.DeleteSessionIDs = append(out.DeleteSessionIDs, in.Collection.SessionIDs...)
	}
	return out, nil
}
