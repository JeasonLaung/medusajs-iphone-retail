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

func decodeSource(r io.Reader) (SourceInput, error) {
	var in SourceInput
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
		if e != nil || !ok || (k != "operation_id" && k != "expected_revision" && k != "name" && k != "type") {
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
	if _, e = d.Token(); e != nil || len(fields) != 4 {
		return in, ErrInput
	}
	if d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	in = SourceInput{fields["operation_id"], fields["expected_revision"], fields["name"], fields["type"]}
	return in, validateSource(in)
}

func (h SourceHandler) DirectoryHTTP(c *gin.Context) { h.sourceHTTP(c, false) }
func (h SourceHandler) CreateHTTP(c *gin.Context)    { h.sourceHTTP(c, true) }
func (h SourceHandler) sourceHTTP(c *gin.Context, write bool) {
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
	if !write {
		out, e := h.Read(ctx, actor)
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
	in, e := decodeSource(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "device.invalid_input", "Invalid device source request")
		return
	}
	out, e := h.Save(ctx, actor, in)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, out)
}
