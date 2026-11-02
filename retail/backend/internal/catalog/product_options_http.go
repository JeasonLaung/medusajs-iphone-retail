package catalog

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mime"
	"net/http"
	"time"
)

func (h Handler) ProductOptionsHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, errEditInput):
			platform.Fail(c, 400, "catalog.invalid_options", "Invalid product options")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current product permission required")
		case errors.Is(e, ErrOptionInUse):
			platform.Fail(c, 409, "catalog.option_in_use", "An active SKU uses this option or value")
		case errors.Is(e, ErrEditConflict):
			platform.Fail(c, 409, "catalog.option_conflict", "Options changed; reload before saving")
		case errors.Is(e, gorm.ErrRecordNotFound):
			platform.Fail(c, 404, "catalog.not_found", "Product not found")
		default:
			platform.Fail(c, 503, "catalog.unavailable", "Options unavailable; retry the same operation")
		}
	}
	if !validProductID(c.Param("id")) || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		fail(errEditInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if c.Request.Method == http.MethodGet {
		out, e := h.ReadProductOptions(ctx, c.Param("id"), actor)
		if e != nil {
			fail(e)
			return
		}
		c.JSON(200, out)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "catalog.content_type", "JSON required")
		return
	}
	in, e := decodeProductOptions(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "catalog.invalid_options", "Invalid product options")
		return
	}
	out, e := h.EditProductOptions(ctx, c.Param("id"), actor, in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}
