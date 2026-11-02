package catalog

import (
	"database/sql"
	"net/url"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Discovery is separate from the merchandising collection list: the source
// sitemap includes active empty collections too. No IDs, metadata, prices,
// inventory or authentication data are part of this public projection.
type SEOCatalogEntry struct {
	Handle    string    `json:"handle"`
	UpdatedAt time.Time `json:"updated_at"`
	Slugs     []string  `json:"slugs" gorm:"-"`
}

func (h StoreHandler) SEOCatalog(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	kind := values.Get("kind")
	valid := err == nil && len(values["kind"]) == 1 && (kind == "products" || kind == "collections")
	for key := range values {
		valid = valid && (key == "kind" || key == "limit" || key == "offset")
	}
	values.Del("kind")
	q, parseErr := parseTaxonomy(values.Encode(), false, false)
	if !valid || parseErr != nil {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid discovery query")
		return
	}
	if h.DB == nil {
		taxonomyFailure(c, errStoreScope)
		return
	}
	entries := []SEOCatalogEntry{}
	var count int64
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		channel, e := h.channel(tx)
		if e != nil {
			return e
		}
		if kind == "collections" {
			base := tx.Table("catalog_collections").Where("deleted_at IS NULL")
			if e = base.Count(&count).Error; e != nil {
				return e
			}
			if e = base.Select("handle,updated_at").Order("created_at DESC,id ASC").Limit(q.limit).Offset(q.offset).Scan(&entries).Error; e != nil {
				return e
			}
			for i := range entries {
				entries[i].Slugs = []string{}
			}
			return nil
		}
		if e = publicProducts(tx, channel).Count(&count).Error; e != nil {
			return e
		}
		var products []struct {
			ID, Handle string
			UpdatedAt  time.Time
		}
		if e = publicProducts(tx, channel).Select("p.id,p.handle,p.updated_at").Order("p.created_at DESC,p.id ASC").Limit(q.limit).Offset(q.offset).Scan(&products).Error; e != nil {
			return e
		}
		ids, positions := []string{}, map[string]int{}
		for _, p := range products {
			positions[p.ID] = len(entries)
			ids = append(ids, p.ID)
			entries = append(entries, SEOCatalogEntry{Handle: p.Handle, UpdatedAt: p.UpdatedAt, Slugs: []string{}})
		}
		if len(ids) == 0 {
			return nil
		}
		var size struct{ Total, Largest, Rows int64 }
		base := tx.Table("catalog_variants").Where("product_id IN ? AND deleted_at IS NULL", ids)
		if e = base.Select("COALESCE(SUM(OCTET_LENGTH(metadata)),0) AS total,COALESCE(MAX(OCTET_LENGTH(metadata)),0) AS largest,COUNT(*) AS `rows`").Scan(&size).Error; e != nil {
			return e
		}
		if size.Total > 4*1024*1024 || size.Largest > 65536 || size.Rows > 10000 {
			return errDetailLimit
		}
		var variants []struct {
			ProductID string
			Metadata  *string
		}
		if e = base.Select("product_id,metadata").Order("product_id ASC,variant_rank ASC,id ASC").Scan(&variants).Error; e != nil {
			return e
		}
		for _, v := range variants {
			display, e := variantDisplay(v.Metadata)
			if e != nil {
				return e
			}
			if display.Slug != nil {
				i := positions[v.ProductID]
				entries[i].Slugs = append(entries[i].Slugs, *display.Slug)
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		taxonomyFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"entries": entries, "count": count, "limit": q.limit, "offset": q.offset})
}
