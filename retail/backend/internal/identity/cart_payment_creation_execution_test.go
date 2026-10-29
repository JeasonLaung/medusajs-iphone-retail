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

type executionCreate func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error)

func (f executionCreate) CreateSession(ctx context.Context, in payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	return f(ctx, in)
}

type executionObserver struct {
	create  executionCreate
	observe func(context.Context, payments.SessionRequest, string) (payments.SessionObservation, error)
}

func (a executionObserver) CreateSession(ctx context.Context, in payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	return a.create(ctx, in)
}
func (a executionObserver) ObserveSession(ctx context.Context, in payments.SessionRequest, id string) (payments.SessionObservation, error) {
	return a.observe(ctx, in, id)
}

func checkCartPaymentCreationExecution(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("creation execution artificial SQL fixture failed")
		}
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	ring, err := protected.New("artificial", map[string][]byte{"artificial": bytes.Repeat([]byte{0x29}, 32)})
	if err != nil {
		t.Fatal("creation execution ring failed")
	}
	response := payments.CreatedSession{Status: "pending", Data: json.RawMessage(`{"id":"new-private-provider-token","large":9007199254740993.000000000000000000001}`)}
	type fixture struct {
		ref        sessionstore.CartReference
		command    cart.PaymentRefreshCommand
		originalID string
	}
	tx := func(fn func(*gorm.DB) error) error { return db.WithContext(ctx).Transaction(fn) }
	makeFixture := func() fixture {
		t.Helper()
		id := uuid.NewString()
		created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: "en"})
		if err != nil {
			t.Fatal("creation execution cart failed")
		}
		f := fixture{created.Reference, cart.PaymentRefreshCommand{OperationID: uuid.NewString(), Version: "1"}, "create-source-" + id}
		collection := "create-collection-" + id
		request := payments.SessionRequest{ProviderID: "artificial", CollectionID: collection, SessionID: f.originalID, Amount: "10", Currency: "usd", Data: json.RawMessage(`{"id":"original-private-token","large":9007199254740993}`)}
		data, context, err := payments.SealSessionFields(ring, request)
		if err != nil {
			t.Fatal("creation source envelope failed")
		}
		exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES(?,'usd',10,'{"value":"10","precision":100}','not_paid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, collection)
		exec(`INSERT INTO cart_payment_collections(id,cart_id,payment_collection_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, "create-link-"+id, f.ref.CartID, collection)
		exec(`INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,context,status,payment_collection_id,created_at,updated_at) VALUES(?,'usd',10,'{"value":"10","precision":100}','artificial',?,?,'pending',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, f.originalID, data, context, collection)
		var claim payments.EffectReceipt
		if err := tx(func(tx *gorm.DB) error {
			var err error
			claim, _, err = cart.ClaimPaymentRefreshSession(ctx, tx, "shop", f.ref, f.command, f.originalID, ring)
			return err
		}); err != nil || !claim.Dispatch {
			t.Fatal("artificial original delete claim failed")
		}
		if err := tx(func(tx *gorm.DB) error {
			_, err := cart.RecordPaymentRefreshSession(ctx, tx, "shop", f.ref, f.command, f.originalID, claim.AttemptID, "succeeded", ring)
			return err
		}); err != nil {
			t.Fatal("artificial original delete outcome failed")
		}
		return f
	}
	registry := func(adapter executionCreate) *payments.CreateAdapters {
		t.Helper()
		r, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"artificial": adapter})
		if err != nil {
			t.Fatal("creation registry fixture failed")
		}
		return r
	}
	run := func(c context.Context, f fixture, r *payments.CreateAdapters) (cart.PaymentExecution, error) {
		return cart.ExecutePaymentCompensationSession(c, db, "shop", f.ref, f.command, f.originalID, ring, r)
	}
	finish := func(f fixture) {
		t.Helper()
		if err := tx(func(tx *gorm.DB) error {
			_, err := cart.CompletePaymentCompensation(ctx, tx, "shop", f.ref, f.command, ring)
			return err
		}); err != nil {
			t.Fatalf("creation compensation completion failed: %v", err)
		}
	}
	first := makeFixture()
	var calls atomic.Int32
	var stableID string
	r := registry(func(callCtx context.Context, in payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		calls.Add(1)
		stableID = in.SessionID
		deadline, ok := callCtx.Deadline()
		if !ok || time.Until(deadline) > 21*time.Second || in.SessionID == first.originalID || in.Amount != "10" {
			return payments.CreateUncertain, payments.CreatedSession{}, errors.New("invalid artificial creation facts")
		}
		lockCtx, cancel := context.WithTimeout(callCtx, 2*time.Second)
		defer cancel()
		err := db.WithContext(lockCtx).Transaction(func(tx *gorm.DB) error {
			operation, err := cart.StagePaymentCompensation(lockCtx, tx, "shop", first.ref, first.command, ring)
			if err != nil {
				return err
			}
			if !operation.Replayed {
				return errors.New("uncommitted compensation")
			}
			receipt, err := payments.InspectEffect(lockCtx, tx, operation.Replacements[0].Create)
			if err == nil && receipt.State != "inflight" {
				return errors.New("uncommitted create claim")
			}
			return err
		})
		if err != nil {
			return payments.CreateUncertain, payments.CreatedSession{}, err
		}
		return payments.CreateSucceeded, response, nil
	})
	result, err := run(ctx, first, r)
	if err != nil || !result.Called || result.Receipt.State != "succeeded" || result.Receipt.Dispatch || calls.Load() != 1 {
		t.Fatalf("committed creation execution failed: %v", err)
	}
	if replay, err := run(ctx, first, r); err != nil || replay.Called || replay.Receipt.State != "succeeded" || calls.Load() != 1 {
		t.Fatal("creation success redispatched")
	}
	finish(first)
	var local struct {
		Data    string
		Context *string
	}
	if db.Table("payment_sessions").Select("data,context").Where("id=? AND deleted_at IS NULL", stableID).Take(&local).Error != nil || bytes.Contains([]byte(local.Data), []byte("new-private-provider-token")) {
		t.Fatal("executed creation did not activate protected session")
	}
	concurrent := makeFixture()
	entered := make(chan struct{})
	release := make(chan struct{})
	var concurrentCalls atomic.Int32
	cr := registry(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		concurrentCalls.Add(1)
		close(entered)
		<-release
		return payments.CreateSucceeded, response, nil
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
		t.Fatal("concurrent creator did not enter")
	}
	replay, replayErr := run(ctx, concurrent, cr)
	close(release)
	completed := <-done
	if replayErr != nil || replay.Called || replay.Receipt.State != "inflight" || completed.err != nil || !completed.result.Called || concurrentCalls.Load() != 1 {
		t.Fatal("concurrent executors duplicated remote session")
	}
	finish(concurrent)
	canceled := makeFixture()
	callCtx, cancel := context.WithCancel(ctx)
	cancelRegistry := registry(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		cancel()
		return payments.CreateSucceeded, response, nil
	})
	if result, err := run(callCtx, canceled, cancelRegistry); err != nil || !result.Called || result.Receipt.State != "succeeded" {
		t.Fatal("cancel lost protected create result")
	}
	finish(canceled)
	revoked := makeFixture()
	revokeRegistry := registry(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		exec("UPDATE cart_ownership SET generation=REPEAT('c',64) WHERE cart_id=?", revoked.ref.CartID)
		return payments.CreateSucceeded, response, nil
	})
	if result, err := run(ctx, revoked, revokeRegistry); err != nil || !result.Called || result.Receipt.State != "succeeded" {
		t.Fatal("revocation lost authorized create fact")
	}
	if result, err := run(ctx, revoked, revokeRegistry); err != cart.ErrDenied || result.Called {
		t.Fatal("revoked owner dispatched creation")
	}
	unknown := makeFixture()
	var unknownCalls atomic.Int32
	ur := registry(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		unknownCalls.Add(1)
		return payments.CreateFailed, response, errors.New("new-private-provider-token")
	})
	if result, err := run(ctx, unknown, ur); err != payments.ErrCreateExecution || !result.Called || result.Receipt.State != "uncertain" {
		t.Fatal("unknown creation result became retryable or leaked")
	}
	if result, err := run(ctx, unknown, ur); err != nil || result.Called || result.Receipt.State != "uncertain" || unknownCalls.Load() != 1 {
		t.Fatal("unknown creation was retried")
	}
	retry := makeFixture()
	var retryCalls atomic.Int32
	var retryID string
	rr := registry(func(_ context.Context, in payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		if retryCalls.Add(1) == 1 {
			retryID = in.SessionID
			return payments.CreateFailed, payments.CreatedSession{}, nil
		}
		if in.SessionID != retryID {
			return payments.CreateUncertain, payments.CreatedSession{}, errors.New("unstable idempotency")
		}
		return payments.CreateSucceeded, response, nil
	})
	if result, err := run(ctx, retry, rr); err != nil || result.Receipt.State != "failed" {
		t.Fatal("known no-create failure not recorded")
	}
	if result, err := run(ctx, retry, rr); err != nil || result.Receipt.State != "succeeded" || result.Receipt.Attempts != 2 || retryCalls.Load() != 2 {
		t.Fatal("known failure retry changed create identity")
	}
	finish(retry)
	blocked := makeFixture()
	var blockedCalls atomic.Int32
	br := registry(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		blockedCalls.Add(1)
		return payments.CreateSucceeded, response, nil
	})
	exec("CREATE TRIGGER create_claim_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SET NEW.state=IF(NEW.state='inflight','invalid',NEW.state)")
	result, err = run(ctx, blocked, br)
	exec("DROP TRIGGER create_claim_fault")
	if err != payments.ErrEffectStorage || result.Called || blockedCalls.Load() != 0 {
		t.Fatal("uncommitted creation claim called provider")
	}
	if result, err := run(ctx, blocked, br); err != nil || !result.Called || blockedCalls.Load() != 1 {
		t.Fatal("rolled-back creation could not retry")
	}
	finish(blocked)
	lost := makeFixture()
	var lostCalls atomic.Int32
	lr := registry(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		lostCalls.Add(1)
		exec("CREATE TRIGGER create_result_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial create result fault'")
		return payments.CreateSucceeded, response, nil
	})
	result, err = run(ctx, lost, lr)
	exec("DROP TRIGGER create_result_fault")
	if err != payments.ErrEffectStorage || !result.Called || result.Receipt.State != "inflight" {
		t.Fatal("lost creation result reported safe retry")
	}
	attempt := result.Receipt.AttemptID
	exec("UPDATE payment_effect_journal SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE scope_id=? AND effect_kind='create_session'", lost.ref.CartID)
	if result, err := run(ctx, lost, lr); err != nil || result.Called || result.Receipt.State != "uncertain" || lostCalls.Load() != 1 {
		t.Fatal("lost create result repeated remote creation")
	}
	if err := tx(func(tx *gorm.DB) error {
		operation, err := cart.StagePaymentCompensation(ctx, tx, "shop", lost.ref, lost.command, ring)
		if err != nil {
			return err
		}
		_, err = payments.RecordSessionCreation(ctx, tx, operation.Replacements[0].Create, attempt, ring, response)
		return err
	}); err != nil {
		t.Fatal("verified creation reconciliation failed")
	}
	finish(lost)
	// Recover the same uncertain attempt through authoritative GET, never POST.
	recovering := makeFixture()
	var observerCalls, recoverCreateCalls atomic.Int32
	observerFault := false
	observer := executionObserver{
		create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			recoverCreateCalls.Add(1)
			return payments.CreateUncertain, payments.CreatedSession{}, errors.New("private create timeout")
		},
		observe: func(c context.Context, request payments.SessionRequest, candidate string) (payments.SessionObservation, error) {
			observerCalls.Add(1)
			if candidate != "artificial-candidate" || request.SessionID == recovering.originalID {
				return payments.SessionObservation{}, errors.New("wrong saved identity")
			}
			if err := db.WithContext(c).Transaction(func(tx *gorm.DB) error {
				_, err := cart.StagePaymentCompensation(c, tx, "shop", recovering.ref, recovering.command, ring)
				return err
			}); err != nil {
				return payments.SessionObservation{}, err
			}
			if observerFault {
				return payments.SessionObservation{}, errors.New("private lookup error")
			}
			return payments.SessionObservation{Session: response}, nil
		},
	}
	or, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"artificial": observer})
	if err != nil {
		t.Fatal("recovery registry failed")
	}
	result, err = run(ctx, recovering, or)
	if err != payments.ErrCreateExecution || result.Receipt.State != "uncertain" {
		t.Fatal("recovery fixture did not become uncertain")
	}
	recoverAttempt := result.Receipt.AttemptID
	reconcile := func(c context.Context, attempt string) (cart.PaymentExecution, error) {
		return cart.ReconcilePaymentCompensationSession(c, db, "shop", recovering.ref, recovering.command, recovering.originalID, attempt, "artificial-candidate", ring, or)
	}
	if result, err := reconcile(ctx, uuid.NewString()); err != cart.ErrConflict || result.Called || observerCalls.Load() != 0 {
		t.Fatal("stale attempt queried provider")
	}
	if result, err := cart.ReconcilePaymentCompensationSession(ctx, db, "shop", recovering.ref, recovering.command, recovering.originalID, recoverAttempt, "artificial-candidate", ring, r); err != payments.ErrEffectInput || result.Called || observerCalls.Load() != 0 {
		t.Fatal("non-observer adapter queried provider")
	}
	observerFault = true
	if result, err := reconcile(ctx, recoverAttempt); err != payments.ErrCreateExecution || !result.Called || result.Receipt.State != "uncertain" {
		t.Fatal("failed lookup cleared uncertainty")
	}
	observerFault = false
	if result, err := reconcile(ctx, recoverAttempt); err != nil || !result.Called || result.Receipt.State != "succeeded" || result.Receipt.Attempts != 1 {
		t.Fatal("verified observation did not recover original attempt")
	}
	if result, err := reconcile(ctx, recoverAttempt); err != nil || result.Called || result.Receipt.State != "succeeded" || observerCalls.Load() != 2 || recoverCreateCalls.Load() != 1 {
		t.Fatal("reconciliation replay re-created or re-queried")
	}
	finish(recovering)
	live := makeFixture()
	var liveClaim payments.EffectReceipt
	if err := tx(func(tx *gorm.DB) error {
		var err error
		liveClaim, _, err = cart.ClaimPaymentCompensationSession(ctx, tx, "shop", live.ref, live.command, live.originalID, ring)
		return err
	}); err != nil || !liveClaim.Dispatch {
		t.Fatal("live observation fixture failed")
	}
	if result, err := cart.ReconcilePaymentCompensationSession(ctx, db, "shop", live.ref, live.command, live.originalID, liveClaim.AttemptID, "artificial-candidate", ring, or); err != nil || result.Called || result.Receipt.State != "inflight" || observerCalls.Load() != 2 {
		t.Fatal("live create lease was queried or retried")
	}
	exec("UPDATE payment_effect_journal SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE scope_id=? AND effect_kind='create_session'", live.ref.CartID)
	cancelCtx, cancelObserve := context.WithCancel(ctx)
	observer.observe = func(c context.Context, request payments.SessionRequest, candidate string) (payments.SessionObservation, error) {
		observerCalls.Add(1)
		if err := db.WithContext(c).Transaction(func(tx *gorm.DB) error {
			_, err := cart.StagePaymentCompensation(c, tx, "shop", live.ref, live.command, ring)
			return err
		}); err != nil {
			return payments.SessionObservation{}, err
		}
		cancelObserve()
		return payments.SessionObservation{Session: response}, nil
	}
	liveRegistry, _ := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"artificial": observer})
	if result, err := cart.ReconcilePaymentCompensationSession(cancelCtx, db, "shop", live.ref, live.command, live.originalID, liveClaim.AttemptID, "artificial-candidate", ring, liveRegistry); err != nil || !result.Called || result.Receipt.State != "succeeded" || result.Receipt.Attempts != 1 || recoverCreateCalls.Load() != 1 {
		t.Fatal("expired lease lookup lost verified result after cancellation")
	}
	finish(live)
	if err := tx(func(tx *gorm.DB) error {
		result, err := cart.ReconcilePaymentCompensationSession(ctx, tx, "shop", live.ref, live.command, live.originalID, liveClaim.AttemptID, "artificial-candidate", ring, or)
		if err != cart.ErrInput || result.Called {
			t.Fatal("observer accepted enclosing transaction")
		}
		return nil
	}); err != nil {
		t.Fatal("observer nested fixture failed")
	}

	// A verified paid recovery is a fact, never an unpaid replacement.
	observedPaid := makeFixture()
	paidObserver := observer
	paidObserver.observe = func(context.Context, payments.SessionRequest, string) (payments.SessionObservation, error) {
		p := response
		p.Status = "authorized"
		return payments.SessionObservation{Session: p}, nil
	}
	paidRegistry, _ := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"artificial": paidObserver})
	result, err = run(ctx, observedPaid, paidRegistry)
	if err != payments.ErrCreateExecution {
		t.Fatal("paid observer fixture not uncertain")
	}
	paidAttempt := result.Receipt.AttemptID
	wrongRef := observedPaid.ref
	wrongRef.Generation = uuid.NewString()
	if result, err := cart.ReconcilePaymentCompensationSession(ctx, db, "shop", wrongRef, observedPaid.command, observedPaid.originalID, paidAttempt, "artificial-candidate", ring, paidRegistry); err == nil || result.Called {
		t.Fatal("wrong generation could reconcile")
	}
	exec("CREATE TRIGGER observe_result_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial observation fault'")
	result, err = cart.ReconcilePaymentCompensationSession(ctx, db, "shop", observedPaid.ref, observedPaid.command, observedPaid.originalID, paidAttempt, "artificial-candidate", ring, paidRegistry)
	exec("DROP TRIGGER observe_result_fault")
	if err != payments.ErrEffectStorage || !result.Called || result.Receipt.State != "uncertain" {
		t.Fatal("failed observation write reported success or safe creation retry")
	}
	if result, err := cart.ReconcilePaymentCompensationSession(ctx, db, "shop", observedPaid.ref, observedPaid.command, observedPaid.originalID, paidAttempt, "artificial-candidate", ring, paidRegistry); err != nil || result.Receipt.State != "succeeded" || result.Receipt.Attempts != 1 {
		t.Fatal("verified paid recovery did not persist original attempt")
	}
	if err := tx(func(tx *gorm.DB) error {
		_, err := cart.CompletePaymentCompensation(ctx, tx, "shop", observedPaid.ref, observedPaid.command, ring)
		return err
	}); err != cart.ErrConflict {
		t.Fatal("paid observation activated unpaid compensation")
	}

	paid := makeFixture()
	paidResponse := response
	paidResponse.Status = "authorized"
	pr := registry(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		return payments.CreateSucceeded, paidResponse, nil
	})
	if result, err := run(ctx, paid, pr); err != nil || result.Receipt.State != "succeeded" {
		t.Fatal("paid creation external fact not saved")
	}
	if err := tx(func(tx *gorm.DB) error {
		_, err := cart.CompletePaymentCompensation(ctx, tx, "shop", paid.ref, paid.command, ring)
		return err
	}); err != cart.ErrConflict {
		t.Fatal("paid session activated as ordinary compensation")
	}
	nested := makeFixture()
	if err := tx(func(tx *gorm.DB) error {
		result, err := cart.ExecutePaymentCompensationSession(ctx, tx, "shop", nested.ref, nested.command, nested.originalID, ring, r)
		if err != cart.ErrInput || result.Called {
			t.Fatal("outer transaction allowed creation")
		}
		return nil
	}); err != nil {
		t.Fatal("nested creator fixture failed")
	}
	wrong, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"other": executionCreate(func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		t.Fatal("unknown creator called")
		return payments.CreateSucceeded, response, nil
	})})
	if err != nil {
		t.Fatal("unknown creator registry failed")
	}
	if result, err := run(ctx, nested, wrong); err != payments.ErrEffectInput || result.Called {
		t.Fatal("unconfigured creator consumed attempt")
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	t.Log(`CART_PAYMENT_CREATION_EXECUTION_REHEARSAL {"claim_committed_before_create":true,"provider_outside_sql_transaction":true,"concurrent_single_create":true,"encrypted_result_then_compensation_activation":true,"success_replay_no_call":true,"cancel_preserves_result":true,"revocation_preserves_fact_no_dispatch":true,"errors_uncertain_redacted":true,"known_failure_stable_idempotency_retry":true,"claim_fault_no_call":true,"lost_result_not_retried":true,"verified_result_reconciliation":true,"observer_recovery_same_attempt":true,"live_lease_not_observed":true,"expired_lease_observed_without_create":true,"observer_outside_sql":true,"observer_error_preserves_uncertainty":true,"observer_stale_attempt_denied":true,"unconfigured_observer_denied":true,"observer_cancel_preserves_fact":true,"observer_replay_no_call":true,"observer_paid_fact_not_activated":true,"observer_result_fault_preserves_uncertainty":true,"observer_wrong_generation_denied":true,"paid_result_not_activated":true,"outer_transaction_denied":true,"unconfigured_creator_denied":true,"artificial_adapter_only":true,"real_provider_called":false,"checkout_complete":false}`)
}
