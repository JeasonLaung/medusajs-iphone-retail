package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (h Handler) ImagesHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, errEditInput):
			platform.Fail(c, 400, "catalog.invalid_images", "Invalid image configuration")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current product permission required")
		case errors.Is(e, ErrEditConflict):
			platform.Fail(c, 409, "catalog.image_conflict", "Images changed; reload before saving")
		case errors.Is(e, ErrImageInUse):
			platform.Fail(c, 409, "catalog.image_in_use", "Remove SKU image assignments before deleting this image")
		case errors.Is(e, gorm.ErrRecordNotFound):
			platform.Fail(c, 404, "catalog.not_found", "Product not found")
		default:
			platform.Fail(c, 503, "catalog.unavailable", "Images unavailable; retry the same operation")
		}
	}
	if !validProductID(c.Param("id")) || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		fail(errEditInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if c.Request.Method == http.MethodGet {
		var out any
		var e error
		if v := c.Param("variant_id"); v != "" {
			out, e = h.ReadVariantImages(ctx, c.Param("id"), v, actor)
		} else {
			out, e = h.ReadImages(ctx, c.Param("id"), actor)
		}
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
	if v := c.Param("variant_id"); v != "" {
		in, e := decodeVariantImages(c.Request.Body)
		if e != nil {
			platform.InvalidJSON(c, e, "catalog.invalid_images", "Invalid SKU image configuration")
			return
		}
		out, e := h.EditVariantImages(ctx, c.Param("id"), v, actor, in)
		if e != nil {
			fail(e)
			return
		}
		c.JSON(200, out)
		return
	}
	in, e := decodeImages(c.Request.Body)
	if e != nil {
		var size *http.MaxBytesError
		if errors.As(e, &size) {
			platform.InvalidJSON(c, e, "catalog.invalid_images", "Invalid image configuration")
		} else {
			fail(e)
		}
		return
	}
	out, e := h.EditImages(ctx, c.Param("id"), actor, in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}

// Only assets deliberately attached to public catalog images are served here.
// Private device and buyback tables are never read by this route.
func (h Handler) ImageAssetHTTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	id := c.Param("image_id")
	if len(id) != 40 || id[:4] != "img_" || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		platform.Fail(c, 404, "catalog.image_not_found", "Image not found")
		return
	}
	u, e := uuid.Parse(id[4:])
	if e != nil || u.Version() != 4 || u.String() != id[4:] {
		platform.Fail(c, 404, "catalog.image_not_found", "Image not found")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "catalog.unavailable", "Image unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var row struct {
		MimeType, SHA256 string
		Content          []byte
	}
	e = h.DB.WithContext(ctx).Table("catalog_image_assets a").Select("a.mime_type,a.sha256,a.content").Joins("JOIN catalog_images i ON i.id=a.image_id AND i.deleted_at IS NULL").Joins("JOIN catalog_products p ON p.id=i.product_id AND p.deleted_at IS NULL").Where("a.image_id=? AND i.url=?", id, "/api/store/catalog-images/"+id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "catalog.image_not_found", "Image not found")
		return
	}
	sum := sha256.Sum256(row.Content)
	if e != nil || len(row.Content) == 0 || len(row.Content) > MaxImageBytes || hex.EncodeToString(sum[:]) != row.SHA256 || (row.MimeType != "image/png" && row.MimeType != "image/jpeg" && row.MimeType != "image/webp") {
		platform.Fail(c, 503, "catalog.unavailable", "Image unavailable")
		return
	}
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Data(200, row.MimeType, row.Content)
}
