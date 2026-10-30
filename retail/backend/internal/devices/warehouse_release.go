package devices

import (
	"context"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type WarehouseRelease struct {
	WarehouseShipment
	Devices []WarehouseAllocationDevice `json:"devices"`
}

func (h Handler) ReadWarehouseRelease(ctx context.Context, actor identity.User, location, o, f string) (WarehouseRelease, error) {
	var out WarehouseRelease
	if !validPathID(location) || !validPathID(o) || !validPathID(f) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := parcelAccessFor(ctx, tx, actor, location, authorization.WarehouseAllocationWrite)
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
		out.WarehouseShipment = WarehouseShipment{OrderID: o, FulfillmentID: f, LocationID: location, Enabled: enabled, OrderActive: p.OrderActive, Supported: p.Supported, Status: p.Status, Revision: p.Revision}
		out.Devices = []WarehouseAllocationDevice{}
		for _, d := range p.Devices {
			out.Devices = append(out.Devices, WarehouseAllocationDevice{ID: d.DeviceID, SerialNumber: d.SerialNumber, AssignmentID: d.AssignmentID})
		}
		return nil
	})
	return out, e
}
func (h Handler) WarehouseReleaseHTTP(c *gin.Context) {
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
		out, e := h.ReadWarehouseRelease(ctx, actor, location, o, f)
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
	out, e := h.ReleaseWarehouseParcelDevice(ctx, actor, location, o, f, c.Param("assignment_id"), in)
	if e != nil {
		parcelFail(c, e)
		return
	}
	c.JSON(200, out)
}
