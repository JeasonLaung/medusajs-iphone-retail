package cart

import (
	"context"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlanPromotionCodes preserves the original insertion-ordered set semantics.
// Existing codes come from live item/shipping adjustments, not cart links.
// Known codes mean undeleted configured promotions, not active or qualified.
func PlanPromotionCodes(existing, requested, known []string, action string) ([]string, error) {
	if action == "" {
		action = "add"
	}
	if action != "add" && action != "replace" && action != "remove" || len(existing) > 10000 || len(requested) > 100 || len(known) > 100 {
		return nil, ErrInput
	}
	valid := func(code string) bool { return code != "" && len(code) <= 191 && utf8.ValidString(code) }
	knownSet := map[string]bool{}
	for _, code := range known {
		if !valid(code) {
			return nil, ErrInput
		}
		knownSet[code] = true
	}
	for _, code := range requested {
		if !valid(code) {
			return nil, ErrInput
		}
	}
	out := []string{}
	seen := map[string]bool{}
	if action != "replace" {
		removed := map[string]bool{}
		if action == "remove" {
			for _, code := range requested {
				removed[code] = true
			}
		}
		for _, code := range existing {
			if code == "" {
				continue
			}
			if !valid(code) {
				return nil, ErrSchema
			}
			if !seen[code] && !removed[code] {
				seen[code] = true
				out = append(out, code)
			}
		}
	}
	if action != "remove" {
		for _, code := range requested {
			if !knownSet[code] {
				return nil, ErrInput
			}
			if !seen[code] {
				seen[code] = true
				out = append(out, code)
			}
		}
	}
	// ReadActive has the same request cap. Reject instead of silently dropping
	// already-applied codes when a cart exceeds the supported combined limit.
	if len(out) > 100 {
		return nil, ErrInput
	}
	return out, nil
}

// ReadPromotionCodes rechecks current cart authority/version, gathers existing
// adjustment codes and validates requested code existence in the caller's
// transaction. It neither applies discounts nor writes relations. Auto promos
// remain a later ReadActive option; linked-only codes are not implicitly kept.
func ReadPromotionCodes(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, requested []string, action string) ([]string, error) {
	if _, err := PlanPromotionCodes(nil, requested, requested, action); err != nil {
		return nil, err
	}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return nil, err
	}
	if promotion.Ready(ctx, tx) != nil {
		return nil, ErrSchema
	}
	existing := []string{}
	for _, spec := range []struct {
		parent, adjustments, key string
		limit                    int
	}{
		{"cart_line_items", "cart_line_adjustments", "item_id", 1000},
		{"cart_shipping_methods", "cart_shipping_adjustments", "shipping_method_id", 100},
	} {
		var parents []struct{ ID string }
		if tx.Table(spec.parent).Select("id").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(spec.limit+1).Scan(&parents).Error != nil || len(parents) > spec.limit {
			return nil, ErrSchema
		}
		ids := []string{}
		for _, p := range parents {
			if !idPattern.MatchString(p.ID) {
				return nil, ErrSchema
			}
			ids = append(ids, p.ID)
		}
		if len(ids) == 0 {
			continue
		}
		var rows []struct {
			ParentID string
			Code     *string
		}
		if tx.Table(spec.adjustments).Select(spec.key+" AS parent_id,code").Where(spec.key+" IN ? AND deleted_at IS NULL", ids).Order(spec.key).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(10001).Scan(&rows).Error != nil || len(rows) > 10000 {
			return nil, ErrSchema
		}
		counts := map[string]int{}
		for _, r := range rows {
			counts[r.ParentID]++
			if counts[r.ParentID] > 100 {
				return nil, ErrSchema
			}
			if r.Code != nil {
				existing = append(existing, *r.Code)
			}
		}
	}
	known := []string{}
	if action != "remove" && len(requested) > 0 {
		var rows []struct{ Code string }
		// Deliberately do not filter status or campaign dates. Original code
		// validation checks existence; ReadActive performs later eligibility.
		if tx.Table("promotions").Select("code").Where("code IN ? AND deleted_at IS NULL", requested).Order("id").Limit(101).Scan(&rows).Error != nil || len(rows) > 100 {
			return nil, ErrSchema
		}
		for _, row := range rows {
			known = append(known, row.Code)
		}
	}
	return PlanPromotionCodes(existing, requested, known, action)
}
