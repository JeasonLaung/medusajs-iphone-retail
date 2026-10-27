package orders

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

type Handler struct{ DB *gorm.DB }

// Summary deliberately excludes metadata, protected subjects, raw totals,
// addresses and provider payloads. Order state does not imply payment state.
type Summary struct {
	ID              string    `json:"id"`
	DisplayID       *string   `json:"display_id"`
	CustomDisplayID *string   `json:"custom_display_id"`
	Version         int       `json:"version"`
	Status          string    `json:"status"`
	IsDraftOrder    bool      `json:"is_draft_order"`
	Email           *string   `json:"email"`
	CurrencyCode    string    `json:"currency_code"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LineCount       int64     `json:"line_count"`
}

type Line struct {
	ID                string  `json:"id"`
	ItemID            string  `json:"item_id"`
	Title             string  `json:"title"`
	VariantSKU        *string `json:"variant_sku"`
	Quantity          string  `json:"quantity"`
	UnitPrice         string  `json:"unit_price"`
	IsTaxInclusive    bool    `json:"is_tax_inclusive"`
	FulfilledQuantity string  `json:"fulfilled_quantity"`
	ShippedQuantity   string  `json:"shipped_quantity"`
	ReturnRequested   string  `json:"return_requested_quantity"`
	ReturnReceived    string  `json:"return_received_quantity"`
	ReturnDismissed   string  `json:"return_dismissed_quantity"`
}

type listQuery struct {
	Limit, Offset         int
	Search, Status, Order string
}

var errQuery = errors.New("invalid order filters")

func parseList(values url.Values) (listQuery, error) {
	q := listQuery{Limit: 20, Order: "o.created_at DESC,o.id ASC"}
	for key, values := range values {
		if len(values) != 1 || values[0] == "" || !utf8.ValidString(values[0]) || strings.ContainsRune(values[0], 0) {
			return q, errQuery
		}
		v := values[0]
		switch key {
		case "limit", "offset":
			for _, c := range v {
				if c < '0' || c > '9' {
					return q, errQuery
				}
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return q, errQuery
			}
			if key == "limit" {
				if n < 1 || n > 100 {
					return q, errQuery
				}
				q.Limit = n
			} else {
				if n > 100000 {
					return q, errQuery
				}
				q.Offset = n
			}
		case "q":
			q.Search = strings.TrimSpace(v)
			if q.Search == "" || utf8.RuneCountInString(q.Search) > 200 {
				return q, errQuery
			}
		case "status":
			switch v {
			case "pending", "completed", "draft", "archived", "canceled", "requires_action":
				q.Status = v
			default:
				return q, errQuery
			}
		case "sort":
			switch v {
			case "created_at_desc":
			case "created_at_asc":
				q.Order = "o.created_at ASC,o.id ASC"
			default:
				return q, errQuery
			}
		default:
			return q, errQuery
		}
	}
	return q, nil
}

func summaryColumns() string {
	return `o.id,CAST(o.display_id AS CHAR) AS display_id,o.custom_display_id,o.version,o.status,o.is_draft_order,o.email,o.currency_code,o.created_at,o.updated_at,(SELECT COUNT(*) FROM order_items i WHERE i.order_id=o.id AND i.version=o.version AND i.deleted_at IS NULL) AS line_count`
}

func (h Handler) List(c *gin.Context) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		platform.Fail(c, 400, "orders.invalid_query", "Invalid order filters")
		return
	}
	q, err := parseList(values)
	if err != nil {
		platform.Fail(c, 400, "orders.invalid_query", "Invalid order filters")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "orders.unavailable", "Orders unavailable")
		return
	}
	rows := make([]Summary, 0)
	var count int64
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := Ready(c.Request.Context(), tx); err != nil {
			return err
		}
		filtered := func() *gorm.DB {
			query := tx.Table("orders o").Where("o.deleted_at IS NULL")
			if q.Status != "" {
				query = query.Where("o.status=?", q.Status)
			}
			if q.Search != "" {
				pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.Search) + "%"
				query = query.Where("(o.id LIKE ? ESCAPE '!' OR o.custom_display_id LIKE ? ESCAPE '!' OR o.email LIKE ? ESCAPE '!' OR CAST(o.display_id AS CHAR) LIKE ? ESCAPE '!')", pattern, pattern, pattern, pattern)
			}
			return query
		}
		if err := filtered().Count(&count).Error; err != nil {
			return err
		}
		return filtered().Select(summaryColumns()).Order(q.Order).Limit(q.Limit).Offset(q.Offset).Scan(&rows).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		platform.Fail(c, 503, "orders.unavailable", "Orders unavailable")
		return
	}
	c.JSON(200, gin.H{"orders": rows, "count": count, "limit": q.Limit, "offset": q.Offset})
}

func (h Handler) Detail(c *gin.Context) {
	id := c.Param("id")
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 || strings.ContainsRune(id, 0) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "orders.invalid_query", "Invalid order request")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "orders.unavailable", "Orders unavailable")
		return
	}
	var order Summary
	lines := make([]Line, 0)
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := Ready(c.Request.Context(), tx); err != nil {
			return err
		}
		if err := tx.Table("orders o").Select(summaryColumns()).Where("o.id=? AND o.deleted_at IS NULL", id).Take(&order).Error; err != nil {
			return err
		}
		if order.LineCount > 1000 {
			return ErrSchema
		}
		// Snapshot names and the selected order version are authoritative. Never
		// replace a deleted variant with current catalog text or current prices.
		if err := tx.Table("order_items i").Joins("JOIN order_line_items l ON l.id=i.item_id AND l.deleted_at IS NULL").
			Where("i.order_id=? AND i.version=? AND i.deleted_at IS NULL", id, order.Version).
			Select(`i.id,i.item_id,l.title,l.variant_sku,CAST(i.quantity AS CHAR) AS quantity,CAST(COALESCE(i.unit_price,l.unit_price) AS CHAR) AS unit_price,l.is_tax_inclusive,CAST(i.fulfilled_quantity AS CHAR) AS fulfilled_quantity,CAST(i.shipped_quantity AS CHAR) AS shipped_quantity,CAST(i.return_requested_quantity AS CHAR) AS return_requested,CAST(i.return_received_quantity AS CHAR) AS return_received,CAST(i.return_dismissed_quantity AS CHAR) AS return_dismissed`).
			Order("i.id ASC").Limit(1001).Scan(&lines).Error; err != nil {
			return err
		}
		// Missing/soft-deleted snapshots cannot silently turn a partially read
		// order into a complete successful response.
		if int64(len(lines)) != order.LineCount {
			return ErrSchema
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "orders.not_found", "Order not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "orders.unavailable", "Orders unavailable")
		return
	}
	c.JSON(200, gin.H{"order": order, "lines": lines})
}
