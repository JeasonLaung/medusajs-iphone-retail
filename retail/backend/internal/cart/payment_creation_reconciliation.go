package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// ReconcilePaymentCompensationSession reads an uncertain create's remote result,
// never re-POSTs it. Recheck current cart ownership/version/freeze/source before
// obtaining a saved request. Only the exact existing attempt can be recovered.
// A live lease is left alone. Unknown remote IDs do not prove creation failed.
// This private coordinator accepts only an ordinary DB pool, no caller SQL lock.
func ReconcilePaymentCompensationSession(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, originalID, attemptID, candidateID string, ring *protected.Ring, adapters *payments.CreateAdapters) (PaymentExecution, error) {
	if db == nil || db.Statement == nil || adapters == nil {
		return PaymentExecution{}, ErrInput
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return PaymentExecution{}, ErrInput
	}
	var execution PaymentExecution
	var key payments.EffectKey
	var request payments.SessionRequest
	var observer payments.SessionObserver
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
		execution.Receipt, err = payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return err
		}
		if execution.Receipt.AttemptID != attemptID || attemptID == "" {
			return ErrConflict
		}
		if execution.Receipt.State == "succeeded" {
			_, err := payments.ReadSessionCreation(ctx, tx, key, ring)
			return err
		}
		if execution.Receipt.State != "inflight" && execution.Receipt.State != "uncertain" {
			return ErrConflict
		}
		// Claim only an existing inflight/uncertain effect. Its DB-clock expiry
		// can transition to uncertain but can NEVER grant new dispatch.
		execution.Receipt, request, err = payments.ClaimSessionRequest(ctx, tx, key, ring, 30*time.Second)
		if err != nil {
			return err
		}
		if execution.Receipt.Dispatch || execution.Receipt.AttemptID != attemptID {
			return ErrConflict
		}
		if execution.Receipt.State != "uncertain" {
			return nil
		}
		observer, err = adapters.Observer(request.ProviderID)
		return err
	})
	if err != nil {
		return PaymentExecution{}, err
	}
	if execution.Receipt.State != "uncertain" {
		return execution, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	execution.Called = true
	response, err := payments.InvokeObservation(callCtx, observer, request, candidateID)
	cancel()
	if err != nil {
		return execution, err
	}
	// This is the already authorized attempt's verified external fact. Caller
	// cancellation/revocation during GET cannot discard it or grant new dispatch.
	savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var saved payments.EffectReceipt
	err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
		var err error
		saved, err = payments.RecordSessionCreation(savedCtx, tx, key, attemptID, ring, response)
		return err
	})
	if err != nil {
		return execution, err
	}
	execution.Receipt = saved
	return execution, nil
}
