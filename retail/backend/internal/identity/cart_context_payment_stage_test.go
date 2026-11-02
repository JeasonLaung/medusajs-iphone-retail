package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkContextPaymentStage(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, options cart.InitialPaymentOptions, fingerprint func() [32]byte) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context payment stage fixture", err)
		}
	}
	// Only owner freeze, operation manifest and protected requests may change.
	// Include every other base table, not just empty payment/order fixtures.
	protected := func() [32]byte {
		t.Helper()
		var tables []string
		if db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' AND table_name NOT IN ('cart_ownership','cart_payment_refresh_operations','payment_effect_journal') ORDER BY table_name").Scan(&tables).Error != nil {
			t.Fatal("context stage protected inventory")
		}
		var all []string
		for _, table := range tables {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("context stage protected rows", table)
			}
			var encoded []string
			for _, row := range rows {
				raw, err := json.Marshal(row)
				if err != nil {
					t.Fatal(err)
				}
				encoded = append(encoded, string(raw))
			}
			sort.Strings(encoded)
			all = append(all, table+strings.Join(encoded, "\n"))
		}
		return sha256.Sum256([]byte(strings.Join(all, "\n")))
	}
	owner := func() map[string]any {
		t.Helper()
		var row map[string]any
		if db.Table("cart_ownership").Where("cart_id=?", ref.CartID).Take(&row).Error != nil {
			t.Fatal("context stage owner")
		}
		return row
	}
	command := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "region", Locale: "en"}
	reject := func(in cart.ContextCommand, authority sessionstore.CartReference, want error) {
		t.Helper()
		before := fingerprint()
		got, err := cart.StageContextPayment(ctx, db, "shop", authority, in, options)
		if err != want || !reflect.DeepEqual(got, cart.ContextPaymentOperation{}) || fingerprint() != before {
			t.Fatalf("stage rejection want %v got %v or changed facts", want, err)
		}
	}
	reject(command, ref, cart.ErrSchema)
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	baseline := fingerprint()
	protectedBefore, ownerBefore := protected(), owner()
	var group sync.WaitGroup
	results := make(chan cart.ContextPaymentOperation, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := cart.StageContextPayment(ctx, db, "shop", ref, command, options)
			if err != nil {
				t.Error("concurrent context stage", err)
				return
			}
			results <- got
		}()
	}
	group.Wait()
	close(results)
	if len(results) != 8 {
		t.Fatal("not all context stage retries recovered")
	}
	fresh := 0
	for got := range results {
		if !got.Replayed {
			fresh++
		}
		if got.State != "staged" || got.Version != "5" || got.Target.Payment.UpdateCollection || got.Target.Payment.Amount != "158.75" || len(got.Effects) != 0 {
			t.Fatal("unchanged staged context falsely completed or deleted sessions", got)
		}
	}
	if fresh != 1 || protected() != protectedBefore {
		t.Fatal("context stage was not unique or changed source business facts")
	}
	afterOwner := owner()
	if afterOwner["payment_refresh_operation_id"] == nil {
		t.Fatal("context intent did not durably freeze")
	}
	afterOwner["payment_refresh_operation_id"] = ownerBefore["payment_refresh_operation_id"]
	if !reflect.DeepEqual(ownerBefore, afterOwner) {
		t.Fatal("staging changed owner/version/checkout beyond the new freeze")
	}
	changed := command
	changed.Locale = "zh-CN"
	reject(changed, ref, cart.ErrConflict)
	changed = command
	changed.OperationID = uuid.NewString()
	reject(changed, ref, cart.ErrConflict)
	wrong := ref
	wrong.Generation = strings.Repeat("a", 64)
	reject(command, wrong, cart.ErrDenied)
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
		t.Fatal("stage source quote")
	}
	exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
	reject(command, ref, cart.ErrConflict)
	exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
	var stored struct{ ManifestJSON string }
	if db.Table("cart_payment_refresh_operations").Select("manifest_json").Where("cart_id=? AND operation_id=?", ref.CartID, command.OperationID).Take(&stored).Error != nil {
		t.Fatal("stage immutable manifest read")
	}
	exec("UPDATE cart_payment_refresh_operations SET manifest_json=JSON_SET(manifest_json,'$.Target.Locale','tampered') WHERE cart_id=? AND operation_id=?", ref.CartID, command.OperationID)
	reject(command, ref, cart.ErrStorage)
	exec("UPDATE cart_payment_refresh_operations SET manifest_json=? WHERE cart_id=? AND operation_id=?", stored.ManifestJSON, ref.CartID, command.OperationID)
	got, err := cart.StageContextPayment(ctx, db.Session(&gorm.Session{NewDB: true}), "shop", ref, command, options)
	if err != nil || !got.Replayed || got.State != "staged" {
		t.Fatal("new transaction did not recover durable original intent", err)
	}
	// Remove only these unexecuted artificial intents to let the parent checkout
	// suite continue. This cleanup is test fixture code, never runtime recovery.
	cleanup := func(in cart.ContextCommand) {
		t.Helper()
		exec("DELETE FROM payment_effect_journal WHERE scope_id=? AND operation_id=? AND state='pending' AND attempts=0", ref.CartID, in.OperationID)
		exec("DELETE FROM cart_payment_refresh_operations WHERE cart_id=? AND operation_id=? AND state='staged'", ref.CartID, in.OperationID)
		exec("UPDATE cart_ownership SET payment_refresh_operation_id=NULL WHERE cart_id=? AND payment_refresh_operation_id=?", ref.CartID, in.OperationID)
	}
	cleanup(command)
	if fingerprint() != baseline {
		t.Fatal("unchanged target staging leaked facts beyond the three intended tables")
	}
	changed = command
	changed.OperationID = uuid.NewString()
	changed.RegionID = "preview-canada"
	exec(`CREATE TRIGGER fail_context_stage_manifest BEFORE INSERT ON cart_payment_refresh_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned context stage failure'`)
	reject(changed, ref, cart.ErrStorage)
	exec("DROP TRIGGER fail_context_stage_manifest")
	protectedBefore = protected()
	got, err = cart.StageContextPayment(ctx, db, "shop", ref, changed, options)
	if err != nil || got.Replayed || got.Target.Currency != "cad" || got.Target.Payment.Amount != "42.000000000000000000001" || len(got.Effects) != 1 || protectedBefore != protected() {
		t.Fatal("changed context did not stage exact protected delete only", err)
	}
	key := got.Effects[0]
	if err := db.Transaction(func(tx *gorm.DB) error {
		receipt, err := payments.InspectEffect(ctx, tx, key)
		if err != nil || receipt.State != "pending" || receipt.AttemptID != "" || receipt.Dispatch {
			t.Fatal("stage consumed a provider attempt", err)
		}
		request, err := payments.ReadSessionRequest(ctx, tx, key, options.Ring)
		if err != nil || request.Amount != "158.75" || request.Currency != "eur" || request.SessionID != key.ResourceID {
			t.Fatal("delete request did not retain original currency and amount", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := fingerprint()
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, _, err := cart.ClaimPaymentRefreshSession(ctx, tx, "shop", ref, cart.PaymentRefreshCommand{OperationID: changed.OperationID, Version: changed.Version}, key.ResourceID, options.Ring)
		return err
	}); err != cart.ErrConflict || fingerprint() != before {
		t.Fatal("old standalone payment operation claimed the context effect", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := cart.StageContextPayment(ctx, tx, "shop", ref, changed, options); err != cart.ErrInput {
			t.Fatal("context stage accepted caller-owned transaction", err)
		}
		return nil
	}); err != nil || fingerprint() != before {
		t.Fatal("nested stage changed caller state", err)
	}
	cleanup(changed)
	if fingerprint() != baseline {
		t.Fatal("stage fixture left a request, freeze or changed business fact")
	}
	checkContextPaymentExecution(t, ctx, db, ref, options, fingerprint)
	t.Log(`CART_CONTEXT_PAYMENT_STAGE {"real_mysql":true,"concurrent_attempts":8,"one_immutable_manifest":true,"same_target_replayed":true,"changed_intent_source_and_manifest_refused":true,"same_amount_sessions_retained":true,"changed_target_protected_original_delete_request":true,"only_three_coordinator_tables_change":true,"late_failure_all_tables_rollback":true,"original_checkout_and_business_unchanged":true,"no_provider_attempt":true,"old_executor_refused":true,"context_finalization_connected":false}`)
}
