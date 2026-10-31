package customer

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

func (h *HTTPHandler) paymentFailure(c *gin.Context, err error) {
	if errors.Is(err, ErrPaymentConflict) {
		platform.Fail(c, 409, "customer.payment_conflict", "Order payment changed; refresh before continuing")
		return
	}
	h.fail(c, err)
}
func (h *HTTPHandler) PrepareOrderPayment(c *gin.Context) {
	if !h.originOK(c, true) {
		return
	}
	fields, ok := h.input(c, "operation_id", "previous_operation_id", "expected_version", "collection_id", "amount", "currency_code")
	if !ok {
		return
	}
	version, e := strconv.Atoi(fields["expected_version"])
	if e != nil || version < 1 || strconv.Itoa(version) != fields["expected_version"] {
		h.fail(c, ErrInvalid)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	out, e := h.store.PrepareOrderPayment(ctx, token, h.orderPayments, orderpayments.Input{OperationID: fields["operation_id"], PreviousOperationID: fields["previous_operation_id"], OrderID: c.Param("id"), ExpectedVersion: version, CollectionID: fields["collection_id"], Amount: fields["amount"], CurrencyCode: fields["currency_code"]})
	if e != nil {
		h.paymentFailure(c, e)
		return
	}
	c.JSON(202, gin.H{"payment": out})
}
func (h *HTTPHandler) ReadOrderPayment(c *gin.Context) {
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
	out, e := h.store.ReadOrderPayment(ctx, token, h.orderPayments, c.Param("id"), c.Param("operation_id"))
	if e != nil {
		h.paymentFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"payment": out})
}
func (h *HTTPHandler) CancelOrderPayment(c *gin.Context) {
	if !h.originOK(c, true) {
		return
	}
	fields, ok := h.input(c, "operation_id")
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	out, e := h.store.CancelOrderPayment(ctx, token, h.orderPayments, c.Param("id"), orderpayments.CancelInput{OperationID: fields["operation_id"], PaymentOperationID: c.Param("operation_id")})
	if e != nil {
		h.paymentFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"payment": out})
}

func (h *HTTPHandler) OrderPaymentContext(c *gin.Context) {
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
	out, e := h.store.OrderPaymentContext(ctx, token, h.orderPayments, c.Param("id"))
	if e != nil {
		h.paymentFailure(c, e)
		return
	}
	c.JSON(200, out)
}
