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

func (h *HTTPHandler) InitialPayment(c *gin.Context) {
	if h == nil || h.initialPayment == nil {
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
	d := json.NewDecoder(c.Request.Body)
	var in InitialPaymentCommand
	err = contactObject(d, func(name string) error {
		if name != "version" {
			return ErrInput
		}
		return contactString(d, &in.Version)
	})
	if err == nil {
		if e := d.Decode(new(any)); e != io.EOF {
			if e == nil {
				err = ErrInput
			} else {
				err = e
			}
		}
	}
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
	if _, err := validateShipping(ShippingCommand{OperationID: in.OperationID, Version: in.Version, OptionID: "checkout"}); err != nil {
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
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
	if !h.allow(c, ctx, "checkout-possession", token) {
		return
	}
	result, err := StartInitialPayment(ctx, h.db, h.store, ref, in, *h.initialPayment)
	if err != nil {
		if err == ErrItemUnavailable || err == ErrItemStock {
			cartHTTPError(c, err)
		} else {
			cartHTTPError(c, sanitize(err))
		}
		return
	}
	c.JSON(200, result)
}

func (h *HTTPHandler) PaymentStatus(c *gin.Context) {
	if h == nil || h.initialPayment == nil {
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
	d := json.NewDecoder(c.Request.Body)
	err = contactObject(d, func(name string) error { return ErrInput })
	if err == nil {
		if e := d.Decode(new(any)); e != io.EOF {
			if e == nil {
				err = ErrInput
			} else {
				err = e
			}
		}
	}
	if err != nil {
		platform.InvalidJSON(c, err, "cart.invalid_input", "Invalid cart request")
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
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
	if !h.allow(c, ctx, "payment-status-possession", token) {
		return
	}
	result, err := ObserveInitialPayment(ctx, h.db, h.store, ref, *h.initialPayment)
	if err != nil {
		if err == ErrItemUnavailable || err == ErrItemStock {
			cartHTTPError(c, err)
		} else {
			cartHTTPError(c, sanitize(err))
		}
		return
	}
	c.JSON(200, result)
}
