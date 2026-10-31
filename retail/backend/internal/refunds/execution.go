package refunds

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"time"
)

// Run is an internal command/worker entry, never browser-supplied evidence.
// Only a committed initial claim may POST. Every later claim is GET-only.
// The first dispatch rechecks current actor grants. Recovery of an already
// dispatched request remains possible after revocation.
func (s Service) Run(ctx context.Context, id string) (Result, error) {
	var out Result
	if s.DB == nil || s.Ring == nil || s.Adapters == nil {
		return out, ErrUnavailable
	}
	var locator operation
	if s.DB.WithContext(ctx).Select("order_id,actor_id,dispatched_at,canceled_at").Where("operation_id=?", id).Take(&locator).Error != nil {
		return out, ErrConflict
	}
	var row operation
	var req payments.RefundRequest
	defer func() { clear(req.Session.Data); clear(req.Session.Context) }()
	post, call := false, false
	claim := uuid.NewString()
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := gate(tx); e != nil {
			return e
		}
		if locator.DispatchedAt == nil && locator.CanceledAt == nil {
			var actor identity.User
			if tx.Where("id=?", locator.ActorID).Take(&actor).Error != nil {
				return ErrUnavailable
			}
			if e := authority(ctx, tx, actor); e != nil {
				return e
			}
		}
		if e := s.scope(tx, locator.OrderID); e != nil {
			return e
		}
		var order struct{ ID string }
		if tx.Table("orders").Select("id").Where("id=?", locator.OrderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error != nil {
			return ErrUnavailable
		}
		if tx.Where("operation_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
			return ErrUnavailable
		}
		out = result(row)
		if row.CanceledAt != nil || row.Applied || row.State == "failed" || row.State == "canceled" {
			return nil
		}
		if row.State == "refunded" {
			return s.apply(ctx, tx, &row)
		}
		var now time.Time
		if tx.Raw("SELECT UTC_TIMESTAMP(6)").Scan(&now).Error != nil {
			return ErrUnavailable
		}
		if row.LeaseUntil != nil && now.Before(*row.LeaseUntil) {
			return nil
		}
		var e error
		req, e = s.request(ctx, tx, row)
		if e != nil {
			return e
		}
		if _, e = s.Adapters.Refunder(row.ProviderID); e != nil {
			return ErrUnavailable
		}
		post = row.DispatchedAt == nil
		if post {
			if e := orders.EnsureNoPendingPaymentTx(tx, row.OrderID); e != nil {
				if errors.Is(e, orders.ErrVersionConflict) {
					return ErrConflict
				}
				return ErrUnavailable
			}
			if e := returned(tx, row.OrderID, row.RequestID); e != nil {
				return e
			}
			if e := exchangeAmount(tx, row); e != nil {
				return e
			}
		}
		updates := map[string]any{"claim_id": claim, "lease_until": now.Add(45 * time.Second), "updated_at": now}
		if post {
			updates["dispatched_at"] = now
			updates["state"] = "uncertain"
		}
		if tx.Model(&operation{}).Where("operation_id=?", id).Updates(updates).Error != nil {
			return ErrUnavailable
		}
		call = true
		return nil
	})
	if e != nil {
		return Result{}, e
	}
	if !call {
		if row.State == "refunded" {
			out = result(row)
		}
		return out, nil
	}
	adapter, e := s.Adapters.Refunder(row.ProviderID)
	if e != nil {
		return out, ErrUnavailable
	}
	observed, e := invoke(ctx, adapter, req, row.RemoteRefundID, post)
	if e != nil {
		return Result{OperationID: row.OperationID, OrderID: row.OrderID, RequestID: row.RequestID, PaymentID: row.PaymentID, Amount: row.Amount, Currency: row.CurrencyCode, State: "uncertain"}, payments.ErrRefundExecution
	}
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := gate(tx); e != nil {
			return e
		}
		var current operation
		if tx.Where("operation_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error != nil {
			return ErrUnavailable
		}
		if current.ClaimID == nil || *current.ClaimID != claim {
			return ErrConflict
		}
		if !matches(current, observed) {
			return ErrConflict
		}
		raw, _ := json.Marshal(observed)
		if tx.Model(&operation{}).Where("operation_id=?", id).Updates(map[string]any{"state": observed.Status, "remote_refund_id": observed.RefundID, "observation": string(raw), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrUnavailable
		}
		current.State = observed.Status
		current.RemoteRefundID = &observed.RefundID
		wire := string(raw)
		current.Observation = &wire
		row = current
		out = result(current)
		return nil
	})
	if e != nil {
		return Result{}, e
	}
	// Observation commits independently, so a local late failure can be retried
	// without either a new POST or a successful second network request.
	if row.State == "refunded" {
		e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := gate(tx); e != nil {
				return e
			}
			var o struct{ ID string }
			if tx.Table("orders").Select("id").Where("id=?", row.OrderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&o).Error != nil {
				return ErrUnavailable
			}
			if tx.Where("operation_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
				return ErrUnavailable
			}
			if e := s.apply(ctx, tx, &row); e != nil {
				return e
			}
			out = result(row)
			return nil
		})
	}
	return out, e
}
func invoke(ctx context.Context, a payments.RefundAdapter, in payments.RefundRequest, remote *string, post bool) (out payments.RefundObservation, err error) {
	defer func() {
		if recover() != nil {
			out = payments.RefundObservation{}
			err = payments.ErrRefundExecution
		}
	}()
	if post {
		return a.CreateRefund(ctx, in)
	}
	if remote != nil {
		return a.ObserveRefund(ctx, in, *remote)
	}
	return a.DiscoverRefund(ctx, in)
}
func matches(r operation, o payments.RefundObservation) bool {
	if o.RefundID == "" || o.PaymentID != r.RemotePaymentID || o.Currency != r.CurrencyCode || o.CreatedAt.IsZero() || r.RemoteRefundID != nil && *r.RemoteRefundID != o.RefundID {
		return false
	}
	a, ok := new(big.Rat).SetString(o.Amount)
	b, yes := new(big.Rat).SetString(r.Amount)
	if !ok || !yes || a.Cmp(b) != 0 {
		return false
	}
	switch o.Status {
	case "pending", "queued", "processing", "refunded", "failed", "canceled":
		return true
	}
	return false
}
func sqlAmount(s string) (string, string, error) {
	n, e := totals.StoredNumber(s, nil)
	if e != nil {
		return "", "", ErrUnavailable
	}
	v, e := totals.Resolve(n)
	if e != nil {
		return "", "", ErrUnavailable
	}
	raw, e := json.Marshal(totals.RawValue{Value: v, Precision: 100})
	if e != nil {
		return "", "", ErrUnavailable
	}
	return v, string(raw), nil
}
func (s Service) apply(ctx context.Context, tx *gorm.DB, r *operation) error {
	if r.Applied {
		return nil
	}
	if r.State != "refunded" || r.Observation == nil {
		return ErrConflict
	}
	var o payments.RefundObservation
	if json.Unmarshal([]byte(*r.Observation), &o) != nil || !matches(*r, o) || o.Status != "refunded" {
		return ErrUnavailable
	}
	var payment struct{ ID string }
	if tx.Table("payments").Select("id").Where("id=? AND deleted_at IS NULL", r.PaymentID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&payment).Error != nil {
		return ErrUnavailable
	}
	var collection struct{ ID string }
	if tx.Table("payment_collections").Select("id").Where("id=? AND deleted_at IS NULL", r.CollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&collection).Error != nil {
		return ErrUnavailable
	}
	scalar, raw, e := sqlAmount(r.Amount)
	if e != nil {
		return e
	}
	negative, negativeRaw, e := sqlAmount("-" + r.Amount)
	if e != nil {
		return e
	}
	refundID := "ref_" + r.OperationID
	now := gorm.Expr("UTC_TIMESTAMP(6)")
	if tx.Table("payment_refunds").Create(map[string]any{"id": refundID, "payment_id": r.PaymentID, "amount": scalar, "raw_amount": raw, "note": "after-sales:" + r.RequestID, "created_by": r.ActorID, "created_at": now, "updated_at": now}).Error != nil {
		return ErrUnavailable
	}
	var ord struct{ Version int }
	if tx.Table("orders").Select("version").Where("id=? AND deleted_at IS NULL", r.OrderID).Take(&ord).Error != nil {
		return ErrUnavailable
	}
	if tx.Table("order_transactions").Create(map[string]any{"id": "ordtr_ref_" + r.OperationID, "order_id": r.OrderID, "version": ord.Version, "amount": negative, "raw_amount": negativeRaw, "currency_code": r.CurrencyCode, "reference": "refund", "reference_id": refundID, "created_at": now, "updated_at": now}).Error != nil {
		return ErrUnavailable
	}
	var movements []struct{ Amount, RawAmount string }
	if tx.Table("payment_refunds f").Select("CAST(f.amount AS CHAR) AS amount,f.raw_amount").Joins("JOIN payments p ON p.id=f.payment_id AND p.deleted_at IS NULL").Where("p.payment_collection_id=? AND f.deleted_at IS NULL", r.CollectionID).Limit(10001).Find(&movements).Error != nil || len(movements) > 10000 {
		return ErrUnavailable
	}
	values := []totals.Number{}
	for _, m := range movements {
		v, e := exact(m.Amount, m.RawAmount)
		if e != nil {
			return e
		}
		values = append(values, totals.Number{Decimal: v})
	}
	total, e := totals.Sum(values...)
	if e != nil {
		return ErrUnavailable
	}
	scalar, raw, e = sqlAmount(total)
	if e != nil {
		return e
	}
	if tx.Table("payment_collections").Where("id=?", r.CollectionID).Updates(map[string]any{"refunded_amount": scalar, "raw_refunded_amount": raw, "updated_at": now}).Error != nil {
		return ErrUnavailable
	}
	if _, e := orders.RefreshChangeSummaryTx(ctx, tx, r.OrderID, ord.Version); e != nil {
		return e
	}
	if e := customernotices.EmitOrderTx(tx, r.OrderID, "refund.created", refundID); e != nil {
		return e
	}
	if tx.Model(&operation{}).Where("operation_id=? AND applied=FALSE", r.OperationID).Updates(map[string]any{"applied": true, "updated_at": now}).Error != nil {
		return ErrUnavailable
	}
	r.Applied = true
	return nil
}
