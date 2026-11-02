package identity_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The provider boundary is artificial; cart/checkout/source, claims, SQL locks
// and failure triggers are real. No source session is actually canceled.
func checkContextPaymentExecution(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, options cart.InitialPaymentOptions, fingerprint func() [32]byte) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context execution fixture", err)
		}
	}
	baseline := fingerprint()
	type fixture struct {
		command cart.ContextCommand
		key     payments.EffectKey
		request payments.SessionRequest
	}
	prepare := func() fixture {
		t.Helper()
		f := fixture{command: cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "preview-canada", Locale: "en"}}
		operation, err := cart.StageContextPayment(ctx, db, "shop", ref, f.command, options)
		if err != nil || len(operation.Effects) != 1 {
			t.Fatal("context execution preparation", err)
		}
		f.key = operation.Effects[0]
		err = db.Transaction(func(tx *gorm.DB) error {
			var err error
			f.request, err = payments.ReadSessionRequest(ctx, tx, f.key, options.Ring)
			return err
		})
		if err != nil {
			t.Fatal("context execution protected request", err)
		}
		return f
	}
	cleanup := func(f fixture) {
		t.Helper()
		// Only receipts from this explicitly artificial adapter are removed to
		// restore the parent's initial checkout. This is not runtime recovery.
		exec("DELETE FROM payment_effect_journal WHERE scope_id=? AND operation_id=?", ref.CartID, f.command.OperationID)
		exec("DELETE FROM cart_payment_refresh_operations WHERE cart_id=? AND operation_id=? AND state='staged'", ref.CartID, f.command.OperationID)
		exec("UPDATE cart_ownership SET payment_refresh_operation_id=NULL WHERE cart_id=? AND payment_refresh_operation_id=?", ref.CartID, f.command.OperationID)
		if fingerprint() != baseline {
			t.Fatal("context execution changed original checkout/cart/source facts")
		}
	}
	registry := func(f fixture, adapter executionDelete) *payments.DeleteAdapters {
		t.Helper()
		r, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{f.request.ProviderID: adapter})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	run := func(c context.Context, f fixture, adapters *payments.DeleteAdapters) (cart.PaymentExecution, error) {
		return cart.ExecuteContextPaymentSession(c, db, "shop", ref, f.command, f.key.ResourceID, options, adapters)
	}
	// A second transaction while the provider blocks proves the durable claim
	// committed and the executor holds no cart lock during its external call.
	f := prepare()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r := registry(f, func(callCtx context.Context, request payments.SessionRequest) (payments.DeleteOutcome, error) {
		calls.Add(1)
		deadline, ok := callCtx.Deadline()
		if !ok || time.Until(deadline) > 21*time.Second || request.Amount != "158.75" || request.Currency != "eur" || request.SessionID != f.key.ResourceID {
			return payments.DeleteUncertain, errors.New("original request drift")
		}
		lockCtx, cancel := context.WithTimeout(callCtx, 3*time.Second)
		defer cancel()
		operation, err := cart.StageContextPayment(lockCtx, db, "shop", ref, f.command, options)
		if err != nil || !operation.Replayed {
			return payments.DeleteUncertain, errors.New("claim not committed or cart lock retained")
		}
		var receipt payments.EffectReceipt
		err = db.WithContext(lockCtx).Transaction(func(tx *gorm.DB) error {
			var err error
			receipt, err = payments.InspectEffect(lockCtx, tx, f.key)
			return err
		})
		if err != nil || receipt.State != "inflight" {
			return payments.DeleteUncertain, errors.New("claim not visible")
		}
		close(entered)
		select {
		case <-release:
			return payments.DeleteSucceeded, nil
		case <-callCtx.Done():
			return payments.DeleteUncertain, callCtx.Err()
		}
	})
	type completed struct {
		result cart.PaymentExecution
		err    error
	}
	done := make(chan completed, 1)
	go func() { result, err := run(ctx, f, r); done <- completed{result, err} }()
	select {
	case <-entered:
	case first := <-done:
		t.Fatal("context provider did not enter", first.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second, err := run(ctx, f, r)
	close(release)
	first := <-done
	if err != nil || second.Called || second.Receipt.State != "inflight" || first.err != nil || !first.result.Called || first.result.Receipt.State != "succeeded" || calls.Load() != 1 {
		t.Fatal("context concurrent execution duplicated or lost result", err, first.err)
	}
	if again, err := run(ctx, f, r); err != nil || again.Called || again.Receipt.State != "succeeded" || calls.Load() != 1 {
		t.Fatal("context successful deletion redispatched", err)
	}
	cleanup(f)

	// Definite no-effect failures may retry the exact original request.
	f = prepare()
	var retries atomic.Int32
	r = registry(f, func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		if retries.Add(1) == 1 {
			return payments.DeleteFailed, nil
		}
		return payments.DeleteSucceeded, nil
	})
	if result, err := run(ctx, f, r); err != nil || result.Receipt.State != "failed" || result.Receipt.Attempts != 1 {
		t.Fatal("definite context failure", err)
	}
	if result, err := run(ctx, f, r); err != nil || result.Receipt.State != "succeeded" || result.Receipt.Attempts != 2 || retries.Load() != 2 {
		t.Fatal("stable context retry", err)
	}
	cleanup(f)

	// Ambiguous errors stay uncertain and cannot authorize another DELETE.
	f = prepare()
	var unknownCalls atomic.Int32
	r = registry(f, func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		unknownCalls.Add(1)
		return payments.DeleteFailed, errors.New("artificial-private-error")
	})
	if result, err := run(ctx, f, r); err != payments.ErrDeleteExecution || result.Receipt.State != "uncertain" || !result.Called {
		t.Fatal("context ambiguity was not protected", err)
	}
	if result, err := run(ctx, f, r); err != nil || result.Called || result.Receipt.State != "uncertain" || unknownCalls.Load() != 1 {
		t.Fatal("uncertain context deletion repeated", err)
	}
	cleanup(f)

	// Revocation and cancellation during an already authorized external call
	// must preserve its exact fact, without granting fresh dispatch authority.
	f = prepare()
	callCtx, cancel := context.WithCancel(ctx)
	r = registry(f, func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		exec("UPDATE cart_ownership SET generation=REPEAT('e',64) WHERE cart_id=?", ref.CartID)
		cancel()
		return payments.DeleteSucceeded, nil
	})
	result, err := run(callCtx, f, r)
	cancel()
	if err != nil || !result.Called || result.Receipt.State != "succeeded" {
		t.Fatal("context revoked/canceled authorized fact lost", err)
	}
	if denied, err := run(ctx, f, r); err != cart.ErrDenied || denied.Called {
		t.Fatal("revoked context authority dispatched", err)
	}
	exec("UPDATE cart_ownership SET generation=? WHERE cart_id=?", ref.Generation, ref.CartID)
	cleanup(f)

	f = prepare()
	var blockedCalls atomic.Int32
	r = registry(f, func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		blockedCalls.Add(1)
		return payments.DeleteSucceeded, nil
	})
	before := fingerprint()
	exec("CREATE TRIGGER context_execution_claim_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial context claim fault'")
	result, err = run(ctx, f, r)
	exec("DROP TRIGGER context_execution_claim_fault")
	if err != payments.ErrEffectStorage || result.Called || blockedCalls.Load() != 0 || fingerprint() != before {
		t.Fatal("uncommitted context claim dispatched", err)
	}
	wrong, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{"other": executionDelete(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		return payments.DeleteUncertain, errors.New("wrong adapter invoked")
	})})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run(ctx, f, wrong); err != payments.ErrEffectInput || result.Called || fingerprint() != before {
		t.Fatal("unknown context adapter consumed a claim", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		result, err := cart.ExecuteContextPaymentSession(ctx, tx, "shop", ref, f.command, f.key.ResourceID, options, r)
		if err != cart.ErrInput || result.Called {
			t.Fatal("context executor held caller transaction", err)
		}
		return nil
	}); err != nil || fingerprint() != before {
		t.Fatal("nested context execution changed facts", err)
	}
	cleanup(f)

	// Lost outcome persistence keeps the committed inflight claim; lease expiry
	// becomes uncertain, never a new DELETE grant.
	f = prepare()
	var lostCalls atomic.Int32
	r = registry(f, func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		lostCalls.Add(1)
		exec("CREATE TRIGGER context_execution_result_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial context result fault'")
		return payments.DeleteSucceeded, nil
	})
	result, err = run(ctx, f, r)
	exec("DROP TRIGGER context_execution_result_fault")
	if err != payments.ErrEffectStorage || !result.Called || result.Receipt.State != "inflight" {
		t.Fatal("lost context outcome falsely retryable", err)
	}
	if replay, err := run(ctx, f, r); err != nil || replay.Called || replay.Receipt.State != "inflight" || lostCalls.Load() != 1 {
		t.Fatal("lost context success redispatched", err)
	}
	if observed, err := cart.ReconcileContextPaymentSession(ctx, db, "shop", ref, f.command, f.key.ResourceID, result.Receipt.AttemptID, options); err != nil || observed.Called || observed.Receipt.State != "inflight" {
		t.Fatal("live context lease queried provider", err)
	}
	exec("UPDATE payment_effect_journal SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE scope_id=? AND operation_id=?", ref.CartID, f.command.OperationID)
	if replay, err := run(ctx, f, r); err != nil || replay.Called || replay.Receipt.State != "uncertain" || lostCalls.Load() != 1 {
		t.Fatal("expired context success redispatched", err)
	}
	checkContextPaymentReconciliation(t, ctx, db, ref, f.command, f.key, f.request, result.Receipt.AttemptID, options, fingerprint)
	cleanup(f)
	t.Log(`CART_CONTEXT_PAYMENT_EXECUTION {"real_mysql":true,"committed_claim_before_call":true,"no_sql_lock_during_provider":true,"concurrent_single_call":true,"original_amount_currency_request":true,"success_not_repeated":true,"definite_failure_stable_retry":true,"ambiguous_failure_uncertain":true,"canceled_revoked_inflight_fact_persisted":true,"revoked_dispatch_denied":true,"claim_failure_rollback_no_call":true,"unknown_adapter_no_claim":true,"nested_transaction_denied":true,"lost_outcome_and_expired_lease_not_retried":true,"original_checkout_cart_and_business_unchanged":true,"artificial_provider_only":true,"final_context_save":false}`)
}
