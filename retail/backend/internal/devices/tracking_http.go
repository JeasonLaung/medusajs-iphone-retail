package devices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"time"
	"unicode/utf8"
)

func decodeEnroll(r io.Reader) (EnrollInput, error) {
	var in EnrollInput
	raw, e := io.ReadAll(io.LimitReader(r, 4097))
	if e != nil || len(raw) > 4096 || !utf8.Valid(raw) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return in, ErrInput
	}
	fields := map[string]string{}
	for d.More() {
		t, e := d.Token()
		k, ok := t.(string)
		if e != nil || !ok || (k != "operation_id" && k != "expected_revision") {
			return in, ErrInput
		}
		if _, ok := fields[k]; ok {
			return in, ErrInput
		}
		var v json.RawMessage
		if d.Decode(&v) != nil || len(v) == 0 || v[0] != '"' {
			return in, ErrInput
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			return in, ErrInput
		}
		fields[k] = s
	}
	if _, e = d.Token(); e != nil || len(fields) != 2 {
		return in, ErrInput
	}
	if d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	in = EnrollInput{fields["operation_id"], fields["expected_revision"]}
	return in, validateEnroll(in)
}
func trackingFail(c *gin.Context, e error) {
	switch {
	case errors.Is(e, ErrInput), errors.Is(e, ErrIdentity):
		platform.Fail(c, 400, "device.invalid_input", "Invalid device request")
	case errors.Is(e, ErrNotFound):
		platform.Fail(c, 404, "device.not_found", "SKU unavailable")
	case errors.Is(e, ErrConflict), errors.Is(e, ErrDuplicate):
		platform.Fail(c, 409, "device.conflict", "Device configuration changed; reload")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Action not permitted")
	default:
		platform.Fail(c, 503, "device.unavailable", "Device tracking unavailable")
	}
}
func (h Handler) TrackingHTTP(c *gin.Context) { h.trackingHTTP(c, false) }
func (h Handler) EnrollHTTP(c *gin.Context)   { h.trackingHTTP(c, true) }
func (h Handler) trackingHTTP(c *gin.Context, write bool) {
	if c.Request.URL.RawQuery != "" {
		trackingFail(c, ErrInput)
		return
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	productID, variantID := c.Param("id"), c.Param("variant_id")
	if !write {
		out, e := h.ReadTracking(ctx, actor, productID, variantID)
		if e != nil {
			trackingFail(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "device.content_type", "JSON required")
		return
	}
	in, e := decodeEnroll(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "device.invalid_input", "Invalid device request")
		return
	}
	out, e := h.Enroll(ctx, actor, productID, variantID, in)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, out)
}
