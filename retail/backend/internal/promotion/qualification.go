package promotion

import (
	"encoding/json"
	"sort"
)

// TargetQualification contains rule membership only. It does not authorize a
// discount, check budgets/usage, reserve quantities or evaluate available stock.
// The context must come from server-owned cart facts, never a public request.
type TargetQualification struct {
	Applicable bool     `json:"applicable"`
	Items      []string `json:"items"`
	Shipping   []string `json:"shipping"`
	Buy        []string `json:"buy"`
	Target     []string `json:"target"`
}

func emptyQualification() TargetQualification {
	return TargetQualification{Items: []string{}, Shipping: []string{}, Buy: []string{}, Target: []string{}}
}

// QualifyTargets preserves the original distinct standard, shipping and buy-get
// filters. In particular buy-get membership does not inherit the standard
// is_discountable/subtotal filters. The allocator validates amounts/quantities.
func QualifyTargets(p ActiveConfiguration, context json.RawMessage) (TargetQualification, error) {
	failed := TargetQualification{}
	out := emptyQualification()
	if len(context) > 1<<20 {
		return failed, ErrRules
	}
	v, err := decode(context)
	if err != nil {
		return failed, err
	}
	object, ok := v.(map[string]any)
	if !ok {
		return failed, ErrRules
	}
	if p.Method == nil {
		return out, nil
	}
	m := p.Method
	if p.Type != "standard" && p.Type != "buyget" || m.Target != "items" && m.Target != "order" && m.Target != "shipping_methods" {
		return failed, ErrRules
	}
	valid, err := MatchRules(context, p.Rules, "")
	if err != nil {
		return failed, err
	}
	if !valid || m.Currency != nil && *m.Currency != "" && object["currency_code"] != *m.Currency {
		return out, nil
	}
	out.Applicable = true
	key, scope := "items", "items"
	if p.Type == "standard" && m.Target == "shipping_methods" {
		key, scope = "shipping_methods", "shipping_methods"
	}
	rows, ok := object[key].([]any)
	if !ok || len(rows) > 1000 {
		return failed, ErrRules
	}
	if p.Type == "buyget" && (m.MinimumBuy == nil || *m.MinimumBuy <= 0 || len(m.BuyRules) == 0 || len(rows) == 0) {
		return out, nil
	}
	seen := map[string]bool{}
	for _, raw := range rows {
		// The standard item filter ignores null entries. Shipping/buy-get
		// require concrete objects; silently broadening their rules is unsafe.
		if raw == nil && p.Type == "standard" && scope == "items" {
			continue
		}
		row, ok := raw.(map[string]any)
		if !ok {
			return failed, ErrRules
		}
		id, ok := row["id"].(string)
		if !ok || id == "" || !readText(id, 191) || seen[id] {
			return failed, ErrRules
		}
		seen[id] = true
		encoded, err := json.Marshal(row)
		if err != nil {
			return failed, ErrRules
		}
		if p.Type == "buyget" {
			buy, err := MatchRules(encoded, m.BuyRules, "items")
			if err != nil {
				return failed, err
			}
			target, err := MatchRules(encoded, m.TargetRules, "items")
			if err != nil {
				return failed, err
			}
			if buy {
				out.Buy = append(out.Buy, id)
			}
			if target {
				out.Target = append(out.Target, id)
			}
			continue
		}
		if scope == "items" {
			if discountable, present := row["is_discountable"]; present && !contextTruthy(discountable) {
				continue
			}
			subtotal, present := row["subtotal"]
			if !present {
				continue
			}
			n, ok := decimal(subtotal)
			if !ok {
				return failed, ErrRules
			}
			if n.Sign() <= 0 {
				continue
			}
			if _, present := row["quantity"]; !present {
				continue
			}
		}
		matches, err := MatchRules(encoded, m.TargetRules, scope)
		if err != nil {
			return failed, err
		}
		if matches {
			if scope == "items" {
				out.Items = append(out.Items, id)
			} else {
				out.Shipping = append(out.Shipping, id)
			}
		}
	}
	return out, nil
}

func contextTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		n, ok := decimal(x)
		return ok && n.Sign() != 0
	default:
		return true
	}
}

// SortCandidates returns a new slice, buy-get first, then descending exact
// value; tied buy-get promotions use minimum buy and apply quantity descending.
// Equal keys retain input order. Method-less candidates cannot be applied and
// go last (the original comparator could dereference an absent method).
func SortCandidates(input []ActiveConfiguration) ([]ActiveConfiguration, error) {
	if len(input) > 1000 {
		return nil, ErrRules
	}
	out := append([]ActiveConfiguration{}, input...)
	for _, p := range out {
		if p.Type != "standard" && p.Type != "buyget" {
			return nil, ErrRules
		}
		if p.Method == nil {
			continue
		}
		n, ok := decimal(p.Method.Value)
		if !ok || n.Sign() < 0 {
			return nil, ErrRules
		}
		for _, q := range []*int64{p.Method.MinimumBuy, p.Method.ApplyQuantity} {
			if q != nil && (*q < 0 || *q > 2147483647) {
				return nil, ErrRules
			}
		}
	}
	quantity := func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Method == nil || b.Method == nil {
			return a.Method != nil && b.Method == nil
		}
		if a.Type != b.Type {
			return a.Type == "buyget"
		}
		av, _ := decimal(a.Method.Value)
		bv, _ := decimal(b.Method.Value)
		if c := av.Cmp(bv); c != 0 {
			return c > 0
		}
		if a.Type == "buyget" {
			if aq, bq := quantity(a.Method.MinimumBuy), quantity(b.Method.MinimumBuy); aq != bq {
				return aq > bq
			}
			return quantity(a.Method.ApplyQuantity) > quantity(b.Method.ApplyQuantity)
		}
		return false
	})
	return out, nil
}
