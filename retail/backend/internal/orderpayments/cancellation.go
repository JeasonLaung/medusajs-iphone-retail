package orderpayments

import (
	"context"
	"errors"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CancelInput struct {
	OperationID        string `json:"operation_id"`
	PaymentOperationID string `json:"payment_operation_id"`
}
type cancellation struct {
	OperationID, PaymentOperationID, AccountID string
	AuthVersion                                uint64
}

func (s Service) private(ctx context.Context, tx *gorm.DB, p Principal, order string) (string, error) {
	if !txReady(tx) || s.Ring == nil {
		return "", ErrUnavailable
	}
	if e := authority(tx, p); e != nil {
		return "", e
	}
	if e := runtime(tx, false); e != nil {
		return "", e
	}
	cart, e := s.scope(tx, p, order)
	if e != nil {
		return "", e
	}
	if e = lockOrder(tx, order); e != nil {
		return "", e
	}
	if orders.Ready(ctx, tx) != nil {
		return "", ErrUnavailable
	}
	return cart, nil
}
func (s Service) ownedOperation(ctx context.Context, tx *gorm.DB, p Principal, order, cart, id string) (operation, error) {
	var row operation
	e := tx.Where("operation_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return row, ErrDenied
	}
	if e != nil {
		return row, ErrUnavailable
	}
	if row.AccountID != p.AccountID || row.StoreID != s.StoreID || row.OrderID != order || row.CartID != cart {
		return operation{}, ErrDenied
	}
	if _, e = s.request(ctx, tx, row); e != nil {
		return operation{}, e
	}
	return row, nil
}

// Read returns only an owned local receipt, including after auth-version change.
// It does not query the provider or imply that a pending session received cash.
func (s Service) Read(ctx context.Context, p Principal, order, id string) (Result, error) {
	var out Result
	if !idPattern.MatchString(order) || !validUUID(id) {
		return out, ErrInput
	}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cart, e := s.private(ctx, tx, p, order)
		if e != nil {
			return e
		}
		row, e := s.ownedOperation(ctx, tx, p, order, cart, id)
		if e != nil {
			return e
		}
		out = result(row)
		return nil
	})
	return out, e
}

// Cancel writes a separate audited local instruction. It remains usable with the
// optional feature disabled, but never cancels an already-dispatched provider
// payment. An external uncertainty must be reconciled rather than recharged.
func (s Service) Cancel(ctx context.Context, p Principal, order string, in CancelInput) (Result, error) {
	var out Result
	if !idPattern.MatchString(order) || !validUUID(in.OperationID) || !validUUID(in.PaymentOperationID) || in.OperationID == in.PaymentOperationID {
		return out, ErrInput
	}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cart, e := s.private(ctx, tx, p, order)
		if e != nil {
			return e
		}
		row, e := s.ownedOperation(ctx, tx, p, order, cart, in.PaymentOperationID)
		if e != nil {
			return e
		}
		var receipt cancellation
		e = tx.Table("order_payment_cancellations").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&receipt).Error
		if e == nil {
			if receipt.PaymentOperationID != row.OperationID || receipt.AccountID != p.AccountID || row.State != "canceled" || row.DispatchedAt != nil {
				return ErrConflict
			}
			out = result(row)
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if row.State != "pending" || row.DispatchedAt != nil || row.Applied {
			return ErrConflict
		}
		effect, e := payments.InspectEffect(ctx, tx, key(row))
		if e != nil {
			return e
		}
		if effect.State != "pending" || effect.Attempts != 0 || effect.AttemptID != "" {
			return ErrConflict
		}
		now := gorm.Expr("UTC_TIMESTAMP(6)")
		r := tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND status='pending' AND deleted_at IS NULL", row.SessionID, row.CollectionID).Updates(map[string]any{"status": "canceled", "updated_at": now})
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		r = tx.Model(&operation{}).Where("operation_id=? AND state='pending' AND dispatched_at IS NULL AND applied=FALSE", row.OperationID).Updates(map[string]any{"state": "canceled", "updated_at": now})
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrUnavailable
		}
		if tx.Table("order_payment_cancellations").Create(map[string]any{"operation_id": in.OperationID, "payment_operation_id": row.OperationID, "account_id": p.AccountID, "auth_version": p.AuthVersion, "created_at": now}).Error != nil {
			return ErrUnavailable
		}
		row.State = "canceled"
		out = result(row)
		return nil
	})
	return out, e
}
