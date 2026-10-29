package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
)

func (h *HTTPHandler) OrderConfirmation(c *gin.Context) {
	if h == nil || !h.ready(c) {
		return
	}
	c.Header("Referrer-Policy", "no-referrer")
	if c.Request.URL.RawQuery != "" || !idPattern.MatchString(c.Param("id")) {
		cartHTTPError(c, ErrInput)
		return
	}
	if !h.originOK(c, false) {
		return
	}
	token, valid, _ := h.cookie(c, "possession")
	if !valid {
		cartHTTPError(c, sessionstore.ErrMissing)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	ref, err := h.sessions.ResolveCart(ctx, token)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	if ref.StoreID != h.store {
		cartHTTPError(c, ErrDenied)
		return
	}
	if !h.allow(c, ctx, "confirmation-possession", token) {
		return
	}
	order, err := ReadConfirmation(ctx, h.db, h.store, ref, c.Param("id"))
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	c.JSON(200, order)
}
