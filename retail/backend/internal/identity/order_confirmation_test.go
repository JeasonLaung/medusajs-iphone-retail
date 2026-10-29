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
	"gorm.io/gorm"
)

func checkOrderConfirmation(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, orderID string, o cart.InitialPaymentOptions) {
	t.Helper()
	const origin = "http://127.0.0.1:18080"
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("order confirmation fixture")
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
	path := "/api/store/orders/" + orderID + "/confirmation"
	call := func(url, requestOrigin string, cookie bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", url, nil)
		if cookie {
			req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: issued.Token})
		}
		if requestOrigin != "" {
			req.Header.Set("Origin", requestOrigin)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("confirmation got %d want %d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || len(w.Result().Cookies()) != 0 {
			t.Fatal("confirmation privacy headers")
		}
		for _, field := range []string{"tr_initial", "payment_id", "payment_session", "capture_id", "request_sha256", "metadata", "generation", "raw_amount", "payload_envelope"} {
			if strings.Contains(w.Body.String(), field) {
				t.Fatal("confirmation protected field exposed")
			}
		}
		return w
	}
	call(path, "", false, 401)
	call(path, "https://foreign.example", true, 403)
	call(path+"?email=other@example.invalid", "", true, 400)
	result := call(path, "", true, 200)
	var order cart.Confirmation
	if json.Unmarshal(result.Body.Bytes(), &order) != nil || order.ID != orderID || order.Email == "" || order.CustomDisplayID == "" || order.DisplayID == "" || order.CreatedAt.IsZero() || order.Amounts.Totals["total"] != "158.75" || order.Payment.Amount != "158.75" || order.Payment.CurrencyCode != "eur" || len(order.Items) != 1 || order.Items[0].ProductTitle == nil || order.Shipping.PostalCode == "" || order.Shipping.FirstName == "" || order.Billing.FirstName == "" {
		t.Fatal("confirmation saved order projection differs")
	}
	// Public catalog retirement and mutations of the original cart presentation
	// must not rewrite the customer's saved order details.
	exec("UPDATE catalog_variants SET deleted_at=UTC_TIMESTAMP(6) WHERE id='add-v'")
	replay := call(path, "", true, 200)
	if replay.Body.String() != result.Body.String() {
		t.Fatal("catalog retirement changed confirmation")
	}
	exec("UPDATE catalog_variants SET deleted_at=NULL WHERE id='add-v'")
	var other string
	if db.Table("orders").Select("id").Where("id<>? AND email=? AND deleted_at IS NULL", orderID, order.Email).Order("id").Limit(1).Scan(&other).Error != nil || other == "" {
		t.Fatal("same email unrelated order fixture absent")
	}
	call("/api/store/orders/"+other+"/confirmation", "", true, 404)
	exec("UPDATE cart_ownership SET active=FALSE WHERE cart_id=?", ref.CartID)
	call(path, "", true, 404)
	exec("UPDATE cart_ownership SET active=TRUE WHERE cart_id=?", ref.CartID)
	exec("UPDATE cart_ownership SET generation=REPEAT('0',64) WHERE cart_id=?", ref.CartID)
	call(path, "", true, 404)
	exec("UPDATE cart_ownership SET generation=? WHERE cart_id=?", ref.Generation, ref.CartID)
	exec("UPDATE orders SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", orderID)
	call(path, "", true, 404)
	exec("UPDATE orders SET deleted_at=NULL WHERE id=?", orderID)
	exec("UPDATE cart_completion_runtime SET enabled=FALSE WHERE id=1")
	call(path, "", true, 503)
	exec("UPDATE cart_completion_runtime SET enabled=TRUE WHERE id=1")
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("confirmation revoke")
	}
	call(path, "", true, 401)
	t.Log(`ORDER_CONFIRMATION_REHEARSAL {"real_gin_mysql_redis":true,"current_cart_and_receipt_authority":true,"same_email_other_order_denied":true,"revoked_generation_and_session_denied":true,"saved_item_address_and_cash":true,"exact_total":"158.75","catalog_retirement_keeps_snapshot":true,"no_provider_payload_metadata_or_raw":true,"private_no_store_no_cookie_extension":true}`)
}
