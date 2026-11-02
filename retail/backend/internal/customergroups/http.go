package customergroups

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/url"
	"strconv"
	"unicode/utf8"
)

func decode(r io.Reader) (Command, error) {
	var out Command
	raw, e := io.ReadAll(io.LimitReader(r, 65537))
	if e != nil || len(raw) > 65536 || !utf8.Valid(raw) {
		return out, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return out, ErrInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		key, e := d.Token()
		name, ok := key.(string)
		if e != nil || !ok || fields[name] != nil {
			return out, ErrInput
		}
		switch name {
		case "operation_id", "expected_revision", "kind", "group_id", "name", "customer_ids":
		default:
			return out, ErrInput
		}
		var v json.RawMessage
		if d.Decode(&v) != nil || string(v) == "null" {
			return out, ErrInput
		}
		fields[name] = v
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != 6 || d.Decode(new(any)) != io.EOF {
		return out, ErrInput
	}
	encoded, _ := json.Marshal(fields)
	if json.Unmarshal(encoded, &out) != nil || !Valid(out) {
		return out, ErrInput
	}
	return out, nil
}
func fail(c *gin.Context, e error) {
	status := 503
	if errors.Is(e, ErrInput) {
		status = 400
	} else if errors.Is(e, ErrConflict) {
		status = 409
	} else if errors.Is(e, authorization.ErrDenied) {
		status = 403
	}
	platform.Fail(c, status, "customer_groups.unavailable", "Customer groups unavailable")
}
func (h Handler) HTTP(c *gin.Context) {
	actor, ok := c.MustGet("admin_user").(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	if c.Request.Method == "GET" {
		if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
			fail(c, ErrInput)
			return
		}
		out, e := h.Read(c.Request.Context(), actor)
		if e != nil {
			fail(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		fail(c, ErrInput)
		return
	}
	kind, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || kind != "application/json" {
		fail(c, ErrInput)
		return
	}
	in, e := decode(c.Request.Body)
	if e != nil {
		fail(c, e)
		return
	}
	out, e := h.Change(c.Request.Context(), actor, in)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
func (h Handler) AccountsHTTP(c *gin.Context) {
	actor, ok := c.MustGet("admin_user").(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	q, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil || c.Request.URL.ForceQuery && c.Request.URL.RawQuery == "" {
		fail(c, ErrInput)
		return
	}
	for k, v := range q {
		if (k != "q" && k != "group_id" && k != "offset") || len(v) != 1 {
			fail(c, ErrInput)
			return
		}
	}
	offset := 0
	if q.Has("offset") {
		offset, e = strconv.Atoi(q.Get("offset"))
		if e != nil || strconv.Itoa(offset) != q.Get("offset") {
			fail(c, ErrInput)
			return
		}
	}
	out, e := h.Accounts(c.Request.Context(), actor, q.Get("q"), q.Get("group_id"), offset)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
