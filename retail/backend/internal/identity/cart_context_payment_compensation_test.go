package identity_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkContextCompensationStage(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, command cart.ContextCommand, key payments.EffectKey, options cart.InitialPaymentOptions, fingerprint func() [32]byte) {
	t.Helper()
	before := fingerprint()
	// Uncertain deletion can never start replacement creation.
	if _, err := cart.StageContextPaymentCompensation(ctx, db, "shop", ref, command, options); err != cart.ErrConflict || fingerprint() != before {
		t.Fatal("ambiguous deletion compensated", err)
	}
}

func checkContextCompensationConfirmed(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, command cart.ContextCommand, key payments.EffectKey, options cart.InitialPaymentOptions, fingerprint func() [32]byte) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := fingerprint()
	exec("CREATE TRIGGER context_compensation_manifest_fault BEFORE INSERT ON cart_payment_refresh_compensations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned compensation rollback'")
	_, err := cart.StageContextPaymentCompensation(ctx, db, "shop", ref, command, options)
	exec("DROP TRIGGER context_compensation_manifest_fault")
	if err != cart.ErrStorage || fingerprint() != before {
		t.Fatal("compensation request did not roll back", err)
	}
	// Compensation must still work after the new target quote changes.
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='preview-cad-price'").Take(&price).Error != nil {
		t.Fatal("compensation price fixture")
	}
	exec(`UPDATE pricing_prices SET amount=77,raw_amount='{"value":"77","precision":100}' WHERE id='preview-cad-price'`)
	var group sync.WaitGroup
	results := make(chan cart.PaymentCompensation, 6)
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := cart.StageContextPaymentCompensation(ctx, db, "shop", ref, command, options)
			if err != nil {
				t.Error("concurrent context compensation", err)
				return
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	if len(results) != 6 {
		t.Fatal("context compensation retries failed")
	}
	fresh := 0
	var saved cart.PaymentCompensation
	for result := range results {
		if !result.Replayed {
			fresh++
		}
		if result.State != "staged" || result.Version != "5" || len(result.Replacements) != 1 || result.Replacements[0].OriginalID != key.ResourceID {
			t.Fatal("wrong compensation", result)
		}
		if saved.OperationID != "" && !reflect.DeepEqual(saved.Replacements, result.Replacements) {
			t.Fatal("unstable replacement identity")
		}
		saved = result
	}
	if fresh != 1 {
		t.Fatal("multiple compensation manifests")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		replacement := saved.Replacements[0].Create
		request, err := payments.ReadSessionRequest(ctx, tx, replacement, options.Ring)
		if err != nil {
			return err
		}
		var raw map[string]json.RawMessage
		if json.Unmarshal(request.Data, &raw) != nil || string(raw["cart_id"]) != "\""+ref.CartID+"\"" || string(raw["session_id"]) != "\""+replacement.ResourceID+"\"" || len(raw["id"]) != 0 || request.Amount != "158.75" || request.Currency != "eur" {
			t.Fatal("compensation used provider response instead of original create metadata")
		}
		receipt, err := payments.InspectEffect(ctx, tx, replacement)
		if err != nil {
			return err
		}
		if receipt.State != "pending" || receipt.Attempts != 0 || receipt.Dispatch {
			t.Fatal("compensation dispatched provider")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='preview-cad-price'", price.Amount, price.Raw)
	checkContextCompensationExecution(t, ctx, db, ref, command, key.ResourceID, options)
	stable := fingerprint()
	if _, err := cart.StageContextPayment(ctx, db, "shop", ref, command, options); err != cart.ErrConflict || fingerprint() != stable {
		t.Fatal("compensation allowed normal delete staging", err)
	}
	if _, err := cart.ExecuteContextPaymentSession(ctx, db, "shop", ref, command, key.ResourceID, options, nil); err != cart.ErrInput {
		t.Fatal("nil executor adapter accepted", err)
	}
	exec("UPDATE cart_payment_refresh_compensations SET manifest_json=JSON_SET(manifest_json,'$.Format',1) WHERE cart_id=? AND operation_id=?", ref.CartID, command.OperationID)
	if _, err := cart.StageContextPaymentCompensation(ctx, db, "shop", ref, command, options); err != cart.ErrStorage {
		t.Fatal("tampered compensation accepted", err)
	}
	// Fixture cleanup removes only this operation's artificial replacement receipts.
	exec("DELETE FROM payment_effect_journal WHERE scope_id=? AND operation_id=? AND effect_kind='create_session'", ref.CartID, command.OperationID)
	exec("DELETE FROM cart_payment_refresh_compensations WHERE cart_id=? AND operation_id=? AND state='staged'", ref.CartID, command.OperationID)
	if fingerprint() != before {
		t.Fatal("compensation preparation changed original business facts")
	}
	t.Log(`CART_CONTEXT_COMPENSATION_STAGE {"real_mysql":true,"ambiguous_deletion_refused":true,"concurrent_attempts":6,"one_stable_manifest":true,"original_authenticated_create_request":true,"quote_drift_recoverable":true,"late_failure_rollback":true,"delete_staging_disabled":true,"tamper_refused":true,"stage_provider_attempts":0,"business_facts_unchanged":true,"replacement_activation":false}`)
}
