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

func checkContextChangedComplete(t *testing.T, ctx context.Context, db *gorm.DB, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("changed context fixture", err)
		}
	}
	exec("UPDATE cart_context_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_context_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
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
	ref, originalCommand := makeCart()
	if _, err := cart.StartInitialPayment(ctx, db, "shop", ref, originalCommand, options); err != nil {
		t.Fatal(err)
	}
	var original map[string]any
	if db.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, originalCommand.OperationID).Take(&original).Error != nil {
		t.Fatal("original binding")
	}
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
		t.Fatal("quote")
	}
	exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
	defer exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
	command := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "region", Locale: "zh-CN"}
	operation, err := cart.StageContextPayment(ctx, db, "shop", ref, command, options)
	if err != nil || len(operation.Effects) != 1 || operation.Target.Payment.Amount != "160" {
		t.Fatal("changed stage", operation, err)
	}
	beforePending := fingerprint(nil)
	if _, err := cart.CompleteContextPayment(ctx, db, "shop", ref, command, options); err != cart.ErrConflict || fingerprint(nil) != beforePending {
		t.Fatal("pending deletion finalized", err)
	}
	deletes, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionDelete(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
		return payments.DeleteSucceeded, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cart.ExecuteContextPaymentSession(ctx, db, "shop", ref, command, operation.Effects[0].ResourceID, options, deletes); err != nil {
		t.Fatal(err)
	}
	exec("CREATE TRIGGER changed_context_final_fault BEFORE INSERT ON cart_context_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned changed context rollback'")
	beforeFinal := fingerprint(nil)
	_, finalErr := cart.CompleteContextPayment(ctx, db, "shop", ref, command, options)
	exec("DROP TRIGGER changed_context_final_fault")
	if finalErr != cart.ErrStorage || fingerprint(nil) != beforeFinal {
		t.Fatal("changed final receipt did not roll back all tables", finalErr)
	}
	var group sync.WaitGroup
	results := make(chan cart.ContextResult, 6)
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			r, e := cart.CompleteContextPayment(ctx, db, "shop", ref, command, options)
			if e != nil {
				t.Error("changed final save", e)
				return
			}
			results <- r
		}()
	}
	group.Wait()
	close(results)
	if len(results) != 6 {
		t.Fatal("changed final retry count")
	}
	fresh := 0
	for r := range results {
		if r.Version != "6" {
			t.Fatal(r)
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("changed save not unique")
	}
	var saved map[string]any
	db.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", ref.CartID, originalCommand.OperationID).Take(&saved)
	if !reflect.DeepEqual(original, saved) {
		t.Fatal("original checkout overwritten")
	}
	if _, err := cart.StartInitialPayment(ctx, db, "shop", ref, originalCommand, options); err != cart.ErrConflict {
		t.Fatal("old checkout reusable", err)
	}
	// Ordinary draft contact/shipping/promotion/context commands can continue
	// against the same now-empty collection and synchronize its exact amount.
	contact, err := cart.ReadContacts(ctx, db, "shop", ref)
	if err != nil {
		t.Fatal(err)
	}
	if r, e := cart.SaveContacts(ctx, db, "shop", ref, cart.ContactCommand{OperationID: uuid.NewString(), Version: "6", Email: contact.Email, Shipping: *contact.Shipping, Billing: *contact.Billing}, options.ManualProviders); e != nil || r.Version != "7" {
		t.Fatal("post-retirement contact", r, e)
	}
	if r, e := cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: "7", OptionID: "delivery-standard"}, options.ManualProviders); e != nil || r.Version != "8" {
		t.Fatal("post-retirement shipping", r, e)
	}
	if r, e := cart.SaveContext(ctx, db, "shop", ref, cart.ContextCommand{OperationID: uuid.NewString(), Version: "8", RegionID: "region", Locale: "en"}, options.ManualProviders); e != nil || r.Version != "9" {
		t.Fatal("post-retirement context", r, e)
	}
	if r, e := cart.CompleteContextPayment(ctx, db, "shop", ref, command, options); e != nil || !r.Replayed || r.Version != "6" {
		t.Fatal("changed historical replay after draft edits", r, e)
	}
	changed := command
	changed.Locale = "en"
	beforeReplay := fingerprint(nil)
	if _, e := cart.CompleteContextPayment(ctx, db, "shop", ref, changed, options); e != cart.ErrConflict || fingerprint(nil) != beforeReplay {
		t.Fatal("changed receipt command accepted", e)
	}
	var calls int
	adapters, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionCreate(func(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		calls++
		if r.Amount != "160" || r.Currency != "eur" {
			t.Error("new amount", r.Amount)
		}
		return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(r, "tr_changed_context", "https://www.mollie.com/checkout/changed")}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	configured := options
	configured.Adapters = adapters
	newCommand := cart.InitialPaymentCommand{OperationID: uuid.NewString(), Version: "9"}
	if r, e := cart.StartInitialPayment(ctx, db, "shop", ref, newCommand, configured); e != nil || r.State != "ready" || r.Version != "10" {
		t.Fatal("new checkout", r, e)
	}
	if r, e := cart.StartInitialPayment(ctx, db, "shop", ref, newCommand, configured); e != nil || r.State != "ready" || calls != 1 {
		t.Fatal("new checkout replay", r, e)
	}
	var collectionID string
	if db.Table("cart_checkout_attempts").Select("collection_id").Where("cart_id=? AND operation_id=?", ref.CartID, newCommand.OperationID).Scan(&collectionID).Error != nil || collectionID != original["collection_id"] {
		t.Fatal("collection identity not preserved")
	}
	exec("UPDATE inventory_levels SET stocked_quantity=stocked_quantity+10 WHERE id='add-level'")
	status := "pending"
	adapters, err = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			t.Error("changed context recreated payment")
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
	if result, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err != nil || result.State != "pending" || result.Version != "10" {
		t.Fatal("context retained pending status/version", result, err)
	}
	status = "paid"
	if result, err := cart.ObserveInitialPayment(ctx, db, "shop", ref, options); err != nil || result.State != "paid_pending_order" || result.Version != "10" {
		t.Fatal("context retained paid status/version", result, err)
	}
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	result, err := cart.CompletePaidOrder(ctx, db, "shop", ref, newCommand, options)
	if err != nil || result.Version != "11" || result.OrderID == "" || result.Replayed {
		t.Fatal("context retained paid completion", result, err)
	}
	if replay, err := cart.CompletePaidOrder(ctx, db, "shop", ref, newCommand, options); err != nil || !replay.Replayed || replay.OrderID != result.OrderID || replay.Version != "11" {
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
	 r.input_version,r.completed_version FROM cart_completion_receipts r WHERE cart_id=?`, ref.CartID, ref.CartID).Scan(&facts).Error != nil || facts.Orders != 1 || facts.Captures != 1 || facts.Notifications != 1 || facts.Reservations == 0 || facts.InputVersion != 9 || facts.CompletedVersion != 11 {
		t.Fatal("context retained order/cash/stock/outbox facts", facts)
	}
	t.Log(`CART_CONTEXT_CHANGED_COMPLETE {"real_mysql":true,"confirmed_deletion_required":true,"pending_all_tables_unchanged":true,"late_receipt_all_tables_rollback":true,"historical_replay_after_draft_edits":true,"changed_receipt_refused":true,"concurrent_saves":6,"one_save":true,"original_checkout_immutable":true,"old_checkout_retired":true,"same_collection_reused":true,"contact_shipping_context_after_retirement":true,"new_amount":"160","new_checkout_once":true,"pending_and_paid_observation":true,"unique_new_order_capture_reservations_outbox":true,"completed_version":11,"artificial_provider_only":true,"http_connected":false}`)
}
