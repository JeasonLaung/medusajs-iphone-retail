package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type paymentSessionSignal struct {
	*sessionstore.Store
	resolved chan struct{}
	once     sync.Once
}

func (s *paymentSessionSignal) Resolve(ctx context.Context, realm sessionstore.Realm, token string) (sessionstore.Principal, error) {
	p, e := s.Store.Resolve(ctx, realm, token)
	s.once.Do(func() { close(s.resolved) })
	return p, e
}

// Dedicated Redis capabilities over explicit SQL ownership; this does not
// claim registration, email delivery or customer browser acceptance.
func checkOrderPaymentCustomerHTTP(t *testing.T, ctx context.Context, db *gorm.DB, s orderpayments.Service, p orderpayments.Principal, in orderpayments.Input) orderpayments.Input {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", MaxRetries: -1, Protocol: 2, DisableIdentity: true, ContextTimeoutEnabled: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	t.Cleanup(func() { client.Close() })
	sessions, e := sessionstore.New(client, "eurorefurb:order-payment-http:v1:")
	if e != nil {
		t.Fatal(e)
	}
	issue := func(realm sessionstore.Realm, id, version string) sessionstore.Issued {
		t.Helper()
		out, e := sessions.Issue(ctx, realm, sessionstore.Principal{UserID: id, Version: version})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { sessions.Revoke(context.Background(), realm, out.Token) })
		return out
	}
	hash, e := bcrypt.GenerateFromPassword([]byte("owned-payment-http-password"), bcrypt.MinCost)
	if e != nil {
		t.Fatal(e)
	}
	if e := db.Exec("UPDATE customer_accounts SET password_hash=? WHERE id=?", string(hash), p.AccountID).Error; e != nil {
		t.Fatal(e)
	}
	ring, _ := protected.New("customer", map[string][]byte{"customer": bytes.Repeat([]byte{48}, 32)})
	store, e := customer.New(db, ring, sessions, s.ReturnOrigin)
	if e != nil {
		t.Fatal(e)
	}
	login, e := store.Login(ctx, "supplement@example.invalid", "owned-payment-http-password")
	if e != nil {
		t.Fatal(e)
	}
	issued := login.Session
	t.Cleanup(func() { sessions.Revoke(context.Background(), sessionstore.Customer, issued.Token) })
	var original struct{ VerifiedAt *time.Time }
	if db.Table("customer_accounts").Select("verified_at").Where("id=?", p.AccountID).Take(&original).Error != nil {
		t.Fatal("original verified timestamp")
	}

	admin := issue(sessionstore.Admin, p.AccountID, strings.Repeat("a", 64))

	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,auth_version,verified_at,created_at,updated_at) VALUES('foreign-payment-account','foreign-payment@example.invalid','artificial-unusable-hash','Other','Fixture',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE customer_accounts SET password_hash=? WHERE id='foreign-payment-account'", string(hash))
	otherLogin, e := store.Login(ctx, "foreign-payment@example.invalid", "owned-payment-http-password")
	if e != nil {
		t.Fatal(e)
	}
	other := otherLogin.Session
	t.Cleanup(func() { sessions.Revoke(context.Background(), sessionstore.Customer, other.Token) })
	h, e := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: s.ReturnOrigin, Secure: strings.HasPrefix(s.ReturnOrigin, "https:"), StoreID: s.StoreID, OrderPayments: &s})
	if e != nil {
		t.Fatal(e)
	}
	router := gin.New()
	h.Register(router.Group("/api/store/customer"))
	cookie := "retail_customer_session"
	if strings.HasPrefix(s.ReturnOrigin, "https:") {
		cookie = "__Host-" + cookie
	}
	path := "/api/store/customer/orders/" + in.OrderID + "/payments"
	id := uuid.NewString()
	fields := map[string]string{"operation_id": id, "expected_version": strconv.Itoa(in.ExpectedVersion), "collection_id": in.CollectionID, "amount": in.Amount, "currency_code": in.CurrencyCode}
	encoded, _ := json.Marshal(fields)
	body := string(encoded)
	call := func(method, path, body, origin, token, csrf string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, s.ReturnOrigin+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if token != "" {
			req.AddCookie(&http.Cookie{Name: cookie, Value: token})
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("payment HTTP %s got %d want %d: %s", method, w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Error("payment privacy headers missing")
		}
		for _, secret := range []string{"session_id", "request_sha256", "provider_id", "auth_version", "cart_id", "password_hash"} {
			if strings.Contains(w.Body.String(), `"`+secret+`"`) {
				t.Error("private payment field leaked", secret)
			}
		}
		return w
	}
	contextResponse := call("GET", path, "", "", issued.Token, "", 200)
	var before orderpayments.Context
	if json.Unmarshal(contextResponse.Body.Bytes(), &before) != nil || before.PendingDifference != "6" || before.Next == nil || before.Next.Amount != "6.00" || len(before.Payments) != 0 {
		t.Fatal("current exact payment consent missing")
	}
	call("GET", path, "", "", other.Token, "", 404)
	call("GET", path+"?status=paid", "", "", issued.Token, "", 404)
	call("POST", path, body, s.ReturnOrigin, "", issued.CSRF, 401)
	call("POST", path, body, s.ReturnOrigin, issued.Token, "", 403)
	call("POST", path, body, "https://foreign.example", issued.Token, issued.CSRF, 403)
	call("POST", path, body, "", issued.Token, issued.CSRF, 403)
	call("POST", path, body, s.ReturnOrigin, admin.Token, admin.CSRF, 401)
	call("POST", path, body, s.ReturnOrigin, other.Token, other.CSRF, 404)
	for _, bad := range []string{strings.Replace(body, `"amount":"6.00"`, `"amount":"6.00","amount":"6.00"`, 1), strings.Replace(body, `"amount":"6.00"`, `"amount":6`, 1), strings.Replace(body, `"amount":"6.00"`, `"amount":null`, 1), strings.Replace(body, `"expected_version":"3"`, `"expected_version":"03"`, 1), body + `{}`, strings.TrimSuffix(body, "}") + `,"account_id":"foreign-payment-account"}`} {
		call("POST", path, bad, s.ReturnOrigin, issued.Token, issued.CSRF, 400)
	}
	call("POST", path+"?status=paid", body, s.ReturnOrigin, issued.Token, issued.CSRF, 404)
	changed := strings.Replace(body, `"amount":"6.00"`, `"amount":"5.00"`, 1)
	call("POST", path, changed, s.ReturnOrigin, issued.Token, issued.CSRF, 409)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Go(func() { call("POST", path, body, s.ReturnOrigin, issued.Token, issued.CSRF, 202) })
	}
	wg.Wait()
	contextResponse = call("GET", path, "", "", issued.Token, "", 200)
	var staged orderpayments.Context
	if json.Unmarshal(contextResponse.Body.Bytes(), &staged) != nil || staged.Next != nil || len(staged.Payments) != 1 || staged.Payments[0].CheckoutURL != nil {
		t.Fatal("staged context offered a second charge or unverified URL")
	}
	// Protected creation is an artificial SQL fixture inside a rollback. No
	// adapter POST or new external fact is claimed by this URL presentation test.
	rollback := errors.New("owned presentation rollback")
	if e := db.Transaction(func(tx *gorm.DB) error {
		var row struct {
			SessionID     string
			RequestSHA256 []byte
		}
		if e := tx.Table("order_payment_operations").Select("session_id,request_sha256").Where("operation_id=?", id).Take(&row).Error; e != nil {
			return e
		}
		key := payments.EffectKey{ScopeID: in.OrderID, OperationID: id, Kind: "create_session", ResourceID: row.SessionID}
		copy(key.Digest[:], row.RequestSHA256)
		receipt, req, e := payments.ClaimSessionRequest(ctx, tx, key, s.Ring, time.Minute)
		if e != nil {
			return e
		}
		created := payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(req, "tr_owned_view", "https://www.mollie.com/checkout/owned")}
		if _, e = payments.RecordSessionCreation(ctx, tx, key, receipt.AttemptID, s.Ring, created); e != nil {
			return e
		}
		if e = tx.Table("order_payment_operations").Where("operation_id=?", id).Updates(map[string]any{"state": "uncertain", "dispatched_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error; e != nil {
			return e
		}
		view, e := s.ViewTx(ctx, tx, p, in.OrderID, id)
		if e != nil || view.CheckoutURL == nil || *view.CheckoutURL != "https://www.mollie.com/checkout/owned" || view.Applied {
			t.Fatal("protected exact-due checkout URL", view, e)
		}
		if e = tx.Exec("UPDATE orders SET version=version+1 WHERE id=?", in.OrderID).Error; e != nil {
			return e
		}
		changed, e := s.ViewTx(ctx, tx, p, in.OrderID, id)
		if e != nil || changed.CheckoutURL != nil {
			t.Fatal("stale version offered payment URL", e)
		}
		if e = tx.Exec("UPDATE orders SET version=version-1 WHERE id=?", in.OrderID).Error; e != nil {
			return e
		}
		if e = tx.Exec("UPDATE payment_effect_journal SET result_envelope=REPEAT('x',100) WHERE scope_id=? AND operation_id=?", in.OrderID, id).Error; e != nil {
			return e
		}
		if _, e = s.ViewTx(ctx, tx, p, in.OrderID, id); e == nil {
			t.Fatal("corrupt creation receipt offered URL")
		}
		return rollback
	}); !errors.Is(e, rollback) {
		t.Fatal("URL fixture failed", e)
	}
	read := path + "/" + id
	w := call("GET", read, "", "", issued.Token, "", 200)
	var response struct {
		Payment orderpayments.Result `json:"payment"`
	}
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Payment.OperationID != id || response.Payment.State != "pending" || !response.Payment.CanCancel || response.Payment.Applied {
		t.Fatal("owned receipt missing")
	}
	call("GET", read, "", "", other.Token, "", 404)
	call("GET", read, "", "", admin.Token, "", 401)
	for _, change := range []string{"enabled=FALSE", "verified_at=NULL", "auth_version=2"} {
		exec("UPDATE customer_accounts SET "+change+" WHERE id=?", p.AccountID)
		call("GET", read, "", "", issued.Token, "", 401)
		exec("UPDATE customer_accounts SET enabled=TRUE,verified_at=?,auth_version=1 WHERE id=?", original.VerifiedAt, p.AccountID)
	}
	cancelBody := `{"operation_id":"` + uuid.NewString() + `"}`
	exec("CREATE TRIGGER payment_http_cancel_failure BEFORE INSERT ON order_payment_cancellations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned HTTP cancellation failure'")
	call("POST", read+"/cancel", cancelBody, s.ReturnOrigin, issued.Token, issued.CSRF, 503)
	exec("DROP TRIGGER payment_http_cancel_failure")
	w = call("GET", read, "", "", issued.Token, "", 200)
	if json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Payment.State != "pending" || !response.Payment.CanCancel {
		t.Fatal("HTTP failed cancellation leaked writes")
	}
	exec("UPDATE order_payment_runtime SET enabled=FALSE WHERE id=1")
	call("POST", read+"/cancel", cancelBody, s.ReturnOrigin, issued.Token, issued.CSRF, 200)
	call("POST", read+"/cancel", cancelBody, s.ReturnOrigin, issued.Token, issued.CSRF, 200)
	call("GET", read, "", "", issued.Token, "", 200)
	exec("UPDATE order_payment_runtime SET enabled=TRUE WHERE id=1")
	signal := &paymentSessionSignal{Store: sessions, resolved: make(chan struct{})}
	gated, e := customer.New(db, ring, signal, s.ReturnOrigin)
	if e != nil {
		t.Fatal(e)
	}
	held := db.WithContext(ctx).Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	var locked struct{ ID string }
	if e := held.Raw("SELECT id FROM customer_accounts WHERE id=? FOR UPDATE", p.AccountID).Scan(&locked).Error; e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := gated.ReadOrderPayment(ctx, issued.Token, &s, in.OrderID, id); done <- e }()
	select {
	case <-signal.resolved:
	case <-ctx.Done():
		t.Fatal("waiting payment session lookup")
	}
	if e := sessions.Revoke(ctx, sessionstore.Customer, issued.Token); e != nil {
		t.Fatal(e)
	}
	if e := held.Rollback().Error; e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != customer.ErrUnauthorized {
			t.Fatal("revocation while waiting for SQL account lock accepted", e)
		}
	case <-ctx.Done():
		t.Fatal("waiting revoked payment read")
	}

	call("GET", read, "", "", issued.Token, "", 401)
	var count int64
	if db.Table("order_payment_operations").Where("operation_id=? AND state='canceled' AND dispatched_at IS NULL", id).Count(&count).Error != nil || count != 1 {
		t.Fatal("HTTP instruction not canceled exactly once")
	}
	t.Log(`ORDER_PAYMENT_CUSTOMER_HTTP_REHEARSAL {"real_redis_customer_capability":true,"authenticated_transaction":true,"revocation_while_waiting_account_lock_denied":true,"csrf_origin":true,"realm_and_owner_denied":true,"strict_wire":true,"concurrent_prepare":6,"private_receipt":true,"revoked_disabled_stale_unverified_denied":true,"late_cancel_rollback":true,"feature_off_read_cancel":true,"exact_current_consent_and_history":true,"active_instruction_no_second_consent":true,"protected_checkout_url":true,"stale_version_hides_url":true,"corrupt_creation_url_denied":true,"provider_calls":0,"registration_browser":false,"payment_notice":false}`)
	in.PreviousOperationID = id
	return in
}
