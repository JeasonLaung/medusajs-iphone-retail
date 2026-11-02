package customer

import (
	"context"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/gin-gonic/gin"
)

func (h *HTTPHandler) TransferCart(c *gin.Context) {
	h.transferCart(c, false)
}
func (h *HTTPHandler) RestoreCartTransfer(c *gin.Context) {
	h.transferCart(c, true)
}
func (h *HTTPHandler) transferCart(c *gin.Context, restore bool) {
	input, ok := h.input(c, "operation_id", "version")
	if !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	name := "retail_cart_possession"
	if h.secure {
		name = "__Host-" + name
	}
	possession, count := "", 0
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
	var paymentOptions []cart.InitialPaymentOptions
	if h.initialPayment != nil {
		paymentOptions = append(paymentOptions, *h.initialPayment)
	}
	var out cart.ContactResult
	err := ErrCartConflict
	if !restore {
		out, err = h.store.TransferCart(ctx, token, possession, h.storeID, cart.CustomerTransferCommand{OperationID: input["operation_id"], Version: input["version"]}, h.manualProviders, paymentOptions...)
	}
	coordinatedReplay := false
	if err == nil && out.Replayed {
		var n int64
		if h.store.db.Table("cart_context_operations").Where("cart_id=? AND operation_id=?", out.CartID, input["operation_id"]).Count(&n).Error != nil {
			h.fail(c, ErrUnavailable)
			return
		}
		coordinatedReplay = n == 1
	}
	if (restore || coordinatedReplay || errors.Is(err, ErrCartConflict)) && h.initialPayment != nil && h.contextPaymentDeletes != nil {
		progress, e := h.store.ContinuePaymentCart(ctx, token, possession, h.storeID, cart.CustomerTransferCommand{OperationID: input["operation_id"], Version: input["version"]}, *h.initialPayment, h.contextPaymentDeletes, restore)
		if e != nil {
			// A dispatched effect can already be durable. Retain the client's
			// operation identity on failures rather than presenting definitive 409.
			h.fail(c, ErrUnavailable)
			return
		}
		c.JSON(200, progress)
		return
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, out)
}
