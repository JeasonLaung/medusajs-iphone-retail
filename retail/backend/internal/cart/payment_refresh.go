package cart

import (
	"context"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PreparePaymentRefresh rechecks cart ownership/version and locks collection
// links, the collection and sessions in the enclosing command's transaction.
// It computes the refreshed cart total AFTER taxes/shipping/promotions. It
// neither updates the collection nor deletes local/provider payment sessions.
// The plan is private; subsequent provider coordination and durable completion
// are mandatory. Never commit a changed plan as if payment refresh succeeded.
func PreparePaymentRefresh(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) (PaymentRefreshPlan, error) {
	failed := PaymentRefreshPlan{}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return failed, err
	}
	if payments.Ready(ctx, tx) != nil {
		return failed, ErrSchema
	}
	amounts, err := assembleAmounts(tx, cart)
	if err != nil {
		return failed, err
	}
	total, exists := amounts.Totals["total"]
	if !exists {
		return failed, ErrSchema
	}
	var links []struct{ PaymentCollectionID string }
	if tx.Table("cart_payment_collections").Select("payment_collection_id").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("payment_collection_id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1001).Scan(&links).Error != nil || len(links) > 1000 {
		return failed, ErrSchema
	}
	var active *PaymentRefreshCollection
	for _, link := range links {
		if !idPattern.MatchString(link.PaymentCollectionID) {
			return failed, ErrSchema
		}
		var rows []struct{ ID, CurrencyCode, Amount, RawAmount string }
		if tx.Table("payment_collections").Select("id,currency_code,CAST(amount AS CHAR) AS amount,raw_amount").Where("id=? AND deleted_at IS NULL", link.PaymentCollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Scan(&rows).Error != nil {
			return failed, ErrSchema
		}
		if len(rows) == 0 {
			continue
		}
		if active != nil {
			return failed, ErrSchema
		}
		row := rows[0]
		n, err := nonNegativeAmount(row.Amount, &row.RawAmount)
		if err != nil {
			return failed, err
		}
		value, err := totals.Resolve(n)
		if err != nil {
			return failed, ErrSchema
		}
		active = &PaymentRefreshCollection{ID: row.ID, Currency: row.CurrencyCode, Amount: value, SessionIDs: []string{}}
		if tx.Table("payment_sessions").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", row.ID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&active.SessionIDs).Error != nil || len(active.SessionIDs) > 100 {
			return failed, ErrSchema
		}
	}
	// The enclosing command has an already-fetched owned cart, matching the
	// original input.cart branch (a missing collection is a no-op, not creation).
	return PlanPaymentRefresh(PaymentRefreshFacts{InputCartProvided: true, Amount: total, Currency: cart.CurrencyCode, Collection: active})
}
