package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func checkCustomerOrderOwnership(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference, orderID string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("customer order fixture")
		}
	}
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{83}, 32)})
	store, e := customer.New(db, ring, sessions, "http://127.0.0.1:18080")
	if e != nil {
		t.Fatal(e)
	}
	makeAccount := func(email string) customer.Login {
		t.Helper()
		a, e := store.Register(ctx, customer.Registration{Email: email, Password: "fixture-customer-order-password", FirstName: "New", LastName: "Buyer", Country: "dk", Locale: "en"})
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE customer_accounts SET verified_at=UTC_TIMESTAMP(6) WHERE id=?", a.ID)
		result, e := store.Login(ctx, email, "fixture-customer-order-password")
		if e != nil {
			t.Fatal(e)
		}
		return result
	}
	var email string
	if db.Table("orders").Select("email").Where("id=?", orderID).Scan(&email).Error != nil || email == "" {
		t.Fatal("email fixture")
	}
	// Email deliberately matches the paid order, but grants no list access.
	a := makeAccount(email)
	b := makeAccount("other-order-owner@example.invalid")
	list, e := store.Orders(ctx, a.Session.Token, 0)
	if e != nil || list.Count != 0 || len(list.Orders) != 0 {
		t.Fatal("email granted historical ownership", e)
	}
	possession, e := sessions.IssueCart(ctx, ref)
	if e != nil {
		t.Fatal(e)
	}
	h, e := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: "http://127.0.0.1:18080"})
	if e != nil {
		t.Fatal(e)
	}
	router := gin.New()
	h.Register(router.Group("/api/store/customer"))
	path := "/api/store/customer/orders/" + orderID + "/attach"
	call := func(method, path, body, origin, csrf string, account bool, cartCookie bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1:18080"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if account {
			req.AddCookie(&http.Cookie{Name: "retail_customer_session", Value: a.Session.Token})
		}
		if cartCookie {
			req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: possession.Token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("customer order HTTP %d want %d: %s", w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("private response")
		}
		return w
	}
	const origin = "http://127.0.0.1:18080"
	call("POST", path, "{}", origin, a.Session.CSRF, false, true, 401)
	call("POST", path, "{}", origin, a.Session.CSRF, true, false, 401)
	call("POST", path, "{}", origin, "", true, true, 403)
	call("POST", path, "{}", "https://foreign.example", a.Session.CSRF, true, true, 403)
	call("POST", path, `{"customer_id":"old"}`, origin, a.Session.CSRF, true, true, 400)
	call("GET", "/api/store/customer/orders?email="+email, "", "", "", true, false, 400)
	call("GET", "/api/store/customer/orders?offset=0;bad", "", "", "", true, false, 400)
	call("GET", "/api/store/customer/orders?offset=01", "", "", "", true, false, 400)
	call("GET", "/api/store/customer/orders?offset=0&offset=0", "", "", "", true, false, 400)
	exec("UPDATE cart_ownership SET active=FALSE WHERE cart_id=?", ref.CartID)
	if e := store.AttachOrder(ctx, a.Session.Token, possession.Token, orderID); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("inactive cart accepted", e)
	}
	exec("UPDATE cart_ownership SET active=TRUE WHERE cart_id=?", ref.CartID)
	exec("CREATE TRIGGER customer_order_late_failure BEFORE INSERT ON audit_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial attach failure'")
	if e := store.AttachOrder(ctx, a.Session.Token, possession.Token, orderID); !errors.Is(e, customer.ErrUnavailable) {
		t.Fatal("late attach failure accepted", e)
	}
	exec("DROP TRIGGER customer_order_late_failure")
	var count int64
	if db.Table("customer_order_ownership").Where("order_id=?", orderID).Count(&count).Error != nil || count != 0 {
		t.Fatal("partial ownership commit")
	}
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- store.AttachOrder(ctx, a.Session.Token, possession.Token, orderID) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent attachment", e)
		}
	}
	call("POST", path, "{}", origin, a.Session.CSRF, true, true, 200)
	if e := store.AttachOrder(ctx, b.Session.Token, possession.Token, orderID); !errors.Is(e, customer.ErrOrderMissing) {
		t.Fatal("ownership stolen", e)
	}
	response := call("GET", "/api/store/customer/orders?offset=0", "", "", "", true, false, 200)
	if json.Unmarshal(response.Body.Bytes(), &list) != nil || list.Count != 1 || len(list.Orders) != 1 || list.Orders[0].ID != orderID || list.Orders[0].Total != "158.75" || len(list.Orders[0].Items) != 1 {
		t.Fatal("owned order projection", response.Body.String())
	}
	for _, field := range []string{"email", "customer_id", "payment_id", "capture_id", "generation", "address", "raw_amount", "metadata"} {
		if strings.Contains(response.Body.String(), field) {
			t.Fatal("private list fields", field)
		}
	}
	other, e := store.Orders(ctx, b.Session.Token, 0)
	if e != nil || other.Count != 0 {
		t.Fatal("cross customer list")
	}
	if db.Table("audit_events").Where("action='customer.order.attach' AND resource_id=?", orderID).Count(&count).Error != nil || count != 1 {
		t.Fatal("attachment receipt duplicated")
	}
	exec("UPDATE orders SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", orderID)
	list, e = store.Orders(ctx, a.Session.Token, 0)
	if e != nil || list.Count != 0 {
		t.Fatal("deleted order exposed")
	}
	exec("UPDATE orders SET deleted_at=NULL WHERE id=?", orderID)
	if sessions.RevokeCart(ctx, possession.Token) != nil {
		t.Fatal("cart revoke")
	}
	list, e = store.Orders(ctx, a.Session.Token, 0)
	if e != nil || list.Count != 1 {
		t.Fatal("long term ownership depended on cart token")
	}
	if e := store.AttachOrder(ctx, a.Session.Token, possession.Token, orderID); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("revoked cart accepted", e)
	}
	exec("UPDATE customer_accounts SET auth_version=auth_version+1 WHERE id=?", a.Customer.ID)
	if _, e := store.Orders(ctx, a.Session.Token, 0); !errors.Is(e, customer.ErrUnauthorized) {
		t.Fatal("revoked customer read", e)
	}
	t.Log(`CUSTOMER_ORDER_OWNERSHIP_REHEARSAL {"real_gin_mysql_redis":true,"two_capabilities_required":true,"email_never_grants_orders":true,"concurrent_attachment_single_audit":true,"late_audit_failure_atomic":true,"other_customer_cannot_steal":true,"long_term_after_cart_revocation":true,"customer_revocation_denied":true,"exact_total":"158.75","private_list_projection":true,"browser_integrated":false}`)
}
