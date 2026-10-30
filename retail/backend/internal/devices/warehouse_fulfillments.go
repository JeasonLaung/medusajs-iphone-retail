package devices

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Dedicated warehouse projection. Never reuse the full-store order response:
// payments, other parcels, prices, customer metadata and device costs are absent.
type WarehouseFulfillment struct {
	ID          string     `json:"id"`
	OrderID     string     `json:"order_id"`
	Reference   string     `json:"reference"`
	LocationID  string     `json:"location_id"`
	CreatedAt   time.Time  `json:"created_at"`
	PackedAt    *time.Time `json:"packed_at"`
	ShippedAt   *time.Time `json:"shipped_at"`
	DeliveredAt *time.Time `json:"delivered_at"`
}
type WarehouseFulfillments struct {
	LocationID   string                 `json:"location_id"`
	Fulfillments []WarehouseFulfillment `json:"fulfillments"`
	Offset       int                    `json:"offset"`
	Limit        int                    `json:"limit"`
	HasMore      bool                   `json:"has_more"`
}
type WarehouseFulfillmentItem struct {
	ID          string  `json:"id"`
	ItemID      *string `json:"item_id" gorm:"column:line_item_id"`
	Title       string  `json:"title"`
	SKU         string  `json:"sku"`
	Quantity    string  `json:"quantity"`
	RawQuantity string  `json:"-"`
}
type WarehouseFulfillmentDevice struct {
	ID           string `json:"id"`
	ItemID       string `json:"item_id"`
	SerialNumber string `json:"serial_number"`
	Status       string `json:"status"`
}
type WarehouseFulfillmentDetail struct {
	WarehouseFulfillment
	Items   []WarehouseFulfillmentItem   `json:"items"`
	Devices []WarehouseFulfillmentDevice `json:"devices"`
	Labels  []ParcelLabel                `json:"labels"`
}

func warehouseFulfillmentQuery(tx *gorm.DB, location string) *gorm.DB {
	return tx.Table("fulfillments f").Joins("JOIN order_fulfillments l ON l.fulfillment_id=f.id AND l.deleted_at IS NULL").Joins("JOIN orders o ON o.id=l.order_id AND o.deleted_at IS NULL").Where("f.location_id=? AND f.deleted_at IS NULL AND f.canceled_at IS NULL", location)
}

const warehouseFulfillmentColumns = "f.id,l.order_id,COALESCE(NULLIF(o.custom_display_id,''),CAST(o.display_id AS CHAR),o.id) AS reference,f.location_id,f.created_at,f.packed_at,f.shipped_at,f.delivered_at"

func (h Handler) ReadWarehouseFulfillments(ctx context.Context, actor identity.User, location, search string, offset int) (WarehouseFulfillments, error) {
	out := WarehouseFulfillments{LocationID: location, Fulfillments: []WarehouseFulfillment{}, Offset: offset, Limit: 50}
	search = strings.TrimSpace(search)
	if !validPathID(location) || !intakeText(search, 100) || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, location, authorization.OrderFulfillmentRead); e != nil {
			return e
		}
		if fulfillment.Ready(ctx, tx) != nil {
			return ErrUnavailable
		}
		query := warehouseFulfillmentQuery(tx, location)
		if search != "" {
			query = query.Where("LOCATE(?,f.id)>0 OR LOCATE(?,o.id)>0 OR LOCATE(?,COALESCE(o.custom_display_id,CAST(o.display_id AS CHAR),''))>0", search, search, search)
		}
		if query.Select(warehouseFulfillmentColumns).Order("f.created_at DESC,f.id DESC,l.order_id").Offset(offset).Limit(51).Scan(&out.Fulfillments).Error != nil {
			return ErrUnavailable
		}
		out.HasMore = len(out.Fulfillments) > 50
		if out.HasMore {
			out.Fulfillments = out.Fulfillments[:50]
		}
		return nil
	})
	return out, err
}
func (h Handler) ReadWarehouseFulfillment(ctx context.Context, actor identity.User, location, orderID, id string) (WarehouseFulfillmentDetail, error) {
	out := WarehouseFulfillmentDetail{Items: []WarehouseFulfillmentItem{}, Devices: []WarehouseFulfillmentDevice{}, Labels: []ParcelLabel{}}
	if !validPathID(location) || !validPathID(orderID) || !validPathID(id) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, location, authorization.OrderFulfillmentRead); e != nil {
			return e
		}
		if fulfillment.Ready(ctx, tx) != nil {
			return ErrUnavailable
		}
		if e := warehouseFulfillmentQuery(tx, location).Select(warehouseFulfillmentColumns).Where("f.id=? AND o.id=?", id, orderID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&out.WarehouseFulfillment).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return ErrUnavailable
		}
		if tx.Table("fulfillment_items").Select("id,line_item_id,title,sku,CAST(quantity AS CHAR) AS quantity,raw_quantity").Where("fulfillment_id=? AND deleted_at IS NULL", id).Order("id").Limit(1001).Scan(&out.Items).Error != nil || len(out.Items) > 1000 {
			return ErrUnavailable
		}
		for i := range out.Items {
			var raw totals.RawValue
			if json.Unmarshal([]byte(out.Items[i].RawQuantity), &raw) != nil {
				return ErrUnavailable
			}
			value, e := totals.Resolve(totals.Number{Decimal: out.Items[i].Quantity, Raw: &raw})
			if e != nil {
				return ErrUnavailable
			}
			out.Items[i].Quantity = value
		}
		if tx.Table("device_assignments a").Select("d.id,a.item_id,d.serial_number,a.status").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL AND d.location_id=?", location).Where("a.fulfillment_id=? AND a.order_id=? AND a.deleted_at IS NULL", id, orderID).Order("a.id").Limit(1001).Scan(&out.Devices).Error != nil || len(out.Devices) > 1000 {
			return ErrUnavailable
		}
		if tx.Table("fulfillment_labels").Select("id,tracking_number,tracking_url,label_url").Where("fulfillment_id=? AND deleted_at IS NULL", id).Order("id").Limit(101).Scan(&out.Labels).Error != nil || len(out.Labels) > 100 {
			return ErrUnavailable
		}
		return nil
	})
	return out, err
}
func (h Handler) RegisterWarehouseFulfillments(private *gin.RouterGroup) {
	private.GET("/warehouse/locations/:location_id/fulfillments", h.WarehouseFulfillmentsHTTP)
	private.GET("/warehouse/locations/:location_id/orders/:order_id/fulfillments/:fulfillment_id", h.WarehouseFulfillmentsHTTP)
}
func (h Handler) WarehouseFulfillmentsHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	query, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil {
		trackingFail(c, ErrInput)
		return
	}
	if id := c.Param("fulfillment_id"); id != "" {
		if len(query) != 0 {
			trackingFail(c, ErrInput)
			return
		}
		out, e := h.ReadWarehouseFulfillment(ctx, actor, c.Param("location_id"), c.Param("order_id"), id)
		if e != nil {
			trackingFail(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	for k, v := range query {
		if (k != "q" && k != "offset") || len(v) != 1 {
			trackingFail(c, ErrInput)
			return
		}
	}
	offset := 0
	if v, ok := query["offset"]; ok {
		offset, e = strconv.Atoi(v[0])
		if e != nil || strconv.Itoa(offset) != v[0] {
			trackingFail(c, ErrInput)
			return
		}
	}
	out, e := h.ReadWarehouseFulfillments(ctx, actor, c.Param("location_id"), query.Get("q"), offset)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, out)
}
