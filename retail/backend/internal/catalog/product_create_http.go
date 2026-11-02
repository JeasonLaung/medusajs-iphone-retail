package catalog

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

func (h Handler) ProductCreateHTTP(c *gin.Context) {
	value, ok := c.Get("admin_user")
	actor, typed := value.(identity.User)
	if !ok || !typed {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, errEditInput):
			platform.Fail(c, 400, "catalog.invalid_product", "Invalid product configuration")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current product permission required")
		case errors.Is(e, ErrEditConflict):
			platform.Fail(c, 409, "catalog.create_conflict", "Handle or operation already used; review before creating")
		default:
			platform.Fail(c, 503, "catalog.unavailable", "Product creation unavailable; retry the same operation")
		}
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		fail(errEditInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if c.Request.Method == "GET" {
		out, e := h.ReadProductCreate(ctx, actor)
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
	in, e := decodeProductCreate(c.Request.Body)
	if e != nil {
		fail(e)
		return
	}
	out, e := h.CreateProduct(ctx, actor, in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}
