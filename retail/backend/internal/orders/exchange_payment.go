package orders

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"time"
)

// Internal exchange participant: initial paid eligibility belongs to the device
// command. Never creates captures, refunds or paid evidence. Only mutable
// unpaid collections are resized; immutable completed collections survive.
func refreshExchangePaymentTx(tx *gorm.DB, amounts AmountDetail) error {
	if amounts.Summary == nil {
		return ErrSchema
	}
	pending, ok := new(big.Rat).SetString(amounts.Summary.PendingDifference)
	if !ok {
		return ErrSchema
	}
	var rows []struct{ ID, Status, CurrencyCode string }
	if tx.Table("payment_collections p").Select("p.id,p.status,p.currency_code").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.id AND l.deleted_at IS NULL").Where("l.order_id=? AND p.deleted_at IS NULL AND p.status IN ('not_paid','awaiting','authorized','partially_authorized')", amounts.OrderID).Order("p.id").Limit(2).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&rows).Error != nil || len(rows) > 1 {
		return ErrSchema
	}
	if len(rows) == 1 && (rows[0].CurrencyCode != amounts.CurrencyCode || rows[0].Status == "authorized" || rows[0].Status == "partially_authorized") {
		return ErrVersionConflict
	}
	// Native flow leaves an existing collection untouched on negative difference.
	if pending.Sign() < 0 {
		return nil
	}
	if len(rows) == 0 && pending.Sign() == 0 {
		return nil
	}
	scalar, raw, e := exchangeNumber(amounts.Summary.PendingDifference)
	if e != nil {
		return e
	}
	now := time.Now().UTC()
	if len(rows) == 1 {
		return tx.Table("payment_collections").Where("id=? AND status IN ('not_paid','awaiting') AND deleted_at IS NULL", rows[0].ID).Updates(map[string]any{"amount": scalar, "raw_amount": raw, "updated_at": now}).Error
	}
	id := "paycol_" + uuid.NewString()
	if tx.Table("payment_collections").Create(map[string]any{"id": id, "currency_code": amounts.CurrencyCode, "amount": scalar, "raw_amount": raw, "status": "not_paid", "created_at": now, "updated_at": now}).Error != nil {
		return ErrSchema
	}
	if tx.Table("order_payment_collections").Create(map[string]any{"id": uuid.NewString(), "order_id": amounts.OrderID, "payment_collection_id": id, "created_at": now, "updated_at": now}).Error != nil {
		return ErrSchema
	}
	return nil
}
