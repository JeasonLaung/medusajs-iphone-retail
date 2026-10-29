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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCartContactsHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, in cart.ContactCommand, fingerprint func() string) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	issued, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal("contact HTTP session failed")
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop"})
	if err != nil {
		t.Fatal("contact HTTP router failed")
	}
	path := "/api/store/carts/" + ref.CartID + "/contacts"
	in.Version = "4"
	in.OperationID = uuid.NewString()
	encoded, _ := json.Marshal(in)
	body := string(encoded)
	call := func(target, payload, key, requestOrigin, csrf string, cookie bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		before := fingerprint()
		req := httptest.NewRequest("POST", target, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
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
			t.Fatalf("contact HTTP status %d want %d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Request-ID") == "" || len(w.Result().Cookies()) != 0 {
			t.Fatal("contact HTTP private/cookie contract differs")
		}
		if strings.Contains(w.Body.String(), in.Email) || strings.Contains(w.Body.String(), in.Shipping.Address1) || strings.Contains(w.Body.String(), "generation") {
			t.Fatal("contact HTTP leaked private fields")
		}
		if status != 200 && before != fingerprint() {
			t.Fatal("contact HTTP rejection changed SQL")
		}
		return w
	}
	call(path, body, in.OperationID, origin, issued.CSRF, false, 401)
	call(path, body, in.OperationID, "https://unrelated.example", issued.CSRF, true, 403)
	call(path, body, in.OperationID, "", issued.CSRF, true, 403)
	call(path, body, in.OperationID, origin, "", true, 403)
	call(path, body, in.OperationID, origin, strings.Repeat("0", 64), true, 403)
	for _, bad := range []string{strings.Replace(body, `"version":"4"`, `"version":"4","email":"evil@example.invalid"`, 1), strings.Replace(body, `"first_name":"New"`, `"first_name":"New","first_name":"Other"`, 1), strings.Replace(body, `"first_name":"New"`, `"id":"old-address","first_name":"New"`, 1), strings.Replace(body, `"version":"4"`, `"unit_price":"0","version":"4"`, 1), strings.Replace(body, `"version":"4"`, `"version":4`, 1), strings.Replace(body, `"last_name":"Customer"`, `"last_name":null`, 1), body + ` {}`} {
		call(path, bad, in.OperationID, origin, issued.CSRF, true, 400)
	}
	call(path, body, "invalid", origin, issued.CSRF, true, 400)
	call(path+"?fields=*", body, in.OperationID, origin, issued.CSRF, true, 400)
	call("/api/store/carts/foreign-cart/contacts", body, in.OperationID, origin, issued.CSRF, true, 404)
	call(path, strings.Repeat(" ", int(platform.MaxJSONBodyBytes))+body, in.OperationID, origin, issued.CSRF, true, 413)
	first := call(path, body, in.OperationID, origin, issued.CSRF, true, 200)
	replay := call(path, body, in.OperationID, origin, issued.CSRF, true, 200)
	var a, b cart.ContactResult
	if json.Unmarshal(first.Body.Bytes(), &a) != nil || json.Unmarshal(replay.Body.Bytes(), &b) != nil || a.Replayed || !b.Replayed || a.Version != "5" || b.Version != a.Version || a.CartID != ref.CartID {
		t.Fatal("contact HTTP durable receipt differs")
	}
	call(path, body, uuid.NewString(), origin, issued.CSRF, true, 409)
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("contact HTTP cleanup failed")
	}
	call(path, body, in.OperationID, origin, issued.CSRF, true, 401)
	t.Log(`CART_CONTACT_HTTP_REHEARSAL {"real_gin_mysql_redis":true,"possession_origin_csrf":true,"nested_duplicate_unknown_and_null_rejected":true,"address_id_and_price_injection_denied":true,"private_no_store_including_413":true,"no_pii_response":true,"no_cookie_extension":true,"replay_and_stale_version":true,"revoked_cookie_denied":true,"checkout_page":false}`)
}
