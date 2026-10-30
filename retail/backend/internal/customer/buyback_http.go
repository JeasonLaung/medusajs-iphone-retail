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
	"strconv"
	"time"
)

func (h *HTTPHandler) CreateBuyback(c *gin.Context) {
	if !h.originOK(c, true) {
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "customer.content_type", "JSON required")
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16384))
	if e != nil {
		platform.InvalidJSON(c, e, "buyback.invalid_input", "Invalid buyback request")
		return
	}
	in, e := buyback.ParseSubmission(raw)
	if e != nil {
		platform.Fail(c, 400, "buyback.invalid_input", "Invalid buyback request")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	out, e := h.store.CreateBuyback(ctx, token, h.storeID, in)
	if e != nil {
		switch {
		case errors.Is(e, buyback.ErrConflict):
			platform.Fail(c, 409, "buyback.submission_conflict", "Submission already used with different details")
		case errors.Is(e, buyback.ErrInput):
			platform.Fail(c, 400, "buyback.invalid_input", "Invalid buyback request")
		default:
			h.fail(c, e)
		}
		return
	}
	c.JSON(201, gin.H{"request": out})
}

func (h *HTTPHandler) BuybackDetail(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.store.Buyback(ctx, token, h.storeID, c.Param("id"))
	if e != nil {
		if errors.Is(e, buyback.ErrMissing) {
			platform.Fail(c, 404, "buyback.not_found", "Buyback request not found")
		} else {
			h.fail(c, e)
		}
		return
	}
	c.JSON(200, gin.H{"request": out})
}

func (h *HTTPHandler) ListBuybacks(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	offset := 0
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		if !noticeOffset.MatchString(c.Request.URL.RawQuery) {
			h.fail(c, ErrInvalid)
			return
		}
		offset, _ = strconv.Atoi(c.Query("offset"))
		if offset > 100000 {
			h.fail(c, ErrInvalid)
			return
		}
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.store.Buybacks(ctx, token, h.storeID, offset)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, out)
}
