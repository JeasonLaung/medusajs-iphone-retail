package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HTTPSessions interface {
	Sessions
	Prelogin(context.Context, sessionstore.Realm) (sessionstore.Issued, error)
	ConsumePrelogin(context.Context, sessionstore.Realm, string, string) error
	VerifyCSRF(context.Context, sessionstore.Realm, string, string) error
	AllowLogin(context.Context, sessionstore.Realm, string) (bool, error)
}
type HTTPOptions struct {
	Sessions  HTTPSessions
	Ring      *protected.Ring
	Origin    string
	Secure    bool
	BridgeKey string
}
type HTTPHandler struct {
	store        *Store
	sessions     HTTPSessions
	origin, host string
	secure       bool
	bridgeSecret []byte
}

func NewHTTP(db *gorm.DB, options HTTPOptions) (*HTTPHandler, error) {
	store, err := New(db, options.Ring, options.Sessions, options.Origin)
	if err != nil {
		return nil, ErrUnavailable
	}
	u, _ := url.Parse(options.Origin)
	if (u.Scheme == "https") != options.Secure {
		return nil, ErrUnavailable
	}
	var key []byte
	if options.BridgeKey != "" {
		var ok bool
		key, ok = bridgeKey(options.BridgeKey)
		if !ok {
			return nil, ErrUnavailable
		}
	}
	return &HTTPHandler{store: store, sessions: options.Sessions, origin: options.Origin, host: u.Host, secure: options.Secure, bridgeSecret: key}, nil
}

// Register is mounted on /api/store/customer by the production API, even when
// disabled. Unconfigured handlers report 503 and never issue capabilities.
func (h *HTTPHandler) Register(group *gin.RouterGroup) {
	private := group.Group("")
	private.Use(platform.PrivateResponse)
	private.Use(h.bridge)
	private.Use(func(c *gin.Context) {
		c.Header("Referrer-Policy", "no-referrer")
		if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
			platform.Fail(c, 404, "not_found", "Resource not found")
			return
		}
		c.Next()
	})
	private.POST("/auth/challenge", h.Challenge)
	private.POST("/auth/register", h.RegisterAccount)
	private.POST("/auth/login", h.LoginAccount)
	private.POST("/auth/verify", h.VerifyAccount)
	private.POST("/auth/resend", h.ResendVerification)
	private.GET("/me", h.Me)
	private.POST("/me", h.UpdateProfile)
	private.GET("/addresses", h.ListAddresses)
	private.POST("/addresses", h.CreateAddress)
	private.POST("/addresses/:id", h.UpdateAddress)
	private.POST("/addresses/:id/delete", h.RemoveAddress)
	private.POST("/auth/logout", h.LogoutAccount)
}
func (h *HTTPHandler) ready(c *gin.Context) bool {
	if h == nil || h.store == nil || h.sessions == nil || h.origin == "" {
		h.fail(c, ErrUnavailable)
		return false
	}
	return true
}
func (h *HTTPHandler) originOK(c *gin.Context, required bool) bool {
	if !h.ready(c) {
		return false
	}
	values := c.Request.Header.Values("Origin")
	if c.Request.Host != h.host || c.GetHeader("Sec-Fetch-Site") == "cross-site" || len(values) > 1 || (len(values) == 1 && values[0] != h.origin) || (required && len(values) != 1) {
		platform.Fail(c, 403, "customer.invalid_origin", "Trusted origin required")
		return false
	}
	return true
}
func (h *HTTPHandler) cookieName(kind string) string {
	prefix := "retail_customer_"
	if h.secure {
		prefix = "__Host-retail_customer_"
	}
	return prefix + kind
}
func (h *HTTPHandler) cookie(c *gin.Context, kind string) (string, bool, bool) {
	value, count := "", 0
	for _, cookie := range c.Request.Cookies() {
		if cookie.Name == h.cookieName(kind) {
			value = cookie.Value
			count++
		}
	}
	return value, count == 1 && tokenOK(value), count > 0
}
func (h *HTTPHandler) setCookie(c *gin.Context, kind, value string, expires time.Time, httpOnly bool) {
	ttl := int(time.Until(expires).Seconds())
	if ttl < 1 {
		ttl = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: h.cookieName(kind), Value: value, Path: "/", Expires: expires, MaxAge: ttl, HttpOnly: httpOnly, Secure: h.secure, SameSite: http.SameSiteLaxMode})
}
func (h *HTTPHandler) clearCookie(c *gin.Context, kind string, httpOnly bool) {
	http.SetCookie(c.Writer, &http.Cookie{Name: h.cookieName(kind), Path: "/", MaxAge: -1, HttpOnly: httpOnly, Secure: h.secure, SameSite: http.SameSiteLaxMode})
}
func customerCSRF(c *gin.Context) string {
	values := c.Request.Header.Values("X-CSRF-Token")
	if len(values) != 1 || !tokenOK(values[0]) {
		return ""
	}
	return values[0]
}
func (h *HTTPHandler) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrAddressMissing):
		platform.Fail(c, 404, "customer.address_not_found", "Address not found")
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrVerification):
		platform.Fail(c, 400, "customer.invalid_request", "Request or verification link is invalid")
	case errors.Is(err, ErrExists):
		platform.Fail(c, 409, "customer.registration_unavailable", "Registration unavailable for this email; sign in or resend verification")
	case errors.Is(err, ErrUnverified):
		c.JSON(403, gin.H{"state": "verification_required", "error": gin.H{"code": "customer.verification_required", "message": "Verify your email before signing in"}})
	case errors.Is(err, ErrCredentials), errors.Is(err, ErrUnauthorized), errors.Is(err, sessionstore.ErrMissing):
		platform.Fail(c, 401, "customer.unauthorized", "Sign in with your email and password")
	case errors.Is(err, sessionstore.ErrCSRF):
		platform.Fail(c, 403, "customer.invalid_csrf", "Valid CSRF token required")
	case errors.Is(err, ErrRateLimited):
		c.Header("Retry-After", "60")
		platform.Fail(c, 429, "customer.rate_limited", "Try again later")
	default:
		platform.Fail(c, 503, "customer.unavailable", "Customer service unavailable")
	}
}

// String-only, bounded JSON prevents duplicate/unknown/null fields, second
// documents and query-string credentials from choosing parser-dependent input.
func (h *HTTPHandler) input(c *gin.Context, allowed ...string) (map[string]string, bool) {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "customer.content_type", "JSON required")
		return nil, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		platform.InvalidJSON(c, err, "customer.invalid_request", "Valid customer JSON required")
		return nil, false
	}
	if !utf8.Valid(body) {
		h.fail(c, ErrInvalid)
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	accepted := map[string]bool{}
	for _, key := range allowed {
		accepted[key] = true
	}
	values := map[string]string{}
	decode := func() error {
		start, err := decoder.Token()
		if err != nil {
			return err
		}
		if start != json.Delim('{') {
			return ErrInvalid
		}
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := value.(string)
			if !ok || !accepted[key] {
				return ErrInvalid
			}
			if _, exists := values[key]; exists {
				return ErrInvalid
			}
			var raw any
			if err := decoder.Decode(&raw); err != nil {
				return err
			}
			text, ok := raw.(string)
			if !ok {
				return ErrInvalid
			}
			values[key] = text
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return ErrInvalid
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			if err != nil {
				return err
			}
			return ErrInvalid
		}
		return nil
	}
	if err := decode(); err != nil {
		platform.InvalidJSON(c, err, "customer.invalid_request", "Valid customer JSON required")
		return nil, false
	}
	return values, true
}
func (h *HTTPHandler) allow(c *gin.Context, ctx context.Context, operation, email string) bool {
	keys := []string{"customer-http:" + operation + "-ip:" + h.clientIP(c)}
	if email != "" {
		normalized, ok := normalizedEmail(email)
		if !ok {
			h.fail(c, ErrInvalid)
			return false
		}
		keys = append(keys, "customer-http:"+operation+"-email:"+normalized)
	}
	for _, key := range keys {
		allowed, err := h.sessions.AllowLogin(ctx, sessionstore.Customer, key)
		if err != nil {
			h.fail(c, ErrUnavailable)
			return false
		}
		if !allowed {
			c.Header("Retry-After", "900")
			platform.Fail(c, 429, "customer.rate_limited", "Try again later")
			return false
		}
	}
	return true
}
func (h *HTTPHandler) prelogin(c *gin.Context, ctx context.Context) bool {
	token, ok, _ := h.cookie(c, "prelogin")
	if !ok || customerCSRF(c) == "" {
		h.fail(c, sessionstore.ErrCSRF)
		return false
	}
	if err := h.sessions.ConsumePrelogin(ctx, sessionstore.Customer, token, customerCSRF(c)); err != nil {
		if errors.Is(err, sessionstore.ErrUnavailable) {
			h.fail(c, ErrUnavailable)
		} else {
			h.fail(c, sessionstore.ErrCSRF)
		}
		return false
	}
	h.clearCookie(c, "prelogin", true)
	return true
}
func (h *HTTPHandler) Challenge(c *gin.Context) {
	if _, ok := h.input(c); !ok {
		return
	}
	if !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if !h.allow(c, ctx, "challenge", "") {
		return
	}
	issued, err := h.sessions.Prelogin(ctx, sessionstore.Customer)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.setCookie(c, "prelogin", issued.Token, issued.ExpiresAt, true)
	c.JSON(200, gin.H{"csrf_token": issued.CSRF, "expires_at": issued.ExpiresAt})
}
func (h *HTTPHandler) RegisterAccount(c *gin.Context) {
	input, ok := h.input(c, "email", "password", "first_name", "last_name", "phone", "country", "locale")
	if !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if !h.prelogin(c, ctx) || !h.allow(c, ctx, "register", input["email"]) {
		return
	}
	account, err := h.store.Register(ctx, Registration{Email: input["email"], Password: input["password"], FirstName: input["first_name"], LastName: input["last_name"], Phone: input["phone"], Country: input["country"], Locale: input["locale"]})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(202, gin.H{"state": "verification_required", "email": account.Email, "delivery": "pending"})
}
func (h *HTTPHandler) LoginAccount(c *gin.Context) {
	input, ok := h.input(c, "email", "password")
	if !ok || !h.originOK(c, true) {
		return
	}
	previous, valid, present := h.cookie(c, "session")
	if present && !valid {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if !h.prelogin(c, ctx) || !h.allow(c, ctx, "login", input["email"]) {
		return
	}
	result, err := h.store.Login(ctx, input["email"], input["password"])
	if err != nil {
		h.fail(c, err)
		return
	}
	if present {
		if err := h.store.Logout(ctx, previous); err != nil {
			_ = h.store.Logout(ctx, result.Session.Token)
			h.fail(c, ErrUnavailable)
			return
		}
	}
	h.setCookie(c, "session", result.Session.Token, result.Session.ExpiresAt, true)
	h.setCookie(c, "csrf", result.Session.CSRF, result.Session.ExpiresAt, false)
	c.JSON(200, gin.H{"customer": result.Customer, "csrf_token": result.Session.CSRF})
}
func (h *HTTPHandler) VerifyAccount(c *gin.Context) {
	input, ok := h.input(c, "token")
	if !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if !h.prelogin(c, ctx) || !h.allow(c, ctx, "verify", "") {
		return
	}
	if err := h.store.Verify(ctx, input["token"]); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true})
}
func (h *HTTPHandler) ResendVerification(c *gin.Context) {
	input, ok := h.input(c, "email", "password", "country", "locale")
	if !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if !h.prelogin(c, ctx) || !h.allow(c, ctx, "resend", input["email"]) {
		return
	}
	if err := h.store.Resend(ctx, input["email"], input["password"], input["country"], input["locale"]); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(202, gin.H{"state": "verification_required", "delivery": "pending"})
}
func (h *HTTPHandler) Me(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	account, err := h.store.Current(ctx, token)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"customer": account})
}
func (h *HTTPHandler) LogoutAccount(c *gin.Context) {
	if _, ok := h.input(c); !ok || !h.originOK(c, true) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if _, err := h.store.Current(ctx, token); err != nil {
		h.fail(c, err)
		return
	}
	if err := h.sessions.VerifyCSRF(ctx, sessionstore.Customer, token, customerCSRF(c)); err != nil {
		h.fail(c, err)
		return
	}
	if err := h.store.Logout(ctx, token); err != nil {
		h.fail(c, err)
		return
	}
	h.clearCookie(c, "session", true)
	h.clearCookie(c, "csrf", false)
	h.clearCookie(c, "prelogin", true)
	c.JSON(200, gin.H{"success": true})
}

func (h *HTTPHandler) UpdateProfile(c *gin.Context) {
	fields, ok := h.input(c, "first_name", "last_name", "phone")
	if !ok || !h.originOK(c, true) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.sessions.VerifyCSRF(ctx, sessionstore.Customer, token, customerCSRF(c)); err != nil {
		h.fail(c, err)
		return
	}
	if !h.allow(c, ctx, "profile", "") {
		return
	}
	account, err := h.store.UpdateProfile(ctx, token, fields)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"customer": account})
}
