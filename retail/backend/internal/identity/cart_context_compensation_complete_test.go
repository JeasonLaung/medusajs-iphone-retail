package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkContextCompensationComplete(t *testing.T, ctx context.Context, db *gorm.DB, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context compensation complete fixture", err)
		}
	}
	fingerprint := func() [32]byte {
		t.Helper()
		var tables []string
		if db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name").Scan(&tables).Error != nil {
			t.Fatal("compensation complete inventory")
		}
		var all []string
		for _, table := range tables {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("compensation complete fingerprint", table)
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
	const provider = "pp_mollie-hosted-checkout_mollie"
	exec("UPDATE inventory_levels SET stocked_quantity=stocked_quantity+20 WHERE id='add-level'")
	exec("UPDATE cart_context_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE cart_context_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE mollie_notification_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	for _, replace := range []bool{false, true} {
		ref, initial := makeCart()
		if result, err := cart.StartInitialPayment(ctx, db, "shop", ref, initial, options); err != nil || result.State != "ready" {
			t.Fatal("compensation original checkout", result, err)
		}
		sourceVersion := uint64(5)
		if replace {
			// Also restore a checkout which already retained its original CREATE
			// across an earlier effective context revision.
			prior := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "region", Locale: "en"}
			if saved, err := cart.CompleteContextPayment(ctx, db, "shop", ref, prior, options); err != nil || saved.Version != "6" {
				t.Fatal("compensation preceding effective revision", saved, err)
			}
			sourceVersion = 6
		}
		finalVersion := strconv.FormatUint(sourceVersion+1, 10)
		var original, oldSession, collection, originalCart map[string]any
		if db.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, initial.OperationID).Take(&original).Error != nil {
			t.Fatal("compensation original immutable row")
		}
		originalSession := original["session_id"]
		if db.Table("payment_sessions").Where("id=?", originalSession).Take(&oldSession).Error != nil || db.Table("payment_collections").Where("id=?", original["collection_id"]).Take(&collection).Error != nil || db.Table("carts").Where("id=?", ref.CartID).Take(&originalCart).Error != nil {
			t.Fatal("compensation original facts")
		}
		command := cart.ContextCommand{OperationID: uuid.NewString(), Version: strconv.FormatUint(sourceVersion, 10), RegionID: "region", Locale: "zh-CN"}
		collision := command
		collision.OperationID = initial.OperationID
		beforeCollision := fingerprint()
		if _, err := cart.StageContextPayment(ctx, db, "shop", ref, collision, options); err != cart.ErrConflict || fingerprint() != beforeCollision {
			t.Fatal("context operation collided with immutable checkout identity", err)
		}
		// The new quote differs, so one deletion is planned. Restoration must
		// subsequently use the original amount even when the target quote drifts.
		var price struct{ Amount, Raw string }
		if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
			t.Fatal("compensation quote")
		}
		exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
		staged, err := cart.StageContextPayment(ctx, db, "shop", ref, command, options)
		if err != nil || len(staged.Effects) != 1 {
			t.Fatal("compensation staged changed quote", err)
		}
		if replace {
			adapters, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{provider: executionDelete(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
				return payments.DeleteSucceeded, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			if result, err := cart.ExecuteContextPaymentSession(ctx, db, "shop", ref, command, staged.Effects[0].ResourceID, options, adapters); err != nil || result.Receipt.State != "succeeded" {
				t.Fatal("compensation confirmed delete", result, err)
			}
		}
		exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
		compensation, err := cart.StageContextPaymentCompensation(ctx, db, "shop", ref, command, options)
		if err != nil || len(compensation.Replacements) != map[bool]int{false: 0, true: 1}[replace] {
			t.Fatal("compensation preparation after quote drift", compensation, err)
		}
		var calls atomic.Int32
		status := "pending"
		adapters, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: executionObserver{
			create: func(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
				calls.Add(1)
				if inScope(r) != ref.CartID || r.Amount != "158.75" || r.Currency != "eur" {
					t.Error("compensation changed original amount")
				}
				return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(r, "tr_compensation_active", "https://www.mollie.com/checkout/restored")}, nil
			},
			observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
				local, action := "pending", "pending"
				if status == "paid" {
					local, action = "authorized", "successful"
				}
				raw, _ := json.Marshal(map[string]any{"id": id, "status": status, "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}})
				return payments.SessionObservation{PaymentID: id, ProviderStatus: status, Action: action, Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: local, Data: raw}}, nil
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		configured := options
		configured.Adapters = adapters
		if replace {
			before := fingerprint()
			if _, err := cart.CompleteContextPaymentCompensation(ctx, db, "shop", ref, command, configured); err == nil || fingerprint() != before {
				t.Fatal("uncreated replacement activated", err)
			}
			if result, err := cart.ExecuteContextCompensationSession(ctx, db, "shop", ref, command, staged.Effects[0].ResourceID, configured); err != nil || result.Receipt.State != "succeeded" || calls.Load() != 1 {
				t.Fatal("replacement creation", result, err)
			}
		}
		before := fingerprint()
		exec("CREATE TRIGGER context_compensation_final_fault BEFORE UPDATE ON cart_payment_refresh_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned compensation final rollback'")
		_, err = cart.CompleteContextPaymentCompensation(ctx, db, "shop", ref, command, configured)
		exec("DROP TRIGGER context_compensation_final_fault")
		if err != cart.ErrStorage || fingerprint() != before {
			t.Fatal("compensation final failure not atomic", err)
		}
		var group sync.WaitGroup
		results := make(chan cart.PaymentCompensation, 6)
		for i := 0; i < 6; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				r, e := cart.CompleteContextPaymentCompensation(ctx, db, "shop", ref, command, configured)
				if e != nil {
					t.Error("concurrent compensation activation", e)
					return
				}
				results <- r
			}()
		}
		group.Wait()
		close(results)
		if len(results) != 6 {
			t.Fatal("compensation activation retries failed")
		}
		fresh := 0
		for r := range results {
			if r.State != "completed" || r.Version != finalVersion || !reflect.DeepEqual(r.Replacements, compensation.Replacements) {
				t.Fatal("compensation activation result", r)
			}
			if !r.Replayed {
				fresh++
			}
		}
		if fresh != 1 {
			t.Fatal("compensation activated more than once")
		}
		replayBefore := fingerprint()
		changed := command
		changed.Locale = "en"
		if _, err := cart.CompleteContextPaymentCompensation(ctx, db, "shop", ref, changed, configured); err != cart.ErrConflict || fingerprint() != replayBefore {
			t.Fatal("changed compensation replay accepted", err)
		}
		wrong := ref
		wrong.Generation = strings.Repeat("0", 64)
		if _, err := cart.CompleteContextPaymentCompensation(ctx, db, "shop", wrong, command, configured); err != cart.ErrDenied || fingerprint() != replayBefore {
			t.Fatal("foreign compensation replay accepted", err)
		}
		var savedOriginal, savedCollection, savedCart map[string]any
		db.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, initial.OperationID).Take(&savedOriginal)
		db.Table("payment_collections").Where("id=?", original["collection_id"]).Take(&savedCollection)
		db.Table("carts").Where("id=?", ref.CartID).Take(&savedCart)
		if !replace {
			delete(original, "effective_cart_version")
			delete(savedOriginal, "effective_cart_version")
		}
		if !reflect.DeepEqual(original, savedOriginal) || !reflect.DeepEqual(collection, savedCollection) || !reflect.DeepEqual(originalCart, savedCart) {
			t.Fatal("compensation rewrote immutable checkout, collection or original context")
		}
		var savedOldSession map[string]any
		if db.Table("payment_sessions").Where("id=?", originalSession).Take(&savedOldSession).Error != nil {
			t.Fatal("original payment session history lost")
		}
		if replace {
			if savedOldSession["deleted_at"] == nil {
				t.Fatal("replaced payment session still live")
			}
			delete(oldSession, "deleted_at")
			delete(oldSession, "updated_at")
			delete(savedOldSession, "deleted_at")
			delete(savedOldSession, "updated_at")
		}
		if !reflect.DeepEqual(oldSession, savedOldSession) {
			t.Fatal("original sealed payment history rewritten")
		}
		active := initial
		if replace {
			active = cart.InitialPaymentCommand{OperationID: command.OperationID, Version: command.Version}
			if _, err := cart.StartInitialPayment(ctx, db, "shop", ref, initial, configured); err != cart.ErrConflict {
				t.Fatal("replaced original checkout still active", err)
			}
		}
		if result, err := cart.StartInitialPayment(ctx, db, "shop", ref, active, configured); err != nil || result.State != "ready" || result.Version != finalVersion {
			t.Fatal("restored checkout not usable", result, err)
		}
		if calls.Load() != int32(map[bool]int{false: 0, true: 1}[replace]) {
			t.Fatal("activation or retry reposted provider creation")
		}
		if result, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, configured); err != nil || result.State != "pending" || result.Version != finalVersion {
			t.Fatal("restored pending status", result, err)
		}
		var falseReceipt int64
		db.Table("cart_context_operations").Where("cart_id=? AND operation_id=?", ref.CartID, command.OperationID).Count(&falseReceipt)
		if falseReceipt != 0 {
			t.Fatal("restoration claimed target context saved")
		}
		status = "paid"
		var order cart.CompletionResult
		if replace {
			var remote string
			if db.Table("mollie_payment_references").Select("remote_id").Where("scope_id=? AND operation_id=? AND effect_kind='create_session'", ref.CartID, command.OperationID).Scan(&remote).Error != nil || remote == "" {
				t.Fatal("replacement notification routing")
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				_, err := payments.EnqueueMollieNotification(ctx, tx, provider, remote)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			observed, err := payments.ProcessMollieNotification(ctx, db, provider, remote, configured.Ring, configured.Adapters)
			if err != nil || observed.Receipt.AttemptID == nil {
				t.Fatal("replacement notification observation", err)
			}
			consumed, err := cart.ConsumeMollieCheckoutObservation(ctx, db, "shop", *observed.Receipt.AttemptID, configured)
			if err != nil || consumed.State != "completed" {
				t.Fatal("replacement notification completion", consumed, err)
			}
			order = consumed.Completion
		} else {
			if _, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, configured); err != nil {
				t.Fatal(err)
			}
			order, err = cart.CompletePaidOrder(ctx, db, "shop", ref, active, configured)
			if err != nil {
				t.Fatal("original retained compensation order", err)
			}
		}
		if order.OrderID == "" || order.Version != strconv.FormatUint(sourceVersion+2, 10) {
			t.Fatal("restored order facts", order)
		}
		if replay, err := cart.CompletePaidOrder(ctx, db, "shop", ref, active, configured); err != nil || !replay.Replayed || replay.OrderID != order.OrderID {
			t.Fatal("restored order replay", replay, err)
		}
	}
	t.Log(`CART_CONTEXT_COMPENSATION_COMPLETE {"real_mysql":true,"zero_deletions_original_retained":true,"confirmed_delete_new_immutable_attempt":true,"preceding_effective_revision":true,"target_quote_drift_independent":true,"uncreated_replacement_refused":true,"late_failure_all_tables_rollback":true,"concurrent_attempts_each":6,"single_activation_each":true,"old_checkout_collection_cart_unchanged":true,"no_false_context_save_receipt":true,"original_or_replacement_pending_status":true,"original_paid_completion":true,"replacement_notification_paid_completion":true,"unique_order_replay":true,"no_provider_repost":true,"artificial_provider_only":true,"http_connected":false}`)
}
