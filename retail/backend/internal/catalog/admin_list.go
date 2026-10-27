package catalog

import (
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// This projection uses authoritative columns shared by the draft and reviewed
// relational schema. Never decode payload/metadata for a list response.
type AdminProductSummary struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Handle       string    `json:"handle"`
	Status       string    `json:"status"`
	Thumbnail    *string   `json:"thumbnail"`
	CollectionID *string   `json:"collection_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	VariantCount int64     `json:"variant_count"`
}

type adminListQuery struct {
	Limit, Offset                       int
	Search, Status, CollectionID, Order string
}

var errListQuery = errors.New("invalid catalog list query")

func parseAdminList(values url.Values) (adminListQuery, error) {
	q := adminListQuery{Limit: 20, Order: "p.created_at DESC, p.id ASC"}
	for key, vs := range values {
		if len(vs) != 1 || vs[0] == "" || !utf8.ValidString(vs[0]) {
			return q, errListQuery
		}
		value := vs[0]
		switch key {
		case "limit", "offset":
			for _, r := range value {
				if r < '0' || r > '9' {
					return q, errListQuery
				}
			}
			n, err := strconv.Atoi(value)
			if err != nil {
				return q, errListQuery
			}
			if key == "limit" {
				if n < 1 || n > 100 {
					return q, errListQuery
				}
				q.Limit = n
			} else {
				if n > 100000 {
					return q, errListQuery
				}
				q.Offset = n
			}
		case "q":
			if utf8.RuneCountInString(value) > 200 || strings.ContainsRune(value, 0) {
				return q, errListQuery
			}
			q.Search = strings.TrimSpace(value)
			if q.Search == "" {
				return q, errListQuery
			}
		case "status":
			switch value {
			case "draft", "proposed", "published", "rejected":
				q.Status = value
			default:
				return q, errListQuery
			}
		case "collection_id":
			if utf8.RuneCountInString(value) > 191 || strings.ContainsRune(value, 0) || strings.TrimSpace(value) != value {
				return q, errListQuery
			}
			q.CollectionID = value
		case "sort":
			switch value {
			case "created_at_desc":
				q.Order = "p.created_at DESC, p.id ASC"
			case "created_at_asc":
				q.Order = "p.created_at ASC, p.id ASC"
			case "title_asc":
				q.Order = "p.title ASC, p.id ASC"
			case "title_desc":
				q.Order = "p.title DESC, p.id ASC"
			default:
				return q, errListQuery
			}
		default:
			return q, errListQuery
		}
	}
	return q, nil
}

func literalLike(value string) string {
	return "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(value) + "%"
}

func (h Handler) AdminProducts(c *gin.Context) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid product filters")
		return
	}
	q, err := parseAdminList(values)
	if err != nil {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid product filters")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
		return
	}
	rows := make([]AdminProductSummary, 0)
	var count int64
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		filtered := func() *gorm.DB {
			db := tx.Table("catalog_products AS p").Where("p.deleted_at IS NULL")
			if q.Status != "" {
				db = db.Where("p.status=?", q.Status)
			}
			if q.CollectionID != "" {
				db = db.Where("p.collection_id=?", q.CollectionID)
			}
			if q.Search != "" {
				pattern := literalLike(q.Search)
				db = db.Where("(p.title LIKE ? ESCAPE '!' OR p.handle LIKE ? ESCAPE '!')", pattern, pattern)
			}
			return db
		}
		if err := filtered().Count(&count).Error; err != nil {
			return err
		}
		return filtered().Select("p.id,p.title,p.handle,p.status,p.thumbnail,p.collection_id,p.created_at,p.updated_at,(SELECT COUNT(*) FROM catalog_variants AS v WHERE v.product_id=p.id AND v.deleted_at IS NULL) AS variant_count").Order(q.Order).Limit(q.Limit).Offset(q.Offset).Scan(&rows).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
		return
	}
	c.JSON(200, gin.H{"products": rows, "count": count, "limit": q.Limit, "offset": q.Offset})
}
