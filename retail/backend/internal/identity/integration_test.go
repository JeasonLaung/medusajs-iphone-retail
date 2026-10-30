package identity_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedAdminAuthentication(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned MySQL and Redis required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "identity_rehearsal"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("owned SQL connection unavailable")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal("owned SQL pool unavailable")
	}
	defer pool.Close()
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("embedded schema invalid")
	}
	result, err := schema.Apply(ctx, pool, plan)
	if err != nil || result.Applied != 67 {
		t.Fatal("ten explicit migrations not applied")
	}
	result, err = schema.Apply(ctx, pool, plan)
	if err != nil || result.Skipped != 67 {
		t.Fatal("ten migrations not repeatable")
	}
	sessions, client, err := sessionstore.Open(ctx, "redis://127.0.0.1:6379/0", "eurorefurb:identity_fixture:v1:")
	if err != nil {
		t.Fatal("store unavailable")
	}
	defer client.Close()
	gin.SetMode(gin.TestMode)
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer slog.SetDefault(oldLogger)
	t.Setenv("STOREFRONT_STORE_ID", "device-source-store")
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "https://admin.example", Secure: true})
	if err != nil {
		t.Fatal("configured router unavailable")
	}
	password := "artificial-password-only"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal("fixture KDF failed")
	}
	u := identity.User{ID: "new-fixture-admin", Email: "new@example.invalid", Name: "Fixture", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: string(hash), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Create(&u).Error != nil {
		t.Fatal("new fixture account creation failed")
	}
	if db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, u.ID) }) != nil {
		t.Fatal("explicit fixture grants failed")
	}
	if db.Exec("UPDATE admin_users SET auth_version=0 WHERE id=?", u.ID).Error == nil {
		t.Fatal("zero revocation generation accepted")
	}
	requests := 0
	call := func(method, path, body, origin, csrf string, cookies []*http.Cookie, status int) *httptest.ResponseRecorder {
		requests++
		req := httptest.NewRequest(method, "https://admin.example"+path, strings.NewReader(body))
		req.RemoteAddr = "192.0.2." + strconv.Itoa(requests%200+1) + ":1234"
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if out.Code != status {
			t.Fatalf("unexpected artificial auth status: %s expected %d received %d", path, status, out.Code)
		}
		if strings.HasPrefix(path, "/api/admin/") && out.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("auth response cache policy missing")
		}
		if strings.Contains(out.Body.String(), "password_hash") || strings.Contains(out.Body.String(), string(hash)) || strings.Contains(out.Body.String(), password) || strings.Contains(out.Body.String(), "auth_version") {
			t.Fatal("credential/internal generation disclosed")
		}
		return out
	}
	const origin = "https://admin.example"
	findCookie := func(out *httptest.ResponseRecorder, name string) *http.Cookie {
		for _, c := range out.Result().Cookies() {
			if c.Name == name {
				return c
			}
		}
		t.Fatal("required cookie missing")
		return nil
	}
	parseCSRF := func(out *httptest.ResponseRecorder) string {
		var body struct {
			CSRF string `json:"csrf_token"`
		}
		if json.Unmarshal(out.Body.Bytes(), &body) != nil || len(body.CSRF) != 64 {
			t.Fatal("CSRF response missing")
		}
		return body.CSRF
	}
	challenge := func() (*http.Cookie, string) {
		out := call("POST", "/api/admin/auth/challenge", "", origin, "", nil, 200)
		cookie := findCookie(out, "__Host-retail_admin_prelogin")
		if !cookie.HttpOnly || !cookie.Secure || cookie.Domain != "" || cookie.Path != "/" || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 600 {
			t.Fatal("challenge cookie attributes invalid")
		}
		return cookie, parseCSRF(out)
	}
	loginBody := `{"email":"new@example.invalid","password":"` + password + `"}`
	login := func(previous *http.Cookie) (*http.Cookie, string) {
		pre, csrf := challenge()
		cookies := []*http.Cookie{pre}
		if previous != nil {
			cookies = append(cookies, previous)
		}
		out := call("POST", "/api/admin/auth/login", loginBody, origin, csrf, cookies, 200)
		session := findCookie(out, "__Host-retail_admin_session")
		csrfCookie := findCookie(out, "__Host-retail_admin_csrf")
		if !session.HttpOnly || !session.Secure || session.Domain != "" || session.Path != "/" || session.SameSite != http.SameSiteStrictMode || session.MaxAge != 43200 || csrfCookie.HttpOnly || csrfCookie.Value != parseCSRF(out) {
			t.Fatal("session cookie attributes invalid")
		}
		return session, csrfCookie.Value
	}
	call("POST", "/api/admin/auth/challenge", "", "", "", nil, 403)
	call("POST", "/api/admin/auth/challenge", "", "https://evil.example", "", nil, 403)
	call("GET", "/api/admin/auth/me", "", origin, "", nil, 401)
	call("GET", "/health", "", origin, "", nil, 200)
	if db.Exec("UPDATE tax_runtime_schema SET reads_enabled=FALSE WHERE id=1").Error != nil {
		t.Fatal("artificial tax readiness setup failed")
	}
	call("GET", "/health", "", origin, "", nil, 503)
	if db.Exec("UPDATE tax_runtime_schema SET reads_enabled=TRUE WHERE id=1").Error != nil {
		t.Fatal("artificial tax readiness restore failed")
	}
	call("GET", "/health", "", origin, "", nil, 200)
	call("POST", "/api/admin/auth/login", loginBody, origin, "", nil, 403)
	call("POST", "/api/admin/auth/login", `{"email":"a","email":"b","password":"x"}`, origin, "", nil, 400)
	badPre, badCSRF := challenge()
	call("POST", "/api/admin/auth/login", `{"email":"new@example.invalid","password":"wrong-artificial-password"}`, origin, badCSRF, []*http.Cookie{badPre}, 401)
	unknownPre, unknownCSRF := challenge()
	call("POST", "/api/admin/auth/login", `{"email":"unknown@example.invalid","password":"wrong-artificial-password"}`, origin, unknownCSRF, []*http.Cookie{unknownPre}, 401)
	pre, csrf := challenge()
	call("POST", "/api/admin/auth/login", loginBody, origin, strings.Repeat("0", 64), []*http.Cookie{pre}, 403)
	out := call("POST", "/api/admin/auth/login", loginBody, origin, csrf, []*http.Cookie{pre}, 200)
	session := findCookie(out, "__Host-retail_admin_session")
	csrf = parseCSRF(out)
	call("POST", "/api/admin/auth/login", loginBody, origin, csrf, []*http.Cookie{pre}, 401)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 200)
	checkAuthorization(t, db, u, func(path string, status int) []byte {
		return call("GET", path, "", origin, "", []*http.Cookie{session}, status).Body.Bytes()
	})
	checkAuthorizationManagement(t, db, u, func(path string, status int) []byte {
		return call("GET", path, "", origin, "", []*http.Cookie{session}, status).Body.Bytes()
	}, func(body string, status int) []byte {
		return call("POST", "/api/admin/authorization/commands", body, origin, csrf, []*http.Cookie{session}, status).Body.Bytes()
	})
	call("POST", "/api/admin/authorization/commands", "{}", origin, "", []*http.Cookie{session}, 403)
	checkWarehouseManagement(t, ctx, db, u, func(path string, status int) []byte {
		return call("GET", path, "", origin, "", []*http.Cookie{session}, status).Body.Bytes()
	}, func(body string, status int) []byte {
		return call("POST", "/api/admin/warehouses", body, origin, csrf, []*http.Cookie{session}, status).Body.Bytes()
	})
	call("POST", "/api/admin/warehouses", "{}", origin, "", []*http.Cookie{session}, 403)
	checkWarehouseScope(t, db, u)
	checkWarehouseUsers(t, db, u, call, session, csrf)
	checkDeviceSources(t, db, u, func(method, path, body string, status int) []byte {
		return call(method, path, body, origin, csrf, []*http.Cookie{session}, status).Body.Bytes()
	})
	call("POST", "/api/admin/device-sources", `{}`, origin, "", []*http.Cookie{session}, 403)
	checkDeviceIdentity(t, db)
	checkDeviceTracking(t, db, u, func(method, path, body string, status int) []byte {
		return call(method, path, body, origin, csrf, []*http.Cookie{session}, status).Body.Bytes()
	})
	call("POST", "/api/admin/products/tracking-product/variants/tracking-v1/device-tracking", `{}`, origin, "", []*http.Cookie{session}, 403)
	checkDeviceIntake(t, db, u, func(method, path, body string, status int) []byte {
		return call(method, path, body, origin, csrf, []*http.Cookie{session}, status).Body.Bytes()
	})
	call("POST", "/api/admin/products/intake-product/variants/intake-v/device-intake", `{}`, origin, "", []*http.Cookie{session}, 403)

	call("PATCH", "/api/admin/products/fixture", `{"title":"fixture","status":"published"}`, origin, "", []*http.Cookie{session}, 403)
	call("PATCH", "/api/admin/products/fixture", `{"title":"fixture","status":"published"}`, origin, csrf, []*http.Cookie{session}, 400)
	call("POST", "/api/admin/inventory-levels/fixture/stock", `{}`, origin, "", []*http.Cookie{session}, 403)
	call("POST", "/api/admin/inventory-levels/fixture/stock", `{}`, origin, csrf, []*http.Cookie{session}, 400)
	call("POST", "/api/admin/inventory-levels/fixture/stock", `{}`, origin, csrf, nil, 401)
	var unimplementedWrites int64
	if db.Table("audit_events").Where("action <> ?", "authorization.bootstrap").Count(&unimplementedWrites).Error != nil || unimplementedWrites != 0 {
		t.Fatal("unimplemented product write created audit data")
	}
	checkNotificationMail(t, ctx, db, u, router, sessions, func(method, path, body, requestOrigin, requestCSRF string, status int) []byte {
		return call(method, path, body, requestOrigin, requestCSRF, []*http.Cookie{session}, status).Body.Bytes()
	}, csrf)
 checkPriceAmountEdit(t,db,u,func(method,path,body string,status int)[]byte{return call(method,path,body,origin,csrf,[]*http.Cookie{session},status).Body.Bytes()})
 call("POST","/api/admin/products/price-edit-product/variants/price-edit-v/prices/price-edit-base",`{}`,origin,"",[]*http.Cookie{session},403)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session, session}, 401)
	call("POST", "/api/admin/auth/logout", "", "", csrf, []*http.Cookie{session}, 403)
	call("POST", "/api/admin/auth/logout", "", origin, "", []*http.Cookie{session}, 403)
	call("POST", "/api/admin/auth/logout", "", origin, strings.Repeat("0", 64), []*http.Cookie{session}, 403)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 200)
	rotated, rotatedCSRF := login(session)
	if rotated.Value == session.Value {
		t.Fatal("login did not rotate capability")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 401)
	call("POST", "/api/admin/auth/logout", "", origin, rotatedCSRF, []*http.Cookie{rotated}, 200)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{rotated}, 401)

	active, _ := login(nil)
	if db.Exec("UPDATE admin_users SET enabled=FALSE,auth_version=auth_version+1 WHERE id=?", u.ID).Error != nil {
		t.Fatal("fixture disable failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 401)
	if db.Exec("UPDATE admin_users SET enabled=TRUE,auth_version=auth_version+1 WHERE id=?", u.ID).Error != nil {
		t.Fatal("fixture reenable failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 401)
	active, _ = login(nil)
	if db.Exec("UPDATE admin_users SET role='warehouse',auth_version=auth_version+1 WHERE id=?", u.ID).Error != nil {
		t.Fatal("fixture role edit failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 401)
	if db.Exec("UPDATE admin_users SET role='admin',auth_version=auth_version+1 WHERE id=?", u.ID).Error != nil {
		t.Fatal("fixture role restore failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 401)
	active, _ = login(nil)
	if db.Exec("UPDATE admin_users SET auth_version=auth_version+1 WHERE id=?", u.ID).Error != nil {
		t.Fatal("fixture revoke all failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 401)
	active, activeCSRF := login(nil)
	changedHash, err := bcrypt.GenerateFromPassword([]byte("changed-artificial-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal("changed fixture KDF failed")
	}
	if db.Exec("UPDATE admin_users SET password_hash=?,auth_version=auth_version+1 WHERE id=?", string(changedHash), u.ID).Error != nil {
		t.Fatal("fixture password change failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 401)
	if db.Exec("UPDATE admin_users SET password_hash=?,auth_version=auth_version+1 WHERE id=?", string(hash), u.ID).Error != nil {
		t.Fatal("fixture password restore failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 401)
	active, activeCSRF = login(nil)
	limitedPre, limitedCSRF := challenge()
	call("POST", "/api/admin/auth/login", loginBody, origin, limitedCSRF, []*http.Cookie{limitedPre}, 429)
	// A formerly valid MySQL session row cannot authenticate in Redis.
	if db.Exec("INSERT INTO admin_sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,DATE_ADD(UTC_TIMESTAMP(),INTERVAL 1 DAY),UTC_TIMESTAMP())", strings.Repeat("a", 64), u.ID).Error != nil {
		t.Fatal("legacy session fixture failed")
	}
	legacy := &http.Cookie{Name: "retail_admin_session", Value: strings.Repeat("a", 64)}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{legacy}, 401)
	customer, err := sessions.Issue(ctx, sessionstore.Customer, sessionstore.Principal{UserID: u.ID, Version: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal("customer fixture failed")
	}
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{{Name: active.Name, Value: customer.Token}}, 401)
	var oldSessions int64
	if db.Table("admin_sessions").Count(&oldSessions).Error != nil || oldSessions != 1 {
		t.Fatal("runtime consulted or changed legacy session table")
	}
	checkCustomerAccounts(t, ctx, db, sessions)
	// Dependency failure must not return a user or success logout.
	_ = client.Close()
	call("GET", "/health", "", origin, "", nil, 503)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 503)
	call("POST", "/api/admin/auth/logout", "", origin, activeCSRF, []*http.Cookie{active}, 503)
	call("POST", "/api/admin/auth/challenge", "", origin, "", nil, 503)
	// A new Redis client proves MySQL failure is separately fail-closed.
	sessions2, client2, err := sessionstore.Open(ctx, "redis://127.0.0.1:6379/0", "eurorefurb:identity_fixture:v1:")
	if err != nil {
		t.Fatal("reopened store unavailable")
	}
	defer client2.Close()
	router, err = server.RouterWithAuth(db, identity.Options{Sessions: sessions2, Origin: origin, Secure: true})
	if err != nil {
		t.Fatal("second fixture router failed")
	}
	_ = pool.Close()
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{active}, 503)
	t.Log("IDENTITY_REHEARSAL " + `{"explicit_migrations":66,"login_and_me":1,"csrf_and_origin_rejection":1,"session_rotation":1,"logout_revocation":1,"disable_reenable_revocation":1,"role_restore_revocation":1,"password_restore_revocation":1,"invalid_credentials_rejection":1,"login_rate_limit":1,"generation_revocation":1,"legacy_and_customer_rejection":1,"redis_failure_rejection":1,"mysql_failure_rejection":1}`)
}
