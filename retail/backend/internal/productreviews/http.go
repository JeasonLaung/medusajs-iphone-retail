package productreviews

import (
	"context"
	"errors"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

func Failure(c *gin.Context, e error) {
	switch {
	case errors.Is(e, ErrInput):
		platform.Fail(c, 400, "review.invalid_request", "Invalid product review request")
	case errors.Is(e, ErrProduct):
		platform.Fail(c, 404, "review.product_unavailable", "Product review unavailable")
	case errors.Is(e, ErrOrder):
		platform.Fail(c, 404, "review.order_unavailable", "Order unavailable")
	case errors.Is(e, ErrIneligible):
		platform.Fail(c, 403, "review.order_ineligible", "Order is not eligible for this product review")
	case errors.Is(e, ErrDuplicate):
		platform.Fail(c, 409, "review.already_submitted", "A review has already been submitted for this product")
	case errors.Is(e, ErrConflict):
		platform.Fail(c, 409, "review.conflict", "Product review changed; refresh before continuing")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "auth.forbidden", "Permission required")
	default:
		platform.Fail(c, 503, "review.unavailable", "Product reviews unavailable")
	}
}
func JSONRequest(c *gin.Context) bool {
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		Failure(c, ErrInput)
		return false
	}
	types := c.Request.Header.Values("Content-Type")
	if len(types) != 1 {
		platform.Fail(c, 415, "review.invalid_content_type", "JSON required")
		return false
	}
	media, _, e := mime.ParseMediaType(types[0])
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "review.invalid_content_type", "JSON required")
		return false
	}
	return true
}
func (s Service) PublicHTTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	q, e := ParseListQuery(c.Request.URL.RawQuery, false)
	if e != nil || c.Request.URL.ForceQuery && c.Request.URL.RawQuery == "" {
		Failure(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := s.PublicPage(ctx, q)
	if e != nil {
		Failure(c, e)
		return
	}
	c.JSON(200, out)
}
func (s Service) AdminHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if c.Request.Method == "GET" {
		q, e := ParseListQuery(c.Request.URL.RawQuery, false)
		if e != nil || c.Request.URL.ForceQuery && c.Request.URL.RawQuery == "" {
			Failure(c, ErrInput)
			return
		}
		out, e := s.AdminPage(ctx, actor, q)
		if e != nil {
			Failure(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	if !JSONRequest(c) {
		return
	}
	in, e := DecodeModeration(c.Request.Body)
	if e != nil {
		Failure(c, e)
		return
	}
	out, e := s.Moderate(ctx, actor, c.Param("id"), in)
	if e != nil {
		Failure(c, e)
		return
	}
	c.JSON(200, gin.H{"review": out})
}
