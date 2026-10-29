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

func checkDeliveryHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	issued, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal("delivery HTTP session failed")
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: []string{"delivery-provider"}})
	if err != nil {
		t.Fatal("delivery HTTP router failed")
	}
	base := "/api/store/carts/" + ref.CartID
	call := func(method, path, body, key, requestOrigin, csrf string, cookie bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		before := fingerprint()
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
		if (method == "GET" || status != 200) && before != fingerprint() {
			t.Fatal("delivery rejected/read HTTP mutated")
		}
		return w
	}
	path := base + "/shipping-methods"
	body := `{"version":"7","option_id":"delivery-standard"}`
	key := uuid.NewString()
	call("GET", base+"/delivery-options", "", "", "", "", false, 401)
	call("GET", base+"/delivery-options?fields=*", "", "", "", "", true, 400)
	call("GET", "/api/store/carts/foreign-cart/delivery-options", "", "", "", "", true, 404)
	read := call("GET", base+"/delivery-options", "", "", "", "", true, 200)
	var options cart.DeliveryOptions
	if json.Unmarshal(read.Body.Bytes(), &options) != nil || len(options.Options) != 2 || !options.Complete {
		t.Fatal("delivery options HTTP differs")
	}
	call("POST", path, body, key, origin, issued.CSRF, false, 401)
	call("POST", path, body, key, "https://unrelated.example", issued.CSRF, true, 403)
	call("POST", path, body, key, origin, "", true, 403)
	for _, bad := range []string{`{"version":"7","option_id":"delivery-standard","amount":"0"}`, `{"version":"7","option_id":"delivery-standard","provider_id":"other"}`, `{"version":"7","option_id":null}`, `{"version":7,"option_id":"delivery-standard"}`, `{"version":"7","option_id":"delivery-standard","option_id":"delivery-pickup"}`, body + ` {}`} {
		call("POST", path, bad, key, origin, issued.CSRF, true, 400)
	}
	call("POST", path, strings.Repeat(" ", int(platform.MaxJSONBodyBytes))+body, key, origin, issued.CSRF, true, 413)
	first := call("POST", path, body, key, origin, issued.CSRF, true, 200)
	replay := call("POST", path, body, key, origin, issued.CSRF, true, 200)
	var a, b cart.ShippingResult
	if json.Unmarshal(first.Body.Bytes(), &a) != nil || json.Unmarshal(replay.Body.Bytes(), &b) != nil || a.Version != "8" || a.Replayed || !b.Replayed || a.OptionID != "delivery-standard" {
		t.Fatal("delivery receipt HTTP differs")
	}
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("delivery session cleanup failed")
	}
	call("POST", path, body, key, origin, issued.CSRF, true, 401)
	t.Log(`CART_DELIVERY_HTTP_REHEARSAL {"real_gin_mysql_redis":true,"possession_origin_csrf":true,"strict_input_price_provider_injection_denied":true,"private_no_store_413":true,"read_only_options":true,"private_provider_data_absent":true,"durable_replay":true,"revoked_cookie_denied":true}`)
}
