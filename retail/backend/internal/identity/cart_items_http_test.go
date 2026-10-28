package identity_test

import (
	"context"
	"crypto/sha256"
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

func checkCartItemsHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("artificial HTTP items")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("owned HTTP item cart failed")
	}
	issued, err := sessions.IssueCart(ctx, created.Reference)
	if err != nil {
		t.Fatal("owned HTTP item session failed")
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop"})
	if err != nil {
		t.Fatal("owned HTTP item router failed")
	}
	path := "/api/store/carts/" + created.Reference.CartID + "/line-items"
	key := uuid.NewString()
	body := `{"version":"1","variant_id":"add-v","quantity":"1"}`
	call := func(target, payload, operation, requestOrigin, csrf string, cookie bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("POST", target, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", operation)
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
			t.Fatalf("item HTTP status %d want %d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Request-ID") == "" || len(w.Result().Cookies()) != 0 {
			t.Fatal("item response cache/cookie contract differs")
		}
		for _, private := range []string{"generation", "price_set_id", "request_sha256", "not-a-snapshot", "metadata", "raw_unit_price"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("private item field escaped")
			}
		}
		return w
	}
	call(path, body, key, origin, issued.CSRF, false, 401)
	call(path, body, key, "https://unrelated.example", issued.CSRF, true, 403)
	call(path, body, key, "", issued.CSRF, true, 403)
	call(path, body, key, origin, "", true, 403)
	call(path, body, key, origin, strings.Repeat("0", 64), true, 403)
	for _, bad := range []string{`{"version":"1","variant_id":"add-v","quantity":"1","unit_price":"0"}`, `{"version":"1","variant_id":"add-v","quantity":1}`, `{"version":"1","variant_id":"add-v","quantity":null}`, `{"version":"1","variant_id":"add-v","quantity":"1","quantity":"2"}`, body + ` {}`} {
		call(path, bad, key, origin, issued.CSRF, true, 400)
	}
	call(path, body, "not-a-key", origin, issued.CSRF, true, 400)
	call(path+"?fields=*", body, key, origin, issued.CSRF, true, 400)
	call("/api/store/carts/not-my-cart/line-items", body, key, origin, issued.CSRF, true, 404)
	call(path, strings.Repeat(" ", int(platform.MaxJSONBodyBytes))+body, key, origin, issued.CSRF, true, 413)
	first := call(path, body, key, origin, issued.CSRF, true, 200)
	replay := call(path, body, key, origin, issued.CSRF, true, 200)
	var a, b cart.AddItemResult
	if json.Unmarshal(first.Body.Bytes(), &a) != nil || json.Unmarshal(replay.Body.Bytes(), &b) != nil || a.Replayed || !b.Replayed || a.LineID != b.LineID || a.Quantity != "1" || a.Version != "2" {
		t.Fatal("HTTP add/replay result differs")
	}
	call(path, body, uuid.NewString(), origin, issued.CSRF, true, 409)
	call(path, `{"version":"2","variant_id":"add-v","quantity":"5"}`, uuid.NewString(), origin, issued.CSRF, true, 422)
	call(path, `{"version":"2","variant_id":"absent","quantity":"1"}`, uuid.NewString(), origin, issued.CSRF, true, 422)
	linePath := path + "/" + a.LineID
	setBody := `{"version":"2","quantity":"2"}`
	call(linePath, setBody, uuid.NewString(), origin, issued.CSRF, false, 401)
	call(linePath, setBody, uuid.NewString(), origin, "", true, 403)
	for _, bad := range []string{`{"version":"2","quantity":2}`, `{"version":"2","quantity":"2","variant_id":"add-v"}`, `{"version":"2","quantity":"2","quantity":"3"}`} {
		call(linePath, bad, uuid.NewString(), origin, issued.CSRF, true, 400)
	}
	setKey := uuid.NewString()
	setReply := call(linePath, setBody, setKey, origin, issued.CSRF, true, 200)
	var updated cart.AddItemResult
	if json.Unmarshal(setReply.Body.Bytes(), &updated) != nil || updated.Quantity != "2" || updated.Version != "3" || updated.LineID != a.LineID {
		t.Fatal("HTTP absolute quantity failed")
	}
	call(linePath, setBody, setKey, origin, issued.CSRF, true, 200)
	call(linePath, `{"version":"3","quantity":"3"}`, setKey, origin, issued.CSRF, true, 409)
	call(path+"/foreign-line", `{"version":"3","quantity":"1"}`, uuid.NewString(), origin, issued.CSRF, true, 404)
	call(linePath, `{"version":"3","quantity":"5"}`, uuid.NewString(), origin, issued.CSRF, true, 422)
	removeKey := uuid.NewString()
	removed := call(linePath, `{"version":"3","quantity":"0"}`, removeKey, origin, issued.CSRF, true, 200)
	var removal cart.AddItemResult
	if json.Unmarshal(removed.Body.Bytes(), &removal) != nil || removal.Quantity != "0" || removal.Version != "4" {
		t.Fatal("HTTP removal failed")
	}
	call(linePath, `{"version":"3","quantity":"0"}`, removeKey, origin, issued.CSRF, true, 200)
	call(linePath, `{"version":"4","quantity":"1"}`, uuid.NewString(), origin, issued.CSRF, true, 404)
	empty, err := cart.ReadAmounts(ctx, db, "shop", created.Reference)
	if err != nil || len(empty.Lines) != 0 || empty.Totals["total"] != "0" {
		t.Fatal("removed line still contributed amounts")
	}
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("item HTTP session cleanup failed")
	}
	call(path, body, key, origin, issued.CSRF, true, 401)
	t.Log(`CART_ITEMS_HTTP_REHEARSAL {"real_gin_mysql_redis":true,"possession_csrf_origin":true,"strict_string_input":true,"private_no_store":true,"no_cookie_refresh":true,"size_limit":true,"price_injection_rejected":true,"absolute_quantity_and_removal":true,"durable_replay":true,"version_and_stock_errors":true,"revoked_cookie_rejected":true,"production_page_mounted":false}`)
}
