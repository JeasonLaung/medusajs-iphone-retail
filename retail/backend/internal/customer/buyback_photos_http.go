package customer

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

func (h *HTTPHandler) buybackPhotoFail(c *gin.Context, e error) {
	switch {
	case errors.Is(e, buyback.ErrMissing), errors.Is(e, buyback.ErrPhotoMissing):
		platform.Fail(c, 404, "BUYBACK_PHOTO_NOT_FOUND", "Buyback photo not found")
	case errors.Is(e, buyback.ErrPhotoState):
		platform.Fail(c, 409, "BUYBACK_INVALID_STATE", "Photo upload unavailable in current state")
	case errors.Is(e, buyback.ErrPhotoLimit):
		platform.Fail(c, 409, "BUYBACK_TOO_MANY_PHOTOS", "Photo limit reached")
	case errors.Is(e, buyback.ErrInput):
		platform.Fail(c, 400, "BUYBACK_INVALID_PHOTO", "Invalid photo")
	default:
		h.fail(c, e)
	}
}
func (h *HTTPHandler) UploadBuybackPhoto(c *gin.Context) {
	if !h.originOK(c, true) {
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "customer.content_type", "JSON required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, buyback.MaxPhotoBodyBytes))
	if e != nil {
		platform.InvalidJSON(c, e, "BUYBACK_INVALID_PHOTO", "Invalid photo")
		return
	}
	in, e := buyback.ParsePhotoUpload(raw)
	if e != nil {
		h.buybackPhotoFail(c, e)
		return
	}
	out, e := h.store.UploadBuybackPhoto(ctx, token, h.storeID, c.Param("id"), in)
	if e != nil {
		h.buybackPhotoFail(c, e)
		return
	}
	c.JSON(201, gin.H{"photo": out})
}
func (h *HTTPHandler) BuybackPhoto(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	out, e := h.store.BuybackPhoto(ctx, token, h.storeID, c.Param("id"), c.Param("photo_id"))
	if e != nil {
		h.buybackPhotoFail(c, e)
		return
	}
	c.JSON(200, out)
}
