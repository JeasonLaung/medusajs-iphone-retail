package cart

import (
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"time"
)

func decodeContext(reader io.Reader) (ContextCommand, error) {
	d := json.NewDecoder(reader)
	var in ContextCommand
	err := contactObject(d, func(name string) error {
		switch name {
		case "version":
			return contactString(d, &in.Version)
		case "region_id":
			return contactString(d, &in.RegionID)
		case "locale":
			return contactString(d, &in.Locale)
		default:
			return ErrInput
		}
	})
	if err != nil {
		return ContextCommand{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ContextCommand{}, ErrInput
	}
	return in, nil
}
func (h *HTTPHandler) Context(c *gin.Context) {
	h.contextCommand(c, "draft")
}

func (h *HTTPHandler) ContextPayment(c *gin.Context) {
	h.contextCommand(c, "payment")
}

func (h *HTTPHandler) ContextPaymentRestore(c *gin.Context) {
	h.contextCommand(c, "restore")
}

func (h *HTTPHandler) contextCommand(c *gin.Context, mode string) {
	if mode != "draft" && (h == nil || h.initialPayment == nil || h.contextPaymentDeletes == nil) {
		cartHTTPError(c, ErrConfiguration)
		return
	}
	if c.Request.URL.RawQuery != "" || !idPattern.MatchString(c.Param("id")) {
		cartHTTPError(c, ErrInput)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "cart.content_type", "JSON required")
		return
	}
	command, err := decodeContext(c.Request.Body)
	if err != nil {
		platform.InvalidJSON(c, err, "cart.invalid_input", "Invalid cart request")
		return
	}
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		cartHTTPError(c, ErrInput)
		return
	}
	command.OperationID = keys[0]
	if _, err := validateContext(command); err != nil {
		cartHTTPError(c, err)
		return
	}
	if !h.ready(c) || !h.originOK(c, true) {
		return
	}
	token, valid, _ := h.cookie(c, "possession")
	if !valid {
		cartHTTPError(c, sessionstore.ErrMissing)
		return
	}
	timeout := 10 * time.Second
	if mode != "draft" {
		timeout = 45 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	ref, err := h.sessions.AuthorizeCartWrite(ctx, token, csrf(c))
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	if ref.StoreID != h.store || ref.CartID != c.Param("id") {
		cartHTTPError(c, ErrDenied)
		return
	}
	if !h.allow(c, ctx, "context-possession", token) {
		return
	}
	if mode != "draft" {
		var progress ContextPaymentProgress
		if mode == "restore" {
			progress, err = ContinueContextPaymentRestore(ctx, h.db, h.store, ref, command, *h.initialPayment)
		} else {
			progress, err = ContinueContextPayment(ctx, h.db, h.store, ref, command, *h.initialPayment, h.contextPaymentDeletes)
		}
		if err != nil {
			cartHTTPError(c, sanitize(err))
			return
		}
		c.JSON(200, progress)
		return
	}
	result, err := SaveContext(ctx, h.db, h.store, ref, command, h.manualProviders)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	// Receipt only; do not expose directory, price or ownership internals.
	c.JSON(200, result)
}
