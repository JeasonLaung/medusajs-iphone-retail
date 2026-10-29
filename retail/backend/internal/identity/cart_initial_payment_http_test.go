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
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkInitialPaymentHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, in cart.InitialPaymentCommand, options cart.InitialPaymentOptions) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	issued, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal("delivery HTTP session failed")
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: []string{"delivery-provider"}, InitialPayment: &options})
	if err != nil {
		t.Fatal("delivery HTTP router failed")
	}
	base := "/api/store/carts/" + ref.CartID
	call := func(method, path, body, key, requestOrigin, csrf string, cookie bool, status int) *httptest.ResponseRecorder {
		t.Helper()

		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
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
			t.Fatalf("delivery HTTP %d want %d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || len(w.Result().Cookies()) != 0 {
			t.Fatal("delivery cache/cookie contract differs")
		}
		for _, field := range []string{"not-for-browser", "provider_id", "location_ids", "generation", "request_sha256", "raw_amount"} {
			if strings.Contains(w.Body.String(), field) {
				t.Fatal("private delivery field escaped")
			}
		}

		return w
	}

	path := base + "/payment-attempts"
	body := `{"version":"4"}`
	key := in.OperationID
	call("POST", path, body, key, origin, issued.CSRF, false, 401)
	call("POST", path, body, key, "https://unrelated.example", issued.CSRF, true, 403)
	call("POST", path, body, key, origin, "", true, 403)
	for _, bad := range []string{`{"version":4}`, `{"version":null}`, `{"version":"4","version":"4"}`, `{"version":"4","amount":"0"}`, `{"version":"4","provider_id":"foreign"}`, body + ` {}`} {
		call("POST", path, bad, key, origin, issued.CSRF, true, 400)
	}
	call("POST", path, strings.Repeat(" ", int(platform.MaxJSONBodyBytes))+body, key, origin, issued.CSRF, true, 413)
	replay := call("POST", path, body, key, origin, issued.CSRF, true, 200)
	var result cart.InitialPaymentResult
	if json.Unmarshal(replay.Body.Bytes(), &result) != nil || result.State != "ready" || result.Version != "5" || result.CheckoutURL == nil {
		t.Fatal("initial HTTP projection differs")
	}
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("initial HTTP revoke failed")
	}
	call("POST", path, body, key, origin, issued.CSRF, true, 401)
	t.Log(`CART_INITIAL_PAYMENT_HTTP_REHEARSAL {"real_gin_mysql_redis":true,"possession_origin_csrf":true,"price_provider_injection_denied":true,"strict_json":true,"private_no_store_413":true,"no_private_provider_fields":true,"no_cookie_extension":true,"revoked_replay_denied":true}`)
}
