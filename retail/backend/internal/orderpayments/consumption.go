package orderpayments

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"time"

	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Consume accepts only an internal immutable notification attempt. The inbox
// worker has already performed the merchant GET and encrypted its result. There
// is no customer-supplied status/remote ID, network call or new checkout here.
// Cash, collection, session, summary, applied and observation consumption commit
// together. Failed consumption leaves the original paid evidence recoverable.
func (s Service) Consume(ctx context.Context, attempt string) (Result, error) {
	if !validUUID(attempt) {
		return Result{}, ErrInput
	}
	if s.DB == nil || s.DB.Statement == nil || txReady(s.DB) || s.Ring == nil || !idPattern.MatchString(s.StoreID) {
		return Result{}, ErrUnavailable
	}
	var locator struct{ ScopeID, OperationID string }
	if s.DB.WithContext(ctx).Table("mollie_notification_observations n").Select("r.scope_id,r.operation_id").Joins("JOIN mollie_payment_references r ON r.provider_id=n.provider_id AND r.remote_id=n.remote_id").Where("n.attempt_id=?", attempt).Take(&locator).Error != nil {
		return Result{}, ErrConflict
	}
	var out Result
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := runtime(tx, false); e != nil {
			return e
		}
		if e := orders.Ready(ctx, tx); e != nil {
			return ErrUnavailable
		}
		if e := lockRecoveryOrder(tx, locator.ScopeID); e != nil {
			return e
		}
		var row operation
		if tx.Where("operation_id=? AND order_id=?", locator.OperationID, locator.ScopeID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
			return ErrConflict
		}
		if row.StoreID != s.StoreID || row.DispatchedAt == nil {
			return ErrConflict
		}
		req, e := s.request(ctx, tx, row)
		if e != nil {
			return e
		}
		defer clear(req.Data)
		defer clear(req.Context)
		created, e := payments.ReadSessionCreation(ctx, tx, key(row), s.Ring)
		if e != nil {
			return e
		}
		var notification struct {
			ProviderID, RemoteID string
			ConsumedAt           *time.Time
		}
		if tx.Table("mollie_notification_observations").Where("attempt_id=?", attempt).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&notification).Error != nil {
			return ErrUnavailable
		}
		var reference struct {
			ScopeID, OperationID, EffectKind, ResourceID string
			RequestSHA256                                []byte
		}
		if tx.Table("mollie_payment_references").Where("provider_id=? AND remote_id=?", notification.ProviderID, notification.RemoteID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&reference).Error != nil {
			return ErrUnavailable
		}
		if notification.ProviderID != row.ProviderID || reference.ScopeID != row.OrderID || reference.OperationID != row.OperationID || reference.EffectKind != "create_session" || reference.ResourceID != row.SessionID || subtle.ConstantTimeCompare(reference.RequestSHA256, row.RequestSHA256) != 1 {
			return ErrConflict
		}
		observed, e := payments.ReadMollieNotificationObservation(ctx, tx, attempt, s.Ring)
		if e != nil {
			return e
		}
		defer clear(observed.Session.Data)
		if !sameRemote(created, observed.Session) || payments.ValidateMollieObservation(req, notification.RemoteID, observed) != nil {
			return ErrConflict
		}
		if row.Applied {
			if row.State != "paid" {
				return ErrConflict
			}
		} else if notification.ConsumedAt == nil {
			if terminal(row.State) {
				// Contradictory late cash requires review, never silently reuse the
				// collection or overwrite an explicit successor with a second charge.
				if observed.ProviderStatus == "paid" {
					return ErrConflict
				}
			} else if observed.ProviderStatus == "paid" {
				if e = s.applyPaid(ctx, tx, &row, req, observed); e != nil {
					return e
				}
			} else {
				if e = s.mergeUnpaid(tx, &row, req, observed); e != nil {
					return e
				}
			}
		}
		if notification.ConsumedAt == nil {
			r := tx.Table("mollie_notification_observations").Where("attempt_id=? AND consumed_at IS NULL", attempt).Update("consumed_at", gorm.Expr("UTC_TIMESTAMP(6)"))
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrUnavailable
			}
		}
		out = result(row)
		return nil
	})
	return out, e
}

func (s Service) mergeUnpaid(tx *gorm.DB, r *operation, req payments.SessionRequest, o payments.SessionObservation) error {
	state := map[string]string{"open": "open", "pending": "open", "authorized": "authorized", "failed": "failed", "expired": "expired", "canceled": "canceled"}[o.ProviderStatus]
	if state == "" {
		return ErrConflict
	}
	if r.State == "authorized" && state == "open" {
		return nil
	}
	if e := s.sessionObserved(tx, *r, req, o.Session); e != nil {
		return e
	}
	if tx.Model(&operation{}).Where("operation_id=? AND applied=FALSE", r.OperationID).Updates(map[string]any{"state": state, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
		return ErrUnavailable
	}
	r.State = state
	return nil
}

func (s Service) sessionObserved(tx *gorm.DB, r operation, req payments.SessionRequest, created payments.CreatedSession) error {
	req.Data = created.Data
	data, context, e := payments.SealSessionFields(s.Ring, req)
	if e != nil {
		return e
	}
	updates := map[string]any{"status": created.Status, "data": data, "context": context, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}
	if created.Status == "authorized" {
		updates["authorized_at"] = gorm.Expr("UTC_TIMESTAMP(6)")
	}
	updated := tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND provider_id=? AND currency_code=? AND deleted_at IS NULL AND status IN('pending','requires_more','pending_authorization','authorized')", r.SessionID, r.CollectionID, r.ProviderID, r.CurrencyCode).Updates(updates)
	if updated.Error != nil || updated.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}

func storedAmount(value string) (string, string, error) {
	n, e := totals.Resolve(totals.Number{Decimal: value})
	if e != nil {
		return "", "", ErrUnavailable
	}
	raw, e := json.Marshal(totals.RawValue{Value: n, Precision: 100})
	if e != nil {
		return "", "", ErrUnavailable
	}
	return n, string(raw), nil
}

func (s Service) applyPaid(ctx context.Context, tx *gorm.DB, r *operation, req payments.SessionRequest, o payments.SessionObservation) error {
	var ord struct {
		Version      int
		CurrencyCode string
	}
	if tx.Table("orders").Select("version,currency_code").Where("id=? AND deleted_at IS NULL", r.OrderID).Take(&ord).Error != nil || ord.Version != r.OrderVersion || ord.CurrencyCode != r.CurrencyCode {
		return ErrConflict
	}
	// Version changes are normally frozen. If out-of-band data nevertheless
	// changed the whole due, retain paid evidence for review rather than silently
	// changing this immutable instruction's amount or inventing an adjustment.
	balance, e := orders.CurrentPendingDifferenceTx(tx, r.OrderID)
	want, ok := new(big.Rat).SetString(r.Amount)
	if e != nil || !ok || balance.Cmp(want) != 0 {
		return ErrConflict
	}
	var links []struct{ OrderID string }
	if tx.Table("order_payment_collections").Select("order_id").Where("payment_collection_id=? AND deleted_at IS NULL", r.CollectionID).Limit(2).Clauses(clause.Locking{Strength: "SHARE"}).Find(&links).Error != nil || len(links) != 1 || links[0].OrderID != r.OrderID {
		return ErrConflict
	}
	var collection struct {
		Amount, RawAmount, CurrencyCode, Status                                                                     string
		AuthorizedAmount, RawAuthorizedAmount, CapturedAmount, RawCapturedAmount, RefundedAmount, RawRefundedAmount *string
		CompletedAt                                                                                                 *time.Time
	}
	if tx.Table("payment_collections").Select("CAST(amount AS CHAR) AS amount,raw_amount,currency_code,status,authorized_amount,raw_authorized_amount,captured_amount,raw_captured_amount,refunded_amount,raw_refunded_amount,completed_at").Where("id=? AND deleted_at IS NULL", r.CollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&collection).Error != nil {
		return ErrUnavailable
	}
	if collection.CurrencyCode != r.CurrencyCode || collection.CompletedAt != nil || (collection.Status != "not_paid" && collection.Status != "awaiting") {
		return ErrConflict
	}
	n, e := totals.StoredNumber(collection.Amount, &collection.RawAmount)
	if e != nil {
		return ErrConflict
	}
	v, e := totals.Resolve(n)
	if e != nil || v != req.Amount {
		return ErrConflict
	}
	for _, pair := range [][2]*string{{collection.AuthorizedAmount, collection.RawAuthorizedAmount}, {collection.CapturedAmount, collection.RawCapturedAmount}, {collection.RefundedAmount, collection.RawRefundedAmount}} {
		if pair[0] == nil {
			if pair[1] != nil {
				return ErrConflict
			}
			continue
		}
		n, e := totals.StoredNumber(*pair[0], pair[1])
		if e != nil {
			return ErrConflict
		}
		v, e := totals.Resolve(n)
		if e != nil || v != "0" {
			return ErrConflict
		}
	}
	for _, table := range []string{"payments", "cart_payment_collections"} {
		var existing []struct{ ID string }
		if tx.Table(table).Select("id").Where("payment_collection_id=?", r.CollectionID).Limit(1).Clauses(clause.Locking{Strength: "SHARE"}).Find(&existing).Error != nil {
			return ErrUnavailable
		}
		if len(existing) != 0 {
			return ErrConflict
		}
	}
	if e = s.sessionObserved(tx, *r, req, o.Session); e != nil {
		return e
	}
	scalar, raw, e := storedAmount(r.Amount)
	if e != nil {
		return e
	}
	zero, zeroRaw, e := storedAmount("0")
	if e != nil {
		return e
	}
	payID, capID := "pay_supp_"+r.OperationID, "cap_supp_"+r.OperationID
	envelope, e := s.Ring.Seal(protected.Context{Purpose: "order-payment-v1", Table: "payments", Column: "data", Row: payID}, o.Session.Data)
	if e != nil {
		return e
	}
	defer clear(envelope)
	now := gorm.Expr("UTC_TIMESTAMP(6)")
	if tx.Table("payments").Create(map[string]any{"id": payID, "amount": scalar, "raw_amount": raw, "currency_code": r.CurrencyCode, "provider_id": r.ProviderID, "data": "ers1." + base64.StdEncoding.EncodeToString(envelope), "payment_collection_id": r.CollectionID, "payment_session_id": r.SessionID, "captured_at": now, "created_at": now, "updated_at": now}).Error != nil {
		return ErrUnavailable
	}
	if tx.Table("payment_captures").Create(map[string]any{"id": capID, "payment_id": payID, "amount": scalar, "raw_amount": raw, "created_by": "order-verified-paid", "created_at": now, "updated_at": now}).Error != nil {
		return ErrUnavailable
	}
	if tx.Table("order_transactions").Create(map[string]any{"id": "ordtr_supp_" + r.OperationID, "order_id": r.OrderID, "version": ord.Version, "amount": scalar, "raw_amount": raw, "currency_code": r.CurrencyCode, "reference": "capture", "reference_id": capID, "created_at": now, "updated_at": now}).Error != nil {
		return ErrUnavailable
	}
	updated := tx.Table("payment_collections").Where("id=? AND completed_at IS NULL", r.CollectionID).Updates(map[string]any{"status": "completed", "authorized_amount": scalar, "raw_authorized_amount": raw, "captured_amount": scalar, "raw_captured_amount": raw, "refunded_amount": zero, "raw_refunded_amount": zeroRaw, "completed_at": now, "updated_at": now})
	if updated.Error != nil || updated.RowsAffected != 1 {
		return ErrConflict
	}
	updated = tx.Table("payment_sessions").Where("id=? AND status='authorized'", r.SessionID).Updates(map[string]any{"status": "captured", "updated_at": now})
	if updated.Error != nil || updated.RowsAffected != 1 {
		return ErrConflict
	}
	if _, e = orders.RefreshCashSummaryTx(ctx, tx, r.OrderID, ord.Version); e != nil {
		return e
	}
	if e = customernotices.EmitOrderTx(tx, r.OrderID, "payment.received", payID); e != nil {
		return ErrUnavailable
	}
	updated = tx.Model(&operation{}).Where("operation_id=? AND applied=FALSE", r.OperationID).Updates(map[string]any{"state": "paid", "applied": true, "updated_at": now})
	if updated.Error != nil || updated.RowsAffected != 1 {
		return ErrConflict
	}
	r.State, r.Applied = "paid", true
	return nil
}
