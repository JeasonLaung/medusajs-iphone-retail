package cart

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type HTTPSessions interface {
	IssueCartScope(context.Context, string) (sessionstore.Issued, error)
	ResolveCartScope(context.Context, string, string) (sessionstore.CartScope, error)
	AllowCartRequest(context.Context, string, string) (bool, error)
	IssueCart(context.Context, sessionstore.CartReference) (sessionstore.Issued, error)
	ResolveCart(context.Context, string) (sessionstore.CartReference, error)
	AuthorizeCartWrite(context.Context, string, string) (sessionstore.CartReference, error)
}

type HTTPOptions struct {
	Sessions HTTPSessions
	Origin   string
	StoreID  string
	Secure   bool
}

type HTTPHandler struct {
	db       *gorm.DB
	sessions HTTPSessions
	origin   string
	store    string
	secure   bool
}

func NewHTTP(db *gorm.DB, options HTTPOptions) (*HTTPHandler, error) {
	u, err := url.Parse(options.Origin)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != options.Origin || db == nil || options.Sessions == nil || !idPattern.MatchString(options.StoreID) {
		return nil, ErrConfiguration
	}
	if u.Scheme != "https" && (u.Scheme != "http" || options.Secure || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) || u.Scheme == "https" && !options.Secure {
		return nil, ErrConfiguration
	}
	return &HTTPHandler{db, options.Sessions, options.Origin, options.StoreID, options.Secure}, nil
}

// An unconfigured handler keeps these routes closed. Public catalog capability
// checkout remains false; draft item commands do not authorize payment.
func (h *HTTPHandler) Register(group *gin.RouterGroup) {
	private := group.Group("")
	private.Use(platform.PrivateResponse)
	private.POST("/cart-sessions", h.Scope)
	private.POST("/carts", h.Create)
	private.GET("/cart", h.Current)
	private.GET("/carts/:id", h.Detail)
	private.GET("/carts/:id/amounts", h.Amounts)
	private.GET("/carts/:id/view", h.View)
	private.POST("/carts/:id/line-items", h.AddItem)
	private.POST("/carts/:id/line-items/:line", h.SetItem)
}

func (h *HTTPHandler) ready(c *gin.Context) bool {
	if h.db == nil || h.sessions == nil || h.origin == "" || h.store == "" {
		platform.Fail(c, 503, "cart.unavailable", "Cart unavailable")
		return false
	}
	return true
}

func (h *HTTPHandler) originOK(c *gin.Context, required bool) bool {
	origins := c.Request.Header.Values("Origin")
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" || len(origins) > 1 || len(origins) == 1 && origins[0] != h.origin || required && len(origins) != 1 {
		platform.Fail(c, 403, "cart.invalid_origin", "Trusted origin required")
		return false
	}
	return true
}

func canonicalToken(s string) bool {
	v, err := hex.DecodeString(s)
	return len(s) == 64 && err == nil && hex.EncodeToString(v) == s
}

func (h *HTTPHandler) cookieName(kind string) string {
	prefix := "retail_cart_"
	if h.secure {
		prefix = "__Host-retail_cart_"
	}
	return prefix + kind
}

func (h *HTTPHandler) cookie(c *gin.Context, kind string) (string, bool, bool) {
	token, count := "", 0
	for _, cookie := range c.Request.Cookies() {
		if cookie.Name == h.cookieName(kind) {
			token, count = cookie.Value, count+1
		}
	}
	return token, count == 1 && canonicalToken(token), count != 0
}

func (h *HTTPHandler) setCookie(c *gin.Context, kind, value string, expires time.Time, httpOnly bool) {
	ttl := int(time.Until(expires).Seconds())
	if ttl < 1 {
		ttl = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: h.cookieName(kind), Value: value, Path: "/", MaxAge: ttl, Expires: expires, HttpOnly: httpOnly, Secure: h.secure, SameSite: http.SameSiteLaxMode})
}

func csrf(c *gin.Context) string {
	values := c.Request.Header.Values("X-CSRF-Token")
	if len(values) != 1 || !canonicalToken(values[0]) {
		return ""
	}
	return values[0]
}

// Reject duplicate/null/unknown fields and trailing documents. IDs, region and
// optional locale are the only browser-selected creation data; no price/owner.
func decodeHTTP(reader io.Reader, create bool) (string, string, error) {
	d := json.NewDecoder(reader)
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		if err != nil {
			return "", "", err
		}
		return "", "", ErrInput
	}
	seen := map[string]bool{}
	region, locale := "", ""
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return "", "", err
		}
		name, ok := key.(string)
		if !ok || seen[name] || !create || name != "region_id" && name != "locale" {
			return "", "", ErrInput
		}
		seen[name] = true
		var value *string
		if err := d.Decode(&value); err != nil {
			return "", "", err
		}
		if value == nil {
			return "", "", ErrInput
		}
		if name == "region_id" {
			region = *value
		} else {
			locale = *value
		}
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') {
		if err != nil {
			return "", "", err
		}
		return "", "", ErrInput
	}
	if err := d.Decode(new(any)); err != io.EOF {
		if err != nil {
			return "", "", err
		}
		return "", "", ErrInput
	}
	if create && (!idPattern.MatchString(region) || locale != "" && (len(locale) > 35 || !localePattern.MatchString(locale))) {
		return "", "", ErrInput
	}
	return region, locale, nil
}

func decodeRequest(c *gin.Context, create bool) (string, string, bool) {
	if c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "cart.invalid_query", "Invalid cart request")
		return "", "", false
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "cart.content_type", "JSON required")
		return "", "", false
	}
	region, locale, err := decodeHTTP(c.Request.Body, create)
	if err != nil {
		platform.InvalidJSON(c, err, "cart.invalid_input", "Invalid cart request")
		return "", "", false
	}
	return region, locale, true
}

func cartHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, sessionstore.ErrMissing):
		platform.Fail(c, 401, "cart.possession_required", "Cart session required")
	case errors.Is(err, sessionstore.ErrCSRF):
		platform.Fail(c, 403, "cart.invalid_csrf", "Invalid CSRF token")
	case errors.Is(err, ErrDenied):
		platform.Fail(c, 404, "cart.not_found", "Cart unavailable")
	case errors.Is(err, ErrConflict):
		platform.Fail(c, 409, "cart.conflict", "Cart operation changed")
	case errors.Is(err, ErrInput):
		platform.Fail(c, 400, "cart.invalid_input", "Invalid cart request")
	case errors.Is(err, ErrRegion):
		platform.Fail(c, 422, "cart.region_unavailable", "Cart region unavailable")
	case errors.Is(err, ErrItemUnavailable):
		platform.Fail(c, 422, "cart.item_unavailable", "Cart item unavailable")
	case errors.Is(err, ErrItemStock):
		platform.Fail(c, 422, "cart.stock_unavailable", "Insufficient inventory")
	default:
		platform.Fail(c, 503, "cart.unavailable", "Cart unavailable")
	}
}

func (h *HTTPHandler) allow(c *gin.Context, ctx context.Context, kind, identity string) bool {
	ok, err := h.sessions.AllowCartRequest(ctx, kind, identity)
	if err != nil {
		cartHTTPError(c, err)
		return false
	}
	if !ok {
		c.Header("Retry-After", "900")
		platform.Fail(c, 429, "cart.rate_limited", "Try again later")
		return false
	}
	return true
}

// Scope never creates a durable cart. Reusing an existing valid scope requires
// its CSRF and does not refresh cookies/TTL. Missing/expired Redis scope can be
// replaced; a Redis outage cannot be mistaken for expiry.
func (h *HTTPHandler) Scope(c *gin.Context) {
	if _, _, ok := decodeRequest(c, false); !ok || !h.ready(c) || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, valid, present := h.cookie(c, "scope")
	if present && !valid {
		cartHTTPError(c, sessionstore.ErrMissing)
		return
	}
	if valid {
		scope, err := h.sessions.ResolveCartScope(ctx, token, csrf(c))
		if err == nil {
			if scope.StoreID != h.store {
				cartHTTPError(c, sessionstore.ErrMissing)
				return
			}
			c.JSON(200, gin.H{"csrf_token": csrf(c), "reused": true})
			return
		}
		if !errors.Is(err, sessionstore.ErrMissing) {
			cartHTTPError(c, err)
			return
		}
	}
	if !h.allow(c, ctx, "scope-ip", c.ClientIP()) {
		return
	}
	issued, err := h.sessions.IssueCartScope(ctx, h.store)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	h.setCookie(c, "scope", issued.Token, issued.ExpiresAt, true)
	h.setCookie(c, "scope_csrf", issued.CSRF, issued.ExpiresAt, false)
	c.JSON(200, gin.H{"csrf_token": issued.CSRF, "expires_at": issued.ExpiresAt, "reused": false})
}

func (h *HTTPHandler) Create(c *gin.Context) {
	region, locale, ok := decodeRequest(c, true)
	if !ok {
		return
	}
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		cartHTTPError(c, ErrInput)
		return
	}
	id, err := uuid.Parse(keys[0])
	if err != nil || id.Version() != 4 || id.String() != keys[0] {
		cartHTTPError(c, ErrInput)
		return
	}
	if !h.ready(c) || !h.originOK(c, true) {
		return
	}
	token, valid, _ := h.cookie(c, "scope")
	if !valid {
		cartHTTPError(c, sessionstore.ErrMissing)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	scope, err := h.sessions.ResolveCartScope(ctx, token, csrf(c))
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	if scope.StoreID != h.store {
		cartHTTPError(c, sessionstore.ErrMissing)
		return
	}
	if !h.allow(c, ctx, "create-scope", token) {
		return
	}
	created, err := Create(ctx, h.db, CreateCommand{keys[0], scope.Hash, h.store, region, locale})
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	snapshot, err := Inspect(ctx, h.db, h.store, created.Reference)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	issued, err := h.sessions.IssueCart(ctx, created.Reference)
	if err != nil {
		// The SQL receipt is durable. Retry this scope/operation; never return
		// an ID or credential on uncertain issuance and never recreate the cart.
		cartHTTPError(c, err)
		return
	}
	h.setCookie(c, "possession", issued.Token, issued.ExpiresAt, true)
	h.setCookie(c, "csrf", issued.CSRF, issued.ExpiresAt, false)
	status := 201
	if created.Replayed {
		status = 200
	}
	c.JSON(status, gin.H{"cart": snapshot, "csrf_token": issued.CSRF, "expires_at": issued.ExpiresAt, "replayed": created.Replayed, "capabilities": gin.H{"items": false, "checkout": false}})
}

func (h *HTTPHandler) Detail(c *gin.Context) {
	h.detail(c, "detail", false)
}

// Current derives the ID from server-resolved possession and still verifies
// current SQL ownership. No readable cart ID cookie or browser storage needed.
func (h *HTTPHandler) Current(c *gin.Context) {
	h.detail(c, "detail", true)
}

func (h *HTTPHandler) Amounts(c *gin.Context) {
	h.detail(c, "amounts", false)
}

func (h *HTTPHandler) View(c *gin.Context) {
	h.detail(c, "view", false)
}

func (h *HTTPHandler) detail(c *gin.Context, mode string, current bool) {
	if c.Request.URL.RawQuery != "" || !current && !idPattern.MatchString(c.Param("id")) {
		cartHTTPError(c, ErrInput)
		return
	}
	if !h.ready(c) || !h.originOK(c, false) {
		return
	}
	token, valid, _ := h.cookie(c, "possession")
	if !valid {
		cartHTTPError(c, sessionstore.ErrMissing)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	ref, err := h.sessions.ResolveCart(ctx, token)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	if ref.StoreID != h.store || !current && ref.CartID != c.Param("id") {
		cartHTTPError(c, ErrDenied)
		return
	}
	if mode != "detail" {
		if mode == "view" {
			result, err := ReadView(ctx, h.db, h.store, ref)
			if err != nil {
				cartHTTPError(c, err)
				return
			}
			c.JSON(200, result)
			return
		}
		result, err := ReadAmounts(ctx, h.db, h.store, ref)
		if err != nil {
			cartHTTPError(c, err)
			return
		}
		c.JSON(200, result)
		return
	}
	snapshot, err := Inspect(ctx, h.db, h.store, ref)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	c.JSON(200, gin.H{"cart": snapshot, "capabilities": gin.H{"items": false, "checkout": false}})
}
