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

// Real activated checkout fixture; save twice, preserving original CREATE and
// payment facts. The caller subsequently exercises paid observation/completion
// or notification consumption using the ORIGINAL initial command.
func checkContextPaymentComplete(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, in cart.InitialPaymentCommand, options cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context complete fixture", err)
		}
	}
	fingerprint := func(tables []string) [32]byte {
		t.Helper()
		if tables == nil {
			if db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name").Scan(&tables).Error != nil {
				t.Fatal("context complete inventory")
			}
		}
		var all []string
		for _, table := range tables {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("context complete fingerprint", table)
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
	command := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "region", Locale: "zh-CN"}
	reject := func(cmd cart.ContextCommand, authority sessionstore.CartReference, want error) {
		t.Helper()
		before := fingerprint(nil)
		got, err := cart.CompleteContextPayment(ctx, db, "shop", authority, cmd, options)
		if err != want || !reflect.DeepEqual(got, cart.ContextResult{}) || fingerprint(nil) != before {
			t.Fatalf("context complete rejection want %v got %v or persisted facts", want, err)
		}
	}
	reject(command, ref, cart.ErrSchema)
	exec("UPDATE cart_context_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_context_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	wrong := ref
	wrong.Generation = strings.Repeat("0", 64)
	reject(command, wrong, cart.ErrDenied)
	bad := command
	bad.Version = "4"
	reject(bad, ref, cart.ErrConflict)
	// A changed amount must not falsely complete or leave a newly staged freeze.
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
		t.Fatal("context complete quote")
	}
	exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
	reject(command, ref, cart.ErrConflict)
	exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
	exec("CREATE TRIGGER context_final_receipt_fault BEFORE INSERT ON cart_context_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned final save rollback'")
	reject(command, ref, cart.ErrStorage)
	exec("DROP TRIGGER context_final_receipt_fault")
	protectedTables := []string{"payment_collections", "payment_sessions", "payment_effect_journal", "mollie_payment_references", "orders", "inventory_levels", "inventory_reservations"}
	protectedBefore := fingerprint(protectedTables)
	var original map[string]any
	if db.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Take(&original).Error != nil || original["effective_cart_version"] != nil {
		t.Fatal("original checkout binding")
	}
	var group sync.WaitGroup
	results := make(chan cart.ContextResult, 6)
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			r, err := cart.CompleteContextPayment(ctx, db, "shop", ref, command, options)
			if err != nil {
				t.Error("concurrent context final save", err)
				return
			}
			results <- r
		}()
	}
	group.Wait()
	close(results)
	fresh := 0
	if len(results) != 6 {
		t.Fatal("context final save retries failed")
	}
	for r := range results {
		if r.CartID != ref.CartID || r.Version != "6" {
			t.Fatal("context final version", r)
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("context final save not unique")
	}
	bad = command
	bad.Locale = "en"
	reject(bad, ref, cart.ErrConflict)
	if result, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != nil || result.Version != "6" || result.State != "ready" {
		t.Fatal("retained original checkout unusable after context save", result, err)
	}
	second := cart.ContextCommand{OperationID: uuid.NewString(), Version: "6", RegionID: "region", Locale: "en"}
	if result, err := cart.CompleteContextPayment(ctx, db, "shop", ref, second, options); err != nil || result.Replayed || result.Version != "7" {
		t.Fatal("second context save did not use effective version", result, err)
	}
	if result, err := cart.CompleteContextPayment(ctx, db, "shop", ref, command, options); err != nil || !result.Replayed || result.Version != "6" {
		t.Fatal("old context receipt lost after later revision", result, err)
	}
	var saved map[string]any
	if db.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Take(&saved).Error != nil {
		t.Fatal("saved effective binding")
	}
	delete(original, "effective_cart_version")
	delete(saved, "effective_cart_version")
	if !reflect.DeepEqual(original, saved) || protectedBefore != fingerprint(protectedTables) {
		t.Fatal("context save changed original CREATE or protected payment/stock facts")
	}
	var owner struct {
		Version                                      uint64
		CheckoutAttemptID, PaymentRefreshOperationID *string
	}
	if db.Table("cart_ownership").Where("cart_id=?", ref.CartID).Take(&owner).Error != nil || owner.Version != 7 || owner.PaymentRefreshOperationID != nil || owner.CheckoutAttemptID == nil || *owner.CheckoutAttemptID != in.OperationID {
		t.Fatal("context save released checkout freeze or kept payment-operation freeze")
	}
	if _, err := cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "7", VariantID: "add-v", Quantity: "1"}); err != cart.ErrConflict {
		t.Fatal("context completion unfroze checkout mutations", err)
	}
	t.Log(`CART_CONTEXT_PAYMENT_COMPLETE {"real_mysql":true,"concurrent_attempts":6,"one_final_save":true,"changed_amount_atomic_refusal":true,"late_receipt_all_tables_rollback":true,"original_create_binding_immutable":true,"two_effective_revisions":true,"original_checkout_retained":true,"old_receipt_replay":true,"no_provider_calls_or_payment_stock_changes":true,"checkout_freeze_retained":true,"final_effective_version":7,"http_connected":false}`)
}

func checkContextRetainedCheckoutCompletion(t *testing.T, ctx context.Context, db *gorm.DB, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context retained order fixture", err)
		}
	}
	exec("UPDATE inventory_levels SET stocked_quantity=stocked_quantity+10 WHERE id='add-level'")
	ref, in := makeCart()
	if result, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != nil || result.State != "ready" {
		t.Fatal("context retained initial checkout", result, err)
	}
	checkContextPaymentComplete(t, ctx, db, ref, in, options)
	status := "pending"
	adapters, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			t.Error("retained context recreated payment")
			return payments.CreateUncertain, payments.CreatedSession{}, nil
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
	options.Adapters = adapters
	if result, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err != nil || result.State != "pending" || result.Version != "7" {
		t.Fatal("context retained pending status/version", result, err)
	}
	status = "paid"
	if result, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err != nil || result.State != "paid_pending_order" || result.Version != "7" {
		t.Fatal("context retained paid status/version", result, err)
	}
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	result, err := cart.CompletePaidOrder(ctx, db, "shop", ref, in, options)
	if err != nil || result.Version != "8" || result.OrderID == "" || result.Replayed {
		t.Fatal("context retained paid completion", result, err)
	}
	if replay, err := cart.CompletePaidOrder(ctx, db, "shop", ref, in, options); err != nil || !replay.Replayed || replay.OrderID != result.OrderID || replay.Version != "8" {
		t.Fatal("context retained order completion replay", replay, err)
	}
	var facts struct {
		Orders, Captures, Notifications, Reservations int64
		InputVersion, CompletedVersion                uint64
	}
	if db.Raw(`SELECT (SELECT COUNT(*) FROM order_carts WHERE cart_id=?) AS orders,
	 (SELECT COUNT(*) FROM payment_captures WHERE payment_id=r.payment_id) AS captures,
	 (SELECT COUNT(*) FROM order_notification_outbox WHERE order_id=r.order_id) AS notifications,
	 (SELECT COUNT(*) FROM inventory_reservations WHERE line_item_id IN (SELECT item_id FROM order_items WHERE order_id=r.order_id)) AS reservations,
	 r.input_version,r.completed_version FROM cart_completion_receipts r WHERE cart_id=?`, ref.CartID, ref.CartID).Scan(&facts).Error != nil || facts.Orders != 1 || facts.Captures != 1 || facts.Notifications != 1 || facts.Reservations == 0 || facts.InputVersion != 4 || facts.CompletedVersion != 8 {
		t.Fatal("context retained order/cash/stock/outbox facts", facts)
	}
	t.Log(`CART_CONTEXT_RETAINED_COMPLETION {"real_mysql":true,"original_input_version":4,"original_cart_version":5,"effective_cart_version":7,"completed_version":8,"pending_and_paid_status":true,"unique_order_capture_reservations_outbox":true,"same_original_command_replay":true,"artificial_provider_only":true}`)
}
