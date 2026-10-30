package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReplacementContext is an authorized preparation read. A qualifying device is
// not reserved by reading this response. The exchange command must re-read all
// facts, create the native exchange/return/new item, and reserve atomically.
type ReplacementContext struct {
	OrderID              string              `json:"order_id"`
	RequestID            string              `json:"request_id"`
	OriginalAssignmentID string              `json:"original_assignment_id"`
	OriginalDeviceID     string              `json:"original_device_id"`
	OriginalItemID       string              `json:"original_item_id"`
	VariantID            string              `json:"variant_id"`
	OrderVersion         int                 `json:"order_version"`
	Eligible             bool                `json:"eligible"`
	Reason               string              `json:"reason"`
	WriteAvailable       bool                `json:"write_available"`
	Revision             string              `json:"revision"`
	Devices              []ReplacementDevice `json:"devices"`
}
type ReplacementDevice struct {
	ID           string `json:"id"`
	SerialNumber string `json:"serial_number"`
	LocationID   string `json:"location_id"`
	LocationName string `json:"location_name"`
	SourceID     string `json:"source_id"`
	SourceName   string `json:"source_name"`
}
type replacementOriginal struct {
	ID, ItemID              string
	ReturnID, ReplacementOf *string
}

func replacementReason(service ServiceContext, activeOrder, paid bool, original replacementOriginal, replacements int64) string {
	switch {
	case service.Type != "exchange":
		return "not_exchange"
	case service.RequestStatus != "processing":
		return "not_processing"
	case service.Status != "returned":
		return "original_not_returned"
	case original.ReplacementOf != nil:
		return "not_original"
	case original.ReturnID != nil:
		return "native_return_exists"
	case !activeOrder:
		return "order_inactive"
	case !paid:
		return "payment_required"
	case replacements > 0:
		return "replacement_exists"
	default:
		return "eligible"
	}
}

func (h Handler) ReadReplacement(ctx context.Context, actor identity.User, o, r, a string) (ReplacementContext, error) {
	var out ReplacementContext
	if !validPathID(o) || !validPathID(r) || !validPathID(a) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := serviceAuthority(ctx, tx, actor, false); e != nil {
			return e
		}
		if e := authorization.CheckTx(ctx, tx, actor, authorization.OrderPaymentRead, authorization.OrderAllocationWrite); e != nil {
			return e
		}
		if payments.Ready(ctx, tx) != nil {
			return ErrUnavailable
		}
		service, _, e := loadService(ctx, tx, o, r, a)
		if e != nil {
			return e
		}
		var original replacementOriginal
		if e = tx.Table("device_assignments").Select("id,item_id,return_id,replacement_of").Where("id=? AND order_id=? AND after_sales_request_id=? AND deleted_at IS NULL", a, o, r).Clauses(clause.Locking{Strength: "SHARE"}).Take(&original).Error; e != nil {
			return e
		}
		// The order line's SKU is authoritative even after inspection has changed
		// the original machine's SKU. Ordinary remaining-to-fulfil capacity does
		// not restrict exchange eligibility (the original has already shipped).
		allocation, e := loadAllocation(tx, o, original.ItemID, 0, false)
		if e != nil {
			return e
		}
		var replacements []struct {
			ID, DeviceID, Status string
			UpdatedAt            time.Time
		}
		if tx.Table("device_assignments").Select("id,device_id,status,updated_at").Where("replacement_of=? AND status IN ('reserved','packed','shipped') AND deleted_at IS NULL", a).Order("id").Limit(1001).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&replacements).Error != nil || len(replacements) > 1000 {
			return ErrUnavailable
		}
		reason := replacementReason(service, allocation.OrderActive, allocation.Paid, original, int64(len(replacements)))
		out = ReplacementContext{OrderID: o, RequestID: r, OriginalAssignmentID: a, OriginalDeviceID: service.DeviceID, OriginalItemID: original.ItemID, VariantID: allocation.VariantID, OrderVersion: allocation.Version, Eligible: reason == "eligible", Reason: reason, Devices: []ReplacementDevice{}}
		if out.Eligible {
			for _, d := range allocation.Devices {
				if d.ID == service.DeviceID {
					continue
				}
				out.Devices = append(out.Devices, ReplacementDevice{ID: d.ID, SerialNumber: d.SerialNumber, LocationID: d.LocationID, LocationName: d.LocationName, SourceID: d.SourceID, SourceName: d.SourceName})
			}
		}
		data, _ := json.Marshal([]any{service.Revision, allocation.Revision, original, replacements})
		digest := sha256.Sum256(data)
		out.Revision = hex.EncodeToString(digest[:])
		return nil
	})
	return out, e
}

func (h Handler) ReplacementHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	var out ReplacementContext
	var e error
	if c.Request.Method != http.MethodGet || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		e = ErrInput
	} else {
		out, e = h.ReadReplacement(ctx, actor, c.Param("id"), c.Param("request_id"), c.Param("assignment_id"))
	}
	if e == nil {
		c.JSON(200, out)
		return
	}
	status := 503
	switch {
	case errors.Is(e, ErrInput):
		status = 400
	case errors.Is(e, authorization.ErrDenied):
		status = 403
	case errors.Is(e, gorm.ErrRecordNotFound):
		status = 404
	case errors.Is(e, ErrConflict):
		status = 409
	}
	platform.Fail(c, status, "devices.replacement_unavailable", "Replacement preparation unavailable; verify the original device and current permission")
}
