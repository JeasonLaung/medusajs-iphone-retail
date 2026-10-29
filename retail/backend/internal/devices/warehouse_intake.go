package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Scope comes from the dedicated server route, never the submitted device body.
type WarehouseIntakeScope struct{ LocationID, Category string }

func (h Handler) intakeContextKey() string {
	if h.WarehouseScope == nil {
		return ""
	}
	return h.WarehouseScope.Category + ":" + h.WarehouseScope.LocationID
}
func (h Handler) intakeAccess(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	if h.WarehouseScope == nil {
		return intakeAuthority(ctx, tx, actor, write)
	}
	scope := h.WarehouseScope
	actions, e := warehouseDeviceActions(scope.Category, write)
	if e != nil || !validPathID(scope.LocationID) {
		return ErrInput
	}
	return authorization.CheckWarehouseTx(ctx, tx, actor, scope.LocationID, actions...)
}
func (h Handler) intakeConfiguration(tx *gorm.DB, productID, variantID string) (IntakeContext, error) {
	out, e := intakeSKU(tx, productID, variantID)
	if e != nil || h.WarehouseScope == nil {
		return out, e
	}
	filtered := []Source{}
	for _, s := range out.Sources {
		if s.LocationID == h.WarehouseScope.LocationID {
			filtered = append(filtered, s)
		}
	}
	out.Sources = filtered
	// Scope is part of the editor digest, so another warehouse/category cannot reuse it.
	b, _ := json.Marshal([]any{out.Revision, h.intakeContextKey(), out.Sources})
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, nil
}
func (h Handler) intakeSources(tx *gorm.DB, devices []IntakeDevice) error {
	if h.WarehouseScope == nil {
		return nil
	}
	for _, d := range devices {
		if d.Category != h.WarehouseScope.Category {
			return authorization.ErrDenied
		}
		var source struct{ LocationID string }
		e := tx.Table("device_sources s").Select("s.location_id").Joins("JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.id=? AND s.type='self' AND s.deleted_at IS NULL", d.SourceID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&source).Error
		if e == gorm.ErrRecordNotFound {
			return authorization.ErrDenied
		}
		if e != nil {
			return ErrUnavailable
		}
		if source.LocationID != h.WarehouseScope.LocationID {
			return authorization.ErrDenied
		}
	}
	return nil
}

type WarehouseIntakeVariant struct {
	ID           string  `json:"id"`
	ProductID    string  `json:"product_id"`
	Title        string  `json:"title"`
	ProductTitle string  `json:"product_title"`
	SKU          *string `json:"sku"`
}
type WarehouseIntakeVariants struct {
	Variants []WarehouseIntakeVariant `json:"variants"`
	Offset   int                      `json:"offset"`
	Limit    int                      `json:"limit"`
	HasMore  bool                     `json:"has_more"`
}

func (h Handler) ReadWarehouseIntakeVariants(ctx context.Context, actor identity.User, q string, offset int) (WarehouseIntakeVariants, error) {
	out := WarehouseIntakeVariants{Variants: []WarehouseIntakeVariant{}, Offset: offset, Limit: 20}
	q = strings.TrimSpace(q)
	if h.WarehouseScope == nil || !intakeText(q, 200) || len(strings.Fields(q)) > 12 || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := h.intakeAccess(ctx, tx, actor, false); e != nil {
			return e
		}
		if _, e := sourceGate(ctx, tx); e != nil {
			return e
		}
		query := tx.Table("catalog_variants v").Select("v.id,v.product_id,v.title,p.title AS product_title,v.sku").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("v.deleted_at IS NULL")
		// Definitions are shared, as in the original variant picker. No prices,
		// metadata, stock levels, costs or customer relations are exposed.
		query = warehouseVariantSearch(query, q)
		if query.Order("v.sku ASC,v.id ASC").Offset(offset).Limit(21).Scan(&out.Variants).Error != nil {
			return ErrUnavailable
		}
		if len(out.Variants) > 20 {
			out.HasMore = true
			out.Variants = out.Variants[:20]
		}
		return nil
	})
	return out, e
}
func (h Handler) warehouseScoped(c *gin.Context) Handler {
	return Handler{DB: h.DB, WarehouseScope: &WarehouseIntakeScope{LocationID: c.Param("location_id"), Category: c.Param("category")}}
}
func (h Handler) WarehouseIntakeHTTP(c *gin.Context) {
	h.warehouseScoped(c).intakeHTTP(c, c.Request.Method == "POST")
}
func (h Handler) WarehousePhotoHTTP(c *gin.Context) {
	if h.DB == nil {
		trackingFail(c, ErrUnavailable)
		return
	}
	scoped := h.warehouseScoped(c)
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	// Authorize before decoding the larger photo body, then recheck in the writer.
	e := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error { return scoped.intakeAccess(c.Request.Context(), tx, actor, true) })
	if e != nil {
		trackingFail(c, e)
		return
	}
	scoped.PhotoUploadHTTP(c)
}
func (h Handler) WarehouseIntakeVariantsHTTP(c *gin.Context) {
	query, e := url.ParseQuery(c.Request.URL.RawQuery)
	offset := 0
	if e != nil || len(query) > 2 {
		trackingFail(c, ErrInput)
		return
	}
	for k, v := range query {
		if (k != "q" && k != "offset") || len(v) != 1 {
			trackingFail(c, ErrInput)
			return
		}
	}
	if v, ok := query["offset"]; ok {
		offset, e = strconv.Atoi(v[0])
		if e != nil || strconv.Itoa(offset) != v[0] {
			trackingFail(c, ErrInput)
			return
		}
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	out, e := h.warehouseScoped(c).ReadWarehouseIntakeVariants(ctx, actor, query.Get("q"), offset)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, out)
}
