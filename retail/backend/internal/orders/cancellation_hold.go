package orders

import "gorm.io/gorm"

// CancellationPendingTx is a current read under the caller's order lock. A
// runtime switch, revoked actor or uncertain provider response cannot release
// a committed cancellation hold. Readers may display the hold; writers must
// reject it before changing amounts, stock, fulfillment or after-sales facts.
func CancellationPendingTx(tx *gorm.DB, order string) (bool, error) {
	if tx == nil || tx.Statement == nil || !checkoutID.MatchString(order) {
		return false, ErrSchema
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return false, ErrSchema
	}
	var rows []struct{ OperationID string }
	if tx.Raw("SELECT operation_id FROM order_cancellations WHERE order_id=? AND completed_at IS NULL LIMIT 1 FOR SHARE", order).Scan(&rows).Error != nil {
		return false, ErrSchema
	}
	return len(rows) != 0, nil
}

func EnsureNoPendingCancellationTx(tx *gorm.DB, order string) error {
	pending, err := CancellationPendingTx(tx, order)
	if err != nil {
		return err
	}
	if pending {
		return ErrVersionConflict
	}
	return nil
}
