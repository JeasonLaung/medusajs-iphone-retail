// Package pricing selects exact configured prices. It does not calculate tax,
// discounts, shipping, or a payable checkout total.
package pricing

import (
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/eurorefurb/retail/internal/totals"
)

var ErrInput = errors.New("pricing configuration unavailable")

// Values other than region/quantity come from trusted application context, never
// arbitrary store query parameters. Anonymous callers have no customer groups.
type Context struct {
	Currency, Region, Channel, Quantity string
	CustomerGroups                      []string
	// QuantityOmitted models the original shipping cart context without a
	// quantity property. It requires Quantity to be empty; product callers
	// continue to require an explicit positive integral quantity.
	QuantityOmitted bool
}
type Rule struct{ Attribute, Operator, Value string }
type ListRule struct {
	Attribute string
	Value     *string
}
type List struct {
	ID, Type, Status string
	StartsAt, EndsAt *time.Time
	RulesCount       *int
	Rules            []ListRule
}
type Price struct {
	ID, SetID, Currency string
	Amount              totals.Number
	Min, Max            *totals.Number
	ListID              *string
	List                *List
	RulesCount          *int
	Rules               []Rule
}
type Preference struct {
	Attribute      string
	Value          *string
	IsTaxInclusive bool
}
type PriceKind struct {
	Type *string `json:"price_list_type"`
}
type Quote struct {
	Amount                 string    `json:"calculated_amount"`
	OriginalAmount         *string   `json:"original_amount"`
	Currency               string    `json:"currency_code"`
	CalculatedTaxInclusive bool      `json:"is_calculated_price_tax_inclusive"`
	OriginalTaxInclusive   bool      `json:"is_original_price_tax_inclusive"`
	CalculatedIsList       bool      `json:"is_calculated_price_price_list"`
	OriginalIsList         bool      `json:"is_original_price_price_list"`
	CalculatedKind         PriceKind `json:"calculated_price"`
	// Retained for future server-side cart receipts; not public DTO fields.
	PriceID, OriginalPriceID, SetID string `json:"-"`
}
type eligible struct {
	price       Price
	amount      string
	value       *big.Rat
	specificity int
}

func number(n totals.Number) (string, *big.Rat, error) {
	s, err := totals.Resolve(n)
	if err != nil {
		return "", nil, ErrInput
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return "", nil, ErrInput
	}
	return s, r, nil
}
func contextValues(c Context, attribute string) []string {
	switch attribute {
	case "region_id":
		return []string{c.Region}
	case "sales_channel_id":
		return []string{c.Channel}
	case "customer.groups.id":
		return c.CustomerGroups
	default:
		return nil
	}
}
func priceMatches(p Price, c Context) bool {
	if p.RulesCount == nil || *p.RulesCount != len(p.Rules) {
		return false
	}
	for _, r := range p.Rules {
		// Current trusted context contains string identities. Numeric operators
		// must never become a string comparison or an unconditional match.
		if r.Operator != "eq" {
			return false
		}
		match := false
		for _, v := range contextValues(c, r.Attribute) {
			if v != "" && r.Value == v {
				match = true
			}
		}
		if !match {
			return false
		}
	}
	return true
}
func listMatches(l *List, c Context, now time.Time) bool {
	if l == nil || l.Status != "active" || (l.Type != "sale" && l.Type != "override") || l.RulesCount == nil || *l.RulesCount != len(l.Rules) || l.StartsAt != nil && now.Before(*l.StartsAt) || l.EndsAt != nil && now.After(*l.EndsAt) {
		return false
	}
	for _, r := range l.Rules {
		if r.Value == nil || len(*r.Value) > 65536 {
			return false
		}
		var scalar string
		values := []string{}
		if json.Unmarshal([]byte(*r.Value), &scalar) == nil {
			values = append(values, scalar)
		} else if json.Unmarshal([]byte(*r.Value), &values) != nil || len(values) > 100 {
			return false
		}
		match := false
		for _, expected := range values {
			for _, v := range contextValues(c, r.Attribute) {
				if v != "" && expected == v {
					match = true
				}
			}
		}
		if !match {
			return false
		}
	}
	return true
}
func taxInclusive(p Price, c Context, prefs []Preference) (bool, error) {
	values := map[string]bool{}
	for _, pref := range prefs {
		if pref.Value == nil {
			continue
		}
		if pref.Attribute == "currency_code" && *pref.Value == c.Currency || pref.Attribute == "region_id" && *pref.Value == c.Region {
			if _, ok := values[pref.Attribute]; ok {
				return false, ErrInput
			}
			values[pref.Attribute] = pref.IsTaxInclusive
		}
	}
	for _, r := range p.Rules {
		if r.Attribute == "region_id" && r.Value == c.Region {
			if v, ok := values["region_id"]; ok {
				return v, nil
			}
		}
	}
	return values["currency_code"], nil
}

// Select preserves the installed pricing repository's specificity/amount order
// and service's override/sale/reference semantics. Exact ties use source ID.
// A true zero base is compared numerically rather than JS truthiness.
func Select(prices []Price, prefs []Preference, c Context, now time.Time) (*Quote, error) {
	var qty *big.Rat
	var err error
	if c.QuantityOmitted {
		if c.Quantity != "" {
			return nil, ErrInput
		}
	} else {
		_, qty, err = number(totals.Number{Decimal: c.Quantity})
		if err != nil || qty.Sign() <= 0 || !qty.IsInt() {
			return nil, ErrInput
		}
	}
	if c.Currency == "" || c.Region == "" {
		return nil, ErrInput
	}
	rows := []eligible{}
	for _, p := range prices {
		if p.Currency != c.Currency || !priceMatches(p, c) {
			continue
		}
		if p.ListID != nil && (!listMatches(p.List, c, now) || p.List.ID != *p.ListID) {
			continue
		}
		var min, max *big.Rat
		if p.Min != nil {
			_, min, err = number(*p.Min)
			if err != nil {
				return nil, err
			}
		}
		if p.Max != nil {
			_, max, err = number(*p.Max)
			if err != nil {
				return nil, err
			}
		}
		if min != nil && max != nil && min.Cmp(max) > 0 {
			return nil, ErrInput
		}
		if c.QuantityOmitted {
			// The installed repository's undefined branch tests only the
			// minimum against one. It intentionally does not test maximum.
			if min != nil && min.Cmp(big.NewRat(1, 1)) > 0 {
				continue
			}
		} else {
			if min != nil && qty.Cmp(min) < 0 || max != nil && qty.Cmp(max) > 0 {
				continue
			}
		}
		amount, value, e := number(p.Amount)
		if e != nil {
			return nil, e
		}
		specificity := len(p.Rules)
		if p.ListID != nil {
			specificity += len(p.List.Rules)
		}
		rows = append(rows, eligible{p, amount, value, specificity})
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.price.ListID != nil) != (b.price.ListID != nil) {
			return a.price.ListID != nil
		}
		if a.specificity != b.specificity {
			return a.specificity > b.specificity
		}
		if n := a.value.Cmp(b.value); n != 0 {
			return n < 0
		}
		return a.price.ID < b.price.ID
	})
	var base, list, original, calculated *eligible
	for i := range rows {
		r := &rows[i]
		if r.price.ListID == nil && base == nil {
			base = r
		}
		if r.price.ListID != nil && list == nil {
			list = r
		}
		if original == nil && r.price.ListID != nil && r.price.List.Type == "override" {
			original = r
		}
	}
	if original == nil {
		original = base
	}
	calculated = base
	if list != nil {
		if list.price.List.Type == "override" {
			calculated = list
			original = list
		} else if base == nil || list.value.Cmp(base.value) <= 0 {
			calculated = list
		}
	}
	if calculated == nil {
		return nil, nil
	}
	q := &Quote{Amount: calculated.amount, Currency: c.Currency, CalculatedIsList: calculated.price.ListID != nil, PriceID: calculated.price.ID, SetID: calculated.price.SetID}
	q.CalculatedTaxInclusive, err = taxInclusive(calculated.price, c, prefs)
	if err != nil {
		return nil, err
	}
	if calculated.price.ListID != nil {
		kind := calculated.price.List.Type
		q.CalculatedKind.Type = &kind
	}
	if original != nil {
		amount := original.amount
		q.OriginalAmount = &amount
		q.OriginalPriceID = original.price.ID
		q.OriginalIsList = original.price.ListID != nil
		q.OriginalTaxInclusive, err = taxInclusive(original.price, c, prefs)
		if err != nil {
			return nil, err
		}
	}
	return q, nil
}
