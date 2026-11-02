package refunds

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type cancellationAssignment struct {
	ID, DeviceID, ItemID, SourceID, Status string
	FulfillmentID                          *string
}

// Check before admitting money and again under the finalizer's device/order
// locks. A canceled parcel does not authorize releasing a still-shipped device,
// a foreign line/source, or a device simultaneously bound to another sale.
func cancellationAssignments(tx *gorm.DB, order string) ([]cancellationAssignment, error) {
	var rows []cancellationAssignment
	if tx.Table("device_assignments").Select("id,device_id,item_id,source_id,status,fulfillment_id").Where("order_id=? AND status IN ('reserved','packed','shipped') AND deleted_at IS NULL", order).Order("device_id,id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(1001).Find(&rows).Error != nil || len(rows) > 1000 {
		return nil, ErrUnavailable
	}
	for _, a := range rows {
		if a.Status != "reserved" || a.FulfillmentID != nil {
			return nil, ErrConflict
		}
		var bindings []struct{ ID string }
		if tx.Table("devices d").Select("d.id").Joins("JOIN orders o ON o.id=? AND o.deleted_at IS NULL", order).Joins("JOIN order_items i ON i.order_id=o.id AND i.version=o.version AND i.item_id=? AND i.deleted_at IS NULL", a.ItemID).Joins("JOIN order_line_items l ON l.id=i.item_id AND l.variant_id=d.variant_id AND l.deleted_at IS NULL").Where("d.id=? AND d.source_id=? AND d.state='reserved' AND d.category='sale' AND d.deleted_at IS NULL", a.DeviceID, a.SourceID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Find(&bindings).Error != nil {
			return nil, ErrUnavailable
		}
		if len(bindings) != 1 {
			return nil, ErrConflict
		}
		var others []struct{ ID string }
		if tx.Table("device_assignments").Select("id").Where("device_id=? AND id<>? AND status IN ('reserved','packed','shipped') AND deleted_at IS NULL", a.DeviceID, a.ID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&others).Error != nil {
			return nil, ErrUnavailable
		}
		if len(others) != 0 {
			return nil, ErrConflict
		}
	}
	return rows, nil
}
