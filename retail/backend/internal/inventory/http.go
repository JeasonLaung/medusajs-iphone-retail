package inventory

import (
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct{ DB *gorm.DB }
type Level struct {
	ID                  string  `json:"id"`
	LocationID          string  `json:"location_id"`
	LocationName        *string `json:"location_name"`
	StockedQuantity     string  `json:"stocked_quantity"`
	ReservedQuantity    string  `json:"reserved_quantity"`
	IncomingQuantity    string  `json:"incoming_quantity"`
	AvailableQuantity   string  `json:"available_quantity" gorm:"-"`
	ReservationQuantity string  `json:"reservation_quantity" gorm:"-"`
	ReservationCount    int64   `json:"reservation_count" gorm:"-"`
	Reconciled          bool    `json:"reconciled" gorm:"-"`
}
type UnmatchedReservation struct {
	LocationID string `json:"location_id"`
	Quantity   string `json:"quantity"`
	Count      int64  `json:"count"`
}
type Item struct {
	InventoryItemID       string                 `json:"inventory_item_id"`
	RequiredQuantity      string                 `json:"required_quantity"`
	SKU                   *string                `json:"sku"`
	Title                 *string                `json:"title"`
	ItemAvailable         bool                   `json:"item_available"`
	Levels                []Level                `json:"levels" gorm:"-"`
	UnmatchedReservations []UnmatchedReservation `json:"unmatched_reservations" gorm:"-"`
}

var errLimit = errors.New("inventory relation limit exceeded")

func validID(s string) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 191 && !strings.ContainsFunc(s, func(r rune) bool { return r < 32 || r == 127 })
}

func (h Handler) AdminVariant(c *gin.Context) {
	productID, variantID := c.Param("id"), c.Param("variant_id")
	if !validID(productID) || !validID(variantID) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "inventory.invalid_query", "Invalid inventory query")
		return
	}
	if Ready(c.Request.Context(), h.DB) != nil {
		platform.Fail(c, 503, "inventory.unavailable", "Inventory unavailable")
		return
	}
	items := make([]Item, 0)
	var variant struct {
		ID              string
		ManageInventory bool
		AllowBackorder  bool
	}
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("catalog_variants v").Select("v.id,v.manage_inventory,v.allow_backorder").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("v.id=? AND v.product_id=? AND v.deleted_at IS NULL", variantID, productID).Take(&variant).Error; err != nil {
			return err
		}
		if err := tx.Table("catalog_variant_inventory_items vi").Select("vi.inventory_item_id,vi.required_quantity,i.sku,i.title,(i.id IS NOT NULL) AS item_available").Joins("LEFT JOIN inventory_items i ON i.id=vi.inventory_item_id AND i.deleted_at IS NULL").Where("vi.variant_id=? AND vi.deleted_at IS NULL", variantID).Order("vi.inventory_item_id ASC").Limit(1001).Scan(&items).Error; err != nil {
			return err
		}
		if len(items) > 1000 {
			return errLimit
		}
		if len(items) == 0 {
			return nil
		}
		ids := make([]string, 0, len(items))
		positions := map[string]int{}
		for i := range items {
			items[i].Levels = []Level{}
			items[i].UnmatchedReservations = []UnmatchedReservation{}
			ids = append(ids, items[i].InventoryItemID)
			positions[items[i].InventoryItemID] = i
		}
		var levels []struct {
			Level
			InventoryItemID string
		}
		if err := tx.Table("inventory_levels l").Select("l.id,l.inventory_item_id,l.location_id,l.stocked_quantity,l.reserved_quantity,l.incoming_quantity,s.name AS location_name").Joins("LEFT JOIN inventory_locations s ON s.id=l.location_id AND s.deleted_at IS NULL").Where("l.inventory_item_id IN ? AND l.deleted_at IS NULL", ids).Order("l.inventory_item_id,l.location_id,l.id").Limit(10001).Scan(&levels).Error; err != nil {
			return err
		}
		if len(levels) > 10000 {
			return errLimit
		}
		var sums []struct {
			InventoryItemID, LocationID, Quantity string
			Count                                 int64
		}
		if err := tx.Table("inventory_reservations").Select("inventory_item_id,location_id,SUM(quantity) AS quantity,COUNT(*) AS count").Where("inventory_item_id IN ? AND deleted_at IS NULL", ids).Group("inventory_item_id,location_id").Order("inventory_item_id,location_id").Limit(10001).Scan(&sums).Error; err != nil {
			return err
		}
		if len(sums) > 10000 {
			return errLimit
		}
		type key struct{ item, location string }
		totals := map[key]int{}
		matched := map[key]bool{}
		for i, r := range sums {
			totals[key{r.InventoryItemID, r.LocationID}] = i
		}
		for _, row := range levels {
			l := row.Level
			l.ReservationQuantity = "0.000000000000000000"
			k := key{row.InventoryItemID, l.LocationID}
			if i, ok := totals[k]; ok {
				l.ReservationQuantity = sums[i].Quantity
				l.ReservationCount = sums[i].Count
				matched[k] = true
			}
			var err error
			l.AvailableQuantity, err = Difference(l.StockedQuantity, l.ReservedQuantity)
			if err != nil {
				return err
			}
			l.Reconciled, err = Equal(l.ReservedQuantity, l.ReservationQuantity)
			if err != nil {
				return err
			}
			i := positions[row.InventoryItemID]
			items[i].Levels = append(items[i].Levels, l)
		}
		for _, r := range sums {
			if !matched[key{r.InventoryItemID, r.LocationID}] {
				i := positions[r.InventoryItemID]
				items[i].UnmatchedReservations = append(items[i].UnmatchedReservations, UnmatchedReservation{r.LocationID, r.Quantity, r.Count})
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "inventory.not_found", "Product variant not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "inventory.unavailable", "Inventory unavailable")
		return
	}
	c.JSON(200, gin.H{"product_id": productID, "variant_id": variantID, "manage_inventory": variant.ManageInventory, "allow_backorder": variant.AllowBackorder, "items": items, "writes_enabled": false})
}
