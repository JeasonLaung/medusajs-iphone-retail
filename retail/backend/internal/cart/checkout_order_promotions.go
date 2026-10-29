package cart

import (
	"context"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Called only after StagePaidOrderAndStock has authorized the frozen checkout,
// matching sealed paid evidence, new order and current stock in THIS transaction.
// Recompute without replacing frozen adjustments or changing the paid amount.
// Counter/attribute registration and its reversal journal commit with the order.
func registerPaidOrderPromotions(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, orderID string) error {
	var attempt initialAttempt
	if initialWhere(tx, ref, in).Take(&attempt).Error != nil {
		return ErrStorage
	}
	version := strconv.FormatUint(attempt.CartVersion, 10)
	readCtx := context.WithValue(ctx, checkoutReadScopeKey{}, checkoutReadScope{tx: tx.Statement.ConnPool, store: store, version: version, operation: in.OperationID, ref: ref})
	codes, err := ReadPromotionCodes(readCtx, tx, store, ref, version, nil, "add")
	if err != nil {
		return err
	}
	facts, err := ReadPromotionContext(readCtx, tx, store, ref, version)
	if err != nil {
		return err
	}
	// Lock every selected configuration, method, rule and budget row, then use
	// current usage counters. Registration must not authorize a stale preview.
	locked := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Session(&gorm.Session{})
	at := time.Now().UTC()
	candidates, err := promotion.ReadActive(locked, promotion.ActiveRequest{Codes: codes, IncludeAutomatic: true, At: at})
	if err != nil {
		return err
	}
	attributes, err := promotion.ReadAttributeUsages(locked, candidates, facts.JSON)
	if err != nil {
		return err
	}
	computed, err := promotion.ComputeCandidates(promotion.ComputeInput{Candidates: candidates, Context: facts.JSON, Epsilon: facts.CurrencyEpsilon, AttributeUsages: attributes})
	if err != nil {
		return err
	}
	// Include target/code/tax treatment in equality, not just the final discount.
	type adjustment struct {
		Kind, Parent, Promotion, Code, Amount string
		Inclusive                             bool
	}
	expected := map[adjustment]int{}
	actions := []promotion.UsageAction{}
	wanted := map[string]bool{}
	for _, a := range computed.Actions {
		var parent, kind string
		switch a.Action {
		case "addItemAdjustment":
			parent, kind = a.ItemID, "item"
		case "addShippingMethodAdjustment":
			parent, kind = a.ShippingMethodID, "shipping"
		case "promotionLimitExceeded", "campaignBudgetExceeded":
			continue
		default:
			return ErrCompletionPromotion
		}
		canonical, e := totals.Resolve(totals.Number{Decimal: a.Amount})
		if e != nil || canonical == "0" {
			return ErrCompletionPromotion
		}
		expected[adjustment{kind, parent, a.PromotionID, a.Code, canonical, a.TaxInclusive}]++
		wanted[a.PromotionID] = true
		actions = append(actions, promotion.UsageAction{PromotionID: a.PromotionID, Amount: canonical})
	}
	type saved struct {
		ParentID, PromotionID, Code, Amount string
		RawAmount                           *string
		Inclusive                           bool
	}
	for _, spec := range []struct{ kind, table, parentTable, parentKey, inclusive string }{
		{"item", "cart_line_adjustments", "cart_line_items", "item_id", "a.is_tax_inclusive"},
		{"shipping", "cart_shipping_adjustments", "cart_shipping_methods", "shipping_method_id", "FALSE"},
	} {
		var rows []saved
		r := tx.Table(spec.table+" a").Select("a."+spec.parentKey+" AS parent_id,a.promotion_id,a.code,CAST(a.amount AS CHAR) AS amount,a.raw_amount,"+spec.inclusive+" AS inclusive").Joins("JOIN "+spec.parentTable+" p ON p.id=a."+spec.parentKey+" AND p.deleted_at IS NULL").Where("p.cart_id=? AND a.deleted_at IS NULL AND a.promotion_id IS NOT NULL", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Order("a.id").Limit(10001).Scan(&rows)
		if r.Error != nil || len(rows) > 10000 {
			return ErrSchema
		}
		for _, row := range rows {
			n, e := nonNegativeAmount(row.Amount, row.RawAmount)
			if e != nil {
				return e
			}
			amount, e := totals.Resolve(n)
			if e != nil {
				return ErrSchema
			}
			key := adjustment{spec.kind, row.ParentID, row.PromotionID, row.Code, amount, row.Inclusive}
			if expected[key] <= 0 {
				return ErrCompletionPromotion
			}
			expected[key]--
		}
	}
	for _, count := range expected {
		if count != 0 {
			return ErrCompletionPromotion
		}
	}
	var links []string
	if tx.Table("cart_promotions").Select("promotion_id").Where("cart_id=? AND deleted_at IS NULL", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1001).Scan(&links).Error != nil || len(links) > 1000 {
		return ErrSchema
	}
	if len(links) != len(wanted) {
		return ErrCompletionPromotion
	}
	for _, id := range links {
		if !wanted[id] {
			return ErrCompletionPromotion
		}
	}
	// Only values resolved from the server's cart context are passed to the
	// usage ledger. Anonymous carts have no customer_id; never invent one.
	registration := promotion.UsageRegistration{Currency: attempt.CurrencyCode, At: at, Attributes: map[string]string{}}
	for _, fact := range attributes {
		if prior, ok := registration.Attributes[fact.Attribute]; ok && prior != fact.Value {
			return ErrCompletionPromotion
		}
		registration.Attributes[fact.Attribute] = fact.Value
	}
	_, err = promotion.RegisterUsageOnce(ctx, tx, promotion.UsageOperation{ScopeID: orderID, OperationID: in.OperationID}, actions, registration)
	return err
}
