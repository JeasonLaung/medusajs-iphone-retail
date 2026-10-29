package notificationmail

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

func (s Store) Register(group *gin.RouterGroup) {
	group.GET("/notification-mail/accounts", s.list)
	group.POST("/notification-mail/accounts", s.save)
}
func actor(c *gin.Context) (identity.User, bool) {
	raw, ok := c.Get("admin_user")
	user, valid := raw.(identity.User)
	if !ok || !valid {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return identity.User{}, false
	}
	return user, true
}
func fail(c *gin.Context, e error) {
	switch {
	case errors.Is(e, ErrInput):
		platform.Fail(c, 400, "mail.invalid_input", "Invalid mailbox configuration")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Action not permitted")
	case errors.Is(e, ErrConflict):
		platform.Fail(c, 409, "mail.conflict", "Mailbox configuration or operation changed; reload before editing")
	case errors.Is(e, ErrHeld):
		platform.Fail(c, 409, "mail.held", "Historical mailbox requires reconciliation")
	default:
		platform.Fail(c, 503, "mail.unavailable", "Mailbox configuration unavailable")
	}
}
func (s Store) list(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	user, ok := actor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := s.Directory(ctx, user)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
func decodeCommand(raw []byte) (Command, error) {
	var in Command
	if !utf8.Valid(raw) {
		return in, ErrInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, e := decoder.Token()
	if e != nil || start != json.Delim('{') {
		return in, ErrInput
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, e := decoder.Token()
		key, ok := token.(string)
		if e != nil || !ok || fields[key] != nil {
			return in, ErrInput
		}
		switch key {
		case "operation_id", "expected_revision", "id", "name", "email", "reply_to", "api_key", "webhook_secret", "enabled", "registration":
		default:
			return in, ErrInput
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return in, ErrInput
		}
		trimmed := bytes.TrimSpace(value)
		if key == "enabled" || key == "registration" {
			if !bytes.Equal(trimmed, []byte("true")) && !bytes.Equal(trimmed, []byte("false")) {
				return in, ErrInput
			}
		} else if len(trimmed) == 0 || trimmed[0] != '"' {
			return in, ErrInput
		}
		fields[key] = value
	}
	if _, e = decoder.Token(); e != nil || len(fields) != 10 || decoder.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	if json.Unmarshal(raw, &in) != nil {
		return in, ErrInput
	}
	return normalize(in)
}
func (s Store) save(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "unsupported_media_type", "JSON body required")
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 16385))
	if e != nil || len(raw) > 16384 {
		platform.Fail(c, 413, "request_too_large", "Mailbox request too large")
		return
	}
	defer clear(raw)
	in, e := decodeCommand(raw)
	if e != nil {
		fail(c, e)
		return
	}
	user, ok := actor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := s.Save(ctx, user, in)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
