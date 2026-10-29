package customer

import (
	"context"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

func (h *HTTPHandler) ListOrders(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	q := c.Request.URL.Query()
	offset := 0
	if c.Request.URL.ForceQuery || len(q) > 1 || len(q) > 0 && len(q["offset"]) != 1 {
		h.fail(c, ErrInvalid)
		return
	}
	if len(q) > 0 {
		var e error
		offset, e = strconv.Atoi(q.Get("offset"))
		if e != nil || strconv.Itoa(offset) != q.Get("offset") {
			h.fail(c, ErrInvalid)
			return
		}
	}
	if c.Request.URL.RawQuery != "" && c.Request.URL.RawQuery != "offset="+strconv.Itoa(offset) {
		h.fail(c, ErrInvalid)
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.store.Orders(ctx, token, offset)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, out)
}
func (h *HTTPHandler) AttachOrder(c *gin.Context) {
	if _, ok := h.input(c); !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	name := "retail_cart_possession"
	if h.secure {
		name = "__Host-" + name
	}
	possession := ""
	count := 0
	for _, cookie := range c.Request.Cookies() {
		if cookie.Name == name {
			possession = cookie.Value
			count++
		}
	}
	if count != 1 || !tokenOK(possession) {
		h.fail(c, ErrUnauthorized)
		return
	}
	if e := h.store.AttachOrder(ctx, token, possession, c.Param("id")); e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, gin.H{"order_id": c.Param("id")})
}

func (h *HTTPHandler) OrderDetail(c *gin.Context) {
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
	out, e := h.store.Order(ctx, token, c.Param("id"))
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, out)
}
