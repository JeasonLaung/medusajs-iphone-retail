package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// ExecuteContextPaymentSession revalidates the durable target, original
// checkout and encrypted source before committing one delete claim. It shares
// the standalone executor's bounded out-of-transaction call and detached fact
// persistence. It never saves the context or releases either checkout freeze.
// An uncertain receipt cannot grant another DELETE; it requires observation.
func ExecuteContextPaymentSession(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, sessionID string, options InitialPaymentOptions, adapters *payments.DeleteAdapters) (PaymentExecution, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return PaymentExecution{}, err
	}
	if adapters == nil {
		return PaymentExecution{}, ErrInput
	}
	var execution PaymentExecution
	var key payments.EffectKey
	var request payments.SessionRequest
	var adapter payments.DeleteSessionAdapter
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operation, err := stageContextPayment(ctx, tx, store, ref, in, options)
		if err != nil {
			return err
		}
		found := false
		for _, candidate := range operation.Effects {
			if candidate.ResourceID == sessionID {
				key, found = candidate, true
				break
			}
		}
		if !found {
			return ErrInput
		}
		execution.Receipt, request, err = payments.ClaimSessionRequest(ctx, tx, key, options.Ring, 30*time.Second)
		if err != nil {
			return err
		}
		// Resolve configuration before commit so it cannot consume an attempt.
		if execution.Receipt.Dispatch {
			adapter, err = adapters.Lookup(request.ProviderID)
		}
		return err
	})
	if err != nil {
		return PaymentExecution{}, err
	}
	return executeClaimedPaymentDelete(ctx, db, execution, key, request, adapter)
}
