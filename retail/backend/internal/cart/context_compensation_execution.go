package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// ExecuteContextCompensationSession commits a protected replacement claim,
// calls the provider outside SQL locks, then persists the exact attempt fact.
// It does not activate the replacement, alter the original checkout or release
// the compensation freeze. Ambiguous creation never grants another POST.
func ExecuteContextCompensationSession(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, originalID string, options InitialPaymentOptions) (PaymentExecution, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return PaymentExecution{}, err
	}
	var execution PaymentExecution
	var key payments.EffectKey
	var request payments.SessionRequest
	var adapter payments.CreateSessionAdapter
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operation, err := stageContextPaymentCompensation(ctx, tx, store, ref, in, options)
		if err != nil {
			return err
		}
		found := false
		for _, replacement := range operation.Replacements {
			if replacement.OriginalID == originalID {
				key, found = replacement.Create, true
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
		if execution.Receipt.Dispatch {
			adapter, err = options.Adapters.Lookup(request.ProviderID)
		} else if execution.Receipt.State == "succeeded" {
			_, err = payments.ReadSessionCreation(ctx, tx, key, options.Ring)
		}
		return err
	})
	if err != nil {
		return PaymentExecution{}, err
	}
	return executeClaimedPaymentCreate(ctx, db, execution, key, request, adapter, options.Ring)
}

// ReconcileContextCompensationSession observes/discovers only an existing
// uncertain create attempt. A candidate is only a locator; the full saved
// merchant/session/cart/amount/currency identity must match. Empty candidate
// requires an explicitly configured discoverer. Never issues POST or DELETE.
func ReconcileContextCompensationSession(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, originalID, attemptID, candidateID string, options InitialPaymentOptions) (PaymentExecution, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return PaymentExecution{}, err
	}
	var execution PaymentExecution
	var key payments.EffectKey
	var request payments.SessionRequest
	var observer payments.SessionObserver
	var discoverer payments.SessionDiscoverer
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operation, err := stageContextPaymentCompensation(ctx, tx, store, ref, in, options)
		if err != nil {
			return err
		}
		found := false
		for _, replacement := range operation.Replacements {
			if replacement.OriginalID == originalID {
				key, found = replacement.Create, true
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
		if attemptID == "" || execution.Receipt.AttemptID != attemptID {
			return ErrConflict
		}
		if execution.Receipt.State == "succeeded" {
			_, err = payments.ReadSessionCreation(ctx, tx, key, options.Ring)
			return err
		}
		if execution.Receipt.State != "inflight" && execution.Receipt.State != "uncertain" {
			return ErrConflict
		}
		execution.Receipt, request, err = payments.ClaimSessionRequest(ctx, tx, key, options.Ring, 30*time.Second)
		if err != nil {
			return err
		}
		if execution.Receipt.Dispatch || execution.Receipt.AttemptID != attemptID {
			return ErrConflict
		}
		if execution.Receipt.State != "uncertain" {
			return nil
		}
		if candidateID != "" {
			observer, err = options.Adapters.Observer(request.ProviderID)
			return err
		}
		adapter, err := options.Adapters.Lookup(request.ProviderID)
		if err != nil {
			return err
		}
		var ok bool
		discoverer, ok = adapter.(payments.SessionDiscoverer)
		if !ok {
			return ErrConfiguration
		}
		return nil
	})
	if err != nil {
		return PaymentExecution{}, err
	}
	if execution.Receipt.State != "uncertain" {
		return execution, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	execution.Called = true
	var observed payments.SessionObservation
	if candidateID == "" {
		observed, err = payments.InvokeDiscovery(callCtx, discoverer, request)
	} else {
		observed, err = payments.InvokeSessionObservation(callCtx, observer, request, candidateID)
	}
	cancel()
	if err != nil {
		return execution, err
	}
	if candidateID == "" {
		candidateID = observed.PaymentID
	}
	if request.ProviderID == hostedMollie {
		err = payments.ValidateMollieObservation(request, candidateID, observed)
	} else {
		err = payments.ValidatePayPalObservation(request, candidateID, observed)
	}
	if err != nil {
		return execution, err
	}
	savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var saved payments.EffectReceipt
	err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
		var err error
		saved, err = payments.RecordSessionCreation(savedCtx, tx, key, attemptID, options.Ring, observed.Session)
		return err
	})
	if err != nil {
		return execution, err
	}
	execution.Receipt = saved
	return execution, nil
}
