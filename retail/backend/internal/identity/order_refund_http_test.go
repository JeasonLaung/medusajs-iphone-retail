package identity_test

import (
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/refunds"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type refundHTTPFixture struct {
	t       *testing.T
	router  *gin.Engine
	cookies []*http.Cookie
	csrf    string
}

func (f *refundHTTPFixture) call(method, path, body string, signed, csrf bool, status int) []byte {
	f.t.Helper()
	req := httptest.NewRequest(method, "https://refund-admin.example"+path, strings.NewReader(body))
	req.RemoteAddr = "192.0.2.16:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://refund-admin.example")
	if signed {
		for _, cookie := range f.cookies {
			req.AddCookie(cookie)
		}
	}
	if csrf {
		req.Header.Set("X-CSRF-Token", f.csrf)
	}
	out := httptest.NewRecorder()
	f.router.ServeHTTP(out, req)
	if out.Code != status {
		f.t.Fatalf("refund HTTP %s %s expected=%d actual=%d body=%s", method, path, status, out.Code, out.Body.String())
	}
	if strings.HasPrefix(path, "/api/admin/") && out.Header().Get("Cache-Control") != "private, no-store" {
		f.t.Fatal("refund private cache header")
	}
	for _, secret := range []string{"request_sha256", "remote_payment_id", "remote_refund_id", "request_envelope", "actor_id", "tr_initial"} {
		if strings.Contains(out.Body.String(), secret) {
			f.t.Fatal("refund HTTP private recovery material")
		}
	}
	return out.Body.Bytes()
}
func newRefundHTTPFixture(t *testing.T, ctx context.Context, s refunds.Service, actor identity.User) *refundHTTPFixture {
	t.Helper()
	sessions, client, e := sessionstore.Open(ctx, "redis://127.0.0.1:6379/0", "eurorefurb:refund_"+uuid.NewString()[:8]+":v1:")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { client.Close() })
	router, e := server.RouterWithAuthAndRefunds(s.DB, identity.Options{Sessions: sessions, Origin: "https://refund-admin.example", Secure: true}, payments.MollieWebhookOptions{}, refunds.Options{Enabled: true, StoreID: s.StoreID, Ring: s.Ring, Adapters: s.Adapters})
	if e != nil {
		t.Fatal(e)
	}
	fixture := &refundHTTPFixture{t: t, router: router}
	challenge := httptest.NewRequest("POST", "https://refund-admin.example/api/admin/auth/challenge", nil)
	challenge.Header.Set("Origin", "https://refund-admin.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, challenge)
	if response.Code != 200 {
		t.Fatal("refund HTTP login challenge")
	}
	var token struct {
		CSRF string `json:"csrf_token"`
	}
	if json.Unmarshal(response.Body.Bytes(), &token) != nil {
		t.Fatal("refund prelogin token")
	}
	loginBody, _ := json.Marshal(map[string]string{"email": actor.Email, "password": "artificial-refund-password"})
	login := httptest.NewRequest("POST", "https://refund-admin.example/api/admin/auth/login", strings.NewReader(string(loginBody)))
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("Origin", "https://refund-admin.example")
	login.Header.Set("X-CSRF-Token", token.CSRF)
	for _, c := range response.Result().Cookies() {
		login.AddCookie(c)
	}
	signed := httptest.NewRecorder()
	router.ServeHTTP(signed, login)
	if signed.Code != 200 {
		t.Fatal("refund HTTP actual admin login")
	}
	fixture.cookies = signed.Result().Cookies()
	if json.Unmarshal(signed.Body.Bytes(), &token) != nil {
		t.Fatal("refund login token")
	}
	fixture.csrf = token.CSRF
	return fixture
}
