package devices

import (
	"context"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// A dedicated projection of the shared shipping facts, without full-store
// context, source identities, payments, or candidates from other parcels.
type WarehouseShipment struct {
	OrderID       string `json:"order_id"`
	FulfillmentID string `json:"fulfillment_id"`
	LocationID    string `json:"location_id"`
	Enabled       bool   `json:"enabled"`
	OrderActive   bool   `json:"order_active"`
	Supported     bool   `json:"supported"`
	Status        string `json:"status"`
	Revision      string `json:"revision"`
}

func warehouseShipmentBinding(p ParcelContext, location string) error {
	if p.LocationID != location || p.Status == "canceled" {
		return gorm.ErrRecordNotFound
	}
	for _, d := range p.Devices {
		if d.LocationID != location {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}
func (h Handler) ReadWarehouseShipment(ctx context.Context, actor identity.User, location, o, f string) (WarehouseShipment, error) {
	var out WarehouseShipment
	if !validPathID(location) || !validPathID(o) || !validPathID(f) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := parcelAccessGate(ctx, tx, actor, location)
		if e != nil {
			return e
		}
		p, _, e := loadParcel(tx, o, f, rev, enabled)
		if e != nil {
			return e
		}
		if e = warehouseShipmentBinding(p, location); e != nil {
			return e
		}
		out = WarehouseShipment{OrderID: o, FulfillmentID: f, LocationID: location, Enabled: enabled, OrderActive: p.OrderActive, Supported: p.Supported, Status: p.Status, Revision: p.Revision}
		return nil
	})
	return out, e
}
func (h Handler) WarehouseShipmentHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if c.Request.URL.RawQuery != "" {
		parcelFail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	location, o, f := c.Param("location_id"), c.Param("order_id"), c.Param("fulfillment_id")
	if c.Request.Method == "GET" {
		out, e := h.ReadWarehouseShipment(ctx, actor, location, o, f)
		if e != nil {
			parcelFail(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "devices.content_type", "JSON required")
		return
	}
	in, e := decodeParcel(c.Request.Body)
	if e != nil {
		parcelFail(c, e)
		return
	}
	out, e := h.ChangeWarehouseShipment(ctx, actor, location, o, f, in)
	if e != nil {
		parcelFail(c, e)
		return
	}
	c.JSON(200, out)
}
