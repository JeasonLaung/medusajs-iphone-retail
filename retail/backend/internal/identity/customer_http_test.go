package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkCustomerHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ring *protected.Ring) {
	t.Helper()
	h, err := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: "https://shop.example", Secure: true})
	if err != nil {
		t.Fatal("customer HTTP configuration failed")
	}
	if _, err := customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: "https://shop.example", Secure: false}); err == nil {
		t.Fatal("insecure customer HTTPS accepted")
	}
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(platform.RequestBoundary(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	h.Register(r.Group("/api/store/customer"))
	jar := map[string]*http.Cookie{}
	calls := 0
	call := func(method, path, body, csrf string, expected int, adjust func(*http.Request)) *httptest.ResponseRecorder {
		t.Helper()
		calls++
		request := httptest.NewRequest(method, "https://shop.example/api/store/customer"+path, strings.NewReader(body))
		request.RemoteAddr = "192.0.2." + strconv.Itoa(calls) + ":1234"
		request.Header.Set("Origin", "https://shop.example")
		request.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range jar {
			request.AddCookie(cookie)
		}
		if adjust != nil {
			adjust(request)
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		if response.Code != expected {
			t.Fatalf("customer HTTP %s %s: got %d expected %d", method, path, response.Code, expected)
		}
		if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("private customer response cache/referrer boundary missing")
		}
		for _, cookie := range response.Result().Cookies() {
			if !strings.HasPrefix(cookie.Name, "__Host-retail_customer_") || cookie.Path != "/" || cookie.Domain != "" || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.HttpOnly != (cookie.Name != "__Host-retail_customer_csrf") {
				t.Fatal("customer cookie security or namespace differs")
			}
			if cookie.MaxAge < 0 {
				delete(jar, cookie.Name)
			} else {
				jar[cookie.Name] = cookie
			}
		}
		return response
	}
	challenge := func() string {
		t.Helper()
		response := call("POST", "/auth/challenge", "{}", "", 200, nil)
		var data struct {
			CSRF string `json:"csrf_token"`
		}
		if json.Unmarshal(response.Body.Bytes(), &data) != nil || len(data.CSRF) != 64 {
			t.Fatal("customer prelogin CSRF absent")
		}
		return data.CSRF
	}
	body := `{"email":"http-customer@example.invalid","password":"artificial-customer-password","first_name":"HTTP","last_name":"Customer","country":"dk","locale":"en"}`
	credentials := `{"email":"http-customer@example.invalid","password":"artificial-customer-password"}`
	call("POST", "/auth/challenge", "{}", "", 403, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	call("POST", "/auth/challenge", "{}", "", 403, func(r *http.Request) { r.Host = "evil.example" })
	call("POST", "/auth/challenge", "{}", "", 403, func(r *http.Request) { r.Header.Add("Origin", "https://shop.example") })
	call("POST", "/auth/challenge", "{}", "", 403, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") })
	call("POST", "/auth/challenge?token=do-not-log", "{}", "", 404, nil)
	call("POST", "/auth/register", body, "", 403, nil)
	csrf := challenge()
	prelogin := *jar["__Host-retail_customer_prelogin"]
	call("POST", "/auth/register", body, "", 403, nil)
	response := call("POST", "/auth/register", body, csrf, 202, nil)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"delivery":"pending"`)) || jar["__Host-retail_customer_session"] != nil {
		t.Fatal("registration claimed delivery or established a session")
	}
	call("POST", "/auth/register", body, csrf, 403, func(r *http.Request) { r.AddCookie(&prelogin) })
	csrf = challenge()
	call("POST", "/auth/login", credentials, csrf, 403, nil)
	var job struct {
		ID      string
		Payload []byte
	}
	if db.WithContext(ctx).Raw("SELECT m.id,m.payload FROM customer_verification_mail m JOIN customer_accounts a ON a.id=m.customer_id WHERE a.email=? AND m.status='pending'", "http-customer@example.invalid").Scan(&job).Error != nil || job.ID == "" {
		t.Fatal("HTTP registration mail intent missing")
	}
	plain, err := ring.Open(protected.Context{Purpose: "customer-verification", Table: "customer_verification_mail", Column: "payload", Row: job.ID}, job.Payload)
	if err != nil {
		t.Fatal("artificial HTTP verification token unavailable")
	}
	var message struct{ URL string }
	if json.Unmarshal(plain, &message) != nil {
		t.Fatal("artificial mail projection invalid")
	}
	parsed, err := url.Parse(message.URL)
	if err != nil {
		t.Fatal("artificial verification URL invalid")
	}
	token := parsed.Query().Get("token")
	csrf = challenge()
	call("POST", "/auth/verify", `{"token":"`+token+`"}`, csrf, 200, nil)
	if jar["__Host-retail_customer_session"] != nil {
		t.Fatal("verification implicitly signed in")
	}
	csrf = challenge()
	login := call("POST", "/auth/login", credentials, csrf, 200, nil)
	session := jar["__Host-retail_customer_session"]
	if session == nil || bytes.Contains(login.Body.Bytes(), []byte(session.Value)) || bytes.Contains(login.Body.Bytes(), []byte("password_hash")) {
		t.Fatal("login token missing or exposed")
	}
	old := *session
	call("GET", "/me", "", "", 200, nil)
	call("GET", "/me", "", "", 403, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	call("GET", "/me", "", "", 401, func(r *http.Request) { r.AddCookie(&old) })
	call("GET", "/me", "", "", 401, func(r *http.Request) { r.Header.Set("Cookie", "__Host-retail_admin_session="+old.Value) })
	csrf = challenge()
	call("POST", "/auth/login", credentials, csrf, 200, nil)
	if jar["__Host-retail_customer_session"].Value == old.Value {
		t.Fatal("login did not rotate customer session")
	}
	call("GET", "/me", "", "", 401, func(r *http.Request) { r.Header.Set("Cookie", old.Name+"="+old.Value) })
	call("POST", "/auth/logout", "{}", strings.Repeat("a", 64), 403, nil)
	call("GET", "/me", "", "", 200, nil)
	call("POST", "/auth/logout", "{}", jar["__Host-retail_customer_csrf"].Value, 200, nil)
	call("GET", "/me", "", "", 401, nil)
	for _, bad := range []string{`{"email":"x","email":"y"}`, `{"email":null}`, `{"admin":true}`, `{} {}`, `[]`} {
		call("POST", "/auth/login", bad, "", 400, nil)
	}
	call("POST", "/auth/login", "{\"email\":\""+string([]byte{255})+"\"}", "", 400, nil)
	call("POST", "/auth/login", "{}", "", 415, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") })
	call("POST", "/auth/login", `{"email":"`+strings.Repeat("x", 17<<10)+`"}`, "", 413, nil)
	call("POST", "/auth/login", strings.Repeat("x", int(platform.MaxJSONBodyBytes)+1), "", 413, nil)
	for range 8 {
		csrf = challenge()
		call("POST", "/auth/login", `{"email":"limited-customer@example.invalid","password":"wrong"}`, csrf, 401, nil)
	}
	csrf = challenge()
	call("POST", "/auth/login", `{"email":"limited-customer@example.invalid","password":"wrong"}`, csrf, 429, nil)
	for range 8 {
		call("POST", "/auth/challenge", "{}", "", 200, func(r *http.Request) { r.RemoteAddr = "198.51.100.7:1234" })
	}
	call("POST", "/auth/challenge", "{}", "", 429, func(r *http.Request) { r.RemoteAddr = "198.51.100.7:1234" })
	disabled := gin.New()
	disabled.Use(platform.RequestBoundary(slog.New(slog.NewJSONHandler(io.Discard, nil))))
	(&customer.HTTPHandler{}).Register(disabled.Group("/api/store/customer"))
	request := httptest.NewRequest("GET", "https://shop.example/api/store/customer/me", nil)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, request)
	if recorder.Code != 503 {
		t.Fatal("unconfigured customer HTTP opened")
	}
	t.Log(`CUSTOMER_HTTP_REHEARSAL {"real_gin_mysql_redis":true,"registration_to_verification_to_login":true,"origin_host_and_fetch_site_checked":true,"private_no_store":true,"customer_cookie_namespace":true,"secure_host_cookie_flags":true,"single_use_prelogin_csrf":true,"unverified_session_denied":true,"verification_does_not_login":true,"login_rotation":true,"duplicate_cookie_rejected":true,"administrator_cookie_rejected":true,"logout_csrf_and_revocation":true,"strict_json_and_size_limit":true,"email_rate_limit":true,"disabled_routes_fail_closed":true,"delivery":"pending","astro_bridge_integrated":false,"mail_delivery_integrated":false}`)
}
