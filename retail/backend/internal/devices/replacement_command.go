package devices

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type ReplacementInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	DeviceID         string `json:"device_id"`
}
type ReplacementResult struct {
	AssignmentID      string `json:"assignment_id"`
	DeviceID          string `json:"device_id"`
	ExchangeID        string `json:"exchange_id"`
	ReturnID          string `json:"return_id"`
	ItemID            string `json:"item_id"`
	OrderVersion      int    `json:"order_version"`
	PendingDifference string `json:"pending_difference"`
	Replayed          bool   `json:"replayed"`
}

func (h Handler) ReplaceDevice(ctx context.Context, actor identity.User, o, r, a string, in ReplacementInput) (out ReplacementResult, err error) {
	if !validPathID(o) || !validPathID(r) || !validPathID(a) || !validPathID(in.DeviceID) || validateEnroll(EnrollInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision}) != nil {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	wire, _ := json.Marshal(in)
	err = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := serviceAuthority(ctx, tx, actor, true); e != nil {
			return e
		}
		_, allocationEnabled, e := allocationGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		before, e := loadReplacementTx(ctx, tx, actor, o, r, a)
		if e != nil {
			return e
		}
		var prior struct{ ActorID, OrderID, RequestID, AssignmentID, RequestJSON, ResultJSON string }
		e = tx.Table("device_service_receipts").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.OrderID != o || prior.RequestID != r || prior.AssignmentID != a || prior.RequestJSON != string(wire) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.DeviceID != in.DeviceID || !validPathID(out.AssignmentID) || !validPathID(out.ReturnID) || !validPathID(out.ItemID) {
				return ErrUnavailable
			}
			var count int64
			if tx.Table("device_assignments x").Joins("JOIN devices d ON d.id=x.device_id AND d.deleted_at IS NULL").Joins("JOIN device_assignments old ON old.id=x.replacement_of AND old.deleted_at IS NULL AND old.return_id=?", out.ReturnID).Joins("JOIN returns ret ON ret.id=old.return_id AND ret.deleted_at IS NULL AND ret.order_id=x.order_id AND ret.exchange_id=?", out.ExchangeID).Joins("JOIN order_exchanges ex ON ex.id=ret.exchange_id AND ex.deleted_at IS NULL AND ex.return_id=ret.id").Joins("JOIN order_line_items li ON li.id=x.item_id AND li.deleted_at IS NULL").Where("x.id=? AND x.device_id=? AND x.order_id=? AND x.item_id=? AND x.replacement_of=? AND x.after_sales_request_id=? AND x.deleted_at IS NULL", out.AssignmentID, in.DeviceID, o, out.ItemID, a, r).Count(&count).Error != nil || count != 1 {
				return gorm.ErrRecordNotFound
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		service, originalDevice, e := loadService(ctx, tx, o, r, a)
		if e != nil {
			return e
		}
		if !service.Enabled || !allocationEnabled {
			return ErrUnavailable
		}
		if !before.Eligible || before.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		found := false
		for _, candidate := range before.Devices {
			if candidate.ID == in.DeviceID {
				found = true
			}
		}
		if !found {
			return ErrConflict
		}
		var d struct {
			ID, VariantID, SerialNumber, State, Category, SourceID, LocationID string
			IMEI, IMEI2, EID                                                   *string
		}
		if tx.Table("devices").Where("id=? AND deleted_at IS NULL", in.DeviceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error != nil {
			return gorm.ErrRecordNotFound
		}
		if d.ID == originalDevice.ID || d.State != "available" || d.Category != "sale" || d.VariantID != before.VariantID {
			return ErrConflict
		}
		var productID string
		if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", d.VariantID).Scan(&productID).Error != nil || productID == "" {
			return ErrConflict
		}
		sku, e := intakeSKU(tx, productID, d.VariantID)
		if e != nil {
			return e
		}
		ident := Identity{SerialNumber: d.SerialNumber}
		if d.IMEI != nil {
			ident.IMEI = *d.IMEI
		}
		if d.IMEI2 != nil {
			ident.IMEI2 = *d.IMEI2
		}
		if d.EID != nil {
			ident.EID = *d.EID
		}
		normalized, e := NormalizeIdentity(ident, sku.RequiresRadio)
		if e != nil || normalized != ident {
			return ErrIdentity
		}
		if e = ClaimIdentityTx(ctx, tx, d.ID, ident, sku.RequiresRadio); e != nil {
			return e
		}
		native, e := orders.ApplyDeviceExchangeTx(ctx, tx, orders.DeviceExchangeInput{OrderID: o, OriginalItemID: before.OriginalItemID, LocationID: originalDevice.LocationID, OriginalAssignmentID: a, RequestID: r, ActorID: actor.ID, ExpectedVersion: before.OrderVersion})
		if e != nil {
			if errors.Is(e, orders.ErrVersionConflict) {
				return ErrConflict
			}
			return e
		}
		out = ReplacementResult{AssignmentID: "dasg_" + uuid.NewString(), DeviceID: d.ID, ExchangeID: native.ExchangeID, ReturnID: native.ReturnID, ItemID: native.ItemID, OrderVersion: native.ReceivedVersion, PendingDifference: native.Amounts.Summary.PendingDifference}
		if tx.Exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,replacement_of,after_sales_request_id,created_at,updated_at) VALUES(?,?,?,?,?,'reserved',?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", out.AssignmentID, d.ID, o, out.ItemID, d.SourceID, a, r).Error != nil {
			return ErrUnavailable
		}
		update := tx.Exec("UPDATE device_assignments SET return_id=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND order_id=? AND return_id IS NULL AND status='returned' AND deleted_at IS NULL", out.ReturnID, a, o)
		if update.Error != nil || update.RowsAffected != 1 {
			return ErrConflict
		}
		update = tx.Exec("UPDATE devices SET state='reserved',updated_at=UTC_TIMESTAMP(6) WHERE id=? AND state='available' AND deleted_at IS NULL", d.ID)
		if update.Error != nil || update.RowsAffected != 1 {
			return ErrConflict
		}
		data, _ := json.Marshal(map[string]any{"assignment_id": out.AssignmentID, "replacement_of": a, "after_sales_request_id": r, "exchange_id": out.ExchangeID, "return_id": out.ReturnID, "item_id": out.ItemID})
		if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,'assign',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), d.ID, o, actor.ID, string(data)).Error != nil {
			return ErrUnavailable
		}
		result, _ := json.Marshal(out)
		// Reuse the global device-service UUID receipt namespace: exact request wire
		// distinguishes inspection and replacement and prevents cross-command replay.
		if tx.Table("device_service_receipts").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "order_id": o, "request_id": r, "assignment_id": a, "request_json": string(wire), "result_json": string(result), "created_at": time.Now().UTC()}).Error != nil {
			return ErrUnavailable
		}
		return tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error
	})
	return out, err
}
