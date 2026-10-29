package inventory

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WarehouseLevel struct {
	ID                string    `json:"id"`
	InventoryItemID   string    `json:"inventory_item_id"`
	SKU               *string   `json:"sku"`
	Title             *string   `json:"title"`
	LocationID        string    `json:"location_id"`
	StockedQuantity   string    `json:"stocked_quantity"`
	ReservedQuantity  string    `json:"reserved_quantity"`
	IncomingQuantity  string    `json:"incoming_quantity"`
	AvailableQuantity string    `json:"available_quantity" gorm:"-"`
	Revision          string    `json:"revision" gorm:"-"`
	UpdatedAt         time.Time `json:"-"`
}
type WarehouseInventory struct {
	LocationID    string           `json:"location_id"`
	Levels        []WarehouseLevel `json:"levels"`
	Offset        int              `json:"offset"`
	Limit         int              `json:"limit"`
	HasMore       bool             `json:"has_more"`
	WritesEnabled bool             `json:"writes_enabled"`
}

func (h Handler) ReadWarehouse(ctx context.Context, actor identity.User, locationID string, offset int) (WarehouseInventory, error) {
	out := WarehouseInventory{LocationID: locationID, Offset: offset, Limit: 50, Levels: []WarehouseLevel{}}
	if !validID(locationID) || offset < 0 || offset > 100000 {
		return out, errStockInput
	}
	if Ready(ctx, h.DB) != nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, locationID, authorization.InventoryRead); e != nil {
			return e
		}
		var cap struct {
			Version int
			Enabled bool
		}
		if tx.Table("inventory_command_capabilities").Clauses(clause.Locking{Strength: "SHARE"}).Where("command='inventory.edit_stock'").Take(&cap).Error != nil || cap.Version != 1 {
			return ErrSchema
		}
		out.WritesEnabled = cap.Enabled
		// No product, variant, supplier, price, cost or reservation-owner projection.
		e := tx.Table("inventory_levels l").Select("l.id,l.inventory_item_id,i.sku,i.title,l.location_id,l.stocked_quantity,l.reserved_quantity,l.incoming_quantity,l.updated_at").Joins("JOIN inventory_items i ON i.id=l.inventory_item_id AND i.deleted_at IS NULL").Where("l.location_id=? AND l.deleted_at IS NULL", locationID).Order("l.id ASC").Offset(offset).Limit(51).Scan(&out.Levels).Error
		if e != nil {
			return e
		}
		if len(out.Levels) > 50 {
			out.HasMore = true
			out.Levels = out.Levels[:50]
		}
		for i := range out.Levels {
			l := &out.Levels[i]
			var e error
			l.AvailableQuantity, e = Difference(l.StockedQuantity, l.ReservedQuantity)
			if e != nil {
				return e
			}
			l.Revision = stockRevision(stockState{l.ID, l.InventoryItemID, l.LocationID, l.StockedQuantity, l.ReservedQuantity, l.IncomingQuantity, l.UpdatedAt})
		}
		return nil
	})
	return out, e
}
func (h Handler) WarehouseInventory(c *gin.Context) {
	query, e := url.ParseQuery(c.Request.URL.RawQuery)
	offset := 0
	if e != nil || len(query) > 1 || len(query["offset"]) > 1 {
		platform.Fail(c, 400, "inventory.invalid_query", "Invalid inventory query")
		return
	}
	for key := range query {
		if key != "offset" {
			platform.Fail(c, 400, "inventory.invalid_query", "Invalid inventory query")
			return
		}
	}
	if value, ok := query["offset"]; ok {
		offset, e = strconv.Atoi(value[0])
		if e != nil || offset < 0 || offset > 100000 || strconv.Itoa(offset) != value[0] {
			platform.Fail(c, 400, "inventory.invalid_query", "Invalid offset")
			return
		}
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.ReadWarehouse(ctx, actor, c.Param("location_id"), offset)
	switch {
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Action not permitted")
	case errors.Is(e, errStockInput):
		platform.Fail(c, 400, "inventory.invalid_query", "Invalid inventory query")
	case e != nil:
		platform.Fail(c, 503, "inventory.unavailable", "Inventory unavailable")
	default:
		c.JSON(200, out)
	}
}
