package customer

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"time"
)

func (h *HTTPHandler) UpdateBuyback(c *gin.Context) {
	if !h.originOK(c, true) {
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "customer.content_type", "JSON required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16384))
	if e != nil {
		platform.InvalidJSON(c, e, "buyback.invalid_input", "Invalid buyback command")
		return
	}
	in, e := buyback.ParseCustomerCommand(raw)
	if e != nil {
		platform.Fail(c, 400, "buyback.invalid_input", "Invalid buyback command")
		return
	}
	out, e := h.store.UpdateBuyback(ctx, token, h.storeID, c.Param("id"), in)
	if e != nil {
		switch {
		case errors.Is(e, buyback.ErrMissing):
			platform.Fail(c, 404, "BUYBACK_NOT_FOUND", "Buyback request not found")
		case errors.Is(e, buyback.ErrCommandState):
			platform.Fail(c, 409, "BUYBACK_INVALID_STATE", "Command unavailable in current state")
		case errors.Is(e, buyback.ErrCommandIdentity):
			platform.Fail(c, 409, "BUYBACK_IDENTITY_REQUIRED", "Complete device identities required")
		case errors.Is(e, buyback.ErrCommandPhotos):
			platform.Fail(c, 409, "BUYBACK_PHOTOS_REQUIRED", "Complete photos required")
		case errors.Is(e, buyback.ErrCommandOffer):
			platform.Fail(c, 409, "BUYBACK_OFFER_CHANGED", "Offer changed; refresh the request")
		case errors.Is(e, buyback.ErrInput):
			platform.Fail(c, 400, "buyback.invalid_input", "Invalid buyback command")
		default:
			h.fail(c, e)
		}
		return
	}
	c.JSON(200, gin.H{"request": out})
}
