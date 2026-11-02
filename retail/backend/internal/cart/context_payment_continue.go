package cart

import (
	"context"
	"strconv"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ContextPaymentProgress exposes only a receipt or an actionable state. Provider
// identities, protected requests, effects and attempt keys stay on the server.
type ContextPaymentProgress struct {
	State      string                    `json:"state"`
	Receipt    *ContextResult            `json:"receipt,omitempty"`
	Completion *ContextPaymentCompletion `json:"completion,omitempty"`
}

type ContextPaymentBinding struct {
	OperationID string `json:"operation_id"`
	Version     string `json:"version"`
}
type ContextPaymentCompletion struct {
	Cart    Snapshot               `json:"cart"`
	Payment *ContextPaymentBinding `json:"payment"`
}

// Reauthorize the current binding after final save. The browser may retain the
// old CREATE, use a compensation CREATE, or clear a retired CREATE only from
// this explicit server result. No provider URL or private effect is returned.
func contextPaymentCompleted(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, state string, receipt *ContextResult) (ContextPaymentProgress, error) {
	result := ContextPaymentProgress{State: state, Receipt: receipt}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := initialReady(ctx, tx); err != nil {
			return err
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		var owner struct{ CheckoutAttemptID *string }
		if tx.Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
			return ErrDenied
		}
		checkout := ""
		if owner.CheckoutAttemptID != nil {
			checkout = *owner.CheckoutAttemptID
		}
		current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", "", checkout)
		if err != nil {
			return err
		}
		completion := &ContextPaymentCompletion{Cart: current}
		if checkout != "" {
			var attempt initialAttempt
			if initialWhere(tx, ref, InitialPaymentCommand{OperationID: checkout}).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&attempt).Error != nil {
				return ErrStorage
			}
			if !attempt.Activated || attempt.Generation != ref.Generation || attempt.InputVersion == 0 || attempt.InputVersion+1 != attempt.CartVersion || strconv.FormatUint(attempt.currentVersion(), 10) != current.Version {
				return ErrConflict
			}
			completion.Payment = &ContextPaymentBinding{OperationID: checkout, Version: strconv.FormatUint(attempt.InputVersion, 10)}
		}
		result.Completion = completion
		return nil
	})
	if err != nil {
		return ContextPaymentProgress{}, err
	}
	return result, nil
}

// ContinueContextPayment performs at most one bounded external mutation. A
// repeated call resumes the durable operation, including exact-attempt GET
// recovery, rather than granting another mutation after an ambiguous result.
func ContinueContextPayment(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions, deletes *payments.DeleteAdapters) (ContextPaymentProgress, error) {
	if deletes == nil {
		return ContextPaymentProgress{}, ErrConfiguration
	}
	if receipt, err := CompleteContextPayment(ctx, db, store, ref, in, options); err == nil {
		return contextPaymentCompleted(ctx, db, store, ref, "saved", &receipt)
	} else if err != ErrConflict {
		return ContextPaymentProgress{}, err
	}
	operation, err := StageContextPayment(ctx, db, store, ref, in, options)
	if err != nil {
		return ContextPaymentProgress{}, err
	}
	// The current checkout has exactly one original hosted payment session.
	// Bound orchestration independently of the lower-level manifest limit.
	if len(operation.Effects) > 1 {
		return ContextPaymentProgress{}, ErrConflict
	}
	for _, effect := range operation.Effects {
		execution, err := ExecuteContextPaymentSession(ctx, db, store, ref, in, effect.ResourceID, options, deletes)
		if err != nil && !(err == payments.ErrDeleteExecution && execution.Receipt.State == "uncertain") {
			return ContextPaymentProgress{}, err
		}
		if execution.Receipt.State == "inflight" || execution.Receipt.State == "uncertain" {
			execution, err = ReconcileContextPaymentSession(ctx, db, store, ref, in, effect.ResourceID, execution.Receipt.AttemptID, options)
			if err != nil {
				return ContextPaymentProgress{}, err
			}
		}
		if execution.Receipt.State != "succeeded" {
			state := "processing"
			if execution.Receipt.State == "failed" || execution.Receipt.State == "uncertain" {
				state = "requires_recovery"
			}
			return ContextPaymentProgress{State: state}, nil
		}
	}
	receipt, err := CompleteContextPayment(ctx, db, store, ref, in, options)
	if err != nil {
		return ContextPaymentProgress{}, err
	}
	return contextPaymentCompleted(ctx, db, store, ref, "saved", &receipt)
}

// ContinueContextPaymentRestore restores the original context using the SAME
// target command as retry identity. It never reports the requested new context
// as saved. Unknown deletions must resolve before compensation can be staged.
func ContinueContextPaymentRestore(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (ContextPaymentProgress, error) {
	if _, err := CompleteContextPaymentCompensation(ctx, db, store, ref, in, options); err == nil {
		return contextPaymentCompleted(ctx, db, store, ref, "restored", nil)
	} else if err != ErrConflict && err != payments.ErrEffectConflict {
		return ContextPaymentProgress{}, err
	}
	operation, err := StageContextPaymentCompensation(ctx, db, store, ref, in, options)
	if err != nil {
		return ContextPaymentProgress{}, err
	}
	if len(operation.Replacements) > 1 {
		return ContextPaymentProgress{}, ErrConflict
	}
	for _, replacement := range operation.Replacements {
		execution, err := ExecuteContextCompensationSession(ctx, db, store, ref, in, replacement.OriginalID, options)
		if err != nil && !(err == payments.ErrCreateExecution && execution.Receipt.State == "uncertain") {
			return ContextPaymentProgress{}, err
		}
		if execution.Receipt.State == "inflight" || execution.Receipt.State == "uncertain" {
			execution, err = ReconcileContextCompensationSession(ctx, db, store, ref, in, replacement.OriginalID, execution.Receipt.AttemptID, "", options)
			if err != nil {
				return ContextPaymentProgress{}, err
			}
		}
		if execution.Receipt.State != "succeeded" {
			state := "processing"
			if execution.Receipt.State == "failed" || execution.Receipt.State == "uncertain" {
				state = "requires_recovery"
			}
			return ContextPaymentProgress{State: state}, nil
		}
	}
	if _, err := CompleteContextPaymentCompensation(ctx, db, store, ref, in, options); err != nil {
		return ContextPaymentProgress{}, err
	}
	return contextPaymentCompleted(ctx, db, store, ref, "restored", nil)
}
