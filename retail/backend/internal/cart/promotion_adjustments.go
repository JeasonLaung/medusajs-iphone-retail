package cart

import (
	"context"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PromotionSkip struct{ Code, Reason string }
type PromotionAdjustmentResult struct {
	ItemAdjustments, ShippingAdjustments, Removed, Promotions int
	Skipped                                                   []PromotionSkip
}

// ReplacePromotionAdjustments is an INTERNAL persistence phase. Actions must
// come from ComputeCandidates in the same complete cart command; this is never
// a public payload or independent authority to discount. It rechecks cart
// authority/version and target membership, but does not recompute rules or
// reserve campaign budgets. Caller MUST roll back the whole transaction on any
// error, finish totals/payment/version/idempotency, and only then commit.
// Cart refresh itself does not register budget usage (registration is checkout).
func ReplacePromotionAdjustments(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, actions []promotion.ComputedAction) (PromotionAdjustmentResult, error) {
	failed := PromotionAdjustmentResult{}
	if len(actions) > 10000 {
		return failed, ErrInput
	}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return failed, err
	}
	if promotion.Ready(ctx, tx) != nil {
		return failed, ErrSchema
	}
	result := PromotionAdjustmentResult{Skipped: []PromotionSkip{}}
	type targetSet struct {
		table, parent, adjustmentTable string
		ids                            []string
		member                         map[string]bool
		remove                         []string
	}
	sets := []*targetSet{
		{table: "cart_line_items", parent: "item_id", adjustmentTable: "cart_line_adjustments", member: map[string]bool{}},
		{table: "cart_shipping_methods", parent: "shipping_method_id", adjustmentTable: "cart_shipping_adjustments", member: map[string]bool{}},
	}
	perTarget := map[string]int{}
	for i, set := range sets {
		limit := 1000
		if i == 1 {
			limit = 100
		}
		var rows []struct{ ID string }
		if tx.Table(set.table).Select("id").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(limit+1).Scan(&rows).Error != nil || len(rows) > limit {
			return failed, ErrSchema
		}
		for _, row := range rows {
			if !idPattern.MatchString(row.ID) {
				return failed, ErrSchema
			}
			set.ids = append(set.ids, row.ID)
			set.member[row.ID] = true
		}
		if len(set.ids) == 0 {
			continue
		}
		var adjustments []struct {
			ID, ParentID string
			Code         *string
		}
		if tx.Table(set.adjustmentTable).Select("id,"+set.parent+" AS parent_id,code").Where(set.parent+" IN ? AND deleted_at IS NULL", set.ids).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(10001).Scan(&adjustments).Error != nil || len(adjustments) > 10000 {
			return failed, ErrSchema
		}
		counts := map[string]int{}
		for _, a := range adjustments {
			counts[a.ParentID]++
			if !idPattern.MatchString(a.ID) || counts[a.ParentID] > 100 {
				return failed, ErrSchema
			}
			// Original compute removes adjustments with any string code, even
			// empty or no-longer-existing codes. Null-code manual rows survive.
			if a.Code != nil {
				set.remove = append(set.remove, a.ID)
			} else {
				perTarget[set.table+":"+a.ParentID]++
			}
		}
	}
	type prepared struct {
		a               promotion.ComputedAction
		set             *targetSet
		id, scalar, raw string
	}
	preparedRows := []prepared{}
	codes := map[string]string{}
	ids := map[string]string{}
	for _, a := range actions {
		if !idPattern.MatchString(a.PromotionID) || a.Code == "" || len(a.Code) > 191 || !utf8.ValidString(a.Code) {
			return failed, ErrInput
		}
		if code, exists := ids[a.PromotionID]; exists && code != a.Code {
			return failed, ErrInput
		}
		if id, exists := codes[a.Code]; exists && id != a.PromotionID {
			return failed, ErrInput
		}
		ids[a.PromotionID], codes[a.Code] = a.Code, a.PromotionID
		var set *targetSet
		var parent string
		switch a.Action {
		case "addItemAdjustment":
			if a.ShippingMethodID != "" {
				return failed, ErrInput
			}
			set, parent = sets[0], a.ItemID
		case "addShippingMethodAdjustment":
			if a.ItemID != "" || a.TaxInclusive {
				return failed, ErrInput
			}
			set, parent = sets[1], a.ShippingMethodID
		case "promotionLimitExceeded", "campaignBudgetExceeded":
			if a.ItemID != "" || a.ShippingMethodID != "" || a.Amount != "" || a.TaxInclusive {
				return failed, ErrInput
			}
			reason := "promotion_limit_exceeded"
			if a.Action == "campaignBudgetExceeded" {
				reason = "campaign_budget_exceeded"
			}
			result.Skipped = append(result.Skipped, PromotionSkip{Code: a.Code, Reason: reason})
			continue
		default:
			return failed, ErrInput
		}
		if !set.member[parent] {
			return failed, ErrDenied
		}
		perTarget[set.table+":"+parent]++
		if perTarget[set.table+":"+parent] > 100 {
			return failed, ErrInput
		}
		scalar, raw, err := sqlAmount(a.Amount)
		if err != nil || a.Amount == "0" {
			return failed, ErrInput
		}
		preparedRows = append(preparedRows, prepared{a: a, set: set, id: "caadj_" + uuid.NewString(), scalar: scalar, raw: raw})
	}
	// Validate code/ID binding before deleting anything; never re-resolve an
	// unknown code to an unrelated promotion or write a nil promotion reference.
	promotionIDs := []string{}
	for id := range ids {
		promotionIDs = append(promotionIDs, id)
	}
	sort.Strings(promotionIDs)
	if len(promotionIDs) > 1000 {
		return failed, ErrInput
	}
	if len(promotionIDs) > 0 {
		var rows []struct {
			ID, Code, Status string
			DeletedAt        *time.Time
		}
		if tx.Table("promotions").Select("id,code,status,deleted_at").Where("id IN ?", promotionIDs).Order("id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(1001).Scan(&rows).Error != nil || len(rows) != len(promotionIDs) {
			return failed, ErrSchema
		}
		for _, p := range rows {
			if p.Code != ids[p.ID] || p.Status != "active" || p.DeletedAt != nil {
				return failed, ErrConflict
			}
		}
	}
	var links []struct {
		PromotionID string
		DeletedAt   *time.Time
	}
	if tx.Table("cart_promotions").Select("promotion_id,deleted_at").Where("cart_id=?", cart.ID).Order("promotion_id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1001).Scan(&links).Error != nil || len(links) > 1000 {
		return failed, ErrSchema
	}
	wanted := map[string]bool{}
	for _, p := range preparedRows {
		wanted[p.a.PromotionID] = true
	}
	for _, set := range sets {
		if len(set.remove) == 0 {
			continue
		}
		r := tx.Table(set.adjustmentTable).Where("id IN ? AND deleted_at IS NULL", set.remove).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != int64(len(set.remove)) {
			return failed, ErrStorage
		}
		result.Removed += len(set.remove)
	}
	for _, p := range preparedRows {
		a := p.a
		if p.set == sets[0] {
			if tx.Exec(`INSERT INTO cart_line_adjustments(id,item_id,promotion_id,code,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, p.id, a.ItemID, a.PromotionID, a.Code, p.scalar, p.raw, a.TaxInclusive).Error != nil {
				return failed, ErrStorage
			}
			result.ItemAdjustments++
		} else {
			if tx.Exec(`INSERT INTO cart_shipping_adjustments(id,shipping_method_id,promotion_id,code,amount,raw_amount,created_at,updated_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, p.id, a.ShippingMethodID, a.PromotionID, a.Code, p.scalar, p.raw).Error != nil {
				return failed, ErrStorage
			}
			result.ShippingAdjustments++
		}
	}
	existing := map[string]bool{}
	for _, link := range links {
		existing[link.PromotionID] = true
		if wanted[link.PromotionID] && link.DeletedAt != nil {
			if tx.Exec("UPDATE cart_promotions SET deleted_at=NULL,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND promotion_id=?", cart.ID, link.PromotionID).Error != nil {
				return failed, ErrStorage
			}
		} else if !wanted[link.PromotionID] && link.DeletedAt == nil {
			if tx.Exec("UPDATE cart_promotions SET deleted_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND promotion_id=?", cart.ID, link.PromotionID).Error != nil {
				return failed, ErrStorage
			}
		}
	}
	for _, id := range promotionIDs {
		if wanted[id] && !existing[id] {
			if tx.Exec(`INSERT INTO cart_promotions(cart_id,promotion_id,id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, cart.ID, id, "capromo_"+uuid.NewString()).Error != nil {
				return failed, ErrStorage
			}
		}
	}
	result.Promotions = len(wanted)
	return result, nil
}
