package devices

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Only included in the opaque revision, never in warehouse device projections.
type warehouseReservation struct {
	ID, DeviceID, OrderID, ItemID, SourceID, Status string
	FulfillmentID                                   *string
	UpdatedAt                                       time.Time
}

// An empty assignment selects the sole active unpacked reservation. Replay
// selects the original canceled assignment; it cannot bind to a later sale.
func readWarehouseReservation(tx *gorm.DB, row WarehouseDevice, assignment string) (*warehouseReservation, error) {
	query := tx.Table("device_assignments a").Select("a.id,a.device_id,a.order_id,a.item_id,a.source_id,a.status,a.fulfillment_id,a.updated_at").Joins("JOIN device_sources s ON s.id=a.source_id AND s.location_id=? AND s.deleted_at IS NULL", row.LocationID).Joins("JOIN orders o ON o.id=a.order_id AND o.deleted_at IS NULL AND o.status NOT IN ('draft','canceled') AND o.is_draft_order=FALSE AND o.canceled_at IS NULL").Where("a.device_id=? AND a.source_id=? AND a.deleted_at IS NULL", row.ID, row.SourceID)
	if assignment == "" {
		query = query.Where("a.status IN ('reserved','packed','shipped') AND NOT EXISTS (SELECT 1 FROM device_assignments x WHERE x.device_id=a.device_id AND x.id<>a.id AND x.status IN ('reserved','packed','shipped') AND x.deleted_at IS NULL)")
	} else {
		query = query.Where("a.id=?", assignment)
	}
	var found []warehouseReservation
	if query.Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Scan(&found).Error != nil {
		return nil, ErrUnavailable
	}
	if len(found) != 1 || found[0].FulfillmentID != nil {
		return nil, nil
	}
	want := "reserved"
	if assignment != "" {
		want = "canceled"
	}
	if found[0].Status != want {
		return nil, nil
	}
	return &found[0], nil
}

func (h Handler) releaseWarehouseReservation(ctx context.Context, actor identity.User, location, category, deviceID string, in WarehouseDeviceCommand) (WarehouseDeviceResult, error) {
	var out WarehouseDeviceResult
	if category != "sale" || !validPathID(location) || !validPathID(deviceID) || !validWarehouseCommand(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorization.CheckWarehouseTx(ctx, tx, actor, location, authorization.WarehouseIntakeRead, authorization.OrderFulfillmentRead, authorization.WarehouseAllocationWrite); err != nil {
			return err
		}
		rev, enabled, err := allocationRuntimeGate(ctx, tx)
		if err != nil {
			return err
		}
		if !enabled {
			return ErrUnavailable
		}
		var row WarehouseDevice
		if err := warehouseDeviceQuery(tx).Where("d.id=? AND d.location_id=? AND d.category='sale' AND d.deleted_at IS NULL", deviceID, location).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return ErrUnavailable
		}
		var prior struct{ ActorID, LocationID, Category, DeviceID, RequestJSON, ResultJSON string }
		err = tx.Table("warehouse_device_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if err == nil {
			if prior.ActorID != actor.ID || prior.LocationID != location || prior.Category != category || prior.DeviceID != deviceID || prior.RequestJSON != string(request) {
				return ErrConflict
			}
			var saved struct {
				WarehouseDeviceResult
				OrderID, ItemID, SourceID string
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &saved) != nil {
				return ErrUnavailable
			}
			out = saved.WarehouseDeviceResult
			if out.AssignmentID == "" || out.ID != deviceID || out.State != "available" || out.Deleted {
				return ErrUnavailable
			}
			bound, err := readWarehouseReservation(tx, row, out.AssignmentID)
			if err != nil {
				return err
			}
			if bound == nil || bound.OrderID != saved.OrderID || bound.ItemID != saved.ItemID || bound.SourceID != saved.SourceID {
				return ErrConflict
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if row.State != "reserved" {
			return ErrConflict
		}
		row.Reservation, err = readWarehouseReservation(tx, row, "")
		if err != nil {
			return err
		}
		if row.Reservation == nil {
			return ErrConflict
		}
		if err = projectWarehouseDevice(&row, rev); err != nil {
			return err
		}
		if row.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		a := row.Reservation
		changed := tx.Exec("UPDATE device_assignments SET status='canceled',fulfillment_id=NULL,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND device_id=? AND status='reserved' AND fulfillment_id IS NULL AND deleted_at IS NULL", a.ID, deviceID)
		if changed.Error != nil {
			return ErrUnavailable
		}
		if changed.RowsAffected != 1 {
			return ErrConflict
		}
		changed = tx.Exec("UPDATE devices SET state='available',updated_at=UTC_TIMESTAMP(6) WHERE id=? AND state='reserved' AND deleted_at IS NULL", deviceID)
		if changed.Error != nil {
			return ErrUnavailable
		}
		if changed.RowsAffected != 1 {
			return ErrConflict
		}
		data, _ := json.Marshal(map[string]string{"assignment_id": a.ID, "item_id": a.ItemID})
		if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,'release',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), deviceID, a.OrderID, actor.ID, string(data)).Error != nil {
			return ErrUnavailable
		}
		out = WarehouseDeviceResult{ID: deviceID, State: "available", AssignmentID: a.ID}
		result, _ := json.Marshal(struct {
			WarehouseDeviceResult
			OrderID, ItemID, SourceID string
		}{out, a.OrderID, a.ItemID, a.SourceID})
		if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO warehouse_device_changes(operation_id,actor_id,location_id,category,device_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, location, category, deviceID, string(request), string(result)).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, err
}
