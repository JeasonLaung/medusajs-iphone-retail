package cart

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// ReconcileContextPaymentSession observes only the saved remote identity of an
// exact uncertain delete attempt. It cannot dispatch DELETE/POST, complete a
// context or release a freeze. Live leases are left alone. Mollie requires
// terminal unpaid evidence; PayPal permits its original local-only retirement
// after verified absence of transaction history under the settlement freeze.
func ReconcileContextPaymentSession(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, sessionID, attemptID string, options InitialPaymentOptions) (PaymentExecution, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return PaymentExecution{}, err
	}
	var execution PaymentExecution
	var key payments.EffectKey
	var request payments.SessionRequest
	var deletion payments.SessionRequest
	var observer payments.SessionObserver
	var remote struct {
		ID     string `json:"id"`
		PayPal struct {
			ID string `json:"order_id"`
		} `json:"paypal"`
	}
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
		execution.Receipt, err = payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return err
		}
		if attemptID == "" || execution.Receipt.AttemptID != attemptID {
			return ErrConflict
		}
		if execution.Receipt.State == "succeeded" {
			return nil
		}
		if execution.Receipt.State != "inflight" && execution.Receipt.State != "uncertain" {
			return ErrConflict
		}
		// Expiry may mark the SAME attempt uncertain. Never call Claim on a
		// pending/failed receipt, which would grant a fresh mutation attempt.
		execution.Receipt, deletion, err = payments.ClaimSessionRequest(ctx, tx, key, options.Ring, 30*time.Second)
		if err != nil {
			return err
		}
		if execution.Receipt.Dispatch || execution.Receipt.AttemptID != attemptID {
			return ErrConflict
		}
		if execution.Receipt.State != "uncertain" {
			return nil
		}
		if json.Unmarshal(deletion.Data, &remote) != nil {
			return ErrStorage
		}
		if deletion.ProviderID != hostedMollie {
			remote.ID = remote.PayPal.ID
		}
		if remote.ID == "" {
			return ErrStorage
		}
		// A delete request contains the provider RESPONSE, not the original
		// create request's cart/redirect metadata. Recover that authenticated
		// request rather than inventing metadata or relaxing observer checks.
		if operation.Target.CheckoutOperationID == "" {
			return ErrConflict
		}
		original := InitialPaymentCommand{OperationID: operation.Target.CheckoutOperationID}
		var row initialAttempt
		if initialWhere(tx, ref, original).Take(&row).Error != nil {
			return ErrStorage
		}
		original.Version = strconv.FormatUint(row.InputVersion, 10)
		creation, err := validateInitialBinding(ctx, tx, ref, original, row, options)
		if err != nil {
			return err
		}
		request, err = payments.ReadSessionRequest(ctx, tx, creation, options.Ring)
		if err != nil {
			return err
		}
		if request.SessionID != deletion.SessionID || request.CollectionID != deletion.CollectionID || request.ProviderID != deletion.ProviderID || request.Amount != deletion.Amount || request.Currency != deletion.Currency {
			return ErrConflict
		}
		observer, err = options.Adapters.Observer(request.ProviderID)
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
	observed, err := payments.InvokeSessionObservation(callCtx, observer, request, remote.ID)
	cancel()
	if err != nil {
		return execution, err
	}
	if request.ProviderID == hostedMollie {
		if err = payments.ValidateMollieObservation(request, remote.ID, observed); err != nil {
			return execution, err
		}
		switch observed.ProviderStatus {
		case "canceled", "expired", "failed":
		default:
			return execution, nil
		}
	} else {
		outcome, err := payments.PayPalRetirementOutcome(request, remote.ID, observed)
		if err != nil {
			return execution, err
		}
		if outcome != payments.DeleteSucceeded {
			return execution, nil
		}
	}
	// Save the verified fact even if authority/cancellation changes during GET.
	// This exact-attempt write grants no cart completion or fresh dispatch.
	savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var saved payments.EffectReceipt
	err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
		var err error
		saved, err = payments.RecordEffect(savedCtx, tx, key, attemptID, "succeeded")
		return err
	})
	if err != nil {
		return execution, err
	}
	execution.Receipt = saved
	return execution, nil
}
