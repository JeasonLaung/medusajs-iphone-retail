package warehouse

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

// Require an exact object at both levels, rejecting duplicate/unknown/missing
// fields before unmarshalling. All values at the address level must be strings.
func object(d *json.Decoder, keys map[string]bool, nested bool) (map[string]json.RawMessage, error) {
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return nil, ErrInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		t, e = d.Token()
		k, ok := t.(string)
		if e != nil || !ok || !keys[k] || fields[k] != nil {
			return nil, ErrInput
		}
		var raw json.RawMessage
		if e = d.Decode(&raw); e != nil {
			return nil, e
		}
		if nested && k == "address" {
			sub := json.NewDecoder(bytes.NewReader(raw))
			if _, e = object(sub, map[string]bool{"address_1": true, "address_2": true, "company": true, "city": true, "country_code": true, "phone": true, "province": true, "postal_code": true}, false); e != nil {
				return nil, e
			}
		} else {
			var v string
			if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &v) != nil {
				return nil, ErrInput
			}
		}
		fields[k] = raw
	}
	if _, e = d.Token(); e != nil {
		return nil, e
	}
	if len(fields) != len(keys) {
		return nil, ErrInput
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return nil, ErrInput
	}
	return fields, nil
}
func decode(reader io.Reader) (Command, error) {
	rawInput, e := io.ReadAll(reader)
	if e != nil {
		return Command{}, e
	}
	if !utf8.Valid(rawInput) {
		return Command{}, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(rawInput))
	fields, e := object(d, map[string]bool{"operation_id": true, "expected_revision": true, "id": true, "name": true, "address": true}, true)
	if e != nil {
		return Command{}, e
	}
	raw, _ := json.Marshal(fields)
	var in Command
	if json.Unmarshal(raw, &in) != nil || validate(in) != nil {
		return Command{}, ErrInput
	}
	return in, nil
}
func fail(c *gin.Context, e error) {
	switch {
	case errors.Is(e, ErrInput):
		platform.Fail(c, 400, "warehouse.invalid_input", "Invalid warehouse request")
	case errors.Is(e, ErrConflict):
		platform.Fail(c, 409, "warehouse.conflict", "Warehouses changed; reload before editing")
	case errors.Is(e, ErrNotFound):
		platform.Fail(c, 404, "warehouse.not_found", "Warehouse unavailable")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Action not permitted")
	default:
		platform.Fail(c, 503, "warehouse.unavailable", "Warehouse management unavailable")
	}
}
func (h Handler) Directory(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.Read(ctx, actor)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
func (h Handler) Command(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "warehouse.content_type", "JSON required")
		return
	}
	in, e := decode(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "warehouse.invalid_input", "Invalid warehouse request")
		return
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.Save(ctx, actor, in)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
