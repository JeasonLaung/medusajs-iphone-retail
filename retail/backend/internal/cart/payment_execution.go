package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// PaymentExecution is private worker output. A call can have happened even when
// persistence fails. In that case Receipt remains the durable pre-call inflight
// claim; do NOT infer failure or retry the provider. Reconcile the same attempt.
type PaymentExecution struct {
	Receipt payments.EffectReceipt
	Called  bool
}

// ExecutePaymentRefreshSession owns two separate transactions and a bounded
// external call between them. It accepts an ordinary pool, never an enclosing
// transaction: provider work MUST NOT hold cart/SQL locks. Recheck current owned
// cart, version, frozen plan, source and protected request before claiming.
// Only a successfully committed Dispatch grants one call. This function does
// not complete the refresh, compensate failures or run as an automatic worker.
func ExecutePaymentRefreshSession(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, sessionID string, ring *protected.Ring, adapters *payments.DeleteAdapters) (PaymentExecution, error) {
	if db == nil || db.Statement == nil || adapters == nil {
		return PaymentExecution{}, ErrInput
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return PaymentExecution{}, ErrInput
	}
	var execution PaymentExecution
	var key payments.EffectKey
	var request payments.SessionRequest
	var adapter payments.DeleteSessionAdapter
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		operation, err := StagePaymentRefresh(ctx, tx, store, ref, in, ring)
		if err != nil {
			return err
		}
		if operation.State != "staged" {
			return ErrConflict
		}
		found := false
		for _, candidate := range operation.Effects {
			if candidate.ResourceID == sessionID {
				key = candidate
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
		// Unknown configuration cannot consume an attempt. Rolling back the whole
		// transaction also rolls back any freshly staged operation/request/freeze.
		if execution.Receipt.Dispatch {
			adapter, err = adapters.Lookup(request.ProviderID)
		}
		return err
	})
	if err != nil {
		return PaymentExecution{}, err
	}
	if !execution.Receipt.Dispatch {
		return execution, nil
	}
	execution.Receipt.Dispatch = false // commit granted this executor, not its caller
	outcome := payments.DeleteFailed
	var callErr error
	if ctx.Err() == nil {
		callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		execution.Called = true
		outcome, callErr = payments.InvokeDelete(callCtx, adapter, request)
		cancel()
	}
	// The provider fact belongs to the already authorized exact attempt. Record
	// it even if browser cancellation, source changes or ownership revocation
	// occurred during the call. This grants no new dispatch/customer authority.
	// Detach cancellation only for this bounded persistence, preserving values.
	savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var saved payments.EffectReceipt
	err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
		var err error
		saved, err = payments.RecordEffect(savedCtx, tx, key, execution.Receipt.AttemptID, string(outcome))
		return err
	})
	if err != nil {
		return execution, err
	}
	execution.Receipt = saved
	return execution, callErr
}
