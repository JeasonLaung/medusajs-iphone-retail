package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// ExecutePaymentCompensationSession owns claim and result transactions, with
// the bounded provider create call BETWEEN them. It accepts only an ordinary
// pool. Stable session/provider requests come from the authorized persisted
// compensation plan. It neither activates the replacement nor releases freeze.
func ExecutePaymentCompensationSession(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, originalID string, ring *protected.Ring, adapters *payments.CreateAdapters) (PaymentExecution, error) {
	if db == nil || db.Statement == nil || adapters == nil {
		return PaymentExecution{}, ErrInput
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return PaymentExecution{}, ErrInput
	}
	var execution PaymentExecution
	var key payments.EffectKey
	var request payments.SessionRequest
	var adapter payments.CreateSessionAdapter
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operation, err := StagePaymentCompensation(ctx, tx, store, ref, in, ring)
		if err != nil {
			return err
		}
		if operation.State != "staged" {
			return ErrConflict
		}
		found := false
		for _, replacement := range operation.Replacements {
			if replacement.OriginalID == originalID {
				key = replacement.Create
				found = true
				break
			}
		}
		if !found {
			return ErrInput
		}
		execution.Receipt, request, err = payments.ClaimSessionRequest(ctx, tx, key, ring, 30*time.Second)
		if err != nil {
			return err
		}
		if execution.Receipt.Dispatch {
			adapter, err = adapters.Lookup(request.ProviderID)
		} else if execution.Receipt.State == "succeeded" {
			_, err = payments.ReadSessionCreation(ctx, tx, key, ring)
		}
		return err
	})
	if err != nil {
		return PaymentExecution{}, err
	}
	if !execution.Receipt.Dispatch {
		return execution, nil
	}
	execution.Receipt.Dispatch = false
	outcome := payments.CreateFailed
	var response payments.CreatedSession
	var callErr error
	if ctx.Err() == nil {
		callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		execution.Called = true
		outcome, response, callErr = payments.InvokeCreate(callCtx, adapter, request)
		cancel()
	}
	// Persist the already authorized attempt's external fact even after caller
	// cancellation/ownership revocation. No fresh dispatch or cart access granted.
	savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var saved payments.EffectReceipt
	err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
		var err error
		if outcome == payments.CreateSucceeded {
			saved, err = payments.RecordSessionCreation(savedCtx, tx, key, execution.Receipt.AttemptID, ring, response)
		} else {
			saved, err = payments.RecordEffect(savedCtx, tx, key, execution.Receipt.AttemptID, string(outcome))
		}
		return err
	})
	if err != nil {
		return execution, err
	}
	execution.Receipt = saved
	return execution, callErr
}
