package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCompletionHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, o cart.InitialPaymentOptions) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("completion HTTP fixture")
		}
	}
	issued, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: []string{"delivery-provider"}, InitialPayment: &o})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/store/carts/" + ref.CartID + "/complete"
	call := func(url, body, requestOrigin, csrf string, cookie bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("POST", url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if requestOrigin != "" {
			req.Header.Set("Origin", requestOrigin)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie {
			req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: issued.Token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("completion HTTP got %d want %d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || len(w.Result().Cookies()) != 0 {
			t.Fatalf("completion response cache/cookies: status=%d body_bytes=%d cache=%q cookies=%d",w.Code,len(body),w.Header().Get("Cache-Control"),len(w.Result().Cookies()))
		}
		for _, private := range []string{"tr_initial", "payment_id", "capture_id", "request_sha256", "generation", "Email", "email", "checkout_url"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("completion private field escaped")
			}
		}
		return w
	}
	call(path, "{}", origin, issued.CSRF, false, 401)
	call(path, "{}", "https://other.example", issued.CSRF, true, 403)
	call(path, "{}", "", issued.CSRF, true, 403)
	call(path, "{}", origin, "", true, 403)
	call(path+"?paid=true", "{}", origin, issued.CSRF, true, 400)
	for _, body := range []string{`{"order_id":"foreign"}`, `{"status":"paid"}`, `{"version":"4"}`, `{"operation_id":"foreign"}`, `{"amount":"0"}`, `{} {}`, `[]`, `null`} {
		call(path, body, origin, issued.CSRF, true, 400)
	}
	call(path, strings.Repeat(" ", int(platform.MaxJSONBodyBytes))+"{}", origin, issued.CSRF, true, 413)
	call(path, "{}", origin, issued.CSRF, true, 503)
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	call(path, "{}", origin, issued.CSRF, true, 409)
	registry, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{
		create: func(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			t.Error("complete issued external payment")
			return payments.CreateUncertain, payments.CreatedSession{}, nil
		},
		observe: func(_ context.Context, r payments.SessionRequest, id string) (payments.SessionObservation, error) {
			raw, _ := json.Marshal(map[string]any{"id": id, "status": "paid", "amount": map[string]string{"value": r.Amount, "currency": strings.ToUpper(r.Currency)}, "metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)}})
			return payments.SessionObservation{PaymentID: id, ProviderStatus: "paid", Action: "successful", Amount: r.Amount, Currency: r.Currency, Session: payments.CreatedSession{Status: "authorized", Data: raw}}, nil
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	observing := o
	observing.Adapters = registry
	if _, err = cart.ObserveInitialPayment(ctx, db, "shop", ref, observing); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'")
	defer exec("UPDATE inventory_command_capabilities SET enabled=FALSE WHERE command='inventory.reservations'")
	exec("CREATE TRIGGER http_completion_late_failure BEFORE INSERT ON cart_completion_receipts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned HTTP late failure'")
	call(path, "{}", origin, issued.CSRF, true, 503)
	exec("DROP TRIGGER http_completion_late_failure")
	var n int64
	if db.Table("order_carts").Where("cart_id=?", ref.CartID).Count(&n).Error != nil || n != 0 {
		t.Fatal("HTTP failure committed partial order")
	}
	first := call(path, "{}", origin, issued.CSRF, true, 200)
	var result struct{ CartID, OrderID, Version string }
	var body map[string]any
	if json.Unmarshal(first.Body.Bytes(), &body) != nil || len(body) != 3 {
		t.Fatal("completion projection")
	}
	result.CartID, _ = body["cart_id"].(string)
	result.OrderID, _ = body["order_id"].(string)
	result.Version, _ = body["version"].(string)
	if result.CartID != ref.CartID || !strings.HasPrefix(result.OrderID, "order_") || result.Version != "6" {
		t.Fatal("completion result differs")
	}
	replay := call(path, "{}", origin, issued.CSRF, true, 200)
	if replay.Body.String() != first.Body.String() {
		t.Fatal("HTTP lost response retry differs")
	}
	exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	call(path, "{}", origin, issued.CSRF, true, 503)
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_ownership SET active=FALSE WHERE cart_id=?", ref.CartID)
	call(path, "{}", origin, issued.CSRF, true, 404)
	exec("UPDATE cart_ownership SET active=TRUE WHERE cart_id=?", ref.CartID)
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("HTTP session revoke")
	}
	call(path, "{}", origin, issued.CSRF, true, 401)
	if db.Table("order_carts").Where("cart_id=?", ref.CartID).Count(&n).Error != nil || n != 1 {
		t.Fatal("HTTP duplicate order")
	}
	t.Log(`CART_COMPLETION_HTTP_REHEARSAL {"real_gin_mysql_redis":true,"fresh_commit":true,"strict_empty_intention":true,"server_resolved_operation":true,"possession_origin_csrf":true,"paid_required":true,"late_failure_retry":true,"closed_cart_replay":true,"no_cookie_extension":true,"private_projection":true,"revoked_replay_denied":true}`)
}
