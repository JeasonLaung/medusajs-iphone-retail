package orders

import (
	"context"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"math/big"
)

// Caller has authorized and locked the order. Preserve scalar/raw precision
// when checking unshipped return eligibility, including canceled parcels.
func AfterSalesShippedTx(ctx context.Context, tx *gorm.DB, orderID, itemID string) (*big.Rat, error) {
	out := new(big.Rat)
	if fulfillment.Ready(ctx, tx) != nil {
		return nil, ErrSchema
	}
	var rows []struct{ Quantity, RawQuantity, FulfillmentID string }
	if tx.Table("fulfillment_items i").Select("CAST(i.quantity AS CHAR) AS quantity,i.raw_quantity,i.fulfillment_id").Joins("JOIN fulfillments f ON f.id=i.fulfillment_id AND f.deleted_at IS NULL AND f.shipped_at IS NOT NULL AND f.canceled_at IS NULL").Joins("JOIN order_fulfillments l ON l.fulfillment_id=f.id AND l.deleted_at IS NULL").Where("l.order_id=? AND i.line_item_id=? AND i.deleted_at IS NULL", orderID, itemID).Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return nil, ErrSchema
	}
	for _, r := range rows {
		var links int64
		if tx.Table("order_fulfillments").Where("fulfillment_id=? AND deleted_at IS NULL", r.FulfillmentID).Count(&links).Error != nil || links != 1 {
			return nil, ErrSchema
		}
		n, e := amountNumber(r.Quantity, &r.RawQuantity)
		if e != nil {
			return nil, e
		}
		value, e := totals.Resolve(n)
		if e != nil {
			return nil, e
		}
		q, ok := new(big.Rat).SetString(value)
		if !ok || q.Sign() < 0 {
			return nil, ErrSchema
		}
		out.Add(out, q)
	}
	return out, nil
}

func PositiveAfterSalesRefund(decimal, raw string) (bool, error) {
	n, e := amountNumber(decimal, &raw)
	if e != nil {
		return false, e
	}
	value, e := totals.Resolve(n)
	if e != nil {
		return false, e
	}
	q, ok := new(big.Rat).SetString(value)
	if !ok {
		return false, ErrSchema
	}
	return q.Sign() > 0, nil
}
