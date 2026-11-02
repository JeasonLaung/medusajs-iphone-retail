package refunds

import (
	"context"
	"math/big"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The order lock is held by both preparation and dispatch.
// Read current order quantities rather than trusting the old request snapshot.
// Financial refund does not receive goods, release reservations or cancel parcels.
func nonDeviceReturn(ctx context.Context, tx *gorm.DB, order, item string, quantity int) error {
	var rows []struct {
		VariantID   *string
		Quantity    string
		RawQuantity *string
	}
	if tx.Table("order_items i").Select("l.variant_id,CAST(i.quantity AS CHAR) AS quantity,i.raw_quantity").Joins("JOIN orders o ON o.id=i.order_id AND o.version=i.version AND o.deleted_at IS NULL").Joins("JOIN order_line_items l ON l.id=i.item_id AND l.deleted_at IS NULL").Where("i.order_id=? AND i.item_id=? AND i.deleted_at IS NULL", order, item).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Find(&rows).Error != nil {
		return ErrUnavailable
	}
	if len(rows) != 1 {
		return ErrConflict
	}
	stored, e := totals.StoredNumber(rows[0].Quantity, rows[0].RawQuantity)
	if e != nil {
		return ErrUnavailable
	}
	decimal, e := totals.Resolve(stored)
	if e != nil {
		return ErrUnavailable
	}
	available, ok := new(big.Rat).SetString(decimal)
	requested := big.NewRat(int64(quantity), 1)
	if !ok || requested.Cmp(available) > 0 {
		return ErrConflict
	}
	var tracked []struct{ ID string }
	if rows[0].VariantID != nil && tx.Table("device_variant_tracking").Select("id").Where("variant_id=? AND deleted_at IS NULL", *rows[0].VariantID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Find(&tracked).Error != nil {
		return ErrUnavailable
	}
	// An existing shipment cannot lose its device requirement by disabling tracking.
	var assignments []struct{ ID string }
	if tx.Table("device_assignments").Select("id").Where("order_id=? AND item_id=? AND status IN ('shipped','returned') AND deleted_at IS NULL", order, item).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&assignments).Error != nil {
		return ErrUnavailable
	}
	if len(tracked) > 0 || len(assignments) > 0 {
		shipped, e := orders.AfterSalesShippedTx(ctx, tx, order, item)
		if e != nil {
			return ErrUnavailable
		}
		if requested.Cmp(new(big.Rat).Sub(available, shipped)) > 0 {
			return ErrConflict
		}
		if len(assignments) > 0 && shipped.Sign() == 0 {
			return ErrConflict
		}
	}
	return nil
}
