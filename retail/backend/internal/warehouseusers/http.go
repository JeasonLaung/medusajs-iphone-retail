package warehouseusers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

func decode(raw []byte) (Command, error) {
	if !utf8.Valid(raw) || len(raw) > 262144 {
		return Command{}, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	t, e := d.Token()
	if e != nil || t != json.Delim('{') {
		return Command{}, ErrInput
	}
	keys := map[string]string{"operation_id": "string", "expected_revision": "string", "id": "string", "name": "string", "email": "string", "password": "string", "enabled": "bool", "mode": "string", "locations": "array", "actions": "array"}
	seen := map[string]bool{}
	for d.More() {
		k, e := d.Token()
		key, ok := k.(string)
		if e != nil || !ok || keys[key] == "" || seen[key] {
			return Command{}, ErrInput
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return Command{}, ErrInput
		}
		switch keys[key] {
		case "string":
			if len(value) == 0 || value[0] != '"' {
				return Command{}, ErrInput
			}
			var s string
			if json.Unmarshal(value, &s) != nil {
				return Command{}, ErrInput
			}
		case "bool":
			if string(value) != "true" && string(value) != "false" {
				return Command{}, ErrInput
			}
		case "array":
			var a []json.RawMessage
			if len(value) == 0 || value[0] != '[' || json.Unmarshal(value, &a) != nil || len(a) > 1000 {
				return Command{}, ErrInput
			}
			for _, item := range a {
				if len(item) == 0 || item[0] != '"' {
					return Command{}, ErrInput
				}
			}
		}
	}
	if _, e = d.Token(); e != nil || len(seen) != len(keys) {
		return Command{}, ErrInput
	}
	if d.Decode(new(any)) != io.EOF {
		return Command{}, ErrInput
	}
	var c Command
	if json.Unmarshal(raw, &c) != nil {
		return c, ErrInput
	}
	return normalize(c)
}
func fail(c *gin.Context, e error) {
	status, code := 503, "warehouse_users.unavailable"
	switch {
	case errors.Is(e, ErrInput):
		status, code = 400, "warehouse_users.invalid_input"
	case errors.Is(e, ErrConflict):
		status, code = 409, "warehouse_users.conflict"
	case errors.Is(e, ErrMissing):
		status, code = 404, "warehouse_users.not_found"
	case errors.Is(e, authorization.ErrDenied):
		status, code = 403, "authorization.denied"
	}
	platform.Fail(c, status, code, "Warehouse user request could not be completed")
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	d, e := h.Read(ctx, actor)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, d)
}
func (h Handler) Command(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "warehouse_users.invalid_media", "JSON required")
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 262145))
	if e != nil {
		fail(c, ErrInput)
		return
	}
	in, e := decode(raw)
	if e != nil {
		fail(c, e)
		return
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	r, e := h.Save(ctx, actor, in)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, r)
}
