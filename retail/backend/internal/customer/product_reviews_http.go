package customer

import (
	"context"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/productreviews"
	"github.com/gin-gonic/gin"
)

func (h *HTTPHandler) productReviewFailure(c *gin.Context, e error) {
	if errors.Is(e, ErrUnauthorized) || errors.Is(e, ErrUnavailable) {
		h.fail(c, e)
		return
	}
	productreviews.Failure(c, e)
}
func (h *HTTPHandler) SubmitProductReview(c *gin.Context) {
	if !h.originOK(c, true) || !productreviews.JSONRequest(c) {
		return
	}
	in, e := productreviews.DecodeSubmission(c.Request.Body)
	if e != nil {
		productreviews.Failure(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	service := productreviews.Service{DB: h.store.db, StoreID: h.storeID}
	out, e := h.store.SubmitProductReview(ctx, token, &service, in)
	if e != nil {
		h.productReviewFailure(c, e)
		return
	}
	c.JSON(201, gin.H{"review": out})
}
func (h *HTTPHandler) ProductReviewPurchases(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	q, e := productreviews.ParseListQuery(c.Request.URL.RawQuery, true)
	if e != nil {
		productreviews.Failure(c, e)
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	service := productreviews.Service{DB: h.store.db, StoreID: h.storeID}
	out, e := h.store.ProductReviewPurchases(ctx, token, &service, q)
	if e != nil {
		h.productReviewFailure(c, e)
		return
	}
	c.JSON(200, out)
}
func (h *HTTPHandler) OrderProductReviews(c *gin.Context) {
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
	service := productreviews.Service{DB: h.store.db, StoreID: h.storeID}
	out, e := h.store.OrderProductReviews(ctx, token, &service, c.Param("id"))
	if e != nil {
		h.productReviewFailure(c, e)
		return
	}
	c.JSON(200, out)
}
