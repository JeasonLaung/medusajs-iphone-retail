package cart

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
)

func decodeShipping(reader io.Reader) (ShippingCommand, error) {
	d := json.NewDecoder(reader)
	var in ShippingCommand
	err := contactObject(d, func(name string) error {
		switch name {
		case "version":
			return contactString(d, &in.Version)
		case "option_id":
			return contactString(d, &in.OptionID)
		default:
			return ErrInput
		}
	})
	if err != nil {
		return ShippingCommand{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		if err != nil {
			return ShippingCommand{}, err
		}
		return ShippingCommand{}, ErrInput
	}
	return in, nil
}
func (h *HTTPHandler) Shipping(c *gin.Context) {
	if c.Request.URL.RawQuery != "" || !idPattern.MatchString(c.Param("id")) {
		cartHTTPError(c, ErrInput)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "cart.content_type", "JSON required")
		return
	}
	in, err := decodeShipping(c.Request.Body)
	if err != nil {
		platform.InvalidJSON(c, err, "cart.invalid_input", "Invalid cart request")
		return
	}
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		cartHTTPError(c, ErrInput)
		return
	}
	in.OperationID = keys[0]
	if _, err := validateShipping(in); err != nil {
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
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
	if !h.allow(c, ctx, "shipping-possession", token) {
		return
	}
	result, err := SaveShipping(ctx, h.db, h.store, ref, in, h.manualProviders)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	c.JSON(200, result)
}
func (h *HTTPHandler) Delivery(c *gin.Context) { h.detail(c, "delivery", false) }
