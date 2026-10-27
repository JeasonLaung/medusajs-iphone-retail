// Package identity authenticates new local accounts only. Legacy accounts,
// sessions and same-email historical business ownership are never imported.
package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	Email        string    `json:"email"`
	AuthVersion  uint64    `json:"-"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "admin_users" }

type Sessions interface {
	Ping(context.Context) error
	Prelogin(context.Context, sessionstore.Realm) (sessionstore.Issued, error)
	ConsumePrelogin(context.Context, sessionstore.Realm, string, string) error
	Issue(context.Context, sessionstore.Realm, sessionstore.Principal) (sessionstore.Issued, error)
	Resolve(context.Context, sessionstore.Realm, string) (sessionstore.Principal, error)
	VerifyCSRF(context.Context, sessionstore.Realm, string, string) error
	Revoke(context.Context, sessionstore.Realm, string) error
	AllowLogin(context.Context, sessionstore.Realm, string) (bool, error)
}

type Accounts interface {
	ByEmail(context.Context, string) (User, error)
	ByID(context.Context, string) (User, error)
}

type sqlAccounts struct{ db *gorm.DB }

func (s sqlAccounts) ByEmail(ctx context.Context, email string) (User, error) {
	var u User
	if s.db == nil {
		return u, errDependency
	}
	err := s.db.WithContext(ctx).Where("email=?", email).First(&u).Error
	return u, err
}
func (s sqlAccounts) ByID(ctx context.Context, id string) (User, error) {
	var u User
	if s.db == nil {
		return u, errDependency
	}
	err := s.db.WithContext(ctx).Where("id=?", id).First(&u).Error
	return u, err
}

var errDependency = errors.New("account dependency unavailable")

// SameAuthority reuses the full session generation comparison inside commands.
func SameAuthority(current, expected User) bool {
	return current.ID == expected.ID && current.Enabled && current.Role == "admin" && current.AuthVersion > 0 && subtle.ConstantTimeCompare([]byte(authVersion(current)), []byte(authVersion(expected))) == 1
}

type Options struct {
	Sessions Sessions
	Origin   string
	Secure   bool
}
type Handler struct {
	accounts Accounts
	sessions Sessions
	origin   string
	secure   bool
}

func (h *Handler) Dependency(ctx context.Context) error {
	if h.sessions == nil {
		return errDependency
	}
	return h.sessions.Ping(ctx)
}

// New keeps the old factory fail-closed until explicit configuration is given.
// It cannot issue sessions or consult the former MySQL session table.
func New(db *gorm.DB) *Handler { return &Handler{accounts: sqlAccounts{db: db}} }
func NewWithOptions(db *gorm.DB, options Options) (*Handler, error) {
	return newHandler(sqlAccounts{db: db}, options)
}
func newHandler(accounts Accounts, options Options) (*Handler, error) {
	u, err := url.Parse(options.Origin)
	if err != nil || u == nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != options.Origin || options.Sessions == nil || accounts == nil {
		return nil, errDependency
	}
	if u.Scheme != "https" && (u.Scheme != "http" || options.Secure || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) {
		return nil, errDependency
	}
	if u.Scheme == "https" && !options.Secure {
		return nil, errDependency
	}
	return &Handler{accounts: accounts, sessions: options.Sessions, origin: options.Origin, secure: options.Secure}, nil
}
func (h *Handler) ready(c *gin.Context) bool {
	if h.sessions == nil || h.origin == "" {
		platform.Fail(c, 503, "auth.unavailable", "Authentication unavailable")
		return false
	}
	return true
}
func (h *Handler) originOK(c *gin.Context) bool {
	if !h.ready(c) {
		return false
	}
	origins := c.Request.Header.Values("Origin")
	if len(origins) != 1 || origins[0] != h.origin || c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		platform.Fail(c, 403, "auth.invalid_origin", "Trusted origin required")
		return false
	}
	return true
}
func (h *Handler) cookieName(kind string) string {
	prefix := "retail_admin_"
	if h.secure {
		prefix = "__Host-retail_admin_"
	}
	return prefix + kind
}
func (h *Handler) setCookie(c *gin.Context, kind, value string, ttl time.Duration, httpOnly bool) {
	http.SetCookie(c.Writer, &http.Cookie{Name: h.cookieName(kind), Value: value, Path: "/", MaxAge: int(ttl.Seconds()), HttpOnly: httpOnly, Secure: h.secure, SameSite: http.SameSiteStrictMode})
}
func (h *Handler) clearCookie(c *gin.Context, kind string, httpOnly bool) {
	http.SetCookie(c.Writer, &http.Cookie{Name: h.cookieName(kind), Value: "", Path: "/", MaxAge: -1, HttpOnly: httpOnly, Secure: h.secure, SameSite: http.SameSiteStrictMode})
}
func (h *Handler) cookie(c *gin.Context, kind string) (string, bool) {
	found := ""
	count := 0
	for _, cookie := range c.Request.Cookies() {
		if cookie.Name == h.cookieName(kind) {
			found = cookie.Value
			count++
		}
	}
	if count != 1 || len(found) != 64 {
		return "", false
	}
	decoded, err := hex.DecodeString(found)
	return found, err == nil && hex.EncodeToString(decoded) == found
}
func csrfHeader(c *gin.Context) string {
	values := c.Request.Header.Values("X-CSRF-Token")
	if len(values) != 1 {
		return ""
	}
	return values[0]
}
func (h *Handler) sessionError(c *gin.Context, err error) {
	if errors.Is(err, sessionstore.ErrUnavailable) {
		platform.Fail(c, 503, "auth.unavailable", "Authentication unavailable")
		return
	}
	if errors.Is(err, sessionstore.ErrCSRF) {
		platform.Fail(c, 403, "auth.invalid_csrf", "Invalid CSRF token")
		return
	}
	platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
}
func authVersion(u User) string {
	// Never returned to the client; includes the durable revocation generation.
	data, _ := json.Marshal([]any{u.ID, u.PasswordHash, u.Role, u.Enabled, u.AuthVersion})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Reject duplicate fields as well as unknown fields and trailing documents.
// Login identity must not depend on a JSON parser's first/last-key behavior.
func decodeLogin(d *json.Decoder, email, password *string) error {
	start, err := d.Token()
	if err != nil {
		return err
	}
	if start != json.Delim('{') {
		return errDependency
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return errDependency
		}
		seen[key] = true
		switch key {
		case "email":
			err = d.Decode(email)
		case "password":
			err = d.Decode(password)
		default:
			return errDependency
		}
		if err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil {
		return err
	}
	if end != json.Delim('}') {
		return errDependency
	}
	if err := d.Decode(new(any)); err != io.EOF {
		if err == nil {
			return errDependency
		}
		return err
	}
	return nil
}
func (h *Handler) Challenge(c *gin.Context) {
	if !h.originOK(c) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	// Bound anonymous challenge allocation before creating any capability.
	allowed, err := h.sessions.AllowLogin(ctx, sessionstore.Admin, "challenge-ip:"+c.ClientIP())
	if err != nil {
		h.sessionError(c, err)
		return
	}
	if !allowed {
		platform.Fail(c, 429, "auth.rate_limited", "Try again later")
		return
	}
	issued, err := h.sessions.Prelogin(ctx, sessionstore.Admin)
	if err != nil {
		h.sessionError(c, err)
		return
	}
	h.setCookie(c, "prelogin", issued.Token, sessionstore.PreloginTTL, true)
	c.JSON(200, gin.H{"csrf_token": issued.CSRF, "expires_at": issued.ExpiresAt})
}
func (h *Handler) Login(c *gin.Context) {
	var input struct {
		Email    string `json:"email" binding:"required,email,max=254"`
		Password string `json:"password" binding:"required,max=72"`
	}
	// Keep invalid-size/JSON classification independent of dependency status.
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "auth.content_type", "JSON required")
		return
	}
	decoder := json.NewDecoder(c.Request.Body)
	if err := decodeLogin(decoder, &input.Email, &input.Password); err != nil {
		platform.InvalidJSON(c, err, "auth.invalid_login", "Email and password required")
		return
	}
	if err := binding.Validator.ValidateStruct(&input); err != nil || len(input.Password) > 72 {
		platform.Fail(c, 400, "auth.invalid_login", "Email and password required")
		return
	}
	if !h.originOK(c) {
		return
	}
	pre, ok := h.cookie(c, "prelogin")
	if !ok {
		platform.Fail(c, 403, "auth.invalid_csrf", "Login challenge required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.sessions.ConsumePrelogin(ctx, sessionstore.Admin, pre, csrfHeader(c)); err != nil {
		h.sessionError(c, err)
		return
	}
	h.clearCookie(c, "prelogin", true)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	for _, key := range []string{"login-ip:" + c.ClientIP(), "login-email:" + email} {
		allowed, err := h.sessions.AllowLogin(ctx, sessionstore.Admin, key)
		if err != nil {
			h.sessionError(c, err)
			return
		}
		if !allowed {
			platform.Fail(c, 429, "auth.rate_limited", "Try again later")
			return
		}
	}
	user, err := h.accounts.ByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 503, "auth.unavailable", "Authentication unavailable")
		return
	}
	encoded := user.PasswordHash
	if err != nil {
		encoded = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	}
	valid := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(input.Password)) == nil
	if err != nil || !valid || !user.Enabled || user.Role != "admin" || user.AuthVersion == 0 {
		platform.Fail(c, 401, "auth.invalid_credentials", "Email or password is incorrect")
		return
	}
	// Re-read after the slow KDF so a concurrent password/role/revocation edit
	// cannot silently create a session using the stale account row.
	current, err := h.accounts.ByID(ctx, user.ID)
	if err != nil {
		platform.Fail(c, 503, "auth.unavailable", "Authentication unavailable")
		return
	}
	version := authVersion(user)
	if subtle.ConstantTimeCompare([]byte(version), []byte(authVersion(current))) != 1 {
		platform.Fail(c, 401, "auth.invalid_credentials", "Account changed; sign in again")
		return
	}
	issued, err := h.sessions.Issue(ctx, sessionstore.Admin, sessionstore.Principal{UserID: user.ID, Version: version})
	if err != nil {
		h.sessionError(c, err)
		return
	}
	if previous, ok := h.cookie(c, "session"); ok {
		if err := h.sessions.Revoke(ctx, sessionstore.Admin, previous); err != nil {
			_ = h.sessions.Revoke(ctx, sessionstore.Admin, issued.Token)
			h.sessionError(c, err)
			return
		}
	}
	h.setCookie(c, "session", issued.Token, sessionstore.SessionTTL, true)
	// Readable double-submit value survives page reload; server checks its
	// digest from the session, not the cookie supplied by the browser.
	h.setCookie(c, "csrf", issued.CSRF, sessionstore.SessionTTL, false)
	c.JSON(200, gin.H{"user": user, "csrf_token": issued.CSRF})
}
func (h *Handler) Require(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	token, ok := h.cookie(c, "session")
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if !h.ready(c) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	principal, err := h.sessions.Resolve(ctx, sessionstore.Admin, token)
	if err != nil {
		h.sessionError(c, err)
		return
	}
	user, err := h.accounts.ByID(ctx, principal.UserID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 401, "auth.unauthorized", "Session expired")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "auth.unavailable", "Authentication unavailable")
		return
	}
	if !user.Enabled || user.Role != "admin" || user.AuthVersion == 0 || subtle.ConstantTimeCompare([]byte(principal.Version), []byte(authVersion(user))) != 1 {
		platform.Fail(c, 401, "auth.unauthorized", "Session expired")
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
		if !h.originOK(c) {
			return
		}
		if err := h.sessions.VerifyCSRF(ctx, sessionstore.Admin, token, csrfHeader(c)); err != nil {
			h.sessionError(c, err)
			return
		}
	}
	c.Set("admin_id", user.ID)
	c.Set("admin_user", user)
	c.Next()
}
func (h *Handler) Me(c *gin.Context) { c.JSON(200, gin.H{"user": c.MustGet("admin_user")}) }
func (h *Handler) Logout(c *gin.Context) {
	token, ok := h.cookie(c, "session")
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if err := h.sessions.Revoke(ctx, sessionstore.Admin, token); err != nil {
		h.sessionError(c, err)
		return
	}
	h.clearCookie(c, "session", true)
	h.clearCookie(c, "csrf", false)
	h.clearCookie(c, "prelogin", true)
	c.JSON(200, gin.H{"success": true})
}
