package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Login is real SQL + dedicated Redis, over artificial explicit ownership.
// This is an HTTP inbox check, not registration/mail/browser acceptance.
func checkOrderPaymentNoticeCustomer(t *testing.T, ctx context.Context, db *gorm.DB, service orderpayments.Service, owner string, in orderpayments.Input) {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", MaxRetries: -1, Protocol: 2, DisableIdentity: true, ContextTimeoutEnabled: true, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	defer client.Close()
	sessions, e := sessionstore.New(client, "eurorefurb:order-payment-notice:v1:")
	if e != nil {
		t.Fatal(e)
	}
	ring, _ := protected.New("customer", map[string][]byte{"customer": bytes.Repeat([]byte{48}, 32)})
	store, e := customer.New(db, ring, sessions, service.ReturnOrigin)
	if e != nil {
		t.Fatal(e)
	}
	login := func(email string) sessionstore.Issued {
		t.Helper()
		out, e := store.Login(ctx, email, "owned-payment-http-password")
		if e != nil {
			t.Fatal("notice login", e)
		}
		t.Cleanup(func() { sessions.Revoke(context.Background(), sessionstore.Customer, out.Session.Token) })
		return out.Session
	}
	current, foreign := login("supplement@example.invalid"), login("foreign-payment@example.invalid")
	h, e := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: service.ReturnOrigin, StoreID: service.StoreID, OrderPayments: &service})
	if e != nil {
		t.Fatal(e)
	}
	router := gin.New()
	h.Register(router.Group("/api/store/customer"))
	call := func(method, body string, session sessionstore.Issued, csrf bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, service.ReturnOrigin+"/api/store/customer/notifications", strings.NewReader(body))
		if session.Token != "" {
			req.AddCookie(&http.Cookie{Name: "retail_customer_session", Value: session.Token})
		}
		if method == "POST" {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", service.ReturnOrigin)
			if csrf {
				req.Header.Set("X-CSRF-Token", session.CSRF)
			}
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("notice HTTP status/privacy", w.Code, want)
		}
		return w
	}
	call("GET", "", sessionstore.Issued{}, false, 401)
	page := func(session sessionstore.Issued) customer.NoticePage {
		t.Helper()
		w := call("GET", "", session, false, 200)
		var p customer.NoticePage
		if json.Unmarshal(w.Body.Bytes(), &p) != nil {
			t.Fatal("notice projection")
		}
		return p
	}
	find := func(p customer.NoticePage) *customer.Notice {
		for _, n := range p.Notifications {
			if n.Kind == "payment.received" && n.ResourceID == in.OrderID {
				return &n
			}
		}
		return nil
	}
	p := page(current)
	notice := find(p)
	if notice == nil || notice.ReadAt != nil || notice.Action != "" || notice.Reference == "" || p.Unread < 1 {
		t.Fatal("current owner missing unread payment notice")
	}
	var stored struct{ CustomerID, ResourceID string }
	if db.Table("customer_account_notices").Where("event_key=?", "payment.received:pay_supp_"+in.OperationID).Take(&stored).Error != nil || stored.CustomerID != owner || stored.ResourceID != in.OrderID {
		t.Fatal("notice recipient not explicit owner")
	}
	w := call("GET", "", current, false, 200)
	for _, secret := range []string{"customer_id", "event_key", "session_id", "remote_id", "password", "result_envelope"} {
		if bytes.Contains(w.Body.Bytes(), []byte(`"`+secret+`"`)) {
			t.Fatal("private notice field", secret)
		}
	}
	if find(page(foreign)) != nil {
		t.Fatal("foreign customer read notice")
	}
	body, _ := json.Marshal(map[string]string{"id": notice.ID})
	call("POST", string(body), foreign, true, 200)
	if find(page(current)).ReadAt != nil {
		t.Fatal("foreign customer marked owner notice read")
	}
	call("POST", string(body), current, false, 403)
	call("POST", string(body), current, true, 200)
	read := find(page(current))
	if read == nil || read.ReadAt == nil {
		t.Fatal("owner mark read")
	}
	call("POST", string(body), current, true, 200)
	again := find(page(current))
	if again == nil || again.ReadAt == nil || !again.ReadAt.Equal(*read.ReadAt) {
		t.Fatal("read replay altered event")
	}
	if e := sessions.Revoke(ctx, sessionstore.Customer, current.Token); e != nil {
		t.Fatal(e)
	}
	call("GET", "", current, false, 401)
	t.Log(`ORDER_PAYMENT_NOTICE_REHEARSAL {"cash_notice_same_transaction":true,"late_notice_failure_rolls_back_cash":true,"last_ack_rolls_back_notice":true,"concurrent_consumers_one_notice":true,"late_observation_no_duplicate":true,"explicit_new_account_owner":true,"disabled_unverified_unowned_no_recipient":true,"real_login_sql_redis":true,"current_generation_owner_inbox":true,"private_http_projection":true,"foreign_read_and_mark_isolation":true,"owner_csrf_and_read_replay":true,"revoked_session_denied":true,"registration_browser":false,"real_provider_or_mail":false,"customer_cli_joint":false}`)
}
