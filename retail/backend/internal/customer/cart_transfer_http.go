package customer

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/gin-gonic/gin"
)

func (h *HTTPHandler) TransferCart(c *gin.Context) {
	input, ok := h.input(c, "operation_id", "version")
	if !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
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
	out, err := h.store.TransferCart(ctx, token, possession, h.storeID, cart.CustomerTransferCommand{OperationID: input["operation_id"], Version: input["version"]}, h.manualProviders, paymentOptions...)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, out)
}
