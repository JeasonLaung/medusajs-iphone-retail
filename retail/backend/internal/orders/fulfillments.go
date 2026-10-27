package orders

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FulfillmentEvidence projects recorded shipments, never operational authority.
// A surviving order link to a missing/deleted shipment remains visible as such.
type FulfillmentEvidence struct {
	OrderID           string            `json:"order_id"`
	Version           int               `json:"version"`
	OperationsEnabled bool              `json:"operations_enabled"`
	Fulfillments      []FulfillmentLink `json:"fulfillments"`
}
type FulfillmentLink struct {
	ID           string             `json:"id"`
	Availability string             `json:"availability"`
	Record       *FulfillmentRecord `json:"record"`
}
type FulfillmentRecord struct {
	LocationID        string              `json:"location_id"`
	ProviderID        *string             `json:"provider_id"`
	ShippingOptionID  *string             `json:"shipping_option_id"`
	DeliveryAddressID *string             `json:"delivery_address_id"`
	RequiresShipping  bool                `json:"requires_shipping"`
	CreatedAt         time.Time           `json:"created_at"`
	PackedAt          *time.Time          `json:"packed_at"`
	ShippedAt         *time.Time          `json:"shipped_at"`
	DeliveredAt       *time.Time          `json:"delivered_at"`
	CanceledAt        *time.Time          `json:"canceled_at"`
	Address           *FulfillmentAddress `json:"address"`
	Items             []FulfillmentItem   `json:"items"`
	Labels            []FulfillmentLabel  `json:"labels"`
}
type FulfillmentAddress struct {
	ID          string  `json:"id"`
	Company     *string `json:"company"`
	FirstName   *string `json:"first_name"`
	LastName    *string `json:"last_name"`
	Address1    *string `json:"address_1" gorm:"column:address_1"`
	Address2    *string `json:"address_2" gorm:"column:address_2"`
	City        *string `json:"city"`
	CountryCode *string `json:"country_code"`
	Province    *string `json:"province"`
	PostalCode  *string `json:"postal_code"`
	Phone       *string `json:"phone"`
}
type FulfillmentItem struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	SKU             string  `json:"sku"`
	Barcode         string  `json:"barcode"`
	Quantity        string  `json:"quantity"`
	LineItemID      *string `json:"line_item_id"`
	InventoryItemID *string `json:"inventory_item_id"`
}
type FulfillmentLabel struct {
	ID             string `json:"id"`
	TrackingNumber string `json:"tracking_number"`
	TrackingURL    string `json:"tracking_url"`
	LabelURL       string `json:"label_url"`
}

func (h Handler) Fulfillments(c *gin.Context) {
	id := c.Param("id")
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 || strings.ContainsRune(id, 0) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "orders.invalid_query", "Invalid order request")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "orders.fulfillments_unavailable", "Order fulfillments unavailable")
		return
	}
	var result FulfillmentEvidence
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if Ready(c.Request.Context(), tx) != nil || fulfillment.Ready(c.Request.Context(), tx) != nil {
			return ErrSchema
		}
		var order struct {
			ID      string
			Version int
		}
		if err := tx.Table("orders").Select("id,version").Where("id=? AND deleted_at IS NULL", id).Take(&order).Error; err != nil {
			return err
		}
		result = FulfillmentEvidence{OrderID: id, Version: order.Version, Fulfillments: []FulfillmentLink{}}
		var links []struct{ FulfillmentID string }
		if tx.Table("order_fulfillments").Select("fulfillment_id").Where("order_id=? AND deleted_at IS NULL", id).Order("fulfillment_id ASC").Limit(101).Scan(&links).Error != nil || len(links) > 100 {
			return ErrSchema
		}
		if len(links) == 0 {
			return nil
		}
		ids := []string{}
		positions := map[string]int{}
		for _, link := range links {
			positions[link.FulfillmentID] = len(result.Fulfillments)
			result.Fulfillments = append(result.Fulfillments, FulfillmentLink{ID: link.FulfillmentID, Availability: "missing"})
			ids = append(ids, link.FulfillmentID)
		}
		var rows []struct {
			ID, LocationID                                          string
			ProviderID, ShippingOptionID, DeliveryAddressID         *string
			RequiresShipping                                        bool
			CreatedAt                                               time.Time
			PackedAt, ShippedAt, DeliveredAt, CanceledAt, DeletedAt *time.Time
		}
		if tx.Table("fulfillments").Select("id,location_id,provider_id,shipping_option_id,delivery_address_id,requires_shipping,created_at,packed_at,shipped_at,delivered_at,canceled_at,deleted_at").Where("id IN ?", ids).Order("id ASC").Scan(&rows).Error != nil {
			return ErrSchema
		}
		activeIDs, addressIDs := []string{}, []string{}
		for _, row := range rows {
			n, ok := positions[row.ID]
			if !ok {
				return ErrSchema
			}
			link := &result.Fulfillments[n]
			if row.DeletedAt != nil {
				link.Availability = "deleted"
				continue
			}
			link.Availability = "available"
			link.Record = &FulfillmentRecord{LocationID: row.LocationID, ProviderID: row.ProviderID, ShippingOptionID: row.ShippingOptionID, DeliveryAddressID: row.DeliveryAddressID, RequiresShipping: row.RequiresShipping, CreatedAt: row.CreatedAt, PackedAt: row.PackedAt, ShippedAt: row.ShippedAt, DeliveredAt: row.DeliveredAt, CanceledAt: row.CanceledAt, Items: []FulfillmentItem{}, Labels: []FulfillmentLabel{}}
			activeIDs = append(activeIDs, row.ID)
			if row.DeliveryAddressID != nil {
				addressIDs = append(addressIDs, *row.DeliveryAddressID)
			}
		}
		if len(activeIDs) == 0 {
			return nil
		}
		if len(addressIDs) > 0 {
			var addresses []FulfillmentAddress
			if tx.Table("fulfillment_addresses").Select("id,company,first_name,last_name,address_1,address_2,city,country_code,province,postal_code,phone").Where("id IN ? AND deleted_at IS NULL", addressIDs).Scan(&addresses).Error != nil {
				return ErrSchema
			}
			for _, address := range addresses {
				for i := range result.Fulfillments {
					record := result.Fulfillments[i].Record
					if record != nil && record.DeliveryAddressID != nil && *record.DeliveryAddressID == address.ID {
						copy := address
						record.Address = &copy
					}
				}
			}
		}
		var items []struct {
			ID, FulfillmentID, Title, SKU, Barcode, Quantity, RawQuantity string
			LineItemID, InventoryItemID                                   *string
		}
		if tx.Table("fulfillment_items").Select("id,fulfillment_id,title,sku,barcode,CAST(quantity AS CHAR) AS quantity,raw_quantity,line_item_id,inventory_item_id").Where("fulfillment_id IN ? AND deleted_at IS NULL", activeIDs).Order("id ASC").Limit(1001).Scan(&items).Error != nil || len(items) > 1000 {
			return ErrSchema
		}
		for _, row := range items {
			quantity, err := requiredPaymentAmount(row.Quantity, row.RawQuantity)
			if err != nil {
				return ErrSchema
			}
			n, ok := positions[row.FulfillmentID]
			if !ok || result.Fulfillments[n].Record == nil {
				return ErrSchema
			}
			record := result.Fulfillments[n].Record
			record.Items = append(record.Items, FulfillmentItem{ID: row.ID, Title: row.Title, SKU: row.SKU, Barcode: row.Barcode, Quantity: quantity, LineItemID: row.LineItemID, InventoryItemID: row.InventoryItemID})
		}
		var labels []struct{ ID, FulfillmentID, TrackingNumber, TrackingURL, LabelURL string }
		if tx.Table("fulfillment_labels").Select("id,fulfillment_id,tracking_number,tracking_url,label_url").Where("fulfillment_id IN ? AND deleted_at IS NULL", activeIDs).Order("id ASC").Limit(1001).Scan(&labels).Error != nil || len(labels) > 1000 {
			return ErrSchema
		}
		for _, row := range labels {
			n, ok := positions[row.FulfillmentID]
			if !ok || result.Fulfillments[n].Record == nil {
				return ErrSchema
			}
			record := result.Fulfillments[n].Record
			record.Labels = append(record.Labels, FulfillmentLabel{ID: row.ID, TrackingNumber: row.TrackingNumber, TrackingURL: row.TrackingURL, LabelURL: row.LabelURL})
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "orders.not_found", "Order not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "orders.fulfillments_unavailable", "Order fulfillments unavailable")
		return
	}
	c.JSON(200, result)
}
