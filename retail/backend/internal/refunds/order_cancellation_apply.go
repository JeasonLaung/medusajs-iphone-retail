package refunds

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FinishCancellation never infers provider success from a browser or an HTTP
// timeout. All refund ledger applications must already have committed. Its
// local stock, reason, notice and terminal receipt form one SQL transaction.
func (s Service) FinishCancellation(ctx context.Context, operationID string) error {
	if s.DB == nil || !validUUID(operationID) {
		return ErrInput
	}
	var locator orderCancellation
	if s.DB.WithContext(ctx).Select("order_id").Where("operation_id=?", operationID).Take(&locator).Error != nil {
		return ErrConflict
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := cancellationGate(tx); err != nil {
			return err
		}
		if err := s.scope(tx, locator.OrderID); err != nil {
			return err
		}
		// Match device commands' global revision -> order -> business row order.
		var device struct {
			Version  int
			Revision uint64
		}
		if tx.Table("device_runtime_schema").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&device).Error != nil || device.Version != 1 || device.Revision == 0 {
			return ErrUnavailable
		}
		var order struct {
			ID       string
			Metadata *string
		}
		if tx.Table("orders").Select("id,metadata").Where("id=? AND deleted_at IS NULL", locator.OrderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error != nil {
			return ErrConflict
		}
		var root orderCancellation
		if tx.Where("operation_id=?", operationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&root).Error != nil {
			return ErrUnavailable
		}
		if root.CompletedAt != nil {
			return nil
		}
		if err := cancellationOrder(tx, root.OrderID, root.ExpectedVersion); err != nil {
			return err
		}
		var pending []struct{ OperationID string }
		if tx.Model(&operation{}).Select("operation_id").Where("cancellation_id=? AND applied=FALSE AND canceled_at IS NULL AND state NOT IN ('failed','canceled')", operationID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&pending).Error != nil {
			return ErrUnavailable
		}
		if len(pending) != 0 {
			return nil
		}
		var paymentIDs []string
		if tx.Table("payments p").Select("p.id").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id AND l.deleted_at IS NULL").Where("l.order_id=? AND p.deleted_at IS NULL", root.OrderID).Order("p.id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&paymentIDs).Error != nil || len(paymentIDs) > 100 {
			return ErrUnavailable
		}
		for _, id := range paymentIDs {
			balance, err := paymentBalance(tx, id)
			if err != nil {
				return err
			}
			if balance.Sign() != 0 {
				return ErrConflict
			}
		}
		nextVersion, err := orders.AdvanceCancellationVersionTx(ctx, tx, root.OrderID, root.ExpectedVersion, root.OperationID)
		if err != nil {
			return err
		}
		var applied []struct{ Amount string }
		if tx.Model(&operation{}).Select("amount").Where("cancellation_id=? AND applied=TRUE", root.OperationID).Limit(1001).Find(&applied).Error != nil || len(applied) > 1000 {
			return ErrUnavailable
		}
		values := []totals.Number{}
		for _, refund := range applied {
			values = append(values, totals.Number{Decimal: refund.Amount})
		}
		credit, err := totals.Sum(values...)
		if err != nil {
			return ErrUnavailable
		}
		scalar, raw, err := sqlAmount(credit)
		if err != nil {
			return err
		}
		if tx.Table("order_credit_lines").Create(map[string]any{"id": "ordcl_cancel_" + root.OperationID, "order_id": root.OrderID, "version": nextVersion, "reference": "order_cancellation", "reference_id": root.OperationID, "amount": scalar, "raw_amount": raw, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrUnavailable
		}
		if _, err := orders.RefreshChangeSummaryTx(ctx, tx, root.OrderID, nextVersion); err != nil {
			return err
		}
		if err := releaseCancellationStock(ctx, tx, root); err != nil {
			return err
		}
		var now time.Time
		if tx.Raw("SELECT UTC_TIMESTAMP(6)").Scan(&now).Error != nil {
			return ErrUnavailable
		}
		metadata, err := cancellationMetadata(order.Metadata, root, now)
		if err != nil {
			return err
		}
		if tx.Exec("UPDATE payment_collections p JOIN order_payment_collections l ON l.payment_collection_id=p.id AND l.deleted_at IS NULL SET p.status='canceled',p.updated_at=? WHERE l.order_id=? AND p.deleted_at IS NULL", now, root.OrderID).Error != nil {
			return ErrUnavailable
		}
		changed := tx.Table("orders").Where("id=? AND version=? AND canceled_at IS NULL", root.OrderID, nextVersion).Updates(map[string]any{"status": "canceled", "canceled_at": now, "metadata": metadata, "updated_at": now})
		if changed.Error != nil || changed.RowsAffected != 1 {
			return ErrConflict
		}
		if err := customernotices.EmitOrderTx(tx, root.OrderID, "order.canceled", root.OrderID); err != nil {
			return err
		}
		if tx.Model(&orderCancellation{}).Where("operation_id=? AND completed_at IS NULL", operationID).Updates(map[string]any{"completed_at": now, "updated_at": now}).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
}

func releaseCancellationStock(ctx context.Context, tx *gorm.DB, root orderCancellation) error {
	var bindings []inventory.ReservationBinding
	if tx.Table("inventory_reservations r").Select("r.id,r.inventory_item_id,r.location_id,r.line_item_id").Where("r.deleted_at IS NULL AND EXISTS(SELECT 1 FROM order_items i WHERE i.order_id=? AND i.item_id=r.line_item_id AND i.deleted_at IS NULL)", root.OrderID).Order("r.inventory_item_id,r.location_id,r.id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1001).Find(&bindings).Error != nil || len(bindings) > 1000 {
		return ErrUnavailable
	}
	if len(bindings) > 0 {
		sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
		if !ok {
			return ErrUnavailable
		}
		if _, err := inventory.ApplyReservations(ctx, sqlTx, inventory.ReservationCommand{OperationID: root.OperationID, OwnerID: root.OrderID, ActorRef: root.ActorID, Kind: "release", Release: bindings}); err != nil {
			return err
		}
	}
	assignments, err := cancellationAssignments(tx, root.OrderID)
	if err != nil {
		return err
	}
	for _, a := range assignments {
		if a.Status != "reserved" || a.FulfillmentID != nil {
			return ErrConflict
		}
		var device struct{ State, Category, SourceID string }
		if tx.Table("devices").Select("state,category,source_id").Where("id=? AND deleted_at IS NULL", a.DeviceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&device).Error != nil || device.State != "reserved" || device.Category != "sale" || device.SourceID != a.SourceID {
			return ErrConflict
		}
		var other []struct{ ID string }
		if tx.Table("device_assignments").Select("id").Where("device_id=? AND id<>? AND status IN ('reserved','packed','shipped') AND deleted_at IS NULL", a.DeviceID, a.ID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&other).Error != nil {
			return ErrUnavailable
		}
		if len(other) != 0 {
			return ErrConflict
		}
		if tx.Table("device_assignments").Where("id=?", a.ID).Updates(map[string]any{"status": "canceled", "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("devices").Where("id=?", a.DeviceID).Updates(map[string]any{"state": "available", "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrUnavailable
		}
		data, _ := json.Marshal(map[string]string{"assignment_id": a.ID, "item_id": a.ItemID, "cancellation_id": root.OperationID})
		if tx.Table("device_events").Create(map[string]any{"id": "devent_" + uuid.NewString(), "device_id": a.DeviceID, "order_id": root.OrderID, "actor_id": root.ActorID, "action": "release", "data": string(data), "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrUnavailable
		}
	}
	if len(assignments) > 0 && tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
		return ErrUnavailable
	}
	return nil
}

// RunCancellationsDue also selects zero-balance orders, whose cancellation has
// no refund child to wake a worker. SQL serializes concurrent finalizers.
func (s Service) RunCancellationsDue(ctx context.Context, cursor string) (WorkResult, string, error) {
	var out WorkResult
	if s.DB == nil || !idPattern.MatchString(s.StoreID) {
		return out, cursor, ErrUnavailable
	}
	var ids []string
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var enabled bool
		if tx.Raw("SELECT enabled FROM order_cancellation_runtime WHERE id=1").Scan(&enabled).Error != nil {
			return ErrUnavailable
		}
		if !enabled {
			return nil
		}
		if err := cancellationGate(tx); err != nil {
			return err
		}
		return tx.Table("order_cancellations r").Select("r.operation_id").Joins("JOIN cart_completion_receipts c ON c.order_id=r.order_id").Joins("JOIN cart_ownership o ON o.cart_id=c.cart_id").Where("o.store_id=? AND r.completed_at IS NULL AND r.operation_id>?", s.StoreID, cursor).Order("r.operation_id").Limit(10).Scan(&ids).Error
	})
	if err != nil {
		return out, cursor, err
	}
	if len(ids) == 0 {
		return out, "", nil
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return out, cursor, err
		}
		out.Selected++
		cursor = id
		if err := s.FinishCancellation(ctx, id); err != nil {
			out.Failed++
		} else {
			out.Processed++
		}
	}
	return out, cursor, nil
}
