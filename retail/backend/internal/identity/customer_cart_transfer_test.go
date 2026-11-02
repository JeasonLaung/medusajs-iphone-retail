package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Called by the owned SQL/Redis cart suite after its ordinary anonymous checkout
// tests. Uses newly registered accounts and artificial catalog/provider facts.
func checkCustomerCartTransfer(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), o cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("owned customer transfer SQL", err)
		}
	}
	ring, _ := protected.New("transfer", map[string][]byte{"transfer": bytes.Repeat([]byte{62}, 32)})
	accounts, e := customer.New(db, ring, sessions, "http://127.0.0.1:18080")
	if e != nil {
		t.Fatal(e)
	}
	makeAccount := func(email string) customer.Login {
		t.Helper()
		a, e := accounts.Register(ctx, customer.Registration{Email: email, Password: "transfer-test-password", FirstName: "New", LastName: "Customer", Country: "dk", Locale: "en"})
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE customer_accounts SET verified_at=UTC_TIMESTAMP(6) WHERE id=?", a.ID)
		login, e := accounts.Login(ctx, email, "transfer-test-password")
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = sessions.Revoke(context.Background(), sessionstore.Customer, login.Session.Token) })
		return login
	}
	a, b := makeAccount("cart-transfer@example.invalid"), makeAccount("cart-transfer-other@example.invalid")
	// Prior paid-order suites retain their real reservations in this owned
	// database. Supply fresh artificial fixture stock for this additional order
	// rather than bypassing the same available-stock checks used by checkout.
	exec("UPDATE inventory_levels SET stocked_quantity=stocked_quantity+5 WHERE id='add-level'")
	ref, in := makeCart()
	possession, e := sessions.IssueCart(ctx, ref)
	if e != nil {
		t.Fatal(e)
	}
	defer sessions.RevokeCart(context.Background(), possession.Token)
	command := cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: in.Version}
	transfer := func(token string, cmd cart.CustomerTransferCommand) (cart.ContactResult, error) {
		return accounts.TransferCart(ctx, token, possession.Token, "shop", cmd, o.ManualProviders)
	}
	if _, e := transfer(a.Session.Token, command); !errors.Is(e, customer.ErrUnavailable) {
		t.Fatal("default transfer gate open", e)
	}
	exec("UPDATE customer_cart_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE customer_cart_runtime SET enabled=FALSE WHERE id=1")
	if _, e := accounts.TransferCart(ctx, a.Session.Token, possession.Token, "other-shop", command, o.ManualProviders); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("cross-store transfer", e)
	}
	count := func(table string) int64 {
		t.Helper()
		var n int64
		if db.Table(table).Count(&n).Error != nil {
			t.Fatal("transfer count")
		}
		return n
	}
	beforeOwner, beforeReceipt := count("customer_cart_ownership"), count("customer_cart_operations")
	beforeContacts, e := cart.ReadContacts(ctx, db, "shop", ref)
	if e != nil {
		t.Fatal(e)
	}
	exec("CREATE TRIGGER transfer_receipt_failure BEFORE INSERT ON customer_cart_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned transfer failure'")
	if _, e := transfer(a.Session.Token, command); !errors.Is(e, customer.ErrUnavailable) {
		t.Fatal("late transfer failure accepted", e)
	}
	exec("DROP TRIGGER transfer_receipt_failure")
	afterContacts, e := cart.ReadContacts(ctx, db, "shop", ref)
	if e != nil || afterContacts.Email != beforeContacts.Email || afterContacts.Version != beforeContacts.Version || count("customer_cart_ownership") != beforeOwner || count("customer_cart_operations") != beforeReceipt {
		t.Fatal("partial transfer committed")
	}
	var wg sync.WaitGroup
	results := make([]cart.ContactResult, 6)
	errs := make([]error, 6)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = transfer(a.Session.Token, command) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.CartID != ref.CartID || r.Version != "5" {
			t.Fatal("concurrent transfer", errs[i], r)
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("transfer applied more than once")
	}
	var legacy *string
	if db.Table("carts").Select("customer_id").Where("id=?", ref.CartID).Scan(&legacy).Error != nil || legacy != nil {
		t.Fatal("legacy customer relation modified")
	}
	contacts, e := cart.ReadContacts(ctx, db, "shop", ref)
	if e != nil || contacts.Email != "cart-transfer@example.invalid" || contacts.Version != "5" {
		t.Fatal("transfer email/version", e)
	}
	view, e := cart.ReadAmounts(ctx, db, "shop", ref)
	if e != nil || len(view.Lines) != 1 || view.Totals["total"] != "158.75" {
		t.Fatal("transfer lost cart items/amounts", e)
	}
	// A different verified account cannot take over a bound cart, even with its
	// current possession. Edited checkout email does not change the saved owner.
	if _, e := transfer(b.Session.Token, cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: "5"}); !errors.Is(e, customer.ErrCartConflict) {
		t.Fatal("foreign account transfer", e)
	}
	exec("UPDATE carts SET email='edited-checkout@example.invalid' WHERE id=?", ref.CartID)
	same := cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: "5"}
	if r, e := transfer(a.Session.Token, same); e != nil || r.Version != "5" {
		t.Fatal("same account noop", e)
	}
	contacts, e = cart.ReadContacts(ctx, db, "shop", ref)
	if e != nil || contacts.Email != "edited-checkout@example.invalid" {
		t.Fatal("same account rewrote checkout email")
	}
	if r, e := transfer(a.Session.Token, command); e != nil || !r.Replayed || r.Version != "5" {
		t.Fatal("lost-response transfer replay", e)
	}
	changed := command
	changed.Version = "5"
	if _, e := transfer(a.Session.Token, changed); !errors.Is(e, customer.ErrCartConflict) {
		t.Fatal("changed retry accepted", e)
	}
	exec("UPDATE customer_accounts SET enabled=FALSE WHERE id=?", a.Customer.ID)
	if _, e := transfer(a.Session.Token, same); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("disabled account replay", e)
	}
	exec("UPDATE customer_accounts SET enabled=TRUE WHERE id=?", a.Customer.ID)
	// Early login: empty cart and then an item without an address both use the
	// real refresh path. No invented country or checkout address is needed.
	id := uuid.NewString()
	blank, e := cart.Create(ctx, db, cart.CreateCommand{OperationID: id, ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: "en"})
	if e != nil {
		t.Fatal(e)
	}
	emptyPossession, e := sessions.IssueCart(ctx, blank.Reference)
	if e != nil {
		t.Fatal(e)
	}
	defer sessions.RevokeCart(context.Background(), emptyPossession.Token)
	emptyIn := cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: "1"}
	if r, e := accounts.TransferCart(ctx, b.Session.Token, emptyPossession.Token, "shop", emptyIn, o.ManualProviders); e != nil || r.Version != "2" {
		t.Fatal("empty cart transfer", e)
	}
	// A new cart with one product but no address exercises forced no-country tax.
	id = uuid.NewString()
	noAddress, e := cart.Create(ctx, db, cart.CreateCommand{OperationID: id, ScopeHash: sha256.Sum256([]byte(id)), StoreID: "shop", RegionID: "region", Locale: "en"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := cart.AddItem(ctx, db, "shop", noAddress.Reference, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"}); e != nil {
		t.Fatal(e)
	}
	p, e := sessions.IssueCart(ctx, noAddress.Reference)
	if e != nil {
		t.Fatal(e)
	}
	defer sessions.RevokeCart(context.Background(), p.Token)
	if r, e := accounts.TransferCart(ctx, b.Session.Token, p.Token, "shop", cart.CustomerTransferCommand{OperationID: uuid.NewString(), Version: "2"}, o.ManualProviders); e != nil || r.Version != "3" {
		t.Fatal("unaddressed item transfer", e)
	}
	// Provider adapter is local artificial evidence only. A webhook-style server
	// completion needs no customer cookie or browser return to save order ownership.
	providerID := "tr_transfer" + strings.ReplaceAll(uuid.NewString(), "-", "")
	registry, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(_ context.Context, r payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(r, providerID, "https://www.mollie.com/checkout/transfer")}, nil
		},
		observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			raw, _ := json.Marshal(map[string]any{"id": id, "status": "paid", "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}})
			return payments.SessionObservation{PaymentID: id, ProviderStatus: "paid", Action: "successful", Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: "authorized", Data: raw}}, nil
		},
	}})
	if e != nil {
		t.Fatal(e)
	}
	o.Adapters = registry
	in.Version = "5"
	if _, e := cart.StartInitialPayment(ctx, db, "shop", ref, in, o); e != nil {
		t.Fatal("transferred checkout", e)
	}
	if _, e := transfer(a.Session.Token, same); !errors.Is(e, customer.ErrCartConflict) {
		t.Fatal("frozen transfer accepted", e)
	}
	if got, e := cart.ObserveInitialPayment(ctx, db, "shop", ref, o); e != nil || got.State != "paid_pending_order" {
		t.Fatal("transferred paid observation", e)
	}
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	// Saved account facts must survive a feature toggle and a late failure must
	// roll back the entire local order/payment/stock/outbox transaction.
	exec("UPDATE customer_cart_runtime SET enabled=FALSE WHERE id=1")
	fingerprint := func() string {
		t.Helper()
		var out strings.Builder
		for _, table := range []string{"orders", "payments", "payment_captures", "order_notification_outbox", "inventory_reservations", "cart_completion_receipts", "customer_order_ownership"} {
			out.WriteString(table)
			out.WriteString(strconv.FormatInt(count(table), 10))
		}
		return out.String()
	}
	before := fingerprint()
	exec("CREATE TRIGGER transfer_order_failure BEFORE INSERT ON customer_order_ownership FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned order owner failure'")
	if _, e := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o); e == nil {
		t.Fatal("owner insert failure accepted")
	}
	exec("DROP TRIGGER transfer_order_failure")
	if fingerprint() != before {
		t.Fatal("failed owner insert partially completed order")
	}
	completed, e := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o)
	if e != nil {
		t.Fatal("server completion without login cookie", e)
	}
	var owner struct{ CustomerID, CartID, StoreID string }
	if db.Table("customer_order_ownership").Where("order_id=?", completed.OrderID).Take(&owner).Error != nil || owner.CustomerID != a.Customer.ID || owner.CartID != ref.CartID || owner.StoreID != "shop" {
		t.Fatal("server completion lost new account owner")
	}
	if r, e := cart.CompleteCurrentPaidOrder(ctx, db, "shop", ref, o); e != nil || r.OrderID != completed.OrderID || !r.Replayed {
		t.Fatal("completed transfer replay", e)
	}
	if _, e := accounts.Order(ctx, a.Session.Token, completed.OrderID); e != nil {
		t.Fatal("new account cannot read its order", e)
	}
	if e := accounts.AttachOrder(ctx, b.Session.Token, possession.Token, completed.OrderID); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("foreign account claimed transferred order", e)
	}
	if _, e := accounts.Order(ctx, b.Session.Token, completed.OrderID); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("foreign account read transferred order", e)
	}
	t.Log(`CUSTOMER_CART_TRANSFER_REHEARSAL {"real_mysql_redis":true,"new_accounts_only":true,"default_off":true,"atomic_refresh_and_receipt":true,"concurrent_replay":6,"same_customer_preserves_email":true,"foreign_customer_denied":true,"empty_and_unaddressed_cart":true,"server_completion_owner":true,"feature_toggle_recovery":true,"owner_failure_atomic":true}`)
}
