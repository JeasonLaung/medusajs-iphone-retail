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
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCartContextHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	issued, e := sessions.IssueCart(ctx, ref)
	if e != nil {
		t.Fatal("context HTTP session", e)
	}
	defer sessions.RevokeCart(ctx, issued.Token)
	router, e := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop"})
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/store/carts/" + ref.CartID + "/context"
	in := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "context-multi", Locale: "zh-CN"}
	b, _ := json.Marshal(in)
	body := string(b)
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
			t.Fatalf("context HTTP got %d want %d: %s", w.Code, status, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Request-ID") == "" || len(w.Result().Cookies()) != 0 {
			t.Fatal("context private/cookie contract")
		}
		if status != 200 && before != fingerprint() {
			t.Fatal("rejected context HTTP changed SQL")
		}
		return w
	}
	call(path, body, in.OperationID, origin, issued.CSRF, false, 401)
	call(path, body, in.OperationID, "https://foreign.example", issued.CSRF, true, 403)
	call(path, body, in.OperationID, "", issued.CSRF, true, 403)
	call(path, body, in.OperationID, origin, "", true, 403)
	call(path, body, in.OperationID, origin, strings.Repeat("0", 64), true, 403)
	call(path+"?fields=*", body, in.OperationID, origin, issued.CSRF, true, 400)
	call("/api/store/carts/foreign/context", body, in.OperationID, origin, issued.CSRF, true, 404)
	for _, bad := range []string{body + ` {}`, strings.Replace(body, `"version":"5"`, `"version":"5","version":"6"`, 1), strings.Replace(body, `"locale":"zh-CN"`, `"locale":null`, 1), strings.Replace(body, `"version":"5"`, `"currency_code":"usd","version":"5"`, 1)} {
		call(path, bad, in.OperationID, origin, issued.CSRF, true, 400)
	}
	for i := 0; i < 2; i++ {
		w := call(path, body, in.OperationID, origin, issued.CSRF, true, 200)
		var receipt cart.ContextResult
		var raw map[string]any
		if json.Unmarshal(w.Body.Bytes(), &receipt) != nil || json.Unmarshal(w.Body.Bytes(), &raw) != nil || len(raw) != 3 || receipt.CartID != ref.CartID || receipt.Version != "6" || receipt.Replayed != (i == 1) {
			t.Fatal("context HTTP receipt")
		}
	}
	call(path, body, uuid.NewString(), origin, issued.CSRF, true, 409)
	// The new mutation has the same bounded possession limiter as contacts.
	denied := false
	for i := 0; i < 33; i++ {
		allowed, err := sessions.AllowCartRequest(ctx, "context-possession", issued.Token)
		if err != nil {
			t.Fatal("context limiter", err)
		}
		if !allowed {
			denied = true
			break
		}
	}
	if !denied {
		t.Fatal("context limiter unbounded")
	}
	w := call(path, body, uuid.NewString(), origin, issued.CSRF, true, 429)
	if w.Header().Get("Retry-After") != "900" {
		t.Fatal("context limiter retry header")
	}

}
