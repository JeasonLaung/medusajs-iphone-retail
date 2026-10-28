package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type unavailableCartIssuer struct{ cart.HTTPSessions }

func (unavailableCartIssuer) IssueCart(context.Context, sessionstore.CartReference) (sessionstore.Issued, error) {
	return sessionstore.Issued{}, sessionstore.ErrUnavailable
}

// Real Gin dispatch and real owned MySQL/Redis; no production resources. The
// single issuance fault below is injected AFTER the real SQL commit to prove
// receipt recovery, not used to substitute for normal authentication/storage.
func checkCartHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, redisClient *redis.Client) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	options := cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop"}
	admin := identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}
	router, err := server.RouterWithAuth(db, admin, options)
	if err != nil {
		t.Fatal("owned cart HTTP configuration failed")
	}
	request := func(handler http.Handler, method, path, body string, cookies []*http.Cookie, csrf, operation string, edits ...func(*http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = "192.0.2.10:12345"
		req.Header.Set("Content-Type", "application/json")
		if method != "GET" {
			req.Header.Set("Origin", origin)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if operation != "" {
			req.Header.Set("Idempotency-Key", operation)
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		for _, edit := range edits {
			edit(req)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("private cart response cached or lacked request ID: method=%s route=%s status=%d", method, path, w.Code)
		}
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("cart HTTP status %d expected %d", w.Code, want)
		}
	}
	field := func(w *httptest.ResponseRecorder, key string) string {
		t.Helper()
		var result map[string]json.RawMessage
		var value string
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || json.Unmarshal(result[key], &value) != nil || value == "" {
			t.Fatal("cart response field missing")
		}
		return value
	}
	cookies := func(w *httptest.ResponseRecorder, prefix, kind string, secure bool) []*http.Cookie {
		t.Helper()
		values := w.Result().Cookies()
		if len(values) != 2 {
			t.Fatal("cart capability cookie pair missing")
		}
		for _, cookie := range values {
			if cookie.Path != "/" || cookie.Domain != "" || cookie.Secure != secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 || cookie.MaxAge > 43200 || cookie.Expires.IsZero() {
				t.Fatal("cart cookie attributes invalid")
			}
			if cookie.Name != prefix+kind && cookie.Name != prefix+kind+"_csrf" && !(kind == "possession" && cookie.Name == prefix+"csrf") {
				t.Fatal("cart cookie namespace incorrect")
			}
			if cookie.HttpOnly != (cookie.Name == prefix+kind) || strings.Contains(w.Body.String(), cookie.Value) && cookie.HttpOnly {
				t.Fatal("bearer cart cookie exposed to JavaScript/body")
			}
		}
		return values
	}
	var initial int64
	db.Table("carts").Count(&initial)
	w := request(router, "POST", "/api/store/cart-sessions", `{}`, nil, "", "")
	status(w, 200)
	scopeCookies := cookies(w, "retail_cart_", "scope", false)
	scopeCSRF := field(w, "csrf_token")
	var scopeToken string
	for _, cookie := range scopeCookies {
		if cookie.Name == "retail_cart_scope" {
			scopeToken = cookie.Value
		}
	}
	w = request(router, "POST", "/api/store/cart-sessions", `{}`, scopeCookies, scopeCSRF, "")
	status(w, 200)
	if w.Header().Get("Set-Cookie") != "" || field(w, "csrf_token") != scopeCSRF {
		t.Fatal("scope reuse refreshed lifetime or changed CSRF")
	}
	operation := uuid.NewString()
	body := `{"region_id":"region","locale":"en"}`
	w = request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, operation)
	status(w, 201)
	possession := cookies(w, "retail_cart_", "possession", false)
	var created struct {
		Cart         cart.Snapshot
		Replayed     bool
		Capabilities struct{ Items, Checkout bool }
	}
	if json.Unmarshal(w.Body.Bytes(), &created) != nil || created.Cart.ID == "" || created.Cart.Locale != "en" || created.Cart.Version != "1" || created.Replayed || created.Capabilities.Items || created.Capabilities.Checkout {
		t.Fatal("created HTTP cart projection incorrect")
	}
	path := "/api/store/carts/" + created.Cart.ID
	w = request(router, "GET", path, "", possession, "", "")
	status(w, 200)
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("cart read refreshed or issued credentials")
	}
	for _, private := range []string{"generation", "metadata", "email", "customer_id", "store_id", "password", "payload", scopeToken} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("private cart data escaped projection")
		}
	}
	current := request(router, "GET", "/api/store/cart", "", possession, "", "")
	status(current, 200)
	if current.Body.String() != w.Body.String() || current.Header().Get("Set-Cookie") != "" {
		t.Fatal("current cart did not resolve the same private snapshot without renewal")
	}
	status(request(router, "GET", "/api/store/cart", "", nil, "", ""), 401)
	status(request(router, "GET", "/api/store/cart", "", scopeCookies, "", ""), 401)
	status(request(router, "GET", "/api/store/cart", "", append(possession, possession[0]), "", ""), 401)
	status(request(router, "GET", "/api/store/cart?id=legacy-cart", "", possession, "", ""), 400)
	status(request(router, "GET", "/api/store/cart", "", possession, "", "", func(r *http.Request) { r.Header.Set("Origin", "https://unrelated.example") }), 403)
	status(request(router, "GET", "/api/store/cart", "", possession, "", "", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }), 403)
	w = request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, operation)
	status(w, 200)
	var replay struct {
		Cart     cart.Snapshot
		Replayed bool
	}
	if json.Unmarshal(w.Body.Bytes(), &replay) != nil || replay.Cart.ID != created.Cart.ID || !replay.Replayed {
		t.Fatal("HTTP retry created another cart")
	}
	var after int64
	if db.Table("carts").Count(&after).Error != nil || after != initial+1 {
		t.Fatal("HTTP retry duplicated durable cart")
	}
	status(request(router, "POST", "/api/store/carts", `{"region_id":"region","locale":"zh-CN"}`, scopeCookies, scopeCSRF, operation), 409)
	status(request(router, "GET", path, "", nil, "", ""), 401)
	status(request(router, "GET", path, "", scopeCookies, "", ""), 401)
	status(request(router, "GET", "/api/store/carts/legacy-cart", "", possession, "", ""), 404)
	status(request(router, "GET", path+"?fields=metadata", "", possession, "", ""), 400)
	status(request(router, "GET", path, "", append(possession, possession[0]), "", ""), 401)
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, "", operation), 403)
	status(request(router, "POST", "/api/store/carts", body, append(scopeCookies, scopeCookies[0]), scopeCSRF, operation), 401)
	status(request(router, "POST", "/api/store/carts", body, possession, scopeCSRF, operation), 401)
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, ""), 400)
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, strings.ToUpper(operation)), 400)
	for _, edit := range []func(*http.Request){
		func(r *http.Request) { r.Header.Del("Origin") },
		func(r *http.Request) { r.Header.Set("Origin", "https://unrelated.example") },
		func(r *http.Request) { r.Header.Add("Origin", origin) },
		func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
	} {
		status(request(router, "POST", "/api/store/cart-sessions", `{}`, nil, "", "", edit), 403)
	}
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, operation, func(r *http.Request) { r.Header.Add("X-CSRF-Token", scopeCSRF) }), 403)
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, operation, func(r *http.Request) { r.Header.Add("Idempotency-Key", operation) }), 400)
	status(request(router, "GET", path, "", possession, "", "", func(r *http.Request) { r.Header.Set("Origin", "https://unrelated.example") }), 403)
	for _, bad := range []string{
		`{"region_id":"region","region_id":"other"}`, `{"region_id":null}`,
		`{"region_id":"region","locale":null}`, `{"region_id":"region","unit_price":"0"}`,
		`{"region_id":"region","customer_id":"old"}`, `{"region_id":"region","store_id":"other-shop"}`,
		`{"region_id":"region","scope_hash":"chosen"}`, `{"region_id":"region"} {}`,
	} {
		status(request(router, "POST", "/api/store/carts", bad, scopeCookies, scopeCSRF, operation), 400)
	}
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, operation, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }), 415)
	status(request(router, "POST", "/api/store/carts", `{"region_id":"`+strings.Repeat("a", (1<<20)+1)+`"}`, scopeCookies, scopeCSRF, operation), 413)
	status(request(router, "POST", "/api/store/carts", `{"region_id":"`+strings.Repeat("a", (1<<20)+1)+`"}`, scopeCookies, scopeCSRF, operation, func(r *http.Request) { r.ContentLength = -1 }), 413)
	status(request(router, "POST", "/api/store/carts", `{"region_id":"missing"}`, scopeCookies, scopeCSRF, uuid.NewString()), 422)

	// Issuance fails after the SQL commit; same scope/key must recover that
	// already committed cart, rather than creating a second one.
	fault := options
	fault.Sessions = unavailableCartIssuer{sessions}
	faultRouter, err := server.RouterWithAuth(db, admin, fault)
	if err != nil {
		t.Fatal("issuance recovery fixture unavailable")
	}
	recoverOperation := uuid.NewString()
	w = request(faultRouter, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, recoverOperation)
	status(w, 503)
	if w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "cart_") {
		t.Fatal("uncertain issuance leaked cart reference or cookie")
	}
	w = request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, recoverOperation)
	status(w, 200)
	recoveredCookies := w.Result().Cookies()
	if json.Unmarshal(w.Body.Bytes(), &replay) != nil || !replay.Replayed || replay.Cart.ID == created.Cart.ID {
		t.Fatal("issuance recovery did not reuse committed receipt")
	}
	if db.Table("carts").Count(&after).Error != nil || after != initial+2 {
		t.Fatal("uncertain issuance duplicated durable cart")
	}
	// The real Redis binding remains present while MySQL authorization changes.
	if db.Exec(`UPDATE cart_ownership SET active=FALSE WHERE cart_id=?`, created.Cart.ID).Error != nil {
		t.Fatal("HTTP revocation fixture failed")
	}
	status(request(router, "GET", path, "", possession, "", ""), 404)
	status(request(router, "GET", "/api/store/cart", "", possession, "", ""), 404)
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, operation), 404)

	closedClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	closedSessions, _ := sessionstore.New(closedClient, "eurorefurb:cart-runtime:v1:")
	_ = closedClient.Close()
	broken := options
	broken.Sessions = closedSessions
	brokenRouter, err := server.RouterWithAuth(db, admin, broken)
	if err != nil {
		t.Fatal("closed dependency fixture unavailable")
	}
	w = request(brokenRouter, "POST", "/api/store/cart-sessions", `{}`, scopeCookies, scopeCSRF, "")
	status(w, 503)
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("Redis outage replaced scope")
	}
	status(request(brokenRouter, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, uuid.NewString()), 503)
	status(request(brokenRouter, "GET", "/api/store/carts/"+replay.Cart.ID, "", recoveredCookies, "", ""), 503)
	status(request(brokenRouter, "GET", "/api/store/cart", "", recoveredCookies, "", ""), 503)
	// Scope eviction loses its retry identity; it never grants old cart access.
	hash := sha256.Sum256([]byte(scopeToken))
	scopeKey := "eurorefurb:cart-runtime:v1:cart:creation-scope:" + hex.EncodeToString(hash[:])
	if redisClient.Del(ctx, scopeKey).Err() != nil {
		t.Fatal("owned scope eviction failed")
	}
	status(request(router, "POST", "/api/store/carts", body, scopeCookies, scopeCSRF, operation), 401)
	w = request(router, "POST", "/api/store/cart-sessions", `{}`, scopeCookies, scopeCSRF, "")
	status(w, 200)
	if field(w, "csrf_token") == scopeCSRF {
		t.Fatal("expired scope credential restored")
	}
	// HTTPS configuration uses host-only Secure cookie names. HTTP remote
	// origins and missing shop config are rejected by the factory.
	secureOptions := options
	secureOptions.Origin, secureOptions.Secure = "https://shop.example", true
	secureRouter, err := server.RouterWithAuth(db, admin, secureOptions)
	if err != nil {
		t.Fatal("secure cart origin configuration failed")
	}
	w = request(secureRouter, "POST", "/api/store/cart-sessions", `{}`, nil, "", "", func(r *http.Request) { r.Header.Set("Origin", "https://shop.example") })
	status(w, 200)
	cookies(w, "__Host-retail_cart_", "scope", true)
	for _, badOrigin := range []string{"http://shop.example", "https://shop.example/", "https://user:pass@shop.example", "https://shop.example?x=1"} {
		bad := options
		bad.Origin = badOrigin
		if _, err := cart.NewHTTP(db, bad); err == nil {
			t.Fatal("invalid cart origin configuration accepted")
		}
	}
	disabled, err := server.RouterWithAuth(db, admin)
	if err != nil {
		t.Fatal("existing admin router factory broken")
	}
	status(request(disabled, "POST", "/api/store/cart-sessions", `{}`, nil, "", ""), 503)
	status(request(disabled, "GET", path, "", possession, "", ""), 503)
	status(request(disabled, "GET", "/api/store/cart", "", possession, "", ""), 503)
	t.Log(`CART_HTTP_REHEARSAL {"real_gin_mysql_redis":true,"scope_cookie_csrf":true,"possession_http_only":true,"strict_origin_and_headers":true,"strict_json_and_size":true,"private_projection":true,"id_alone_denied":true,"scoped_creation_replay":true,"issuance_failure_recovered":true,"revoked_replay_denied":true,"redis_failure_closed":true,"scope_eviction_closed":true,"secure_cookie_configuration":true,"unconfigured_routes_closed":true,"current_possession_lookup":true,"browser_verified":false,"checkout_integrated":false}`)
}
