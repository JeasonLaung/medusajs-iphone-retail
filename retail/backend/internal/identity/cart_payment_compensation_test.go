package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCartPaymentCompensation(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("compensation artificial SQL fixture failed")
		}
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	ring, err := protected.New("original", map[string][]byte{"original": bytes.Repeat([]byte{0x73}, 32)})
	if err != nil {
		t.Fatal("compensation ring fixture failed")
	}
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("compensation owned scope")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("compensation cart fixture failed")
	}
	ref := created.Reference
	command := cart.PaymentRefreshCommand{OperationID: uuid.NewString(), Version: "1"}
	exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES('compensation-collection','usd',10,'{"value":"10","precision":100}','not_paid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_payment_collections(id,cart_id,payment_collection_id,created_at,updated_at) VALUES('compensation-link',?,'compensation-collection',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	for _, id := range []string{"compensation-a", "compensation-b", "compensation-c"} {
		request := payments.SessionRequest{ProviderID: "artificial", CollectionID: "compensation-collection", SessionID: id, Amount: "10", Currency: "usd", Data: json.RawMessage(`{"id":"original-private-token","session_id":"old-id","mollie_attempt":"old-attempt","large":9007199254740993}`), Context: json.RawMessage(`{"idempotency_key":"old-key","private":"context"}`)}
		data, context, err := payments.SealSessionFields(ring, request)
		if err != nil {
			t.Fatal("compensation session fixture failed")
		}
		exec(`INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,context,status,payment_collection_id,created_at,updated_at) VALUES(?,'usd',10,'{"value":"10","precision":100}','artificial',?,?,'pending','compensation-collection',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, data, context)
	}
	transaction := func(fn func(*gorm.DB) error) error { return db.WithContext(ctx).Transaction(fn) }
	claimDelete := func(id string) payments.EffectReceipt {
		t.Helper()
		var receipt payments.EffectReceipt
		if err := transaction(func(tx *gorm.DB) error {
			var err error
			receipt, _, err = cart.ClaimPaymentRefreshSession(ctx, tx, "shop", ref, command, id, ring)
			return err
		}); err != nil || !receipt.Dispatch {
			t.Fatal("compensation delete fixture claim failed")
		}
		return receipt
	}
	recordDelete := func(id, attempt, outcome string) {
		t.Helper()
		if err := transaction(func(tx *gorm.DB) error {
			_, err := cart.RecordPaymentRefreshSession(ctx, tx, "shop", ref, command, id, attempt, outcome, ring)
			return err
		}); err != nil {
			t.Fatal("compensation delete fixture outcome failed")
		}
	}
	stage := func() (cart.PaymentCompensation, error) {
		var result cart.PaymentCompensation
		err := transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.StagePaymentCompensation(ctx, tx, "shop", ref, command, ring)
			return err
		})
		return result, err
	}
	complete := func() (cart.PaymentCompensation, error) {
		var result cart.PaymentCompensation
		err := transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.CompletePaymentCompensation(ctx, tx, "shop", ref, command, ring)
			return err
		})
		return result, err
	}
	first := claimDelete("compensation-a")
	second := claimDelete("compensation-b")
	recordDelete("compensation-a", first.AttemptID, "succeeded")
	recordDelete("compensation-b", second.AttemptID, "uncertain")
	if _, err := stage(); err != cart.ErrConflict {
		t.Fatal("unknown delete entered compensation")
	}
	recordDelete("compensation-b", second.AttemptID, "failed")
	exec(`INSERT INTO cart_payment_collections(id,cart_id,payment_collection_id,created_at,updated_at) VALUES('shared-compensation-link','another-cart','compensation-collection',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	if _, err := stage(); err != cart.ErrConflict {
		t.Fatal("shared collection granted compensation authority")
	}
	if err := transaction(func(tx *gorm.DB) error {
		_, _, err := cart.ClaimPaymentRefreshSession(ctx, tx, "shop", ref, command, "compensation-c", ring)
		return err
	}); err != cart.ErrConflict {
		t.Fatal("shared collection granted delete authority")
	}
	exec("UPDATE cart_payment_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE id='shared-compensation-link'")
	// Fail after create request staging; no orphan compensation request remains.
	exec("CREATE TRIGGER compensation_stage_fault BEFORE INSERT ON cart_payment_refresh_compensations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial compensation stage fault'")
	_, stageErr := stage()
	exec("DROP TRIGGER compensation_stage_fault")
	var count int64
	if db.Table("payment_effect_journal").Where("scope_id=? AND effect_kind='create_session'", ref.CartID).Count(&count).Error != nil || count != 0 || stageErr != cart.ErrStorage {
		t.Fatal("failed compensation stage left orphan creates")
	}
	operation, err := stage()
	if err != nil || operation.State != "staged" || len(operation.Replacements) != 1 || operation.Replacements[0].OriginalID != "compensation-a" || operation.Replayed {
		t.Fatalf("compensation plan failed: %v", err)
	}
	replacement := operation.Replacements[0]
	if replacement.Create.ResourceID == replacement.OriginalID {
		t.Fatal("compensation reused deleted remote session key")
	}
	if replay, err := stage(); err != nil || !replay.Replayed || replay.Replacements[0].Create != replacement.Create {
		t.Fatal("compensation retry changed stable create request")
	}
	if err := transaction(func(tx *gorm.DB) error {
		_, _, err := cart.ClaimPaymentRefreshSession(ctx, tx, "shop", ref, command, "compensation-c", ring)
		return err
	}); err != cart.ErrConflict {
		t.Fatal("compensation allowed another delete")
	}
	if err := transaction(func(tx *gorm.DB) error {
		_, err := cart.CompletePaymentRefresh(ctx, tx, "shop", ref, command, ring)
		return err
	}); err != cart.ErrConflict {
		t.Fatal("compensation allowed normal completion")
	}
	var receipt payments.EffectReceipt
	var request payments.SessionRequest
	if err := transaction(func(tx *gorm.DB) error {
		var err error
		receipt, request, err = cart.ClaimPaymentCompensationSession(ctx, tx, "shop", ref, command, "compensation-a", ring)
		return err
	}); err != nil || !receipt.Dispatch || request.SessionID != replacement.Create.ResourceID {
		t.Fatal("compensation create claim failed")
	}
	var data, context map[string]json.RawMessage
	if json.Unmarshal(request.Data, &data) != nil || json.Unmarshal(request.Context, &context) != nil || string(data["session_id"]) != `"`+request.SessionID+`"` || string(data["mollie_attempt"]) != string(data["session_id"]) || string(context["idempotency_key"]) != string(data["session_id"]) {
		t.Fatal("compensation create idempotency reused old request")
	}
	response := payments.CreatedSession{Status: "pending", Data: json.RawMessage(`{"id":"replacement-private-token","large":9007199254740993.000000000000000000001}`)}
	record := func(attempt string, r *protected.Ring, response payments.CreatedSession) error {
		return transaction(func(tx *gorm.DB) error {
			_, err := payments.RecordSessionCreation(ctx, tx, replacement.Create, attempt, r, response)
			return err
		})
	}
	if err := record(uuid.NewString(), ring, response); err != payments.ErrEffectConflict {
		t.Fatal("wrong creation attempt recorded")
	}
	if err := record(receipt.AttemptID, nil, response); err != payments.ErrEffectProtection {
		t.Fatal("missing result key accepted")
	}
	if _, err := complete(); err != payments.ErrEffectConflict {
		t.Fatal("unconfirmed creation completed compensation")
	}
	// A real paid creation is a durable external fact, but must never be
	// activated as an ordinary unpaid replacement. This artificial enclosing
	// transaction rolls back the injected paid result after asserting rejection.
	paid := response
	paid.Status = "authorized"
	if err := transaction(func(tx *gorm.DB) error {
		if _, err := payments.RecordSessionCreation(ctx, tx, replacement.Create, receipt.AttemptID, ring, paid); err != nil {
			return err
		}
		_, err := cart.CompletePaymentCompensation(ctx, tx, "shop", ref, command, ring)
		return err
	}); err != cart.ErrConflict {
		t.Fatal("paid creation activated as ordinary replacement")
	}
	// Fail result update before success: no false succeeded receipt is committed.
	exec("CREATE TRIGGER creation_result_fault BEFORE UPDATE ON payment_effect_journal FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial result fault'")
	recordErr := record(receipt.AttemptID, ring, response)
	exec("DROP TRIGGER creation_result_fault")
	if recordErr != payments.ErrEffectStorage {
		t.Fatal("result write fault ignored")
	}
	if err := transaction(func(tx *gorm.DB) error {
		saved, err := payments.InspectEffect(ctx, tx, replacement.Create)
		if err != nil {
			return err
		}
		if saved.State != "inflight" {
			t.Fatal("failed result write committed success")
		}
		return nil
	}); err != nil {
		t.Fatal("failed result receipt read failed")
	}
	if err := record(receipt.AttemptID, ring, response); err != nil {
		t.Fatalf("protected creation result failed: %v", err)
	}
	var resultRow struct{ ResultEnvelope []byte }
	if db.Table("payment_effect_journal").Select("result_envelope").Where("scope_id=? AND effect_kind='create_session'", ref.CartID).Take(&resultRow).Error != nil || bytes.Contains(resultRow.ResultEnvelope, []byte("replacement-private-token")) {
		t.Fatal("creation result stored plaintext")
	}
	if err := record(receipt.AttemptID, ring, response); err != nil {
		t.Fatal("same creation response replay failed")
	}
	changed := response
	changed.Data = json.RawMessage(`{"id":"different-provider-result"}`)
	if err := record(receipt.AttemptID, ring, changed); err != payments.ErrEffectConflict {
		t.Fatal("changed creation response overwrote success")
	}
	tampered := append([]byte{}, resultRow.ResultEnvelope...)
	tampered[len(tampered)-1] ^= 1
	exec("UPDATE payment_effect_journal SET result_envelope=? WHERE scope_id=? AND effect_kind='create_session'", tampered, ref.CartID)
	if _, err := complete(); err != payments.ErrEffectProtection {
		t.Fatal("corrupt creation result completed compensation")
	}
	exec("UPDATE payment_effect_journal SET result_envelope=? WHERE scope_id=? AND effect_kind='create_session'", resultRow.ResultEnvelope, ref.CartID)
	unchanged := func() {
		t.Helper()
		var owner struct {
			Version                   uint64
			PaymentRefreshOperationID *string
		}
		var collection struct{ CurrencyCode, Amount string }
		if db.Table("cart_ownership").Where("cart_id=?", ref.CartID).Take(&owner).Error != nil || owner.Version != 1 || owner.PaymentRefreshOperationID == nil {
			t.Fatal("partial compensation released freeze")
		}
		if db.Table("payment_sessions").Where("payment_collection_id='compensation-collection' AND deleted_at IS NULL").Count(&count).Error != nil || count != 3 {
			t.Fatal("partial compensation changed active sessions")
		}
		if db.Table("payment_collections").Select("currency_code,CAST(amount AS CHAR) AS amount").Where("id='compensation-collection'").Take(&collection).Error != nil || collection.CurrencyCode != "usd" || collection.Amount != "10.000000000000000000" {
			t.Fatal("compensation changed original collection")
		}
	}
	unchanged()
	exec("CREATE TRIGGER compensation_finish_fault BEFORE UPDATE ON cart_payment_refresh_compensations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial finish fault'")
	_, finishErr := complete()
	exec("DROP TRIGGER compensation_finish_fault")
	if finishErr != cart.ErrStorage {
		t.Fatal("compensation finish fault ignored")
	}
	unchanged()
	done, err := complete()
	if err != nil || done.State != "completed" || done.Version != "2" || done.Replayed {
		t.Fatalf("compensation completion failed: %v", err)
	}
	var restored struct {
		Data    string
		Context *string
		Status  string
	}
	if db.Table("payment_sessions").Select("data,context,status").Where("id=? AND deleted_at IS NULL", replacement.Create.ResourceID).Take(&restored).Error != nil || restored.Status != "pending" {
		t.Fatal("replacement local session not activated")
	}
	restoredRequest, err := payments.OpenSessionFields(ring, request, restored.Data, restored.Context)
	if err != nil || !bytes.Contains(restoredRequest.Data, []byte("replacement-private-token")) || !bytes.Contains(restoredRequest.Data, []byte("9007199254740993.000000000000000000001")) {
		t.Fatal("replacement session lost verified provider result")
	}
	if db.Table("payment_sessions").Where("id='compensation-a' AND deleted_at IS NOT NULL").Count(&count).Error != nil || count != 1 {
		t.Fatal("replaced original not invalidated")
	}
	if db.Table("payment_sessions").Where("id IN ('compensation-b','compensation-c') AND deleted_at IS NULL").Count(&count).Error != nil || count != 2 {
		t.Fatal("untouched originals changed")
	}
	if replay, err := complete(); err != nil || !replay.Replayed || replay.Version != "2" {
		t.Fatal("compensation completed replay failed")
	}
	if err := transaction(func(tx *gorm.DB) error { _, err := cart.LockForWrite(ctx, tx, "shop", ref, "2"); return err }); err != nil {
		t.Fatal("completed compensation freeze not released")
	}
	revoked := ref
	revoked.Generation = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := transaction(func(tx *gorm.DB) error {
		_, err := cart.CompletePaymentCompensation(ctx, tx, "shop", revoked, command, ring)
		return err
	}); err != cart.ErrDenied {
		t.Fatal("revoked compensation replay granted")
	}
	// Abort an amount/currency refresh before any provider call: no replacement
	// is needed, yet the durable operation must release its freeze exactly once.
	empty, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("empty compensation scope")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("empty compensation cart failed")
	}
	emptyCommand := cart.PaymentRefreshCommand{OperationID: uuid.NewString(), Version: "1"}
	exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES('empty-compensation-collection','usd',10,'{"value":"10","precision":100}','not_paid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_payment_collections(id,cart_id,payment_collection_id,created_at,updated_at) VALUES('empty-compensation-link',?,'empty-compensation-collection',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, empty.Reference.CartID)
	if err := transaction(func(tx *gorm.DB) error {
		_, err := cart.StagePaymentRefresh(ctx, tx, "shop", empty.Reference, emptyCommand, ring)
		return err
	}); err != nil {
		t.Fatal("empty refresh stage failed")
	}
	if err := transaction(func(tx *gorm.DB) error {
		result, err := cart.CompletePaymentCompensation(ctx, tx, "shop", empty.Reference, emptyCommand, ring)
		if err == nil && (result.State != "completed" || result.Version != "2" || len(result.Replacements) != 0) {
			t.Fatal("empty compensation created sessions")
		}
		return err
	}); err != nil {
		t.Fatal("zero-effect compensation did not finish")
	}
	if err := transaction(func(tx *gorm.DB) error {
		_, err := cart.LockForWrite(ctx, tx, "shop", empty.Reference, "2")
		return err
	}); err != nil {
		t.Fatal("empty compensation retained freeze")
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	t.Log(`CART_PAYMENT_COMPENSATION_REHEARSAL {"unknown_delete_reconciled_first":true,"shared_collection_effects_denied":true,"successful_deletes_only":true,"stable_new_session_and_idempotency":true,"stage_failure_no_orphan_create":true,"delete_and_normal_completion_blocked":true,"creation_attempt_bound":true,"encrypted_result_atomic_with_success":true,"changed_result_denied":true,"tampered_result_denied":true,"paid_creation_not_activated":true,"zero_effect_abort":true,"finish_failure_atomic":true,"replacement_session_activated":true,"original_collection_preserved":true,"untouched_sessions_preserved":true,"version_and_unfreeze_atomic":true,"completed_replay":true,"revoked_replay_denied":true,"real_provider_called":false,"create_executor_integrated":false,"checkout_complete":false}`)
}
