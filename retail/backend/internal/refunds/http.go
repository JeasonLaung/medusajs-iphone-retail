package refunds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"time"
	"unicode/utf8"
)

// Options is explicit process configuration, independent of the SQL gate.
// Constructing a handler does not dispatch work, migrate or enable any gate.
type Options struct {
	Enabled  bool
	StoreID  string
	Ring     *protected.Ring
	Adapters *payments.CreateAdapters
}
type Handler struct {
	Service Service
	Enabled bool
}

func NewHTTP(dbService Service, enabled bool) Handler {
	return Handler{Service: dbService, Enabled: enabled}
}

// Flat string-only objects reject duplicate/unknown/null/nested values before
// dispatch. IDs and amounts have further command validation, never float input.
func decodeStrings(r io.Reader, required, optional []string) (map[string]string, error) {
	raw, e := io.ReadAll(io.LimitReader(r, 4097))
	if e != nil || len(raw) > 4096 || !utf8.Valid(raw) {
		return nil, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	first, e := d.Token()
	if e != nil || first != json.Delim('{') {
		return nil, ErrInput
	}
	allowed := map[string]bool{}
	for _, k := range required {
		allowed[k] = true
	}
	for _, k := range optional {
		allowed[k] = true
	}
	out := map[string]string{}
	for d.More() {
		k, e := d.Token()
		name, ok := k.(string)
		if e != nil || !ok || !allowed[name] {
			return nil, ErrInput
		}
		if _, exists := out[name]; exists {
			return nil, ErrInput
		}
		v, e := d.Token()
		value, ok := v.(string)
		if e != nil || !ok || value == "" {
			return nil, ErrInput
		}
		out[name] = value
	}
	end, e := d.Token()
	if e != nil || end != json.Delim('}') {
		return nil, ErrInput
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, ErrInput
	}
	for _, k := range required {
		if out[k] == "" {
			return nil, ErrInput
		}
	}
	return out, nil
}
func (h Handler) HTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var out any
	var err error
	status := 200
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		err = ErrInput
	} else if c.Request.Method == "GET" {
		var view Context
		view, err = h.Service.Read(ctx, actor, c.Param("id"), c.Param("request_id"))
		view.Enabled = view.Enabled && h.Enabled
		out = view
	} else if !h.Enabled {
		err = ErrUnavailable
	} else {
		media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if e != nil || media != "application/json" {
			platform.Fail(c, 415, "refunds.content_type", "JSON required")
			return
		}
		if original := c.Param("operation_id"); original != "" {
			var fields map[string]string
			fields, err = decodeStrings(c.Request.Body, []string{"operation_id"}, nil)
			if err == nil {
				out, err = h.Service.Cancel(ctx, actor, c.Param("id"), c.Param("request_id"), CancelInput{OperationID: fields["operation_id"], RefundOperationID: original})
			}
		} else {
			var fields map[string]string
			fields, err = decodeStrings(c.Request.Body, []string{"operation_id", "payment_id", "amount"}, []string{"previous_operation_id"})
			if err == nil {
				out, err = h.Service.Prepare(ctx, actor, Input{OperationID: fields["operation_id"], OrderID: c.Param("id"), RequestID: c.Param("request_id"), PaymentID: fields["payment_id"], Amount: fields["amount"], PreviousOperationID: fields["previous_operation_id"]})
				status = 202
			}
		}
	}
	if err == nil {
		c.JSON(status, out)
		return
	}
	status = 503
	switch {
	case errors.Is(err, ErrInput):
		status = 400
	case errors.Is(err, authorization.ErrDenied):
		status = 403
	case errors.Is(err, ErrConflict):
		status = 409
	}
	platform.Fail(c, status, "refunds.unavailable", "Refund unavailable; reload the original request and verify permission")
}
