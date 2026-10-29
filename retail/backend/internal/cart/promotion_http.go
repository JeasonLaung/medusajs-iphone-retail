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

func decodePromotion(reader io.Reader) (PromotionCommand, error) {
	d := json.NewDecoder(reader)
	var in PromotionCommand
	err := contactObject(d, func(name string) error {
		switch name {
		case "version":
			return contactString(d, &in.Version)
		case "codes":
			token, err := d.Token()
			if err != nil || token != json.Delim('[') {
				return ErrInput
			}
			in.Codes = []string{}
			for d.More() {
				if len(in.Codes) >= 100 {
					return ErrInput
				}
				var code string
				if err := contactString(d, &code); err != nil {
					return err
				}
				in.Codes = append(in.Codes, code)
			}
			token, err = d.Token()
			if err != nil || token != json.Delim(']') {
				return ErrInput
			}
			return nil
		default:
			return ErrInput
		}
	})
	if err != nil {
		return PromotionCommand{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return PromotionCommand{}, ErrInput
	}
	return in, nil
}
func (h *HTTPHandler) Promotions(c *gin.Context) {
	if c.Request.URL.RawQuery != "" || !idPattern.MatchString(c.Param("id")) {
		cartHTTPError(c, ErrInput)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "cart.content_type", "JSON required")
		return
	}
	in, err := decodePromotion(c.Request.Body)
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
	if _, err := validatePromotion(in); err != nil {
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
	if !h.allow(c, ctx, "promotion-possession", token) {
		return
	}
	result, err := SavePromotions(ctx, h.db, h.store, ref, in, h.manualProviders)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	c.JSON(200, result)
}
func (h *HTTPHandler) AppliedPromotions(c *gin.Context) { h.detail(c, "promotions", false) }
