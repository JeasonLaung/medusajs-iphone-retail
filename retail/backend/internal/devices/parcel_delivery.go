package devices

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Delivery records a shipped manual device parcel, retaining its payment,
// stock, device and tracking records. Authorization and receipt recovery run
// through changeParcel before this transaction-local transition.
func finishParcelDelivery(tx *gorm.DB, actor identity.User, order, parcel string, in ParcelInput, request []byte, before ParcelContext, facts parcelFacts, rev uint64, enabled bool, result *ParcelResult) error {
	if !before.OrderActive || !before.Supported || before.Status != "shipped" || before.Revision != in.ExpectedRevision || len(before.Items) == 0 || len(before.Devices) == 0 || facts.Record.PackedAt == nil || facts.Record.ShippedAt == nil || facts.Record.DeliveredAt != nil || facts.Record.CanceledAt != nil {
		return ErrConflict
	}
	var assignments []struct{ ID, OrderID string }
	if tx.Table("device_assignments").Select("id,order_id").Where("fulfillment_id=? AND deleted_at IS NULL", parcel).Order("id").Limit(1001).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&assignments).Error != nil || len(assignments) != len(before.Devices) {
		return ErrConflict
	}
	for _, a := range assignments {
		if a.OrderID != order {
			return ErrConflict
		}
	}
	counts := map[string]int{}
	for _, d := range before.Devices {
		line, ok := facts.Lines[d.ItemID]
		if !ok || line.VariantID != d.VariantID || d.Status != "shipped" || d.State != "shipped" || d.LocationID != before.LocationID || d.SourceID != d.AssignmentSourceID {
			return ErrConflict
		}
		counts[d.ItemID]++
	}
	seen := map[string]bool{}
	for _, item := range before.Items {
		if item.ItemID == nil || item.InventoryItemID == nil || seen[*item.ItemID] || counts[*item.ItemID] == 0 {
			return ErrConflict
		}
		seen[*item.ItemID] = true
		equal, e := inventory.Equal(item.Quantity, strconv.Itoa(counts[*item.ItemID]))
		if e != nil || !equal {
			return ErrConflict
		}
	}
	if len(seen) != len(counts) {
		return ErrConflict
	}
	for item, count := range counts {
		line := facts.Lines[item]
		if !nonnegative(line.DeliveredQuantity) || !nonnegative(line.ShippedQuantity) {
			return ErrConflict
		}
		next, e := inventory.Difference(line.DeliveredQuantity, "-"+strconv.Itoa(count))
		if e != nil {
			return ErrConflict
		}
		margin, e := inventory.Difference(line.ShippedQuantity, next)
		if e != nil || !nonnegative(margin) {
			return ErrConflict
		}
		if tx.Exec("UPDATE order_items SET delivered_quantity=?,raw_delivered_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", next, rawCount(next), line.ID).Error != nil {
			return ErrUnavailable
		}
	}
	if tx.Exec("UPDATE fulfillments SET delivered_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE id=?", parcel).Error != nil || tx.Exec("UPDATE orders SET updated_at=UTC_TIMESTAMP(6) WHERE id=?", order).Error != nil || tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
		return ErrUnavailable
	}
	if customernotices.EmitOrderTx(tx, order, "delivery.created", parcel) != nil {
		return ErrUnavailable
	}
	after, afterFacts, e := loadParcel(tx, order, parcel, rev+1, enabled)
	if e != nil {
		return e
	}
	*result = ParcelResult{FulfillmentID: parcel, Action: "deliver", AssignmentIDs: []string{}}
	for _, d := range before.Devices {
		result.AssignmentIDs = append(result.AssignmentIDs, d.AssignmentID)
	}
	audit := func(c ParcelContext, f parcelFacts) string {
		b, _ := json.Marshal(map[string]any{"context": c, "stock": f.Packing.Stock, "reservations": f.Packing.Reservations, "lines": f.Lines})
		return string(b)
	}
	encoded, _ := json.Marshal(result)
	return tx.Table("order_device_parcel_operations").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "order_id": order, "fulfillment_id": parcel, "request_json": string(request), "before_json": audit(before, facts), "after_json": audit(after, afterFacts), "result_json": string(encoded), "created_at": time.Now().UTC()}).Error
}
