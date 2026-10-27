package catalog

import (
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The server binds the shop. Client filters never select another store/channel.
type StoreHandler struct {
	DB      *gorm.DB
	StoreID string
}

var errStoreScope = errors.New("store catalog unavailable")

func (h StoreHandler) channel(tx *gorm.DB) (string, error) {
	if !validProductID(h.StoreID) {
		return "", errStoreScope
	}
	if err := Ready(tx.Statement.Context, tx); err != nil {
		return "", err
	}
	var channel string
	result := tx.Table("commerce_stores s").Select("c.id").Joins("JOIN commerce_sales_channels c ON c.id=s.default_sales_channel_id AND c.deleted_at IS NULL AND c.is_disabled=FALSE").Where("s.id=? AND s.deleted_at IS NULL", h.StoreID).Limit(1).Scan(&channel)
	if result.Error != nil || result.RowsAffected != 1 || channel == "" {
		return "", errStoreScope
	}
	return channel, nil
}

type taxonomyQuery struct {
	limit, offset  int
	handle, parent string
	hasParent      bool
}

func parseTaxonomy(raw string, category, detail bool) (taxonomyQuery, error) {
	q := taxonomyQuery{limit: 100}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return q, errListQuery
	}
	for key, vs := range values {
		if detail || len(vs) != 1 || vs[0] == "" || !utf8.ValidString(vs[0]) || containsControl(vs[0]) {
			return q, errListQuery
		}
		value := vs[0]
		switch key {
		case "limit", "offset":
			n, err := strconv.Atoi(value)
			if err != nil || strconv.Itoa(n) != value || n < 0 {
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
		case "handle":
			if utf8.RuneCountInString(value) > 512 {
				return q, errListQuery
			}
			q.handle = value
		case "parent_category_id":
			if !category || !validProductID(value) {
				return q, errListQuery
			}
			q.parent = value
			q.hasParent = true
		default:
			return q, errListQuery
		}
	}
	return q, nil
}

type StoreCategoryBase struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Handle           string  `json:"handle"`
	Rank             int     `json:"rank"`
	ParentCategoryID *string `json:"parent_category_id"`
	ProductCount     int64   `json:"product_count" gorm:"-"`
}
type StoreCategoryAncestor struct {
	StoreCategoryBase
	ParentCategory *StoreCategoryAncestor `json:"parent_category"`
}
type StoreCategory struct {
	StoreCategoryBase
	ParentCategory *StoreCategoryAncestor `json:"parent_category"`
	Children       []StoreCategoryBase    `json:"category_children"`
}
type StoreCollection struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Handle       string `json:"handle"`
	ProductCount int64  `json:"product_count"`
}

func taxonomyFailure(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "catalog.not_found", "Catalog resource not found")
	} else {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
	}
}

func (h StoreHandler) Categories(c *gin.Context) { h.categories(c, false) }
func (h StoreHandler) Category(c *gin.Context)   { h.categories(c, true) }
func (h StoreHandler) categories(c *gin.Context, detail bool) {
	c.Header("Cache-Control", "no-store")
	q, err := parseTaxonomy(c.Request.URL.RawQuery, true, detail)
	if err != nil || detail && !validProductID(c.Param("id")) {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid category query")
		return
	}
	if h.DB == nil {
		taxonomyFailure(c, errStoreScope)
		return
	}
	output := []StoreCategory{}
	count := 0
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		channel, e := h.channel(tx)
		if e != nil {
			return e
		}
		rows := []StoreCategoryBase{}
		if e = tx.Table("catalog_categories").Select("id,name,description,handle,`rank`,parent_category_id").Where("deleted_at IS NULL AND is_active=TRUE AND is_internal=FALSE").Order("`rank` ASC,id ASC").Limit(1001).Scan(&rows).Error; e != nil {
			return e
		}
		if len(rows) > 1000 {
			return errDetailLimit
		}
		positions := map[string]int{}
		for i, r := range rows {
			positions[r.ID] = i
		}
		var counts []struct {
			CategoryID string
			Count      int64
		}
		if e = tx.Table("catalog_category_products cp").Select("cp.category_id,COUNT(DISTINCT p.id) AS count").Joins("JOIN catalog_products p ON p.id=cp.product_id AND p.deleted_at IS NULL AND p.status='published'").Joins("JOIN catalog_product_channels pc ON pc.product_id=p.id AND pc.sales_channel_id=? AND pc.deleted_at IS NULL", channel).Joins("JOIN catalog_categories c ON c.id=cp.category_id AND c.deleted_at IS NULL AND c.is_active=TRUE AND c.is_internal=FALSE").Group("cp.category_id").Limit(1001).Scan(&counts).Error; e != nil {
			return e
		}
		if len(counts) > 1000 {
			return errDetailLimit
		}
		for _, value := range counts {
			if i, ok := positions[value.CategoryID]; ok {
				rows[i].ProductCount = value.Count
			}
		}
		// A hidden/deleted ancestor is not published through an otherwise public
		// child. The original relationship remains intact in persistent history.
		for i := range rows {
			if p := rows[i].ParentCategoryID; p != nil {
				if _, ok := positions[*p]; !ok {
					rows[i].ParentCategoryID = nil
				}
			}
		}
		for _, r := range rows {
			seen := map[string]bool{r.ID: true}
			parent := r.ParentCategoryID
			for depth := 0; parent != nil; depth++ {
				if depth >= 16 || seen[*parent] {
					return errDetailLimit
				}
				seen[*parent] = true
				parent = rows[positions[*parent]].ParentCategoryID
			}
		}
		for _, r := range rows {
			match := !detail || r.ID == c.Param("id")
			match = match && (q.handle == "" || r.Handle == q.handle)
			if q.hasParent {
				match = match && (q.parent == "null" && r.ParentCategoryID == nil || r.ParentCategoryID != nil && *r.ParentCategoryID == q.parent)
			}
			if !match {
				continue
			}
			count++
			if !detail && (count <= q.offset || len(output) >= q.limit) {
				continue
			}
			item := StoreCategory{StoreCategoryBase: r, Children: []StoreCategoryBase{}}
			var build func(*string) *StoreCategoryAncestor
			build = func(id *string) *StoreCategoryAncestor {
				if id == nil {
					return nil
				}
				p := rows[positions[*id]]
				return &StoreCategoryAncestor{p, build(p.ParentCategoryID)}
			}
			item.ParentCategory = build(r.ParentCategoryID)
			for _, child := range rows {
				if child.ParentCategoryID != nil && *child.ParentCategoryID == r.ID {
					item.Children = append(item.Children, child)
				}
			}
			output = append(output, item)
		}
		if detail && count == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		taxonomyFailure(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=0, must-revalidate")
	if detail {
		c.JSON(200, gin.H{"product_category": output[0]})
	} else {
		c.JSON(200, gin.H{"product_categories": output, "count": count, "limit": q.limit, "offset": q.offset})
	}
}

func (h StoreHandler) Collections(c *gin.Context) { h.collections(c, false) }
func (h StoreHandler) Collection(c *gin.Context)  { h.collections(c, true) }
func (h StoreHandler) collections(c *gin.Context, detail bool) {
	c.Header("Cache-Control", "no-store")
	q, err := parseTaxonomy(c.Request.URL.RawQuery, false, detail)
	if err != nil || detail && !validProductID(c.Param("id")) {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid collection query")
		return
	}
	if h.DB == nil {
		taxonomyFailure(c, errStoreScope)
		return
	}
	rows := []StoreCollection{}
	var count int64
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		channel, e := h.channel(tx)
		if e != nil {
			return e
		}
		base := tx.Table("catalog_collections c").Where("c.deleted_at IS NULL AND EXISTS(SELECT 1 FROM catalog_products p JOIN catalog_product_channels pc ON pc.product_id=p.id AND pc.sales_channel_id=? AND pc.deleted_at IS NULL WHERE p.collection_id=c.id AND p.deleted_at IS NULL AND p.status='published')", channel)
		if detail {
			base = base.Where("c.id=?", c.Param("id"))
		}
		if q.handle != "" {
			base = base.Where("c.handle=?", q.handle)
		}
		if e = base.Count(&count).Error; e != nil {
			return e
		}
		if detail && count == 0 {
			return gorm.ErrRecordNotFound
		}
		return base.Select("c.id,c.title,c.handle,(SELECT COUNT(DISTINCT p.id) FROM catalog_products p JOIN catalog_product_channels pc ON pc.product_id=p.id AND pc.sales_channel_id=? AND pc.deleted_at IS NULL WHERE p.collection_id=c.id AND p.deleted_at IS NULL AND p.status='published') AS product_count", channel).Order("c.id ASC").Limit(q.limit).Offset(q.offset).Scan(&rows).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		taxonomyFailure(c, err)
		return
	}
	if detail && len(rows) != 1 {
		taxonomyFailure(c, errStoreScope)
		return
	}
	c.Header("Cache-Control", "public, max-age=0, must-revalidate")
	if detail {
		c.JSON(200, gin.H{"collection": rows[0]})
	} else {
		c.JSON(200, gin.H{"collections": rows, "count": count, "limit": q.limit, "offset": q.offset})
	}
}
