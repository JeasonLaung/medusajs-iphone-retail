package catalog

import (
	"strconv"
	"strings"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct{ DB *gorm.DB }

func paging(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "24"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit < 1 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 100000 {
		offset = 100000
	}
	return limit, offset
}

func (h Handler) Products(c *gin.Context) { h.products(c, false) }
func (h Handler) products(c *gin.Context, admin bool) {
	limit, offset := paging(c)
	q := h.DB.Model(&Product{}).Where("deleted_at IS NULL")
	if !admin {
		q = q.Where("status=?", "published")
	}
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q = q.Where("title LIKE ? OR handle LIKE ?", "%"+s+"%", "%"+s+"%")
	}
	if s := c.Query("handle"); s != "" {
		q = q.Where("handle=?", s)
	}
	if s := c.Query("collection_id"); s != "" {
		q = q.Where("collection_id=?", s)
	}
	if s := c.Query("category_id"); s != "" {
		q = q.Where("id IN (?)", h.DB.Table("catalog_category_products").Select("product_id").Where("category_id=?", s))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		platform.DBError(c, err)
		return
	}
	var products []Product
	if err := q.Order("created_at DESC, id ASC").Limit(limit).Offset(offset).Find(&products).Error; err != nil {
		platform.DBError(c, err)
		return
	}
	result := make([]Object, 0, len(products))
	for _, p := range products {
		obj, err := h.product(c, p, admin)
		if err != nil {
			platform.DBError(c, err)
			return
		}
		result = append(result, obj)
	}
	c.JSON(200, gin.H{"products": result, "count": count, "limit": limit, "offset": offset})
}
func (h Handler) Product(c *gin.Context) { h.detail(c, false) }
func (h Handler) detail(c *gin.Context, admin bool) {
	var p Product
	q := h.DB.Where("deleted_at IS NULL AND (id=? OR handle=?)", c.Param("id"), c.Param("id"))
	if !admin {
		q = q.Where("status=?", "published")
	}
	if err := q.First(&p).Error; err != nil {
		platform.DBError(c, err)
		return
	}
	obj, err := h.product(c, p, admin)
	if err != nil {
		platform.DBError(c, err)
		return
	}
	c.JSON(200, gin.H{"product": obj})
}
func (h Handler) product(c *gin.Context, p Product, admin bool) (Object, error) {
	out := Decode(p.Payload)
	out["title"] = p.Title
	out["handle"] = p.Handle
	out["status"] = p.Status
	out["thumbnail"] = p.Thumbnail
	var rows []Variant
	if err := h.DB.Where("product_id=? AND deleted_at IS NULL", p.ID).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	variants := make([]Object, 0, len(rows))
	currency := strings.ToLower(c.DefaultQuery("currency_code", "eur"))
	region := c.Query("region_id")
	for _, v := range rows {
		obj := Decode(v.Payload)
		var prices []Price
		q := h.DB.Where("variant_id=? AND deleted_at IS NULL", v.ID)
		if !admin {
			q = q.Where("currency_code=?", currency)
		}
		if err := q.Order("amount ASC").Find(&prices).Error; err != nil {
			return nil, err
		}
		publicPrices := []Object{}
		for _, p := range prices {
			if admin {
				publicPrices = append(publicPrices, Object{"id": p.ID, "currency_code": p.CurrencyCode, "amount": p.Amount, "region_id": p.RegionID, "price_list_id": p.PriceListID, "min_quantity": p.MinQuantity, "max_quantity": p.MaxQuantity})
				continue
			}
			// Do not silently apply promotional or unknown conditional prices.
			if p.PriceListID != nil || p.MinQuantity != nil || p.MaxQuantity != nil {
				continue
			}
			payload := Decode(p.Payload)
			rules, _ := payload["rules"].([]any)
			supported := true
			for _, r := range rules {
				rule, ok := r.(map[string]any)
				if !ok || rule["attribute"] != "region_id" || rule["value"] != region {
					supported = false
				}
			}
			if !supported || p.RegionID != nil && *p.RegionID != region {
				continue
			}
			if obj["calculated_price"] == nil {
				obj["calculated_price"] = Object{"calculated_amount": p.Amount, "original_amount": p.Amount, "currency_code": p.CurrencyCode}
			}
		}
		if admin {
			obj["prices"] = publicPrices
		}
		variants = append(variants, obj)
	}
	out["variants"] = variants
	return out, nil
}
func (h Handler) Categories(c *gin.Context) {
	var rows []Category
	if err := h.DB.Where("deleted_at IS NULL AND is_active=? AND is_internal=?", true, false).Order("id").Find(&rows).Error; err != nil {
		platform.DBError(c, err)
		return
	}
	out := []Object{}
	for _, r := range rows {
		out = append(out, Decode(r.Payload))
	}
	c.JSON(200, gin.H{"product_categories": out, "count": len(out)})
}
func (h Handler) Collections(c *gin.Context) {
	var rows []Collection
	if err := h.DB.Where("deleted_at IS NULL").Order("id").Find(&rows).Error; err != nil {
		platform.DBError(c, err)
		return
	}
	out := []Object{}
	for _, r := range rows {
		out = append(out, Decode(r.Payload))
	}
	c.JSON(200, gin.H{"collections": out, "count": len(out)})
}
