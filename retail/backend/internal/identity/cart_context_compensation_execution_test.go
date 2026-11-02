package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type contextCompensationAdapter struct {
	create   executionCreate
	observe  func(context.Context, payments.SessionRequest, string) (payments.SessionObservation, error)
	discover func(context.Context, payments.SessionRequest) (payments.SessionObservation, error)
}

func (a contextCompensationAdapter) CreateSession(c context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	return a.create(c, r)
}
func (a contextCompensationAdapter) ObserveSession(c context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
	return a.observe(c, r, id)
}
func (a contextCompensationAdapter) DiscoverSession(c context.Context, r payments.SessionRequest) (payments.SessionObservation, error) {
	return a.discover(c, r)
}

func checkContextCompensationExecution(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, command cart.ContextCommand, originalID string, options cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	prepare := func() payments.EffectKey {
		t.Helper()
		operation, err := cart.StageContextPaymentCompensation(ctx, db, "shop", ref, command, options)
		if err != nil || len(operation.Replacements) != 1 {
			t.Fatal("execution compensation fixture", err)
		}
		return operation.Replacements[0].Create
	}
	reset := func() payments.EffectKey {
		// Only artificial receipts in this exact owned fixture are removed.
		exec("DELETE FROM mollie_payment_references WHERE scope_id=? AND operation_id=? AND effect_kind='create_session'", ref.CartID, command.OperationID)
		exec("DELETE FROM payment_effect_journal WHERE scope_id=? AND operation_id=? AND effect_kind='create_session'", ref.CartID, command.OperationID)
		exec("DELETE FROM cart_payment_refresh_compensations WHERE cart_id=? AND operation_id=?", ref.CartID, command.OperationID)
		return prepare()
	}
	configure := func(adapter payments.CreateSessionAdapter) cart.InitialPaymentOptions {
		t.Helper()
		registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter})
		if err != nil {
			t.Fatal(err)
		}
		configured := options
		configured.Adapters = registry
		return configured
	}
	run := func(c context.Context, o cart.InitialPaymentOptions) (cart.PaymentExecution, error) {
		return cart.ExecuteContextCompensationSession(c, db, "shop", ref, command, originalID, o)
	}
	response := func(r payments.SessionRequest) payments.CreatedSession {
		return payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(r, "tr_context_compensation", "https://www.mollie.com/checkout/context")}
	}
	key := prepare()
	// This old RR snapshot must read the concurrently committed result, and
	// idempotent result recording must compare against the CURRENT envelope.
	old := db.WithContext(ctx).Begin()
	if old.Error != nil {
		t.Fatal(old.Error)
	}
	defer old.Rollback()
	var count int64
	if old.Table("payment_effect_journal").Count(&count).Error != nil {
		t.Fatal("old snapshot fixture")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var expected payments.CreatedSession
	configured := configure(executionCreate(func(callCtx context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		calls.Add(1)
		deadline, ok := callCtx.Deadline()
		if !ok || time.Until(deadline) > 21*time.Second || inScope(r) != ref.CartID || r.Amount != "158.75" || r.Currency != "eur" || r.SessionID != key.ResourceID {
			return payments.CreateUncertain, payments.CreatedSession{}, errors.New("replacement request drift")
		}
		lockCtx, cancel := context.WithTimeout(callCtx, 3*time.Second)
		defer cancel()
		operation, err := cart.StageContextPaymentCompensation(lockCtx, db, "shop", ref, command, options)
		if err != nil || !operation.Replayed {
			return payments.CreateUncertain, payments.CreatedSession{}, errors.New("claim not committed or SQL lock retained")
		}
		expected = response(r)
		close(entered)
		select {
		case <-release:
			return payments.CreateSucceeded, expected, nil
		case <-callCtx.Done():
			return payments.CreateUncertain, payments.CreatedSession{}, callCtx.Err()
		}
	}))
	type completed struct {
		result cart.PaymentExecution
		err    error
	}
	done := make(chan completed, 1)
	go func() { r, e := run(ctx, configured); done <- completed{r, e} }()
	select {
	case <-entered:
	case r := <-done:
		t.Fatal("replacement did not enter", r.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second, err := run(ctx, configured)
	close(release)
	first := <-done
	if err != nil || second.Called || second.Receipt.State != "inflight" || first.err != nil || !first.result.Called || first.result.Receipt.State != "succeeded" || calls.Load() != 1 {
		t.Fatal("concurrent replacement duplicated", err, first.err)
	}
	if _, err := payments.RecordSessionCreation(ctx, old, key, first.result.Receipt.AttemptID, options.Ring, expected); err != nil {
		t.Fatal("old RR result record was not current", err)
	}
	if created, err := payments.ReadSessionCreation(ctx, old, key, options.Ring); err != nil || string(created.Data) != string(expected.Data) {
		t.Fatal("old RR result read was not current", err)
	}
	if old.Rollback().Error != nil {
		t.Fatal("old snapshot rollback")
	}
	if replay, err := run(ctx, configured); err != nil || replay.Called || replay.Receipt.State != "succeeded" || calls.Load() != 1 {
		t.Fatal("successful replacement repeated", err)
	}

	key = reset()
	var retries atomic.Int32
	configured = configure(executionCreate(func(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		if retries.Add(1) == 1 {
			return payments.CreateFailed, payments.CreatedSession{}, nil
		}
		return payments.CreateSucceeded, response(r), nil
	}))
	if result, err := run(ctx, configured); err != nil || result.Receipt.State != "failed" {
		t.Fatal("definite replacement failure", err)
	}
	if result, err := run(ctx, configured); err != nil || result.Receipt.State != "succeeded" || result.Receipt.Attempts != 2 || retries.Load() != 2 {
		t.Fatal("replacement retry changed identity", err)
	}

	key = reset()
	var creates, queries atomic.Int32
	mode := "error"
	observe := func(callCtx context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
		queries.Add(1)
		lockCtx, cancel := context.WithTimeout(callCtx, 3*time.Second)
		defer cancel()
		if _, err := cart.StageContextPaymentCompensation(lockCtx, db, "shop", ref, command, options); err != nil {
			return payments.SessionObservation{}, err
		}
		if mode == "error" {
			return payments.SessionObservation{}, errors.New("private replacement lookup error")
		}
		amount := r.Amount
		if mode == "amount" {
			amount = "0"
		}
		raw, _ := json.Marshal(map[string]any{"id": id, "status": "open", "amount": map[string]string{"value": amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}})
		return payments.SessionObservation{PaymentID: id, ProviderStatus: "open", Action: "not_supported", Amount: amount, Currency: r.Currency, Session: payments.CreatedSession{Status: "pending", Data: raw}}, nil
	}
	adapter := contextCompensationAdapter{create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		creates.Add(1)
		return payments.CreateFailed, payments.CreatedSession{}, errors.New("private ambiguous replacement")
	}, observe: observe, discover: func(c context.Context, r payments.SessionRequest) (payments.SessionObservation, error) {
		return observe(c, r, "tr_discovered_context")
	}}
	configured = configure(adapter)
	result, err := run(ctx, configured)
	if err != payments.ErrCreateExecution || !result.Called || result.Receipt.State != "uncertain" {
		t.Fatal("ambiguous replacement was retryable", err)
	}
	if replay, err := run(ctx, configured); err != nil || replay.Called || replay.Receipt.State != "uncertain" || creates.Load() != 1 {
		t.Fatal("ambiguous replacement reposted", err)
	}
	recover := func(c context.Context, attempt, candidate string) (cart.PaymentExecution, error) {
		return cart.ReconcileContextCompensationSession(c, db, "shop", ref, command, originalID, attempt, candidate, configured)
	}
	if got, err := recover(ctx, uuid.NewString(), "tr_context_candidate"); err != cart.ErrConflict || got.Called || queries.Load() != 0 {
		t.Fatal("wrong replacement attempt queried", err)
	}
	if got, err := recover(ctx, result.Receipt.AttemptID, "tr_context_candidate"); err != payments.ErrCreateExecution || !got.Called || got.Receipt.State != "uncertain" {
		t.Fatal("replacement query error discarded", err)
	}
	mode = "amount"
	if got, err := recover(ctx, result.Receipt.AttemptID, "tr_context_candidate"); err == nil || !got.Called || got.Receipt.State != "uncertain" {
		t.Fatal("wrong replacement amount recovered", err)
	}
	mode = "pending"
	if got, err := recover(ctx, result.Receipt.AttemptID, ""); err != nil || !got.Called || got.Receipt.State != "succeeded" || creates.Load() != 1 {
		t.Fatal("lost replacement discovery failed", err)
	}
	previous := queries.Load()
	if got, err := recover(ctx, result.Receipt.AttemptID, ""); err != nil || got.Called || queries.Load() != previous {
		t.Fatal("successful replacement discovery repeated", err)
	}

	key = reset()
	var lostCalls atomic.Int32
	configured = configure(executionCreate(func(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		lostCalls.Add(1)
		exec("CREATE TRIGGER context_comp_create_result_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned replacement result fault'")
		return payments.CreateSucceeded, response(r), nil
	}))
	result, err = run(ctx, configured)
	exec("DROP TRIGGER context_comp_create_result_fault")
	if err != payments.ErrEffectStorage || !result.Called || result.Receipt.State != "inflight" {
		t.Fatal("lost replacement result falsely retryable", err)
	}
	if got, err := cart.ReconcileContextCompensationSession(ctx, db, "shop", ref, command, originalID, result.Receipt.AttemptID, "tr_context_candidate", configured); err != nil || got.Called || got.Receipt.State != "inflight" {
		t.Fatal("live replacement lease queried", err)
	}
	exec("UPDATE payment_effect_journal SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE scope_id=? AND operation_id=? AND effect_kind='create_session'", ref.CartID, command.OperationID)
	if replay, err := run(ctx, configured); err != nil || replay.Called || replay.Receipt.State != "uncertain" || lostCalls.Load() != 1 {
		t.Fatal("expired replacement reposted", err)
	}
	// No result can be manufactured to permit activation after uncertainty.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := payments.ReadSessionCreation(ctx, tx, key, options.Ring); err != payments.ErrEffectConflict {
			t.Fatal("uncertain replacement exposed create result", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if got, err := cart.ExecuteContextCompensationSession(ctx, tx, "shop", ref, command, originalID, configured); err != cart.ErrInput || got.Called {
			t.Fatal("replacement executed inside caller transaction", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Log(`CART_CONTEXT_COMPENSATION_EXECUTION {"real_mysql":true,"committed_claim_before_call":true,"outside_sql_locks":true,"concurrent_single_create":true,"old_rr_snapshot_current_result_read_and_record":true,"definite_failure_stable_retry":true,"ambiguous_create_not_reposted":true,"exact_attempt_observation":true,"query_errors_wrong_amount_refused":true,"lost_result_discovered_without_post":true,"successful_recovery_no_query":true,"lost_persistence_expiry_uncertain":true,"live_lease_no_query":true,"nested_transaction_denied":true,"artificial_provider_only":true,"replacement_activation":false}`)
}
