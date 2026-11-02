package cart

import (
	"context"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A retired checkout leaves its original collection available for a new
// payment. Only a current owned, unpaid collection with no live sessions or
// cash facts is reusable. An arbitrary linked collection grants no permission.
func reusablePaymentCollection(ctx context.Context, tx *gorm.DB, cartID string) (string, error) {
	var ids []string
	if tx.Table("cart_payment_collections").Select("payment_collection_id").Where("cart_id=? AND deleted_at IS NULL", cartID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(2).Scan(&ids).Error != nil {
		return "", ErrStorage
	}
	if len(ids) == 0 {
		return "", nil
	}
	if len(ids) != 1 {
		return "", ErrConflict
	}
	// Only our retired checkout can leave a reusable collection. Imported or
	// dangling links must retain the ordinary draft-command conflict boundary.
	var attempts []struct{ OperationID string }
	if tx.Table("cart_checkout_attempts").Select("operation_id").Where("cart_id=? AND collection_id=?", cartID, ids[0]).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Scan(&attempts).Error != nil {
		return "", ErrStorage
	}
	if len(attempts) != 1 {
		return "", ErrConflict
	}
	var live []struct{ ID string }
	if tx.Table("payment_sessions").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", ids[0]).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Scan(&live).Error != nil {
		return "", ErrStorage
	}
	if len(live) != 0 {
		return "", ErrConflict
	}
	_, sessions, _, err := paymentSources(ctx, tx, cartID, PaymentRefreshPlan{CollectionID: ids[0], UpdateCollection: true, DeleteSessionIDs: []string{}})
	if err != nil {
		return "", err
	}
	if len(sessions) != 0 {
		return "", ErrConflict
	}
	var cash int64
	if tx.Table("payments").Where("payment_collection_id=? AND deleted_at IS NULL", ids[0]).Count(&cash).Error != nil {
		return "", ErrStorage
	}
	if cash != 0 {
		return "", ErrConflict
	}
	return ids[0], nil
}

// Synchronize an empty collection after an ordinary draft command. Existing
// sessions continue to require explicit provider coordination instead.
func syncEmptyPaymentCollection(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) error {
	id, err := reusablePaymentCollection(ctx, tx, ref.CartID)
	if err != nil || id == "" {
		return err
	}
	plan, err := PreparePaymentRefresh(ctx, tx, store, ref, version)
	if err != nil {
		return err
	}
	if !plan.UpdateCollection {
		return nil
	}
	if plan.CollectionID != id || len(plan.DeleteSessionIDs) != 0 {
		return ErrConflict
	}
	scalar, raw, err := sqlAmount(plan.Amount)
	if err != nil {
		return err
	}
	r := tx.Table("payment_collections").Where("id=? AND deleted_at IS NULL", id).Updates(map[string]any{"amount": scalar, "raw_amount": raw, "currency_code": plan.Currency, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return ErrStorage
	}
	return nil
}
