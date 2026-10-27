package catalog

import (
	"database/sql"
	"errors"
	"net/url"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdminOptionValue struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Rank  *int   `json:"rank"`
}
type AdminOption struct {
	ID     string             `json:"id"`
	Title  string             `json:"title"`
	Values []AdminOptionValue `json:"values"`
}
type AdminImage struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Rank int    `json:"rank"`
}
type AdminProductDetail struct {
	AdminProductSummary
	Subtitle    *string       `json:"subtitle"`
	Description *string       `json:"description"`
	Revision    string        `json:"revision" gorm:"-"`
	Options     []AdminOption `json:"options" gorm:"-"`
	Images      []AdminImage  `json:"images" gorm:"-"`
}
type AdminVariantValue struct {
	ID       string `json:"id"`
	OptionID string `json:"option_id"`
	Value    string `json:"value"`
}
type AdminVariant struct {
	ID                    string              `json:"id"`
	ProductID             string              `json:"product_id"`
	Title                 string              `json:"title"`
	SKU                   *string             `json:"sku"`
	Thumbnail             *string             `json:"thumbnail"`
	VariantRank           *int                `json:"variant_rank"`
	ManageInventory       bool                `json:"manage_inventory"`
	AllowBackorder        bool                `json:"allow_backorder"`
	Options               []AdminVariantValue `json:"options" gorm:"-"`
	UnassignedValueIDs    []string            `json:"unassigned_option_value_ids" gorm:"-"`
	ConfigurationComplete bool                `json:"configuration_complete" gorm:"-"`
	Images                []AdminImage        `json:"images" gorm:"-"`
}

var errDetailLimit = errors.New("catalog relation limit exceeded")

func validProductID(id string) bool {
	return id != "" && utf8.ValidString(id) && utf8.RuneCountInString(id) <= 191 && !containsControl(id)
}
func containsControl(s string) bool {
	for _, r := range s {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

// The reviewed schema is required. A legacy payload is never a fallback.
func detailBase(tx *gorm.DB, id string) (AdminProductDetail, error) {
	p := AdminProductDetail{Options: []AdminOption{}, Images: []AdminImage{}}
	err := tx.Table("catalog_products AS p").Select("p.id,p.title,p.handle,p.status,p.thumbnail,p.collection_id,p.created_at,p.updated_at,p.subtitle,p.description,(SELECT COUNT(*) FROM catalog_variants v WHERE v.product_id=p.id AND v.deleted_at IS NULL) AS variant_count").Where("p.id=? AND p.deleted_at IS NULL", id).Take(&p).Error
	if err == nil {
		p.Revision = editRevision(EditableProduct{Title: p.Title, Subtitle: p.Subtitle, Description: p.Description, Status: p.Status, UpdatedAt: p.UpdatedAt})
	}
	return p, err
}

func productOptions(tx *gorm.DB, id string) ([]AdminOption, error) {
	type optionRow struct{ ID, Title string }
	var rows []optionRow
	err := tx.Table("catalog_options o").Select("DISTINCT o.id,o.title").Joins("JOIN catalog_product_options po ON po.product_option_id=o.id AND po.deleted_at IS NULL").Where("po.product_id=? AND o.deleted_at IS NULL", id).Order("o.title ASC,o.id ASC").Limit(1001).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) > 1000 {
		return nil, errDetailLimit
	}
	out := make([]AdminOption, 0, len(rows))
	index := map[string]int{}
	for _, o := range rows {
		index[o.ID] = len(out)
		out = append(out, AdminOption{ID: o.ID, Title: o.Title, Values: []AdminOptionValue{}})
	}
	type valueRow struct {
		AdminOptionValue
		OptionID string
	}
	var values []valueRow
	err = tx.Table("catalog_option_values v").Select("DISTINCT v.id,v.value,v.rank,v.option_id").Joins("JOIN catalog_product_option_values pv ON pv.product_option_value_id=v.id AND pv.deleted_at IS NULL").Joins("JOIN catalog_product_options po ON po.id=pv.product_product_option_id AND po.deleted_at IS NULL AND po.product_option_id=v.option_id").Joins("JOIN catalog_options o ON o.id=v.option_id AND o.deleted_at IS NULL").Where("po.product_id=? AND v.deleted_at IS NULL", id).Order("v.rank IS NULL ASC,v.rank ASC,v.id ASC").Limit(10001).Scan(&values).Error
	if err != nil {
		return nil, err
	}
	if len(values) > 10000 {
		return nil, errDetailLimit
	}
	for _, v := range values {
		i, ok := index[v.OptionID]
		if ok {
			out[i].Values = append(out[i].Values, v.AdminOptionValue)
		}
	}
	return out, nil
}

func (h Handler) AdminProduct(c *gin.Context) {
	id := c.Param("id")
	if !validProductID(id) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid product identifier or query")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
		return
	}
	var p AdminProductDetail
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		p, err = detailBase(tx, id)
		if err != nil {
			return err
		}
		p.Options, err = productOptions(tx, id)
		if err != nil {
			return err
		}
		err = tx.Table("catalog_images").Select("id,url,`rank`").Where("product_id=? AND deleted_at IS NULL", id).Order("`rank` ASC,id ASC").Limit(1001).Scan(&p.Images).Error
		if len(p.Images) > 1000 {
			return errDetailLimit
		}
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "catalog.not_found", "Product not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
		return
	}
	c.JSON(200, gin.H{"product": p})
}

func (h Handler) AdminVariants(c *gin.Context) {
	id := c.Param("id")
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || !validProductID(id) {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid product variant query")
		return
	}
	for key := range values {
		if key != "limit" && key != "offset" {
			platform.Fail(c, 400, "catalog.invalid_query", "Invalid product variant query")
			return
		}
	}
	q, err := parseAdminList(values)
	if err != nil {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid product variant query")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
		return
	}
	variants := make([]AdminVariant, 0)
	var count int64
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		p, err := detailBase(tx, id)
		if err != nil {
			return err
		}
		count = p.VariantCount
		options, err := productOptions(tx, id)
		if err != nil {
			return err
		}
		allowed := map[string]AdminVariantValue{}
		for _, o := range options {
			for _, v := range o.Values {
				allowed[v.ID] = AdminVariantValue{ID: v.ID, OptionID: o.ID, Value: v.Value}
			}
		}
		if err = tx.Table("catalog_variants").Select("id,product_id,title,sku,thumbnail,variant_rank,manage_inventory,allow_backorder").Where("product_id=? AND deleted_at IS NULL", id).Order("variant_rank IS NULL ASC,variant_rank ASC,id ASC").Limit(q.Limit).Offset(q.Offset).Scan(&variants).Error; err != nil {
			return err
		}
		if len(variants) == 0 {
			return nil
		}
		ids := make([]string, 0, len(variants))
		positions := map[string]int{}
		for i := range variants {
			v := &variants[i]
			ids = append(ids, v.ID)
			positions[v.ID] = i
			v.Options = []AdminVariantValue{}
			v.UnassignedValueIDs = []string{}
			v.Images = []AdminImage{}
		}
		var choices []struct{ VariantID, OptionValueID string }
		if err = tx.Table("catalog_variant_options").Select("variant_id,option_value_id").Where("variant_id IN ?", ids).Order("variant_id,option_value_id").Limit(10001).Scan(&choices).Error; err != nil {
			return err
		}
		if len(choices) > 10000 {
			return errDetailLimit
		}
		seen := map[string]map[string]bool{}
		invalid := map[string]bool{}
		for _, choice := range choices {
			v := &variants[positions[choice.VariantID]]
			value, ok := allowed[choice.OptionValueID]
			if !ok {
				v.UnassignedValueIDs = append(v.UnassignedValueIDs, choice.OptionValueID)
				invalid[v.ID] = true
				continue
			}
			if seen[v.ID] == nil {
				seen[v.ID] = map[string]bool{}
			}
			if seen[v.ID][value.OptionID] {
				invalid[v.ID] = true
			}
			seen[v.ID][value.OptionID] = true
			v.Options = append(v.Options, value)
		}
		for i := range variants {
			v := &variants[i]
			v.ConfigurationComplete = !invalid[v.ID] && len(seen[v.ID]) == len(options)
		}
		var images []struct {
			AdminImage
			VariantID string
		}
		err = tx.Table("catalog_images i").Select("DISTINCT i.id,i.url,i.rank,vi.variant_id").Joins("JOIN catalog_variant_images vi ON vi.image_id=i.id AND vi.deleted_at IS NULL").Where("vi.variant_id IN ? AND i.product_id=? AND i.deleted_at IS NULL", ids, id).Order("i.rank ASC,i.id ASC").Limit(10001).Scan(&images).Error
		if len(images) > 10000 {
			return errDetailLimit
		}
		if err != nil {
			return err
		}
		for _, im := range images {
			variants[positions[im.VariantID]].Images = append(variants[positions[im.VariantID]].Images, im.AdminImage)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "catalog.not_found", "Product not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
		return
	}
	c.JSON(200, gin.H{"variants": variants, "count": count, "limit": q.Limit, "offset": q.Offset})
}
