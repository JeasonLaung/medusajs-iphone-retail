package identity_test

import (
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func checkPaymentOptions(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, o cart.InitialPaymentOptions) {
	t.Helper()
	check := func(n int) {
		t.Helper()
		v, err := cart.ReadPaymentOptions(ctx, db, "shop", ref, o)
		if err != nil || v.CartID != ref.CartID || v.Version != "4" || v.AttemptStarted || len(v.Providers) != n {
			t.Fatalf("payment options %d unavailable: %v", n, err)
		}
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("payment option fixture")
		}
	}
	check(1)
	exec("UPDATE payment_providers SET is_enabled=FALSE WHERE id=?", "pp_mollie-hosted-checkout_mollie")
	check(0)
	exec("UPDATE payment_providers SET is_enabled=TRUE WHERE id=?", "pp_mollie-hosted-checkout_mollie")
	exec("UPDATE commerce_region_payment_providers SET deleted_at=UTC_TIMESTAMP(6) WHERE id='checkout-region-provider'")
	check(0)
	exec("UPDATE commerce_region_payment_providers SET deleted_at=NULL WHERE id='checkout-region-provider'")
	wrong := ref
	wrong.Generation = strings.Repeat("0", 64)
	if _, err := cart.ReadPaymentOptions(ctx, db, "shop", wrong, o); err == nil {
		t.Fatal("wrong generation read options")
	}
	exec("UPDATE cart_checkout_runtime SET enabled=FALSE WHERE id=1")
	if _, err := cart.ReadPaymentOptions(ctx, db, "shop", ref, o); err != cart.ErrSchema {
		t.Fatal("closed checkout gate exposed options")
	}
	exec("UPDATE cart_checkout_runtime SET enabled=TRUE WHERE id=1")
	issued, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: o.ReturnOrigin, StoreID: "shop", InitialPayment: &o})
	if err != nil {
		t.Fatal(err)
	}
	call := func(suffix, origin string, cookie bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/store/carts/"+ref.CartID+"/payment-options"+suffix, nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if cookie {
			req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: issued.Token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want || w.Header().Get("Cache-Control") != "private, no-store" || len(w.Result().Cookies()) != 0 {
			t.Fatalf("private payment options got %d want %d", w.Code, want)
		}
		return w
	}
	w := call("", o.ReturnOrigin, true, 200)
	var out cart.PaymentOptions
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.CartID != ref.CartID || len(out.Providers) != 1 || out.Providers[0].ID != "pp_mollie-hosted-checkout_mollie" {
		t.Fatal("HTTP payment options differ")
	}
	for _, secret := range []string{"api_key", "checkout_url", "operation_id", "generation", "tr_initial", "email"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("private payment detail leaked")
		}
	}
	call("", "https://foreign.example", true, 403)
	call("", o.ReturnOrigin, false, 401)
	call("?provider=arbitrary", o.ReturnOrigin, true, 400)
}
