package refunds

import (
	"github.com/eurorefurb/retail/internal/orders"
	"gorm.io/gorm"
	"math/big"
)

// exchangeCredit is called under the order lock. Unresolved commands across
// every payment/request reserve current order credit as well as captured cash.
// The excluded command has already reserved itself when checked before dispatch.
func exchangeCredit(tx *gorm.DB, order, exclude string) (*big.Rat, error) {
	pending, e := orders.CurrentPendingDifferenceTx(tx, order)
	if e != nil {
		return nil, ErrUnavailable
	}
	credit := new(big.Rat).Neg(pending)
	var holds []operation
	if tx.Where("order_id=? AND operation_id<>? AND canceled_at IS NULL AND applied=FALSE AND state NOT IN ('failed','canceled')", order, exclude).Limit(10001).Find(&holds).Error != nil || len(holds) > 10000 {
		return nil, ErrUnavailable
	}
	for _, h := range holds {
		n, ok := new(big.Rat).SetString(h.Amount)
		if !ok || n.Sign() <= 0 {
			return nil, ErrUnavailable
		}
		credit.Sub(credit, n)
	}
	return credit, nil
}
func exchangeAmount(tx *gorm.DB, r operation) error {
	var request struct{ Type string }
	if tx.Table("after_sales_requests").Select("type").Where("id=? AND order_id=? AND deleted_at IS NULL", r.RequestID, r.OrderID).Take(&request).Error != nil {
		return ErrConflict
	}
	if request.Type != "exchange" {
		return nil
	}
	credit, e := exchangeCredit(tx, r.OrderID, r.OperationID)
	if e != nil {
		return e
	}
	amount, ok := new(big.Rat).SetString(r.Amount)
	// One successful instruction per request: settle its full available credit,
	// without changing the existing partial-return refund contract.
	if !ok || credit.Sign() <= 0 || amount.Cmp(credit) != 0 {
		return ErrConflict
	}
	return nil
}
