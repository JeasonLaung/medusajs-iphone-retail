package promotion

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/eurorefurb/retail/internal/totals"
)

var ErrCompute = errors.New("invalid promotion computation facts or configuration")

// AttributeUsageSnapshot must be looked up in the same snapshot as the cart and
// configuration. A map entry with Found=false explicitly represents no row;
// an absent entry means the lookup was not performed and is rejected.
type AttributeUsageSnapshot struct {
	Attribute, Value, Used string
	Found                  bool
}

// ComputeInput is internal, server-owned data. Candidates are the desired manual
// and automatic promotions already read with ReadActive, not arbitrary codes.
// Context contains fresh pre-discount subtotal/original_total, never stored
// discounted totals. This phase does not load a cart or authorize a mutation.
type ComputeInput struct {
	Candidates      []ActiveConfiguration
	Context         json.RawMessage
	Epsilon         string
	AttributeUsages map[string]AttributeUsageSnapshot
}

type ComputedAction struct {
	Action, Code, PromotionID string
	ItemID, ShippingMethodID  string
	Amount                    string
	TaxInclusive              bool
}

type ComputeResult struct {
	Actions   []ComputedAction
	Discounts map[string]string
}

// ComputeCandidates coordinates the original qualification/priority/arithmetic
// branches and per-candidate preview budget feedback. It intentionally does not
// increment budgets while previewing: registration aggregates the final actions
// under current locks and can reject the whole transaction. This preview is not
// permission to persist adjustments or complete checkout.
func ComputeCandidates(in ComputeInput) (ComputeResult, error) {
	failed := ComputeResult{}
	if len(in.Context) > 1<<20 || len(in.AttributeUsages) > 1000 {
		return failed, ErrCompute
	}
	epsilon, ok := decimal(in.Epsilon)
	if !ok || epsilon.Sign() < 0 {
		return failed, ErrCompute
	}
	parsed, err := decode(in.Context)
	if err != nil {
		return failed, ErrCompute
	}
	context, ok := parsed.(map[string]any)
	if !ok {
		return failed, ErrCompute
	}
	candidates, err := SortCandidates(in.Candidates)
	if err != nil {
		return failed, err
	}
	result := ComputeResult{Actions: []ComputedAction{}, Discounts: map[string]string{}}
	buyConsumed, targetConsumed := map[string][]totals.BuyGetQuantity{}, map[string][]totals.BuyGetQuantity{}
	seenIDs, seenCodes := map[string]bool{}, map[string]bool{}
	for _, p := range candidates {
		if !usageID.MatchString(p.ID) || p.Code == "" || !readText(p.Code, 191) || seenIDs[p.ID] || seenCodes[p.Code] || p.Used < 0 || p.Limit != nil && *p.Limit < 0 {
			return failed, ErrCompute
		}
		seenIDs[p.ID], seenCodes[p.Code] = true, true
		m := p.Method
		if m == nil {
			continue
		}
		add := func(kind, id, amount string, inclusive bool) {
			a := ComputedAction{Action: kind, Code: p.Code, PromotionID: p.ID, Amount: amount, TaxInclusive: inclusive}
			if kind == "addItemAdjustment" {
				a.ItemID = id
			} else if kind == "addShippingMethodAdjustment" {
				a.ShippingMethodID = id
			}
			result.Actions = append(result.Actions, a)
		}
		// Attribute limits precede promotion limits/rules in the original.
		if b := p.Budget; b != nil {
			if !usageID.MatchString(b.ID) || b.Type != "spend" && b.Type != "usage" && b.Type != "use_by_attribute" {
				return failed, ErrCompute
			}
			if _, err := previewExceeded(b, "0", nil); err != nil {
				return failed, err
			}
			if b.Type == "use_by_attribute" {
				if b.Attribute == nil {
					return failed, ErrCompute
				}
				var value any
				switch *b.Attribute {
				case "customer_id":
					value = context["customer_id"]
					if value == nil {
						if c, ok := context["customer"].(map[string]any); ok {
							value = c["id"]
						}
					}
				case "customer_email":
					value = context["email"]
				default:
					return failed, ErrCompute
				}
				attribute, ok := value.(string)
				usage, lookedUp := in.AttributeUsages[b.ID]
				if !ok || attribute == "" || !readText(attribute, 65535) || !lookedUp || usage.Attribute != *b.Attribute || usage.Value != attribute {
					return failed, ErrCompute
				}
				if usage.Found {
					exceeded, err := previewExceeded(b, "1", &usage.Used)
					if err != nil {
						return failed, err
					}
					if exceeded {
						add("campaignBudgetExceeded", "", "", false)
						continue
					}
				}
			}
		}
		if p.Limit != nil && p.Used >= *p.Limit {
			add("promotionLimitExceeded", "", "", false)
			continue
		}
		qualified, err := QualifyTargets(p, in.Context)
		if err != nil {
			return failed, err
		}
		if !qualified.Applicable {
			continue
		}
		filter := func(id string, amount string) (bool, error) {
			exceeded, err := previewExceeded(p.Budget, amount, nil)
			if err != nil {
				return false, err
			}
			if exceeded {
				add("campaignBudgetExceeded", "", "", false)
			} else if p.Type == "standard" {
				if m.Target == "shipping_methods" {
					add("addShippingMethodAdjustment", id, amount, false)
				} else {
					add("addItemAdjustment", id, amount, p.TaxInclusive)
				}
			}
			return !exceeded, nil
		}
		members := func(ids []string) map[string]bool {
			out := map[string]bool{}
			for _, id := range ids {
				out[id] = true
			}
			return out
		}
		if p.Type == "buyget" {
			if len(qualified.Buy) == 0 || len(qualified.Target) == 0 {
				continue
			}
			buy, target := members(qualified.Buy), members(qualified.Target)
			rows, _ := context["items"].([]any)
			lines := []totals.BuyGetLine{}
			for _, raw := range rows {
				row := raw.(map[string]any) // qualification validated every row
				id := row["id"].(string)
				subtotal, err := computeAmount(row["subtotal"])
				quantity, qerr := computeQuantity(row["quantity"])
				if err != nil || qerr != nil {
					return failed, ErrCompute
				}
				lines = append(lines, totals.BuyGetLine{ID: id, Subtotal: subtotal, Quantity: quantity, BuyEligible: buy[id], TargetEligible: target[id]})
			}
			apply := int64(0)
			if m.ApplyQuantity != nil {
				apply = *m.ApplyQuantity
			}
			r, err := totals.AllocateBuyGetFiltered(totals.BuyGetInput{Code: p.Code, Type: m.Type, Value: m.Value, MinimumBuy: *m.MinimumBuy, ApplyQuantity: apply, MaxQuantity: m.MaxQuantity, Items: lines, Previous: result.Discounts, BuyConsumed: buyConsumed, TargetConsumed: targetConsumed}, filter)
			if err != nil || r.IterationLimit {
				return failed, ErrCompute
			}
			result.Discounts, buyConsumed, targetConsumed = r.Discounts, r.BuyConsumed, r.TargetConsumed
			for _, amount := range r.Amounts {
				add("addItemAdjustment", amount.ID, amount.Amount, false)
			}
			continue
		}
		allocation := ""
		if m.Allocation != nil {
			allocation = *m.Allocation
		} else if m.Target == "order" {
			allocation = "across"
		}
		if m.Target == "shipping_methods" {
			ids := members(qualified.Shipping)
			rows, _ := context["shipping_methods"].([]any)
			methods := []totals.ShippingAllocationMethod{}
			for _, raw := range rows {
				row := raw.(map[string]any)
				id := row["id"].(string)
				if !ids[id] {
					continue
				}
				subtotal := "0"
				if row["subtotal"] != nil {
					subtotal, err = computeAmount(row["subtotal"])
					if err != nil {
						return failed, err
					}
				}
				methods = append(methods, totals.ShippingAllocationMethod{ID: id, Subtotal: subtotal})
			}
			var maximum *int
			if m.MaxQuantity != nil {
				q := int(*m.MaxQuantity)
				maximum = &q
			}
			r, err := totals.AllocateShippingFiltered(totals.ShippingAllocationInput{Allocation: allocation, Type: m.Type, Value: m.Value, MaxQuantity: maximum, Methods: methods, Previous: result.Discounts}, filter)
			if err != nil {
				return failed, err
			}
			result.Discounts = r.Discounts
		} else {
			ids := members(qualified.Items)
			rows, _ := context["items"].([]any)
			lines := []totals.ItemAllocationLine{}
			for _, raw := range rows {
				if raw == nil {
					continue
				}
				row := raw.(map[string]any)
				id := row["id"].(string)
				if !ids[id] {
					continue
				}
				subtotal, err := computeAmount(row["subtotal"])
				quantity, qerr := computeQuantity(row["quantity"])
				original := subtotal
				if p.TaxInclusive {
					original, err = computeAmount(row["original_total"])
				}
				if err != nil || qerr != nil {
					return failed, ErrCompute
				}
				lines = append(lines, totals.ItemAllocationLine{ID: id, Subtotal: subtotal, OriginalTotal: original, Quantity: quantity})
			}
			r, err := totals.AllocateItemsFiltered(totals.ItemAllocationInput{Allocation: allocation, Type: m.Type, Value: m.Value, Epsilon: in.Epsilon, IsTaxInclusive: p.TaxInclusive, MaxQuantity: m.MaxQuantity, Items: lines, Previous: result.Discounts}, filter)
			if err != nil {
				return failed, err
			}
			result.Discounts = r.Discounts
		}
	}
	return result, nil
}

func computeAmount(value any) (string, error) {
	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case json.Number:
		raw = string(v)
	default:
		return "", ErrCompute
	}
	s, err := totals.Resolve(totals.Number{Decimal: raw})
	if err != nil {
		return "", ErrCompute
	}
	n, ok := decimal(s)
	if !ok || n.Sign() < 0 {
		return "", ErrCompute
	}
	return s, nil
}

func computeQuantity(value any) (int64, error) {
	n, ok := decimal(value)
	if !ok || !n.IsInt() || !n.Num().IsInt64() || n.Sign() <= 0 || n.Num().Int64() > 2147483647 {
		return 0, ErrCompute
	}
	return n.Num().Int64(), nil
}

func previewExceeded(b *CampaignBudget, amount string, attributeUsed *string) (bool, error) {
	if b == nil {
		return false, nil
	}
	used, ok := decimal(b.Used)
	if !ok || used.Sign() < 0 {
		return false, ErrCompute
	}
	delta, ok := decimal(amount)
	if !ok || delta.Sign() < 0 {
		return false, ErrCompute
	}
	if b.Type != "spend" {
		delta = big.NewRat(1, 1)
	}
	if attributeUsed != nil {
		used, ok = decimal(*attributeUsed)
		if !ok || used.Sign() < 0 {
			return false, ErrCompute
		}
	}
	var limit *big.Rat
	if b.Limit != nil {
		limit, ok = decimal(*b.Limit)
		if !ok || limit.Sign() < 0 {
			return false, ErrCompute
		}
	}
	if b.Type == "use_by_attribute" && attributeUsed == nil || limit == nil {
		return false, nil
	}
	return new(big.Rat).Add(used, delta).Cmp(limit) > 0, nil
}
