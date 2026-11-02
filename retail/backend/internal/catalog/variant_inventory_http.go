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
	"net/url"
	"strconv"
	"time"
)

func (h Handler) VariantInventoryLinksHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, errEditInput):
			platform.Fail(c, 400, "catalog.invalid_inventory_links", "Select inventory items and positive exact quantities")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current product and inventory permissions required")
		case errors.Is(e, ErrTrackedInventory):
			platform.Fail(c, 409, "catalog.tracked_inventory", "Device-tracked inventory must retain its unique SKU and quantity of one")
		case errors.Is(e, ErrEditConflict):
			platform.Fail(c, 409, "catalog.inventory_links_conflict", "Inventory links changed; reload before saving")
		case errors.Is(e, gorm.ErrRecordNotFound):
			platform.Fail(c, 404, "catalog.not_found", "Product or SKU not found")
		default:
			platform.Fail(c, 503, "catalog.unavailable", "Inventory links unavailable; retry the same operation")
		}
	}
	if !validProductID(c.Param("id")) || !validProductID(c.Param("variant_id")) {
		fail(errEditInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	p, v := c.Param("id"), c.Param("variant_id")
	if c.FullPath() == "/api/admin/products/:id/variants/:variant_id/inventory-items/choices" {
		values, e := url.ParseQuery(c.Request.URL.RawQuery)
		offset := 0
		if e != nil {
			fail(errEditInput)
			return
		}
		for k, vs := range values {
			if (k != "q" && k != "offset") || len(vs) != 1 {
				fail(errEditInput)
				return
			}
		}
		if raw, exists := values["offset"]; exists {
			offset, e = strconv.Atoi(raw[0])
			if e != nil || strconv.Itoa(offset) != raw[0] {
				fail(errEditInput)
				return
			}
		}
		out, e := h.InventoryLinkChoices(ctx, p, v, c.Query("q"), offset, actor)
		if e != nil {
			fail(e)
			return
		}
		c.JSON(200, out)
		return
	}
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		fail(errEditInput)
		return
	}
	if c.Request.Method == http.MethodGet {
		out, e := h.ReadInventoryLinks(ctx, p, v, actor)
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
	in, e := decodeInventoryLinks(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "catalog.invalid_inventory_links", "Invalid inventory links")
		return
	}
	out, e := h.EditInventoryLinks(ctx, p, v, actor, in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}
