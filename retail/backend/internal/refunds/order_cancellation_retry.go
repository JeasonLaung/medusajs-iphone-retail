package refunds

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"

	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RetryCancellationRefund is an explicit new instruction after a definitive
// provider failure/cancellation. An uncertain instruction can only be observed;
// neither refreshing the page nor replaying the root generates another POST.
func (s Service) RetryCancellationRefund(ctx context.Context, actor identity.User, order, previous, next string) (CancellationView, error) {
	var out CancellationView
	if !idPattern.MatchString(order) || !validUUID(previous) || !validUUID(next) || previous == next {
		return out, ErrInput
	}
	if s.DB == nil || s.Ring == nil || s.Adapters == nil {
		return out, ErrUnavailable
	}
	wire, _ := json.Marshal([]string{order, previous, next})
	sum := sha256.Sum256(wire)
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := cancellationAuthority(ctx, tx, actor); err != nil {
			return err
		}
		if err := cancellationGate(tx); err != nil {
			return err
		}
		if err := s.scope(tx, order); err != nil {
			return err
		}
		var o struct{ ID string }
		if tx.Table("orders").Select("id").Where("id=? AND deleted_at IS NULL", order).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&o).Error != nil {
			return ErrConflict
		}
		var old operation
		if tx.Where("operation_id=? AND order_id=? AND cancellation_id IS NOT NULL", previous, order).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&old).Error != nil {
			return ErrConflict
		}
		var replay operation
		err := tx.Where("operation_id=?", next).Take(&replay).Error
		if err == nil {
			if replay.ActorID != actor.ID || replay.OrderID != order || replay.PreviousOperationID == nil || *replay.PreviousOperationID != previous || subtle.ConstantTimeCompare(replay.InputSHA256, sum[:]) != 1 {
				return ErrConflict
			}
			out, err = cancellationView(tx, order, true)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if old.Applied || old.CanceledAt != nil || old.State != "failed" && old.State != "canceled" {
			return ErrConflict
		}
		old.ActorID = actor.ID
		// Recheck the current operator, while preserving the root's original
		// actor for its cancellation reason and audit. Refund dispatch grants
		// belong to this new refund instruction's current actor.
		var root orderCancellation
		if tx.Where("operation_id=? AND completed_at IS NULL", *old.CancellationID).Take(&root).Error != nil {
			return ErrConflict
		}
		if err := cancellationOrder(tx, order, root.ExpectedVersion); err != nil {
			return err
		}
		var children []struct{ OperationID string }
		if tx.Model(&operation{}).Select("operation_id").Where("previous_operation_id=?", previous).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&children).Error != nil {
			return ErrUnavailable
		}
		if len(children) != 0 {
			return ErrConflict
		}
		if err := reserve(tx, old.PaymentID, old.Amount); err != nil {
			return err
		}
		request, err := s.request(ctx, tx, old)
		clear(request.Session.Data)
		clear(request.Session.Context)
		if err != nil {
			return err
		}
		if tx.Model(&operation{}).Create(map[string]any{"operation_id": next, "previous_operation_id": previous, "cancellation_id": old.CancellationID, "request_id": nil, "actor_id": actor.ID, "order_id": order, "payment_id": old.PaymentID, "collection_id": old.CollectionID, "amount": old.Amount, "currency_code": old.CurrencyCode, "input_sha256": sum[:], "scope_id": old.ScopeID, "checkout_operation_id": old.CheckoutOperationID, "session_id": old.SessionID, "request_sha256": old.RequestSHA256, "provider_id": old.ProviderID, "remote_payment_id": old.RemotePaymentID, "state": "pending", "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrConflict
		}
		out, err = cancellationView(tx, order, true)
		return err
	})
	return out, err
}
