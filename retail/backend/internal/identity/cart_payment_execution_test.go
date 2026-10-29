package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type executionDelete func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error)

func (f executionDelete) DeleteSession(ctx context.Context, in payments.SessionRequest) (payments.DeleteOutcome, error) {
	return f(ctx, in)
}

func checkCartPaymentExecution(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("execution artificial SQL fixture failed")
		}
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	ring, err := protected.New("artificial", map[string][]byte{"artificial": bytes.Repeat([]byte{0x38}, 32)})
	if err != nil {
		t.Fatal("execution ring fixture failed")
	}
	type fixture struct {
		ref     sessionstore.CartReference
		command cart.PaymentRefreshCommand
		session string
	}
	makeFixture := func() fixture {
		t.Helper()
		id := uuid.NewString()
		created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: "en"})
		if err != nil {
			t.Fatal("execution owned cart fixture failed")
		}
		f := fixture{created.Reference, cart.PaymentRefreshCommand{OperationID: uuid.NewString(), Version: "1"}, "execution-session-" + id}
		collection := "execution-collection-" + id
		request := payments.SessionRequest{ProviderID: "artificial", CollectionID: collection, SessionID: f.session, Amount: "10", Currency: "usd", Data: json.RawMessage(`{"id":"artificial-private-token","large":9007199254740993}`)}
		data, context, err := payments.SealSessionFields(ring, request)
		if err != nil {
			t.Fatal("execution source envelope fixture failed")
		}
		exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES(?,'usd',10,'{"value":"10","precision":100}','not_paid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, collection)
		exec(`INSERT INTO cart_payment_collections(id,cart_id,payment_collection_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, "execution-link-"+id, f.ref.CartID, collection)
		exec(`INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,context,status,payment_collection_id,created_at,updated_at) VALUES(?,'usd',10,'{"value":"10","precision":100}','artificial',?,?,'pending',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, f.session, data, context, collection)
		return f
	}
	registry := func(adapter executionDelete) *payments.DeleteAdapters {
		t.Helper()
		r, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{"artificial": adapter})
		if err != nil {
			t.Fatal("execution adapter fixture failed")
		}
		return r
	}
	run := func(c context.Context, f fixture, r *payments.DeleteAdapters) (cart.PaymentExecution, error) {
		return cart.ExecutePaymentRefreshSession(c, db, "shop", f.ref, f.command, f.session, ring, r)
	}
	finish := func(f fixture) {
		t.Helper()
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, err := cart.CompletePaymentRefresh(ctx, tx, "shop", f.ref, f.command, ring)
			return err
		}); err != nil {
			t.Fatalf("executed payment finish failed: %v", err)
		}
	}
	// A second DB transaction from inside the adapter must obtain the cart lock:
	// neither the claim transaction nor an outer caller transaction may hold it.
	f := makeFixture()
	var calls atomic.Int32
	r := registry(func(callCtx context.Context, in payments.SessionRequest) (payments.DeleteOutcome, error) {
		calls.Add(1)
		deadline, ok := callCtx.Deadline()
		if !ok || time.Until(deadline) > 21*time.Second || in.SessionID != f.session || in.Amount != "10" || !bytes.Contains(in.Data, []byte("9007199254740993")) {
			return payments.DeleteUncertain, errors.New("artificial invalid call facts")
		}
		lockCtx, cancel := context.WithTimeout(callCtx, 2*time.Second)
		defer cancel()
		err := db.WithContext(lockCtx).Transaction(func(tx *gorm.DB) error {
			operation, err := cart.StagePaymentRefresh(lockCtx, tx, "shop", f.ref, f.command, ring)
			if err != nil {
				return err
			}
			if !operation.Replayed {
				return errors.New("claim not committed")
			}
			receipt, err := payments.InspectEffect(lockCtx, tx, operation.Effects[0])
			if err != nil {
				return err
			}
			if receipt.State != "inflight" {
				return errors.New("claim not visible")
			}
			return nil
		})
		if err != nil {
			return payments.DeleteUncertain, err
		}
		return payments.DeleteSucceeded, nil
	})
	result, err := run(ctx, f, r)
	if err != nil || !result.Called || result.Receipt.State != "succeeded" || result.Receipt.Dispatch || calls.Load() != 1 {
		t.Fatalf("committed claim / outside transaction execution failed: %v", err)
	}
	if replay, err := run(ctx, f, r); err != nil || replay.Called || replay.Receipt.State != "succeeded" || calls.Load() != 1 {
		t.Fatal("successful provider delete redispatched")
	}
	finish(f)
	// Two executors overlap while the artificial adapter is still in flight.
	concurrent := makeFixture()
	entered := make(chan struct{})
	release := make(chan struct{})
	var concurrentCalls atomic.Int32
	cr := registry(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		concurrentCalls.Add(1)
		close(entered)
		<-release
		return payments.DeleteSucceeded, nil
	})
	type completion struct {
		result cart.PaymentExecution
		err    error
	}
	done := make(chan completion, 1)
	go func() { result, err := run(ctx, concurrent, cr); done <- completion{result, err} }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("concurrent adapter never entered")
	}
	second, secondErr := run(ctx, concurrent, cr)
	close(release)
	first := <-done
	if secondErr != nil || second.Called || second.Receipt.State != "inflight" || first.err != nil || !first.result.Called || concurrentCalls.Load() != 1 {
		t.Fatal("concurrent executors duplicated delete")
	}
	finish(concurrent)
	// Cancellation after remote success must not cancel the persistence of fact.
	canceled := makeFixture()
	callCtx, cancel := context.WithCancel(ctx)
	canceledRegistry := registry(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		cancel()
		return payments.DeleteSucceeded, nil
	})
	result, err = run(callCtx, canceled, canceledRegistry)
	if err != nil || !result.Called || result.Receipt.State != "succeeded" {
		t.Fatal("caller cancellation lost verified provider success")
	}
	finish(canceled)
	// Revocation during an already authorized call must preserve its external
	// fact without granting the old owner any fresh dispatch or cart completion.
	revoked := makeFixture()
	revokedRegistry := registry(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		exec("UPDATE cart_ownership SET generation=REPEAT('e',64) WHERE cart_id=?", revoked.ref.CartID)
		return payments.DeleteSucceeded, nil
	})
	result, err = run(ctx, revoked, revokedRegistry)
	if err != nil || !result.Called || result.Receipt.State != "succeeded" {
		t.Fatal("ownership revocation lost authorized in-flight external fact")
	}
	if replay, err := run(ctx, revoked, revokedRegistry); err != cart.ErrDenied || replay.Called {
		t.Fatal("revoked owner granted another execution")
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := cart.CompletePaymentRefresh(ctx, tx, "shop", revoked.ref, revoked.command, ring)
		return err
	}); err != cart.ErrDenied {
		t.Fatal("revoked owner completed refresh")
	}
	// All raw adapter errors override even a claimed definite failure.
	unknown := makeFixture()
	var unknownCalls atomic.Int32
	unknownRegistry := registry(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		unknownCalls.Add(1)
		return payments.DeleteFailed, errors.New("artificial-private-token")
	})
	result, err = run(ctx, unknown, unknownRegistry)
	if err != payments.ErrDeleteExecution || !result.Called || result.Receipt.State != "uncertain" {
		t.Fatal("ambiguous adapter error was retryable or leaked")
	}
	if replay, err := run(ctx, unknown, unknownRegistry); err != nil || replay.Called || replay.Receipt.State != "uncertain" || unknownCalls.Load() != 1 {
		t.Fatal("ambiguous external effect repeated")
	}
	retry := makeFixture()
	var retryCalls atomic.Int32
	retryRegistry := registry(func(_ context.Context, in payments.SessionRequest) (payments.DeleteOutcome, error) {
		if in.SessionID != retry.session {
			return payments.DeleteUncertain, errors.New("request drift")
		}
		if retryCalls.Add(1) == 1 {
			return payments.DeleteFailed, nil
		}
		return payments.DeleteSucceeded, nil
	})
	result, err = run(ctx, retry, retryRegistry)
	if err != nil || result.Receipt.State != "failed" || result.Receipt.Attempts != 1 {
		t.Fatal("known no-effect failure not recorded")
	}
	result, err = run(ctx, retry, retryRegistry)
	if err != nil || result.Receipt.State != "succeeded" || result.Receipt.Attempts != 2 || retryCalls.Load() != 2 {
		t.Fatal("known failure did not retry with stable request")
	}
	finish(retry)
	// Claim write failure must prevent calls; the next try can safely stage anew.
	blocked := makeFixture()
	var blockedCalls atomic.Int32
	blockedRegistry := registry(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		blockedCalls.Add(1)
		return payments.DeleteSucceeded, nil
	})
	exec("CREATE TRIGGER execution_claim_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial claim fault'")
	result, err = run(ctx, blocked, blockedRegistry)
	exec("DROP TRIGGER execution_claim_fault")
	if err != payments.ErrEffectStorage || result.Called || blockedCalls.Load() != 0 {
		t.Fatal("uncommitted claim called provider")
	}
	if _, err := run(ctx, blocked, blockedRegistry); err != nil || blockedCalls.Load() != 1 {
		t.Fatal("rolled-back claim could not retry")
	}
	finish(blocked)
	// The provider succeeds but local outcome persistence fails. Do not repeat it.
	lost := makeFixture()
	var lostCalls atomic.Int32
	lostRegistry := registry(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		lostCalls.Add(1)
		exec("CREATE TRIGGER execution_result_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial result fault'")
		return payments.DeleteSucceeded, nil
	})
	result, err = run(ctx, lost, lostRegistry)
	exec("DROP TRIGGER execution_result_fault")
	if err != payments.ErrEffectStorage || !result.Called || result.Receipt.State != "inflight" {
		t.Fatal("lost result persistence reported safe retry")
	}
	if replay, err := run(ctx, lost, lostRegistry); err != nil || replay.Called || replay.Receipt.State != "inflight" || lostCalls.Load() != 1 {
		t.Fatal("unrecorded success repeated provider call")
	}
	exec("UPDATE payment_effect_journal SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE scope_id=?", lost.ref.CartID)
	if replay, err := run(ctx, lost, lostRegistry); err != nil || replay.Called || replay.Receipt.State != "uncertain" || lostCalls.Load() != 1 {
		t.Fatal("expired unrecorded success redispatched")
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := cart.RecordPaymentRefreshSession(ctx, tx, "shop", lost.ref, lost.command, lost.session, result.Receipt.AttemptID, "succeeded", ring)
		return err
	}); err != nil {
		t.Fatal("verified reconciliation could not restore lost outcome")
	}
	finish(lost)
	nested := makeFixture()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result, err := cart.ExecutePaymentRefreshSession(ctx, tx, "shop", nested.ref, nested.command, nested.session, ring, r)
		if err != cart.ErrInput || result.Called {
			t.Fatal("outer transaction allowed provider work")
		}
		return nil
	}); err != nil {
		t.Fatal("nested fixture failed")
	}
	wrongRegistry, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{"other": executionDelete(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		t.Fatal("unconfigured provider adapter called")
		return payments.DeleteSucceeded, nil
	})})
	if err != nil {
		t.Fatal("unknown provider fixture failed")
	}
	if result, err := run(ctx, nested, wrongRegistry); err != payments.ErrEffectInput || result.Called {
		t.Fatal("unknown adapter consumed external attempt")
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	t.Log(`CART_PAYMENT_EXECUTION_REHEARSAL {"committed_claim_before_call":true,"no_database_transaction_during_provider":true,"concurrent_single_call":true,"verified_success_persisted_after_cancel":true,"inflight_fact_preserved_after_revocation":true,"revoked_dispatch_and_completion_denied":true,"adapter_errors_uncertain_and_redacted":true,"known_failure_stable_retry":true,"claim_failure_no_call":true,"result_storage_failure_not_retried":true,"expired_unrecorded_success_uncertain":true,"verified_reconciliation":true,"nested_transaction_denied":true,"unconfigured_provider_denied":true,"artificial_adapter_only":true,"real_provider_called":false,"compensation_integrated":false}`)
}
