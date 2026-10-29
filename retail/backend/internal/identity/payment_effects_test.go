package identity_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkPaymentEffects(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	key := payments.EffectKey{ScopeID: "artificial-cart", OperationID: uuid.NewString(), Kind: "delete_session", ResourceID: "artificial-session", Digest: sha256.Sum256([]byte("artificial immutable outbound request"))}
	run := func(fn func(*gorm.DB) (payments.EffectReceipt, error)) (payments.EffectReceipt, error) {
		var r payments.EffectReceipt
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; r, err = fn(tx); return err })
		return r, err
	}
	stage := func(k payments.EffectKey) (payments.EffectReceipt, error) {
		return run(func(tx *gorm.DB) (payments.EffectReceipt, error) { return payments.StageEffect(ctx, tx, k) })
	}
	claim := func(k payments.EffectKey) (payments.EffectReceipt, error) {
		return run(func(tx *gorm.DB) (payments.EffectReceipt, error) {
			return payments.ClaimEffect(ctx, tx, k, 30*time.Second)
		})
	}
	record := func(attempt, outcome string) (payments.EffectReceipt, error) {
		return run(func(tx *gorm.DB) (payments.EffectReceipt, error) {
			return payments.RecordEffect(ctx, tx, key, attempt, outcome)
		})
	}
	if _, err := payments.StageEffect(ctx, db, key); err != payments.ErrEffectInput {
		t.Fatal("autocommit effect stage accepted")
	}
	for _, bad := range []payments.EffectKey{{}, {ScopeID: "scope", OperationID: uuid.NewString(), Kind: "capture", ResourceID: "session", Digest: key.Digest}} {
		if _, err := stage(bad); err != payments.ErrEffectInput {
			t.Fatal("invalid effect accepted")
		}
	}
	first, err := stage(key)
	if err != nil || first.State != "pending" || first.Dispatch || first.Attempts != 0 {
		t.Fatal("effect stage failed")
	}
	changed := key
	changed.Digest = sha256.Sum256([]byte("changed provider payload"))
	if _, err := stage(changed); err != payments.ErrEffectConflict {
		t.Fatal("changed effect binding accepted")
	}
	if _, err := claim(changed); err != payments.ErrEffectConflict {
		t.Fatal("changed effect dispatched")
	}
	// Separate real transactions contend on one durable row, not a Go mutex.
	var wg sync.WaitGroup
	receipts := make(chan payments.EffectReceipt, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := claim(key)
			if err != nil {
				failures <- err
				return
			}
			receipts <- r
		}()
	}
	wg.Wait()
	close(receipts)
	close(failures)
	for err := range failures {
		t.Fatalf("concurrent effect claim failed: %v", err)
	}
	var winner payments.EffectReceipt
	dispatched := 0
	for r := range receipts {
		if r.Dispatch {
			dispatched++
			winner = r
		}
		if r.State != "inflight" || r.Attempts != 1 {
			t.Fatal("concurrent claim changed attempt")
		}
	}
	if dispatched != 1 {
		t.Fatal("provider effect granted multiple dispatches")
	}
	if r, err := record(winner.AttemptID, "failed"); err != nil || r.State != "failed" || r.Dispatch {
		t.Fatal("definitive failure not recorded")
	}
	retry, err := claim(key)
	if err != nil || !retry.Dispatch || retry.Attempts != 2 || retry.AttemptID == winner.AttemptID {
		t.Fatal("known failure did not get fenced retry")
	}
	if _, err := record(winner.AttemptID, "succeeded"); err != payments.ErrEffectConflict {
		t.Fatal("stale response overwrote new attempt")
	}
	if r, err := record(retry.AttemptID, "uncertain"); err != nil || r.State != "uncertain" {
		t.Fatal("ambiguous response lost")
	}
	if r, err := claim(key); err != nil || r.Dispatch || r.Attempts != 2 || r.State != "uncertain" {
		t.Fatal("ambiguous provider effect automatically retried")
	}
	if r, err := record(retry.AttemptID, "succeeded"); err != nil || r.State != "succeeded" {
		t.Fatal("late definitive response not reconciled")
	}
	if r, err := record(retry.AttemptID, "succeeded"); err != nil || r.State != "succeeded" {
		t.Fatal("definitive response replay failed")
	}
	if _, err := record(retry.AttemptID, "failed"); err != payments.ErrEffectConflict {
		t.Fatal("success overwritten")
	}
	if r, err := claim(key); err != nil || r.Dispatch || r.State != "succeeded" {
		t.Fatal("success redispatched")
	}
	// A process crash leaves inflight. Expiry records uncertainty, not a new lease.
	crash := key
	crash.ResourceID = "crashed-session"
	if _, err := stage(crash); err != nil {
		t.Fatal("crash fixture stage failed")
	}
	original, err := claim(crash)
	if err != nil || !original.Dispatch {
		t.Fatal("crash fixture claim failed")
	}
	if db.Exec("UPDATE payment_effect_journal SET lease_until=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE resource_id=?", crash.ResourceID).Error != nil {
		t.Fatal("crash fixture expiry failed")
	}
	if r, err := claim(crash); err != nil || r.Dispatch || r.State != "uncertain" || r.AttemptID != original.AttemptID || r.Attempts != 1 {
		t.Fatal("expired lease redispatched provider effect")
	}
	// Claim visibility/dispatch is contingent on commit; rollback leaves pending.
	rolled := key
	rolled.ResourceID = "rolled-back-session"
	if _, err := stage(rolled); err != nil {
		t.Fatal("rollback fixture stage failed")
	}
	sentinel := errors.New("artificial later failure")
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, err := payments.ClaimEffect(ctx, tx, rolled, 30*time.Second)
		if err != nil || !r.Dispatch {
			t.Fatal("transactional claim failed")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal("caller rollback failed")
	}
	if r, err := stage(rolled); err != nil || r.State != "pending" || r.Attempts != 0 {
		t.Fatal("rolled back claim became durable")
	}
	create := key
	create.Kind = "create_session"
	if r, err := stage(create); err != nil || r.State != "pending" {
		t.Fatal("compensation effect collided with delete effect")
	}
	if db.Exec("UPDATE payment_runtime_schema SET reads_enabled=FALSE WHERE id=1").Error != nil {
		t.Fatal("effect schema fixture failed")
	}
	if _, err := claim(key); err != payments.ErrSchema {
		t.Fatal("disabled schema replay accepted")
	}
	if db.Exec("UPDATE payment_runtime_schema SET reads_enabled=TRUE WHERE id=1").Error != nil {
		t.Fatal("effect schema restore failed")
	}
	t.Log(`PAYMENT_EFFECTS_REHEARSAL {"concurrent_claims":8,"single_dispatch":1,"immutable_request":true,"known_failure_retry":true,"stale_attempt_fenced":true,"uncertain_not_retried":true,"lease_expiry_uncertain":true,"late_response_reconciliation":true,"success_replay":true,"caller_rollback":true,"create_delete_separate":true,"schema_and_transaction_required":true,"provider_called":false,"coordinator_complete":false}`)
}
