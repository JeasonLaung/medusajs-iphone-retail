package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkPaymentRequests(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	old := bytes.Repeat([]byte{0x19}, 32)
	fresh := bytes.Repeat([]byte{0x42}, 32)
	ring, err := protected.New("original", map[string][]byte{"original": old})
	if err != nil {
		t.Fatal("artificial payment ring failed")
	}
	rotated, err := protected.New("rotated", map[string][]byte{"original": old, "rotated": fresh})
	if err != nil {
		t.Fatal("artificial rotated ring failed")
	}
	unavailable, err := protected.New("rotated", map[string][]byte{"rotated": fresh})
	if err != nil {
		t.Fatal("artificial missing-key ring failed")
	}
	key := payments.EffectKey{ScopeID: "request-cart", OperationID: uuid.NewString(), Kind: "delete_session", ResourceID: "source-session"}
	in := payments.SessionRequest{ProviderID: "pp_paypal_paypal", CollectionID: "source-collection", SessionID: "source-session", Amount: "9007199254740993.000000000000000000001", Currency: "eur", Data: json.RawMessage(`{"id":"artificial-private-provider-token","large":9007199254740993,"fraction":1.000000000000000000001}`), Context: json.RawMessage(`{"artificial":"private-context"}`)}
	stage := func(k payments.EffectKey, r *protected.Ring, request payments.SessionRequest) (payments.EffectKey, payments.EffectReceipt, error) {
		var saved payments.EffectKey
		var receipt payments.EffectReceipt
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			saved, receipt, err = payments.StageSessionRequest(ctx, tx, k, r, request)
			return err
		})
		return saved, receipt, err
	}
	claim := func(k payments.EffectKey, r *protected.Ring) (payments.EffectReceipt, payments.SessionRequest, error) {
		var receipt payments.EffectReceipt
		var request payments.SessionRequest
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			receipt, request, err = payments.ClaimSessionRequest(ctx, tx, k, r, 30*time.Second)
			return err
		})
		return receipt, request, err
	}
	blob := func(k payments.EffectKey) []byte {
		t.Helper()
		var row struct{ RequestEnvelope []byte }
		if db.Table("payment_effect_journal").Select("request_envelope").Where("scope_id=? AND operation_id=? AND effect_kind=? AND resource_id=?", k.ScopeID, k.OperationID, k.Kind, k.ResourceID).Take(&row).Error != nil {
			t.Fatal("request envelope read failed")
		}
		return row.RequestEnvelope
	}
	before := func() int64 {
		var n int64
		if db.Table("payment_effect_journal").Count(&n).Error != nil {
			t.Fatal("request count unavailable")
		}
		return n
	}
	count := before()
	if k, r, err := stage(key, nil, in); err != payments.ErrEffectProtection || k != (payments.EffectKey{}) || r.State != "" || before() != count {
		t.Fatal("missing key staged partial request")
	}
	key, receipt, err := stage(key, ring, in)
	if err != nil || key.Digest == ([32]byte{}) || receipt.State != "pending" || receipt.Dispatch {
		t.Fatal("encrypted provider request staging failed")
	}
	original := blob(key)
	if bytes.Contains(original, []byte("artificial-private-provider-token")) || bytes.Contains(original, []byte("private-context")) || bytes.Contains(original, []byte("source-collection")) {
		t.Fatal("payment recovery snapshot stored plaintext")
	}
	if k, r, err := stage(key, rotated, in); err != nil || k != key || r.State != "pending" || !bytes.Equal(original, blob(key)) {
		t.Fatal("rotated replay changed original envelope")
	}
	changed := in
	changed.Data = json.RawMessage(`{"id":"different-provider-token"}`)
	if _, _, err := stage(key, ring, changed); err != payments.ErrEffectConflict {
		t.Fatal("changed provider payload reused bound request")
	}
	wrongDigest := key
	wrongDigest.Digest[0] ^= 1
	if _, _, err := claim(wrongDigest, ring); err != payments.ErrEffectConflict {
		t.Fatal("wrong request digest read")
	}
	// No decryption key, no claim: missing/wrong/corrupt data never advances state.
	if r, v, err := claim(key, unavailable); err != payments.ErrEffectProtection || r.State != "" || v.SessionID != "" {
		t.Fatal("missing decryption key granted dispatch")
	}
	if r, v, err := claim(key, nil); err != payments.ErrEffectProtection || r.State != "" || v.SessionID != "" {
		t.Fatal("nil decryption ring granted dispatch")
	}
	tampered := append([]byte{}, original...)
	tampered[len(tampered)-1] ^= 1
	write := func(k payments.EffectKey, value []byte) {
		t.Helper()
		if db.Table("payment_effect_journal").Where("scope_id=? AND operation_id=? AND effect_kind=? AND resource_id=?", k.ScopeID, k.OperationID, k.Kind, k.ResourceID).Update("request_envelope", value).Error != nil {
			t.Fatal("artificial envelope mutation failed")
		}
	}
	write(key, tampered)
	if _, _, err := claim(key, ring); err != payments.ErrEffectProtection {
		t.Fatal("tampered payment snapshot granted dispatch")
	}
	if _, _, err := stage(key, ring, in); err != payments.ErrEffectProtection {
		t.Fatal("replay silently repaired corrupt payment snapshot")
	}
	write(key, original)
	// Same plaintext/digest at a second key must still have different authenticated context.
	sibling := key
	sibling.OperationID = uuid.NewString()
	sibling, _, err = stage(sibling, ring, in)
	if err != nil {
		t.Fatal("sibling request stage failed")
	}
	siblingEnvelope := blob(sibling)
	write(sibling, original)
	if _, _, err := claim(sibling, ring); err != payments.ErrEffectProtection {
		t.Fatal("cross-operation envelope substitution granted dispatch")
	}
	write(sibling, siblingEnvelope)
	receipt, restored, err := claim(key, rotated)
	if err != nil || !receipt.Dispatch || receipt.Attempts != 1 || restored.ProviderID != in.ProviderID || restored.CollectionID != in.CollectionID || restored.SessionID != in.SessionID || restored.Amount != in.Amount || restored.Currency != in.Currency || !bytes.Equal(restored.Data, in.Data) || !bytes.Equal(restored.Context, in.Context) {
		t.Fatal("restored provider request lost exact snapshot or advanced on failed decrypt")
	}
	// Mimic a restarted worker: use a new transaction and no caller input payload.
	replay, restoredAgain, err := claim(key, ring)
	if err != nil || replay.Dispatch || replay.AttemptID != receipt.AttemptID || !bytes.Equal(restoredAgain.Data, in.Data) {
		t.Fatal("restart did not recover same provider request")
	}
	var recordErr error
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, recordErr = payments.RecordEffect(ctx, tx, key, receipt.AttemptID, "uncertain")
		return recordErr
	}); err != nil {
		t.Fatal("request uncertainty record failed")
	}
	if r, v, err := claim(key, rotated); err != nil || r.Dispatch || r.State != "uncertain" || !bytes.Equal(v.Data, in.Data) {
		t.Fatal("reconciliation could not recover private request")
	}
	// A receipt made by the low-level API has no snapshot and cannot dispatch via this API.
	legacy := key
	legacy.ResourceID = "receipt-without-request"
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := payments.StageEffect(ctx, tx, legacy); return err }); err != nil {
		t.Fatal("bare receipt fixture failed")
	}
	if _, _, err := claim(legacy, ring); err != payments.ErrEffectProtection {
		t.Fatal("bare receipt dispatched as recoverable request")
	}
	// Snapshot update failure rolls back the fresh receipt too.
	failure := key
	failure.OperationID = uuid.NewString()
	failure.Digest = [32]byte{}
	count = before()
	if db.Exec("CREATE TRIGGER payment_request_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial request write fault'").Error != nil {
		t.Fatal("request fault fixture unavailable")
	}
	_, _, err = stage(failure, ring, in)
	if db.Exec("DROP TRIGGER payment_request_fault").Error != nil {
		t.Fatal("request fault fixture cleanup failed")
	}
	if err != payments.ErrEffectStorage || before() != count {
		t.Fatal("failed snapshot write committed orphan receipt")
	}
	rollback := key
	rollback.OperationID = uuid.NewString()
	rollback.Digest = [32]byte{}
	sentinel := errors.New("artificial later command failure")
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, _, err := payments.StageSessionRequest(ctx, tx, rollback, ring, in)
		if err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || before() != count {
		t.Fatal("later command rollback retained payment request")
	}
	t.Log(`PAYMENT_REQUESTS_REHEARSAL {"encrypted_at_rest":true,"exact_amount_and_json_numbers":true,"immutable_request":true,"key_rotation_retains_ciphertext":true,"missing_key_no_dispatch":true,"tamper_no_repair":true,"cross_operation_swap_denied":true,"fresh_transaction_recovery":true,"uncertain_read_without_dispatch":true,"missing_snapshot_no_dispatch":true,"snapshot_write_failure_atomic":true,"later_rollback_atomic":true,"provider_called":false,"coordinator_complete":false}`)
}
