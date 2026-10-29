package devices

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"time"
	"unicode/utf8"
)

func uniqueJSON(d *json.Decoder) error {
	t, e := d.Token()
	if e != nil {
		return ErrInput
	}
	switch t {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			s, ok := k.(string)
			if e != nil || !ok || seen[s] {
				return ErrInput
			}
			seen[s] = true
			if e := uniqueJSON(d); e != nil {
				return e
			}
		}
		t, e = d.Token()
		if e != nil || t != json.Delim('}') {
			return ErrInput
		}
	case json.Delim('['):
		for d.More() {
			if e := uniqueJSON(d); e != nil {
				return e
			}
		}
		t, e = d.Token()
		if e != nil || t != json.Delim(']') {
			return ErrInput
		}
	}
	return nil
}
func decodeIntake(r io.Reader) (IntakeInput, error) {
	var in IntakeInput
	raw, e := io.ReadAll(io.LimitReader(r, 1048577))
	if e != nil || len(raw) > 1048576 || !utf8.Valid(raw) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if uniqueJSON(d) != nil || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		return in, ErrInput
	}
	// Every wire field is explicit, including nullable grade/battery and empty radios.
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || len(top) != 3 {
		return in, ErrInput
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(top["devices"], &rows) != nil {
		return in, ErrInput
	}
	for _, row := range rows {
		if len(row) != 13 {
			return in, ErrInput
		}
		for k, v := range row {
			if bytes.Equal(v, []byte("null")) && k != "grade" && k != "battery_health" {
				return in, ErrInput
			}
		}
	}
	return in, validateIntake(in)
}
func (h Handler) IntakeContextHTTP(c *gin.Context) { h.intakeHTTP(c, false) }
func (h Handler) IntakeHTTP(c *gin.Context)        { h.intakeHTTP(c, true) }
func (h Handler) intakeHTTP(c *gin.Context, write bool) {
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	productID, variantID := c.Param("id"), c.Param("variant_id")
	if !write {
		out, e := h.ReadIntake(ctx, actor, productID, variantID)
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
	in, e := decodeIntake(c.Request.Body)
	if e != nil {
		trackingFail(c, e)
		return
	}
	out, e := h.Intake(ctx, actor, productID, variantID, in)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, out)
}
