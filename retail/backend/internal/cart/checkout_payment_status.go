package cart

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"strings"
	"time"
)

type PaymentStatus struct {
	CartID  string `json:"cart_id"`
	Version string `json:"version"`
	State   string `json:"state"`
}

func currentInitialCommand(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference) (InitialPaymentCommand, error) {
	if !refOK(store, ref) {
		return InitialPaymentCommand{}, ErrDenied
	}
	if err := initialReady(ctx, tx); err != nil {
		return InitialPaymentCommand{}, err
	}
	channel, err := storeChannel(ctx, tx, store)
	if err != nil {
		return InitialPaymentCommand{}, err
	}

	// Lock owner once before any cart facts, avoiding SHARE->UPDATE conversion
	// deadlocks when several return polls reach the same cart concurrently.
	var owner struct{ CheckoutAttemptID *string }
	if tx.Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
		return InitialPaymentCommand{}, ErrDenied
	}
	if owner.CheckoutAttemptID == nil {
		return InitialPaymentCommand{}, ErrConflict
	}
	if _, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", "", *owner.CheckoutAttemptID); err != nil {
		return InitialPaymentCommand{}, err
	}
	var row initialAttempt
	if tx.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, *owner.CheckoutAttemptID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&row).Error != nil {
		return InitialPaymentCommand{}, ErrStorage
	}
	return InitialPaymentCommand{OperationID: *owner.CheckoutAttemptID, Version: strconv.FormatUint(row.InputVersion, 10)}, nil
}
func observationContext(id string) protected.Context {
	return protected.Context{Purpose: "checkout-observation-v1", Table: "cart_checkout_observations", Column: "result_envelope", Row: id}
}
func statusState(row initialAttempt) string {
	if row.NeedsReview {
		return "requires_review"
	}
	if row.ObservedStatus == nil {
		return "pending"
	}
	switch *row.ObservedStatus {
	case "paid":
		return "paid_pending_order"
	case "authorized":
		return "authorized"
	case "canceled", "expired", "failed":
		return "failed"
	default:
		return "pending"
	}
}

// Payment status never accepts remote IDs or success/amount from the browser.
// Provider GET runs outside SQL; the verified external fact is saved even after
// caller cancellation. Applying it rechecks current ownership and amount binding.
func ObserveInitialPayment(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, o InitialPaymentOptions) (PaymentStatus, error) {
	if db == nil || db.Statement == nil {
		return PaymentStatus{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return PaymentStatus{}, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return PaymentStatus{}, err
	}
	var in InitialPaymentCommand
	var row initialAttempt
	var key payments.EffectKey
	var request payments.SessionRequest
	var observer payments.SessionObserver
	var candidate string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		in, err = currentInitialCommand(ctx, tx, store, ref)
		if err != nil {
			return err
		}
		row, key, err = stageInitialPayment(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		receipt, err := payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return err
		}
		// Unknown creation without a saved remote ID needs webhook reconciliation.
		if receipt.State != "succeeded" {
			return nil
		}
		created, err := payments.ReadSessionCreation(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(created.Data, &identity) != nil || identity.ID == "" {
			return ErrStorage
		}
		candidate = identity.ID
		request, err = payments.ReadSessionRequest(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		observer, err = o.Adapters.Observer(request.ProviderID)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PaymentStatus{}, err
	}
	out := PaymentStatus{ref.CartID, strconv.FormatUint(row.CartVersion, 10), statusState(row)}
	if candidate == "" {
		out.State = "requires_review"
		return out, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	observed, err := payments.InvokeSessionObservation(callCtx, observer, request, candidate)
	cancel()
	if err != nil {
		return PaymentStatus{}, err
	}
	if err = payments.ValidateMollieObservation(request, candidate, observed); err != nil {
		return PaymentStatus{}, err
	}
	id := uuid.NewString()
	raw, err := json.Marshal(observed)
	if err != nil || len(raw) > protected.MaxPlaintext {
		return PaymentStatus{}, ErrStorage
	}
	defer clear(raw)
	envelope, err := o.Ring.Seal(observationContext(id), raw)
	if err != nil {
		return PaymentStatus{}, err
	}
	defer clear(envelope)
	savedCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
		// This append-only row is external evidence, not an ownership grant.
		return tx.Table("cart_checkout_observations").Create(map[string]any{"id": id, "cart_id": ref.CartID, "operation_id": in.OperationID, "request_sha256": key.Digest[:], "result_envelope": envelope, "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error
	})
	saveCancel()
	if err != nil {
		return PaymentStatus{}, ErrStorage
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		row, _, err = stageInitialPayment(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		previous := ""
		if row.ObservedStatus != nil {
			previous = *row.ObservedStatus
		}
		next, review := mergeObservedPayment(previous, observed.ProviderStatus)
		row.NeedsReview = row.NeedsReview || review
		row.ObservedStatus = &next
		if r := initialWhere(tx, ref, in).Updates(map[string]any{"observed_status": next, "needs_review": row.NeedsReview}); r.Error != nil {
			return ErrStorage
		}
		// Do not overwrite the immutable create receipt or classify authorization
		// as cash. Session 'authorized' matches source mapping for either fact.
		var local struct{ Status string }
		if tx.Table("payment_sessions").Select("status").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", row.SessionID, row.CollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&local).Error != nil {
			return ErrStorage
		}
		statuses := map[string]string{"open": "pending", "pending": "pending", "paid": "authorized", "authorized": "authorized", "canceled": "canceled", "expired": "error", "failed": "error"}
		if r := tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", row.SessionID, row.CollectionID).Updates(map[string]any{"status": func() string {
			if local.Status == "captured" {
				return "captured"
			}
			if local.Status == "authorized" && (next == "open" || next == "pending") {
				return "authorized"
			}
			return statuses[next]
		}(), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}); r.Error != nil || r.RowsAffected != 1 {
			return ErrStorage
		}
		out.State = statusState(row)
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, err
}
func mergeObservedPayment(previous, next string) (string, bool) {
	failed := func(s string) bool { return s == "canceled" || s == "expired" || s == "failed" }
	if previous == "paid" {
		return previous, failed(next)
	}
	if next == "paid" {
		return next, failed(previous)
	}
	if previous == "authorized" {
		if failed(next) {
			return previous, true
		}
		return previous, false
	}
	if failed(previous) {
		return previous, next == "authorized"
	}
	if strings.TrimSpace(next) != next {
		return previous, true
	}
	return next, false
}
