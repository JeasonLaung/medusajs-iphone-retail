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
	"gorm.io/gorm"
	"io"
	"mime"
	"time"
	"unicode/utf8"
)

func decodeService(r io.Reader) (ServiceInput, error) {
	var in ServiceInput
	raw, e := io.ReadAll(io.LimitReader(r, 12289))
	if e != nil || len(raw) > 12288 || !utf8.Valid(raw) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if uniqueJSON(d) != nil || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 10 {
		return in, ErrInput
	}
	for k, v := range fields {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return in, ErrInput
		}
		if k != "battery_health" && k != "photos" && (len(v) == 0 || v[0] != '"') {
			return in, ErrInput
		}
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || !validService(in) {
		return in, ErrInput
	}
	return in, nil
}
func (h Handler) ServiceHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	var e error
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		e = ErrInput
	} else if c.Request.Method == "GET" {
		var out ServiceContext
		out, e = h.ReadService(ctx, actor, c.Param("id"), c.Param("request_id"), c.Param("assignment_id"))
		if e == nil {
			c.JSON(200, out)
			return
		}
	} else {
		media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || media != "application/json" {
			platform.Fail(c, 415, "devices.content_type", "JSON required")
			return
		}
		var in ServiceInput
		in, e = decodeService(c.Request.Body)
		if e == nil {
			var out ServiceResult
			out, e = h.ServiceDevice(ctx, actor, c.Param("id"), c.Param("request_id"), c.Param("assignment_id"), in)
			if e == nil {
				c.JSON(200, out)
				return
			}
		}
	}
	status := 503
	switch {
	case errors.Is(e, ErrInput):
		status = 400
	case errors.Is(e, authorization.ErrDenied):
		status = 403
	case errors.Is(e, gorm.ErrRecordNotFound), errors.Is(e, ErrNotFound):
		status = 404
	case errors.Is(e, ErrConflict):
		status = 409
	}
	platform.Fail(c, status, "devices.service_unavailable", "Device service unavailable; verify the request, device and current permission")
}
