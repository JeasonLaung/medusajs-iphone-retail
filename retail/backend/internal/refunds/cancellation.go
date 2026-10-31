package refunds

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CancelInput is a new, audited local instruction. It never invokes a provider
// cancellation endpoint. A dispatched refund must instead finish reconciliation.
type CancelInput struct {
	OperationID       string `json:"operation_id"`
	RefundOperationID string `json:"refund_operation_id"`
}
type cancellation struct{ OperationID, RefundOperationID, ActorID, OrderID, RequestID string }

func (s Service) Cancel(ctx context.Context, actor identity.User, order, request string, in CancelInput) (Result, error) {
	var out Result
	if !validUUID(in.OperationID) || !validUUID(in.RefundOperationID) || !idPattern.MatchString(order) || !idPattern.MatchString(request) {
		return out, ErrInput
	}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authority(ctx, tx, actor); e != nil {
			return e
		}
		if e := gate(tx); e != nil {
			return e
		}
		if e := s.scope(tx, order); e != nil {
			return e
		}
		var o struct{ ID string }
		if tx.Table("orders").Select("id").Where("id=? AND deleted_at IS NULL", order).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&o).Error != nil {
			return ErrConflict
		}
		var row operation
		if tx.Where("operation_id=? AND order_id=? AND request_id=?", in.RefundOperationID, order, request).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
			return ErrConflict
		}
		var prior cancellation
		e := tx.Table("order_refund_cancellations").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.OrderID != order || prior.RequestID != request || prior.RefundOperationID != in.RefundOperationID || row.CanceledAt == nil {
				return ErrConflict
			}
			out = result(row)
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if row.DispatchedAt != nil || row.CanceledAt != nil || row.State != "pending" || row.Applied || row.Observation != nil {
			return ErrConflict
		}
		now := gorm.Expr("UTC_TIMESTAMP(6)")
		r := tx.Model(&operation{}).Where("operation_id=? AND dispatched_at IS NULL AND canceled_at IS NULL", row.OperationID).Updates(map[string]any{"canceled_at": now, "canceled_by": actor.ID, "updated_at": now})
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrUnavailable
		}
		if tx.Table("order_refund_cancellations").Create(map[string]any{"operation_id": in.OperationID, "refund_operation_id": in.RefundOperationID, "actor_id": actor.ID, "order_id": order, "request_id": request, "created_at": now}).Error != nil {
			return ErrUnavailable
		}
		if tx.Where("operation_id=?", row.OperationID).Take(&row).Error != nil {
			return ErrUnavailable
		}
		out = result(row)
		return nil
	})
	return out, err
}
