package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func checkInitialPayments(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, source sessionstore.CartReference) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("initial payment fixture", err)
		}
	}
	const provider = "pp_mollie-hosted-checkout_mollie"
	exec("INSERT INTO payment_providers(id,is_enabled,created_at,updated_at) VALUES(?,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", provider)
	exec("INSERT INTO commerce_region_payment_providers(region_id,payment_provider_id,id,created_at,updated_at) VALUES('region',?,'checkout-region-provider',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", provider)
	ring, err := protected.New("initial", map[string][]byte{"initial": bytes.Repeat([]byte{0x71}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	var uncertain atomic.Bool
	adapters, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: executionCreate(func(callCtx context.Context, in payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
		calls.Add(1)
		if uncertain.Load() {
			return payments.CreateUncertain, payments.CreatedSession{}, nil
		}
		if in.Amount != "158.75" || in.Currency != "eur" || !strings.HasPrefix(in.SessionID, "payses_") || !bytes.Contains(in.Data, []byte(in.SessionID)) {
			t.Error("initial provider request differs")
		}
		// A distinct SQL connection observes the committed freeze before dispatch.
		var freeze string
		if db.WithContext(callCtx).Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", inScope(in)).Scan(&freeze).Error != nil || freeze == "" {
			t.Error("provider dispatched before durable freeze")
		}
		return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: json.RawMessage(`{"id":"tr_initial","_links":{"checkout":{"href":"https://www.mollie.com/checkout/initial"}},"private":"hidden"}`)}, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	options := cart.InitialPaymentOptions{Ring: ring, Adapters: adapters, ReturnOrigin: "http://127.0.0.1:18080", ManualProviders: []string{"delivery-provider"}}
	contacts, err := cart.ReadContacts(ctx, db, "shop", source)
	if err != nil {
		t.Fatal(err)
	}
	makeCart := func() (sessionstore.CartReference, cart.InitialPaymentCommand) {
		t.Helper()
		id := uuid.NewString()
		created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: id, ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: "en"})
		if err != nil {
			t.Fatal(err)
		}
		ref := created.Reference
		if _, err = cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"}); err != nil {
			t.Fatal(err)
		}
		if _, err = cart.SaveContacts(ctx, db, "shop", ref, cart.ContactCommand{OperationID: uuid.NewString(), Version: "2", Email: contacts.Email, Shipping: *contacts.Shipping, Billing: *contacts.Billing}, options.ManualProviders); err != nil {
			t.Fatal(err)
		}
		if _, err = cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: "3", OptionID: "delivery-standard"}, options.ManualProviders); err != nil {
			t.Fatal(err)
		}
		return ref, cart.InitialPaymentCommand{OperationID: uuid.NewString(), Version: "4"}
	}
	ref, in := makeCart()
	count := func(table string) int64 {
		var n int64
		if db.Table(table).Count(&n).Error != nil {
			t.Fatal("count failed")
		}
		return n
	}
	before := count("cart_checkout_attempts")
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != cart.ErrSchema {
		t.Fatalf("default gate differs: %v", err)
	}
	exec("UPDATE cart_checkout_runtime SET enabled=TRUE WHERE id=1")
	checkPaymentOptions(t, ctx, db, sessions, ref, options)
	exec("UPDATE payment_providers SET is_enabled=FALSE WHERE id=?", provider)
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != cart.ErrConfiguration {
		t.Fatalf("disabled provider differs: %v", err)
	}
	exec("UPDATE payment_providers SET is_enabled=TRUE WHERE id=?", provider)
	exec("UPDATE inventory_levels SET stocked_quantity=0 WHERE id='add-level'")
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != cart.ErrItemStock {
		t.Fatalf("missing stock differs: %v", err)
	}
	exec("UPDATE inventory_levels SET stocked_quantity=10 WHERE id='add-level'")
	if calls.Load() != 0 || count("cart_checkout_attempts") != before {
		t.Fatal("rejection dispatched or stored attempt")
	}
	exec(`CREATE TRIGGER fail_initial_attempt BEFORE INSERT ON cart_checkout_attempts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned initial failure'`)
	collections := count("payment_collections")
	effects := count("payment_effect_journal")
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != cart.ErrStorage {
		t.Fatalf("late rollback differs: %v", err)
	}
	exec("DROP TRIGGER fail_initial_attempt")
	if count("payment_collections") != collections || count("payment_effect_journal") != effects || calls.Load() != 0 {
		t.Fatal("initial staging rollback partial")
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, options)
			if err != nil || got.Version != "5" || got.CartID != ref.CartID || got.State != "ready" && got.State != "processing" {
				t.Errorf("concurrent initial result: %v %s", err, got.State)
			}
		}()
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate provider dispatch")
	}
	result, err := cart.StartInitialPayment(ctx, db, "shop", ref, in, options)
	if err != nil || result.State != "ready" || result.CheckoutURL == nil || *result.CheckoutURL != "https://www.mollie.com/checkout/initial" {
		t.Fatalf("ready replay: %v %s", err, result.State)
	}
	available, optionsErr := cart.ReadPaymentOptions(ctx, db, "shop", ref, options)
	if optionsErr != nil || !available.AttemptStarted || len(available.Providers) != 0 || available.Version != "5" {
		t.Fatal("frozen cart exposed another payment initiation")
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("hidden")) || bytes.Contains(encoded, []byte("tr_initial")) {
		t.Fatal("private provider data in response")
	}
	var saved string
	if db.Table("payment_sessions").Select("data").Where("id=(SELECT session_id FROM cart_checkout_attempts WHERE cart_id=?)", ref.CartID).Scan(&saved).Error != nil || !strings.HasPrefix(saved, "ers1.") || strings.Contains(saved, "hidden") {
		t.Fatal("new session not sealed")
	}
	if _, err = cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: "5", OptionID: "delivery-pickup"}, options.ManualProviders); err != cart.ErrConflict {
		t.Fatal("checkout freeze allowed shipping change")
	}
	if _, err = cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "5", VariantID: "add-v", Quantity: "1"}); err != cart.ErrConflict {
		t.Fatal("checkout freeze allowed item change")
	}
	changed := in
	changed.OperationID = uuid.NewString()
	changed.Version = "5"
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, changed, options); err != cart.ErrConflict {
		t.Fatal("new attempt bypassed freeze")
	}
	wrong := ref
	wrong.Generation = strings.Repeat("a", 64)
	if _, err = cart.StartInitialPayment(ctx, db, "shop", wrong, in, options); err != cart.ErrDenied {
		t.Fatal("wrong owner got receipt")
	}
	exec("INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) SELECT 'foreign-cart',collection_id,'initial-shared-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM cart_checkout_attempts WHERE cart_id=?", ref.CartID)
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != cart.ErrConflict {
		t.Fatal("shared collection receipt exposed")
	}
	exec("DELETE FROM cart_payment_collections WHERE id='initial-shared-link'")
	var originalData string
	if db.Table("payment_sessions").Select("data").Where("id=(SELECT session_id FROM cart_checkout_attempts WHERE cart_id=?)", ref.CartID).Scan(&originalData).Error != nil {
		t.Fatal("sealed replay read failed")
	}
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != nil {
		t.Fatal(err)
	}
	var replayData string
	if db.Table("payment_sessions").Select("data").Where("id=(SELECT session_id FROM cart_checkout_attempts WHERE cart_id=?)", ref.CartID).Scan(&replayData).Error != nil || originalData != replayData {
		t.Fatal("ready replay rewrote session")
	}
	// A ready replay never reseals/overwrites changed provider state.
	exec("UPDATE payment_sessions SET status='authorized' WHERE id=(SELECT session_id FROM cart_checkout_attempts WHERE cart_id=?)", ref.CartID)
	result, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options)
	if err != nil || result.State != "requires_review" || result.CheckoutURL != nil {
		t.Fatal("authorized session redirected to old checkout")
	}
	httpRef, httpIn := makeCart()
	checkInitialPaymentHTTP(t, ctx, db, sessions, httpRef, httpIn, options)
	checkPaymentObservations(t, ctx, db, sessions, httpRef, options)
	unknownRef, unknownIn := makeCart()
	uncertain.Store(true)
	result, err = cart.StartInitialPayment(ctx, db, "shop", unknownRef, unknownIn, options)
	if err != nil || result.State != "requires_review" {
		t.Fatal("unknown provider outcome not retained")
	}
	result, err = cart.StartInitialPayment(ctx, db, "shop", unknownRef, unknownIn, options)
	if err != nil || result.State != "requires_review" || calls.Load() != 3 {
		t.Fatal("unknown creation was repeated")
	}
	uncertain.Store(false)
	completionRef, completionIn := makeCart()
	if _, err = cart.StartInitialPayment(ctx, db, "shop", completionRef, completionIn, options); err != nil {
		t.Fatal(err)
	}
	checkPaidOrderCompletion(t, ctx, db, completionRef, completionIn, options)
	checkPaidOrderPromotionCompletion(t, ctx, db, makeCart, options)
	completionHTTPRef, completionHTTPIn := makeCart()
	if _, err = cart.StartInitialPayment(ctx, db, "shop", completionHTTPRef, completionHTTPIn, options); err != nil {
		t.Fatal(err)
	}
	checkCompletionHTTP(t, ctx, db, sessions, completionHTTPRef, options)
	checkCheckoutBrowser(t, ctx, db, sessions, options)
	exec("UPDATE cart_checkout_runtime SET enabled=FALSE WHERE id=1")
	if _, err = cart.StartInitialPayment(ctx, db, "shop", ref, in, options); err != cart.ErrSchema {
		t.Fatal("disabled initial replay allowed")
	}
	t.Log(`CART_INITIAL_PAYMENT_REHEARSAL {"real_mysql_redis":true,"provider_isolated_adapter":true,"default_gate_closed":true,"current_provider_and_stock":true,"atomic_refresh_binding":true,"late_failure_rollback":true,"concurrent_requests":8,"one_provider_dispatch":true,"committed_freeze_before_dispatch":true,"exact_settlement":"158.75","sealed_session":true,"immutable_retry":true,"unknown_not_reposted":true,"mutations_frozen":true,"authorized_not_redirected":true,"no_reservation_no_order":true}`)
}
func inScope(in payments.SessionRequest) string {
	var data struct {
		CartID string `json:"cart_id"`
	}
	_ = json.Unmarshal(in.Data, &data)
	return data.CartID
}
