package catalog

import (
	"database/sql"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type storeProductQuery struct {
	limit, offset                        int
	search, handle, collection, category string
	region, quantity                     string
}

func parseStoreProducts(raw string, detail bool) (storeProductQuery, error) {
	q := storeProductQuery{limit: 12, quantity: "1"}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return q, errListQuery
	}
	for key, vs := range values {
		if detail && key != "region_id" && key != "quantity" || len(vs) != 1 || vs[0] == "" || !utf8.ValidString(vs[0]) || containsControl(vs[0]) {
			return q, errListQuery
		}
		v := vs[0]
		switch key {
		case "region_id":
			if !validProductID(v) {
				return q, errListQuery
			}
			q.region = v
		case "quantity":
			n, e := strconv.Atoi(v)
			if e != nil || strconv.Itoa(n) != v || n < 1 || n > 1000000000 {
				return q, errListQuery
			}
			q.quantity = v
		case "limit", "offset":
			n, e := strconv.Atoi(v)
			if e != nil || strconv.Itoa(n) != v || n < 0 {
				return q, errListQuery
			}
			if key == "limit" {
				if n < 1 || n > 100 {
					return q, errListQuery
				}
				q.limit = n
			} else {
				if n > 100000 {
					return q, errListQuery
				}
				q.offset = n
			}
		case "q", "handle":
			if utf8.RuneCountInString(v) > 512 {
				return q, errListQuery
			}
			if key == "q" {
				q.search = v
			} else {
				q.handle = v
			}
		case "collection_id", "category_id":
			if !validProductID(v) {
				return q, errListQuery
			}
			if key == "collection_id" {
				q.collection = v
			} else {
				q.category = v
			}
		default:
			return q, errListQuery
		}
	}
	if values.Has("quantity") && q.region == "" {
		return q, errListQuery
	}
	return q, nil
}

// Price projection requires an explicit valid region; stock and checkout remain
// independent capabilities and cannot be inferred from a configured price.
type StoreProduct struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Handle       string         `json:"handle"`
	Subtitle     *string        `json:"subtitle"`
	Description  *string        `json:"description"`
	Thumbnail    *string        `json:"thumbnail"`
	CollectionID *string        `json:"collection_id"`
	Options      []AdminOption  `json:"options" gorm:"-"`
	Images       []AdminImage   `json:"images" gorm:"-"`
	Variants     []StoreVariant `json:"variants" gorm:"-"`
}
type StoreVariant struct {
	ID                    string              `json:"id"`
	Title                 string              `json:"title"`
	SKU                   *string             `json:"sku"`
	Thumbnail             *string             `json:"thumbnail"`
	VariantRank           *int                `json:"variant_rank"`
	Options               []AdminVariantValue `json:"options" gorm:"-"`
	Images                []AdminImage        `json:"images" gorm:"-"`
	ConfigurationComplete bool                `json:"configuration_complete" gorm:"-"`
	CalculatedPrice       *pricing.Quote      `json:"calculated_price" gorm:"-"`
}

func publicProducts(tx *gorm.DB, channel string) *gorm.DB {
	return tx.Table("catalog_products p").Where("p.deleted_at IS NULL AND p.status='published'").Where("EXISTS (SELECT 1 FROM catalog_product_channels pc WHERE pc.product_id=p.id AND pc.sales_channel_id=? AND pc.deleted_at IS NULL)", channel)
}
func (h StoreHandler) Products(c *gin.Context) { h.products(c, false) }
func (h StoreHandler) Product(c *gin.Context)  { h.products(c, true) }
func (h StoreHandler) products(c *gin.Context, detail bool) {
	c.Header("Cache-Control", "no-store")
	q, err := parseStoreProducts(c.Request.URL.RawQuery, detail)
	if err != nil || detail && !validProductID(c.Param("id")) {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid product query")
		return
	}
	if h.DB == nil {
		taxonomyFailure(c, errStoreScope)
		return
	}
	products := []StoreProduct{}
	var count int64
	var priceContext *pricing.Context
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		channel, e := h.channel(tx)
		if e != nil {
			return e
		}
		if q.region != "" {
			var region struct{ CurrencyCode string }
			if e = tx.Table("commerce_regions").Select("currency_code").Where("id=? AND deleted_at IS NULL", q.region).Take(&region).Error; e != nil {
				return e
			}
			if !regionCurrency.MatchString(region.CurrencyCode) {
				return pricing.ErrInput
			}
			var currencies int64
			if e = tx.Table("commerce_store_currencies sc").Joins("JOIN commerce_currencies cur ON cur.code=sc.currency_code AND cur.deleted_at IS NULL").Where("sc.store_id=? AND sc.currency_code=? AND sc.deleted_at IS NULL", h.StoreID, region.CurrencyCode).Count(&currencies).Error; e != nil {
				return e
			}
			if currencies != 1 {
				return pricing.ErrInput
			}
			priceContext = &pricing.Context{Region: q.region, Currency: region.CurrencyCode, Channel: channel, Quantity: q.quantity}
		}
		base := func() *gorm.DB {
			db := publicProducts(tx, channel)
			if detail {
				db = db.Where("p.id=?", c.Param("id"))
			}
			if q.handle != "" {
				db = db.Where("p.handle=?", q.handle)
			}
			if q.search != "" {
				p := literalLike(q.search)
				db = db.Where("(p.title LIKE ? ESCAPE '!' OR p.handle LIKE ? ESCAPE '!')", p, p)
			}
			if q.collection != "" {
				db = db.Where("p.collection_id=? AND EXISTS (SELECT 1 FROM catalog_collections col WHERE col.id=p.collection_id AND col.deleted_at IS NULL)", q.collection)
			}
			if q.category != "" {
				db = db.Where("EXISTS (SELECT 1 FROM catalog_category_products cp JOIN catalog_categories cat ON cat.id=cp.category_id AND cat.deleted_at IS NULL AND cat.is_active=TRUE AND cat.is_internal=FALSE WHERE cp.product_id=p.id AND cp.category_id=?)", q.category)
			}
			return db
		}
		if e = base().Count(&count).Error; e != nil {
			return e
		}
		if detail && count == 0 {
			return gorm.ErrRecordNotFound
		}
		// Deleted collection IDs are not public relationships.
		if e = base().Select("p.id,p.title,p.handle,p.subtitle,p.description,p.thumbnail,(SELECT col.id FROM catalog_collections col WHERE col.id=p.collection_id AND col.deleted_at IS NULL) AS collection_id").Order("p.id ASC").Limit(q.limit).Offset(q.offset).Scan(&products).Error; e != nil {
			return e
		}
		budget := 10000
		for i := range products {
			if e = publicProductRelations(tx, &products[i], &budget); e != nil {
				return e
			}
		}
		if priceContext != nil {
			ids := []string{}
			for _, p := range products {
				for _, v := range p.Variants {
					if v.ConfigurationComplete {
						ids = append(ids, v.ID)
					}
				}
			}
			quotes, e := pricing.Read(tx, ids, *priceContext, time.Now().UTC())
			if e != nil {
				return e
			}
			for i := range products {
				for j := range products[i].Variants {
					v := &products[i].Variants[j]
					v.CalculatedPrice = quotes[v.ID]
				}
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		taxonomyFailure(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=0, must-revalidate")
	capabilities := gin.H{"pricing": priceContext != nil, "availability": false, "checkout": false}
	if detail {
		c.JSON(200, gin.H{"product": products[0], "capabilities": capabilities})
	} else {
		c.JSON(200, gin.H{"products": products, "count": count, "limit": q.limit, "offset": q.offset, "capabilities": capabilities})
	}
}

func publicProductRelations(tx *gorm.DB, p *StoreProduct, budget *int) error {
	var err error
	p.Options, err = productOptions(tx, p.ID)
	if err != nil {
		return err
	}
	p.Images = []AdminImage{}
	p.Variants = []StoreVariant{}
	if err = tx.Table("catalog_images").Select("id,url,`rank`").Where("product_id=? AND deleted_at IS NULL", p.ID).Order("`rank`,id").Limit(1001).Scan(&p.Images).Error; err != nil {
		return err
	}
	if len(p.Images) > 1000 {
		return errDetailLimit
	}
	if err = tx.Table("catalog_variants").Select("id,title,sku,thumbnail,variant_rank").Where("product_id=? AND deleted_at IS NULL", p.ID).Order("variant_rank IS NULL,variant_rank,id").Limit(1001).Scan(&p.Variants).Error; err != nil {
		return err
	}
	if len(p.Variants) > 1000 {
		return errDetailLimit
	}
	allowed := map[string]AdminVariantValue{}
	for _, o := range p.Options {
		*budget -= 1
		for _, v := range o.Values {
			allowed[v.ID] = AdminVariantValue{ID: v.ID, OptionID: o.ID, Value: v.Value}
			*budget -= 1
		}
	}
	*budget -= len(p.Images) + len(p.Variants)
	if *budget < 0 {
		return errDetailLimit
	}
	if len(p.Variants) == 0 {
		return nil
	}
	ids := []string{}
	positions := map[string]int{}
	for i := range p.Variants {
		v := &p.Variants[i]
		ids = append(ids, v.ID)
		positions[v.ID] = i
		v.Options = []AdminVariantValue{}
		v.Images = []AdminImage{}
	}
	var choices []struct{ VariantID, OptionValueID string }
	if err = tx.Table("catalog_variant_options").Select("variant_id,option_value_id").Where("variant_id IN ?", ids).Order("variant_id,option_value_id").Limit(*budget + 1).Scan(&choices).Error; err != nil {
		return err
	}
	*budget -= len(choices)
	if *budget < 0 {
		return errDetailLimit
	}
	seen := map[string]map[string]bool{}
	invalid := map[string]bool{}
	for _, ch := range choices {
		v := &p.Variants[positions[ch.VariantID]]
		value, ok := allowed[ch.OptionValueID]
		if !ok {
			invalid[v.ID] = true
			continue
		} // Never expose a foreign option-value ID.
		if seen[v.ID] == nil {
			seen[v.ID] = map[string]bool{}
		}
		if seen[v.ID][value.OptionID] {
			invalid[v.ID] = true
		}
		seen[v.ID][value.OptionID] = true
		v.Options = append(v.Options, value)
	}
	for i := range p.Variants {
		v := &p.Variants[i]
		v.ConfigurationComplete = !invalid[v.ID] && len(seen[v.ID]) == len(p.Options)
	}
	var images []struct {
		AdminImage
		VariantID string
	}
	if err = tx.Table("catalog_images i").Select("DISTINCT i.id,i.url,i.rank,vi.variant_id").Joins("JOIN catalog_variant_images vi ON vi.image_id=i.id AND vi.deleted_at IS NULL").Where("vi.variant_id IN ? AND i.product_id=? AND i.deleted_at IS NULL", ids, p.ID).Order("i.rank,i.id,vi.variant_id").Limit(*budget + 1).Scan(&images).Error; err != nil {
		return err
	}
	*budget -= len(images)
	if *budget < 0 {
		return errDetailLimit
	}
	for _, im := range images {
		v := &p.Variants[positions[im.VariantID]]
		v.Images = append(v.Images, im.AdminImage)
	}
	return nil
}
