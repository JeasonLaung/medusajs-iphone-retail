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
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCartPaymentOperations(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("artificial payment operation fixture failed")
		}
	}
	ring, err := protected.New("artificial", map[string][]byte{"artificial": bytes.Repeat([]byte{0x64}, 32)})
	if err != nil {
		t.Fatal("operation ring fixture failed")
	}
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("artificial payment operation owner")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("operation owned cart fixture failed")
	}
	ref := created.Reference
	command := cart.PaymentRefreshCommand{OperationID: uuid.NewString(), Version: "1"}
	stage := func(reference sessionstore.CartReference, in cart.PaymentRefreshCommand, r *protected.Ring) (cart.PaymentRefreshOperation, error) {
		var result cart.PaymentRefreshOperation
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.StagePaymentRefresh(ctx, tx, "shop", reference, in, r)
			return err
		})
		return result, err
	}
	claim := func(session string) (payments.EffectReceipt, error) {
		var receipt payments.EffectReceipt
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var request payments.SessionRequest
			var err error
			receipt, request, err = cart.ClaimPaymentRefreshSession(ctx, tx, "shop", ref, command, session, ring)
			if err == nil && (request.ProviderID != "artificial" || request.Amount != "10" || !bytes.Contains(request.Data, []byte("private-provider-token"))) {
				t.Fatal("claimed provider request lost source snapshot")
			}
			return err
		})
		return receipt, err
	}
	record := func(session, attempt, outcome string) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, err := cart.RecordPaymentRefreshSession(ctx, tx, "shop", ref, command, session, attempt, outcome, ring)
			return err
		})
	}
	complete := func() (cart.PaymentRefreshOperation, error) {
		var result cart.PaymentRefreshOperation
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.CompletePaymentRefresh(ctx, tx, "shop", ref, command, ring)
			return err
		})
		return result, err
	}
	if _, err := cart.StagePaymentRefresh(ctx, db, "shop", ref, command, ring); err != cart.ErrInput {
		t.Fatal("autocommit payment operation accepted")
	}
	if _, err := stage(ref, command, ring); err != cart.ErrSchema {
		t.Fatal("disabled operation capability accepted")
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	noop, err := stage(ref, command, ring)
	if err != nil || noop.State != "completed" || noop.Version != "1" || noop.Plan.UpdateCollection || noop.Replayed || len(noop.Effects) != 0 {
		t.Fatal("missing collection no-op failed")
	}
	if replay, err := stage(ref, command, ring); err != nil || !replay.Replayed || replay.Version != "1" {
		t.Fatal("no-op receipt replay failed")
	}
	command.OperationID = uuid.NewString()
	exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) VALUES('operation-collection','usd',10,'{"value":"10","precision":100}','not_paid',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_payment_collections(id,cart_id,payment_collection_id,created_at,updated_at) VALUES('operation-link',?,'operation-collection',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	for _, id := range []string{"operation-session-a", "operation-session-b"} {
		input := payments.SessionRequest{ProviderID: "artificial", CollectionID: "operation-collection", SessionID: id, Amount: "10", Currency: "usd", Data: json.RawMessage(`{"id":"private-provider-token","large":9007199254740993}`), Context: json.RawMessage(`{"private":"recovery-context"}`)}
		data, context, err := payments.SealSessionFields(ring, input)
		if err != nil {
			t.Fatal("source runtime envelope fixture failed")
		}
		exec(`INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,context,status,payment_collection_id,created_at,updated_at) VALUES(?,'usd',10,'{"value":"10","precision":100}','artificial',?,?,'pending','operation-collection',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, data, context)
	}
	// Paid source state is rejected before staging any external delete actions.
	exec("UPDATE payment_sessions SET status='authorized' WHERE id='operation-session-a'")
	if _, err := stage(ref, command, ring); err != cart.ErrConflict {
		t.Fatal("authorized session staged for delete")
	}
	exec("UPDATE payment_sessions SET status='pending' WHERE id='operation-session-a'")
	exec("UPDATE payment_collections SET captured_amount=1,raw_captured_amount='{\"value\":\"1\",\"precision\":100}' WHERE id='operation-collection'")
	if _, err := stage(ref, command, ring); err != cart.ErrConflict {
		t.Fatal("captured amount staged for replacement")
	}
	exec("UPDATE payment_collections SET captured_amount=NULL,raw_captured_amount=NULL WHERE id='operation-collection'")
	exec(`INSERT INTO payments(id,amount,raw_amount,currency_code,provider_id,payment_collection_id,payment_session_id,created_at,updated_at) VALUES('operation-existing-payment',10,'{"value":"10","precision":100}','usd','artificial','operation-collection','operation-session-a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	if _, err := stage(ref, command, ring); err != cart.ErrConflict {
		t.Fatal("existing payment record staged for session deletion")
	}
	exec("UPDATE payments SET deleted_at=UTC_TIMESTAMP(6) WHERE id='operation-existing-payment'")
	if _, err := stage(ref, command, nil); err != payments.ErrEffectProtection {
		t.Fatal("missing business key staged provider requests")
	}
	// Fail the manifest insert after session request staging: all receipts roll back.
	exec("CREATE TRIGGER payment_operation_stage_fault BEFORE INSERT ON cart_payment_refresh_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial manifest fault'")
	_, stageErr := stage(ref, command, ring)
	exec("DROP TRIGGER payment_operation_stage_fault")
	var count int64
	if db.Table("payment_effect_journal").Where("scope_id=?", ref.CartID).Count(&count).Error != nil || count != 0 || stageErr != cart.ErrStorage {
		t.Fatal("manifest failure retained orphan effects")
	}
	staged, err := stage(ref, command, ring)
	if err != nil || staged.State != "staged" || staged.Version != "1" || staged.Replayed || len(staged.Effects) != 2 || staged.Plan.Amount != "0" || staged.Plan.Currency != "eur" {
		t.Fatalf("payment operation staging failed: %v", err)
	}
	if replay, err := stage(ref, command, ring); err != nil || !replay.Replayed || len(replay.Effects) != 2 {
		t.Fatalf("staged operation replay failed: %v", err)
	}
	if snapshot, err := cart.Inspect(ctx, db, "shop", ref); err != nil || snapshot.Version != "1" {
		t.Fatal("frozen cart became unreadable")
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := cart.LockForWrite(ctx, tx, "shop", ref, "1"); return err }); err != cart.ErrConflict {
		t.Fatal("ordinary write bypassed frozen operation")
	}
	exec("UPDATE cart_item_runtime SET enabled=TRUE WHERE id=1")
	_, itemErr := cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "unavailable", Quantity: "1"})
	exec("UPDATE cart_item_runtime SET enabled=FALSE WHERE id=1")
	if itemErr != cart.ErrConflict {
		t.Fatal("item mutation bypassed payment freeze")
	}
	other := command
	other.OperationID = uuid.NewString()
	if _, err := stage(ref, other, ring); err != cart.ErrConflict {
		t.Fatal("competing payment operation bypassed freeze")
	}
	stale := command
	stale.Version = "2"
	if _, err := stage(ref, stale, ring); err != cart.ErrConflict {
		t.Fatal("changed cart version reused operation")
	}
	wrong := ref
	wrong.Generation = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := stage(wrong, command, ring); err != cart.ErrDenied {
		t.Fatal("stale generation replayed operation")
	}
	// Source or target drift must block dispatch even if an out-of-band writer
	// ignored the normal freeze. Restore only these artificial fixture mutations.
	var source struct{ Data string }
	if db.Table("payment_sessions").Select("data").Where("id='operation-session-a'").Take(&source).Error != nil {
		t.Fatal("source envelope fixture unavailable")
	}
	exec("UPDATE payment_sessions SET data='{}' WHERE id='operation-session-a'")
	if _, err := claim("operation-session-a"); err != cart.ErrConflict {
		t.Fatal("changed provider source dispatched")
	}
	exec("UPDATE payment_sessions SET data=? WHERE id='operation-session-a'", source.Data)
	exec(`INSERT INTO cart_line_items(id,cart_id,title,quantity,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,is_custom_price,is_giftcard,created_at,updated_at) VALUES('operation-target-drift',?,'Artificial drift',1,FALSE,TRUE,FALSE,1,'{"value":"1","precision":100}',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	if _, err := claim("operation-session-a"); err != cart.ErrConflict {
		t.Fatal("changed cart target dispatched")
	}
	exec("DELETE FROM cart_line_items WHERE id='operation-target-drift'")
	exec("UPDATE cart_ownership SET payment_refresh_operation_id=NULL WHERE cart_id=?", ref.CartID)
	if _, err := claim("operation-session-a"); err != cart.ErrConflict {
		t.Fatal("missing durable freeze granted dispatch")
	}
	exec("UPDATE cart_ownership SET payment_refresh_operation_id=? WHERE cart_id=?", command.OperationID, ref.CartID)
	first, err := claim("operation-session-a")
	if err != nil || !first.Dispatch {
		t.Fatalf("first private claim failed: %v", err)
	}
	if err := record("operation-session-a", first.AttemptID, "succeeded"); err != nil {
		t.Fatal("verified artificial first outcome failed")
	}
	unchanged := func() {
		t.Helper()
		var collection struct{ CurrencyCode, Amount string }
		var deleted int64
		var owner struct {
			Version                   uint64
			PaymentRefreshOperationID *string
		}
		if db.Table("payment_collections").Select("currency_code,CAST(amount AS CHAR) AS amount").Where("id='operation-collection'").Take(&collection).Error != nil || collection.CurrencyCode != "usd" || collection.Amount != "10.000000000000000000" {
			t.Fatal("partial operation changed collection")
		}
		if db.Table("payment_sessions").Where("payment_collection_id='operation-collection' AND deleted_at IS NOT NULL").Count(&deleted).Error != nil || deleted != 0 {
			t.Fatal("partial operation deleted local sessions")
		}
		if db.Table("cart_ownership").Where("cart_id=?", ref.CartID).Take(&owner).Error != nil || owner.Version != 1 || owner.PaymentRefreshOperationID == nil || *owner.PaymentRefreshOperationID != command.OperationID {
			t.Fatal("partial operation released durable freeze")
		}
	}
	if _, err := complete(); err != cart.ErrConflict {
		t.Fatal("partial provider success completed operation")
	}
	unchanged()
	second, err := claim("operation-session-b")
	if err != nil || !second.Dispatch {
		t.Fatal("second private claim failed")
	}
	if err := record("operation-session-b", second.AttemptID, "uncertain"); err != nil {
		t.Fatal("unknown outcome recording failed")
	}
	if replay, err := claim("operation-session-b"); err != nil || replay.Dispatch || replay.State != "uncertain" {
		t.Fatal("unknown operation redispatched")
	}
	if _, err := complete(); err != cart.ErrConflict {
		t.Fatal("unknown outcome completed operation")
	}
	unchanged()
	if err := record("operation-session-b", uuid.NewString(), "succeeded"); err != payments.ErrEffectConflict {
		t.Fatal("wrong attempt completed provider receipt")
	}
	if err := record("operation-session-b", second.AttemptID, "succeeded"); err != nil {
		t.Fatal("late verified outcome not recovered")
	}
	exec("CREATE TRIGGER payment_operation_finish_fault BEFORE UPDATE ON cart_payment_refresh_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial finish fault'")
	_, finishErr := complete()
	exec("DROP TRIGGER payment_operation_finish_fault")
	if finishErr != cart.ErrStorage {
		t.Fatal("late finish fault ignored")
	}
	unchanged()
	done, err := complete()
	if err != nil || done.State != "completed" || done.Version != "2" || done.Replayed {
		t.Fatalf("verified operation completion failed: %v", err)
	}
	if db.Table("payment_sessions").Where("payment_collection_id='operation-collection' AND deleted_at IS NOT NULL").Count(&count).Error != nil || count != 2 {
		t.Fatal("completed operation did not invalidate exact local sessions")
	}
	var collection struct{ CurrencyCode, RawAmount string }
	if db.Table("payment_collections").Select("currency_code,raw_amount").Where("id='operation-collection'").Take(&collection).Error != nil || collection.CurrencyCode != "eur" || !bytes.Contains([]byte(collection.RawAmount), []byte(`"value":"0"`)) {
		t.Fatal("completed operation lost refreshed exact amount")
	}
	if snapshot, err := cart.Inspect(ctx, db, "shop", ref); err != nil || snapshot.Version != "2" {
		t.Fatal("completed cart version unavailable")
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := cart.LockForWrite(ctx, tx, "shop", ref, "2"); return err }); err != nil {
		t.Fatal("completed payment freeze not released")
	}
	if replay, err := complete(); err != nil || !replay.Replayed || replay.Version != "2" {
		t.Fatal("completed payment operation replay failed")
	}
	exec("UPDATE cart_ownership SET generation=REPEAT('f',64) WHERE cart_id=?", ref.CartID)
	if _, err := complete(); err != cart.ErrDenied {
		t.Fatal("revoked owner replayed completed payment operation")
	}
	exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	t.Log(`CART_PAYMENT_OPERATIONS_REHEARSAL {"default_closed":true,"no_collection_noop":true,"server_read_encrypted_requests":true,"paid_state_denied":true,"existing_payment_denied":true,"manifest_failure_atomic":true,"durable_write_freeze":true,"frozen_cart_readable":true,"competing_operation_denied":true,"version_generation_rechecked":true,"source_target_and_freeze_drift_denied":true,"partial_success_not_completed":true,"uncertain_not_retried":true,"verified_attempt_required":true,"late_finish_failure_atomic":true,"complete_amount_session_version_atomic":true,"completed_replay":true,"revoked_replay_denied":true,"provider_called":false,"compensation_integrated":false,"checkout_complete":false}`)
}
