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

func checkContextPaymentReconciliation(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, command cart.ContextCommand, key payments.EffectKey, request payments.SessionRequest, attempt string, options cart.InitialPaymentOptions, fingerprint func() [32]byte) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context recovery fixture", err)
		}
	}
	var calls atomic.Int32
	mode := "pending"
	var during func()
	adapter := executionObserver{create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		t.Error("context recovery created payment")
		return payments.CreateUncertain, payments.CreatedSession{}, nil
	}, observe: func(callCtx context.Context, in payments.SessionRequest, id string) (payments.SessionObservation, error) {
		calls.Add(1)
		deadline, ok := callCtx.Deadline()
		var saved struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(request.Data, &saved)
		if !ok || time.Until(deadline) > 11*time.Second || id != saved.ID || in.Amount != request.Amount || in.Currency != request.Currency || in.SessionID != request.SessionID || inScope(in) != ref.CartID {
			return payments.SessionObservation{}, errors.New("wrong saved recovery identity")
		}
		lockCtx, cancel := context.WithTimeout(callCtx, 3*time.Second)
		defer cancel()
		if operation, err := cart.StageContextPayment(lockCtx, db, "shop", ref, command, options); err != nil || !operation.Replayed {
			return payments.SessionObservation{}, errors.New("recovery held cart lock")
		}
		if mode == "error" {
			return payments.SessionObservation{}, errors.New("artificial-private-recovery-error")
		}
		status, amount, remote := mode, in.Amount, id
		if mode == "amount" || mode == "identity" {
			status = "canceled"
		}
		if mode == "amount" {
			amount = "0"
		}
		if mode == "identity" {
			remote = "tr_wrong_identity"
		}
		actions := map[string]string{"open": "not_supported", "pending": "pending", "paid": "successful", "authorized": "authorized", "canceled": "canceled", "expired": "failed", "failed": "failed"}
		local := map[string]string{"open": "pending", "pending": "pending", "paid": "authorized", "authorized": "authorized", "canceled": "canceled", "expired": "error", "failed": "error"}
		raw, _ := json.Marshal(map[string]any{"id": remote, "status": status, "amount": map[string]string{"value": amount, "currency": strings.ToUpper(in.Currency)}, "metadata": map[string]string{"session_id": in.SessionID, "cart_id": inScope(in)}})
		if during != nil {
			during()
		}
		return payments.SessionObservation{PaymentID: remote, ProviderStatus: status, Action: actions[status], Amount: amount, Currency: in.Currency, Session: payments.CreatedSession{Status: local[status], Data: raw}}, nil
	}}
	registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{request.ProviderID: adapter})
	if err != nil {
		t.Fatal(err)
	}
	configured := options
	configured.Adapters = registry
	run := func(c context.Context, in cart.ContextCommand, authority sessionstore.CartReference, recoveryAttempt string) (cart.PaymentExecution, error) {
		return cart.ReconcileContextPaymentSession(c, db, "shop", authority, in, key.ResourceID, recoveryAttempt, configured)
	}
	before := fingerprint()
	checkContextCompensationStage(t, ctx, db, ref, command, key, options, fingerprint)
	if got, err := run(ctx, command, ref, uuid.NewString()); err != cart.ErrConflict || got.Called || fingerprint() != before || calls.Load() != 0 {
		t.Fatal("wrong context recovery attempt observed", err)
	}
	changed := command
	changed.Locale = "zh-CN"
	if got, err := run(ctx, changed, ref, attempt); err != cart.ErrConflict || got.Called || fingerprint() != before || calls.Load() != 0 {
		t.Fatal("changed recovery target observed", err)
	}
	wrong := ref
	wrong.Generation = strings.Repeat("a", 64)
	if got, err := run(ctx, command, wrong, attempt); err != cart.ErrDenied || got.Called || fingerprint() != before || calls.Load() != 0 {
		t.Fatal("wrong recovery authority observed", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		got, err := cart.ReconcileContextPaymentSession(ctx, tx, "shop", ref, command, key.ResourceID, attempt, configured)
		if err != cart.ErrInput || got.Called {
			t.Fatal("recovery accepted enclosing SQL transaction", err)
		}
		return nil
	}); err != nil || fingerprint() != before || calls.Load() != 0 {
		t.Fatal("nested recovery changed facts", err)
	}
	for _, state := range []string{"open", "pending", "paid", "authorized", "error", "amount", "identity"} {
		mode = state
		got, err := run(ctx, command, ref, attempt)
		if !got.Called || got.Receipt.State != "uncertain" || got.Receipt.Dispatch || fingerprint() != before {
			t.Fatal("nonterminal/invalid recovery released context", state, err)
		}
		if state == "error" && err != payments.ErrCreateExecution || (state == "amount" || state == "identity") && err == nil || (state == "open" || state == "pending" || state == "paid" || state == "authorized") && err != nil {
			t.Fatal("unexpected observation boundary", state, err)
		}
	}
	mode = "canceled"
	exec("CREATE TRIGGER context_recovery_result_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial recovery persistence fault'")
	got, err := run(ctx, command, ref, attempt)
	exec("DROP TRIGGER context_recovery_result_fault")
	if err != payments.ErrEffectStorage || !got.Called || got.Receipt.State != "uncertain" || fingerprint() != before {
		t.Fatal("recovery persistence fault lost exact uncertain attempt", err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	during = func() {
		exec("UPDATE cart_ownership SET generation=REPEAT('e',64) WHERE cart_id=?", ref.CartID)
		cancel()
	}
	got, err = run(callCtx, command, ref, attempt)
	cancel()
	if err != nil || !got.Called || got.Receipt.State != "succeeded" || got.Receipt.AttemptID != attempt || got.Receipt.Dispatch {
		t.Fatal("verified terminal observation lost after cancel/revoke", err)
	}
	if denied, err := run(ctx, command, ref, attempt); err != cart.ErrDenied || denied.Called {
		t.Fatal("revoked recovered context queried provider", err)
	}
	exec("UPDATE cart_ownership SET generation=? WHERE cart_id=?", ref.Generation, ref.CartID)
	count := calls.Load()
	if replay, err := run(ctx, command, ref, attempt); err != nil || replay.Called || replay.Receipt.State != "succeeded" || calls.Load() != count {
		t.Fatal("successful recovery repeated provider observation", err)
	}
	checkContextCompensationConfirmed(t, ctx, db, ref, command, key, options, fingerprint)
	t.Log(`CART_CONTEXT_PAYMENT_RECONCILIATION {"real_mysql":true,"read_only_observer":true,"saved_remote_identity_only":true,"exact_attempt_required":true,"current_target_and_authority_required":true,"no_sql_lock_during_observation":true,"nonterminal_and_paid_protected":true,"errors_and_mismatched_facts_refused":true,"result_persistence_failure_recoverable":true,"cancel_revoke_preserves_verified_terminal_fact":true,"successful_replay_no_get":true,"no_create_or_delete":true,"final_context_save":false}`)
}
