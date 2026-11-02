package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WarehouseInspection struct {
	Device  WarehouseDevice `json:"device"`
	Repair  bool            `json:"repair"`
	Enabled bool            `json:"enabled"`
}

func warehouseInspectionEnabled(tx *gorm.DB) (bool, error) {
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("device_service_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&gate).Error != nil || gate.Version != 1 {
		return false, ErrUnavailable
	}
	return gate.Enabled, nil
}

// Scope and current repair status participate in the revision. No order/customer
// identifiers or original private notes are projected to a warehouse account.
func loadWarehouseInspection(tx *gorm.DB, location, device string, revision uint64) (WarehouseInspection, string, error) {
	var out WarehouseInspection
	if e := warehouseDeviceQuery(tx).Where("d.id=? AND d.location_id=? AND d.deleted_at IS NULL", device, location).Take(&out.Device).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return out, "", ErrNotFound
		}
		return out, "", e
	}
	enabled, e := warehouseInspectionEnabled(tx)
	if e != nil {
		return out, "", e
	}
	out.Enabled = enabled
	var assignment struct {
		ID, OrderID string
		RequestID   *string
	}
	e = tx.Table("device_assignments").Select("id,order_id,after_sales_request_id AS request_id").Where("device_id=? AND status='returned' AND deleted_at IS NULL", device).Order("created_at DESC,id DESC").Take(&assignment).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, "", ErrUnavailable
	}
	var request struct{ Type, Status string }
	if assignment.RequestID != nil {
		if tx.Table("after_sales_requests").Select("type,status").Where("id=? AND deleted_at IS NULL", *assignment.RequestID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&request).Error != nil {
			return out, "", ErrConflict
		}
		out.Repair = request.Type == "repair" && request.Status == "processing"
	}
	if e := projectWarehouseDevice(&out.Device, revision); e != nil {
		return out, "", e
	}
	b, _ := json.Marshal([]any{out.Device.Revision, location, "returned", assignment, request})
	sum := sha256.Sum256(b)
	out.Device.Revision = hex.EncodeToString(sum[:])
	out.Device.AllowedActions = []string{}
	return out, assignment.OrderID, nil
}

func (h Handler) ReadWarehouseInspection(ctx context.Context, actor identity.User, location, device string) (WarehouseInspection, error) {
	var out WarehouseInspection
	if !validPathID(location) || !validPathID(device) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actions, _ := warehouseDeviceActions("returned", false)
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, location, actions...); e != nil {
			return e
		}
		rev, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		out, _, e = loadWarehouseInspection(tx, location, device, rev)
		if e == nil && out.Device.Category != "returned" {
			return ErrNotFound
		}
		return e
	})
	return out, e
}

func (h Handler) InspectWarehouseDevice(ctx context.Context, actor identity.User, location, device string, in ServiceInput) (WarehouseDeviceResult, error) {
	var out WarehouseDeviceResult
	if !validPathID(location) || !validPathID(device) || !validService(in) || in.Action != "inspect" {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	wire, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actions, _ := warehouseDeviceActions("returned", true)
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, location, actions...); e != nil {
			return e
		}
		rev, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		before, order, e := loadWarehouseInspection(tx, location, device, rev)
		if e != nil {
			return e
		}
		if !before.Enabled {
			return ErrUnavailable
		}
		var prior struct{ ActorID, LocationID, Category, DeviceID, RequestJSON, ResultJSON string }
		e = tx.Table("warehouse_device_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.LocationID != location || prior.Category != "returned" || prior.DeviceID != device || prior.RequestJSON != string(wire) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.ID != device {
				return ErrUnavailable
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		d := before.Device
		if d.Category != "returned" || (d.State != "inspection" && d.State != "repair") || d.HasActiveAssignment || d.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		var product string
		if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", in.VariantID).Scan(&product).Error != nil || product == "" {
			return ErrConflict
		}
		sku, e := intakeSKU(tx, product, in.VariantID)
		if e != nil {
			return e
		}
		var sources int64
		if tx.Table("device_sources").Where("location_id=? AND deleted_at IS NULL", location).Count(&sources).Error != nil || sources == 0 {
			return ErrConflict
		}
		if e := verifyPhotosTx(tx, in.Photos); e != nil {
			return e
		}
		state, category := serviceState(in.Result, before.Repair)
		if category == "" {
			category = d.Category
		}
		if in.Result == "pass" {
			if validateStoredConfiguration(sku.Options, Configuration{Grade: &in.Grade, BatteryHealth: &in.BatteryHealth}) != nil || before.Repair && in.VariantID != d.VariantID {
				return ErrConflict
			}
			id := Identity{SerialNumber: d.SerialNumber}
			if d.IMEI != nil {
				id.IMEI = *d.IMEI
			}
			if d.IMEI2 != nil {
				id.IMEI2 = *d.IMEI2
			}
			if d.EID != nil {
				id.EID = *d.EID
			}
			if e := ClaimIdentityTx(ctx, tx, d.ID, id, sku.RequiresRadio); e != nil {
				return e
			}
		}
		if in.Result == "supplier_return" {
			var source struct{ Type string }
			if tx.Table("device_sources").Select("type").Where("id=? AND deleted_at IS NULL", d.SourceID).Take(&source).Error != nil || source.Type != "supplier" {
				return ErrConflict
			}
		}
		if tx.Exec("UPDATE devices SET variant_id=?,state=?,category=?,grade=?,battery_health=?,notes=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", in.VariantID, state, category, in.Grade, in.BatteryHealth, in.Notes, device).Error != nil {
			return ErrUnavailable
		}
		data, _ := json.Marshal(map[string]any{"result": in.Result, "notes": in.Notes, "photos": in.Photos, "before": map[string]any{"state": d.State, "variant_id": d.VariantID, "grade": d.Grade, "battery_health": d.BatteryHealth}, "after": map[string]any{"state": state, "variant_id": in.VariantID, "grade": in.Grade, "battery_health": in.BatteryHealth}})
		var orderID any
		if order != "" {
			orderID = order
		}
		if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), device, orderID, actor.ID, "inspect", string(data)).Error != nil {
			return ErrUnavailable
		}
		out = WarehouseDeviceResult{ID: device, State: state}
		result, _ := json.Marshal(out)
		if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil || tx.Exec("INSERT INTO warehouse_device_changes(operation_id,actor_id,location_id,category,device_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, location, "returned", device, string(wire), string(result)).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}

func (h Handler) WarehouseInspectionHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	var e error
	if c.Param("category") != "returned" || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		e = ErrInput
	} else if c.Request.Method == "GET" {
		var out WarehouseInspection
		out, e = h.ReadWarehouseInspection(ctx, actor, c.Param("location_id"), c.Param("device_id"))
		if e == nil {
			c.JSON(200, out)
			return
		}
	} else {
		media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || media != "application/json" {
			platform.Fail(c, 415, "devices.content_type", "JSON required")
			return
		}
		var in ServiceInput
		in, e = decodeService(c.Request.Body)
		if e == nil {
			var out WarehouseDeviceResult
			out, e = h.InspectWarehouseDevice(ctx, actor, c.Param("location_id"), c.Param("device_id"), in)
			if e == nil {
				c.JSON(200, out)
				return
			}
		}
	}
	trackingFail(c, e)
}
