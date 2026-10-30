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

func decodeReplacement(r io.Reader) (ReplacementInput, error) {
	var in ReplacementInput
	raw, e := io.ReadAll(io.LimitReader(r, 4097))
	if e != nil || len(raw) > 4096 || !utf8.Valid(raw) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if uniqueJSON(d) != nil || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 {
		return in, ErrInput
	}
	for _, v := range fields {
		if len(v) == 0 || v[0] != '"' {
			return in, ErrInput
		}
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || validateEnroll(EnrollInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision}) != nil || !validPathID(in.DeviceID) {
		return in, ErrInput
	}
	return in, nil
}
func (h Handler) ReplaceDeviceHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	var e error
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		e = ErrInput
	} else {
		media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || media != "application/json" {
			platform.Fail(c, 415, "devices.content_type", "JSON required")
			return
		}
		var in ReplacementInput
		in, e = decodeReplacement(c.Request.Body)
		if e == nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
			defer cancel()
			var out ReplacementResult
			out, e = h.ReplaceDevice(ctx, actor, c.Param("id"), c.Param("request_id"), c.Param("assignment_id"), in)
			if e == nil {
				c.JSON(200, out)
				return
			}
		}
	}
	status := 503
	switch {
	case errors.Is(e, ErrInput), errors.Is(e, ErrIdentity):
		status = 400
	case errors.Is(e, authorization.ErrDenied):
		status = 403
	case errors.Is(e, gorm.ErrRecordNotFound):
		status = 404
	case errors.Is(e, ErrConflict):
		status = 409
	}
	platform.Fail(c, status, "devices.replacement_unavailable", "Replacement unavailable; verify current device, order and permission")
}
