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

func (h Handler) VariantEditHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, errEditInput):
			platform.Fail(c, 400, "catalog.invalid_variant", "Invalid SKU configuration")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current product permission required")
		case errors.Is(e, ErrTrackedInventory):
			platform.Fail(c, 409, "catalog.tracked_inventory", "A device-tracked SKU must retain its inventory identity")
		case errors.Is(e, ErrInventoryCodeConflict):
			platform.Fail(c, 409, "catalog.inventory_code_conflict", "Inventory SKU already exists; its stock was not reassigned")
		case errors.Is(e, ErrVariantCodeConflict):
			platform.Fail(c, 409, "catalog.variant_code_conflict", "SKU, barcode, EAN or UPC is already in use")
		case errors.Is(e, ErrEditConflict):
			platform.Fail(c, 409, "catalog.variant_conflict", "SKU configuration changed or combination already exists; reload before saving")
		case errors.Is(e, gorm.ErrRecordNotFound):
			platform.Fail(c, 404, "catalog.not_found", "Product or SKU not found")
		default:
			platform.Fail(c, 503, "catalog.unavailable", "SKU editing unavailable; retry the same operation")
		}
	}
	if !validProductID(c.Param("id")) || !validProductID(c.Param("variant_id")) || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		fail(errEditInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if c.Request.Method == http.MethodGet {
		out, e := h.ReadVariantEdit(ctx, c.Param("id"), c.Param("variant_id"), actor)
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
	in, e := decodeVariantEdit(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "catalog.invalid_variant", "Invalid SKU configuration")
		return
	}
	out, e := h.EditVariant(ctx, c.Param("id"), c.Param("variant_id"), actor, in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}
