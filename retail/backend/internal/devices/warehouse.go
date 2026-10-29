package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
)

type WarehouseDevice struct {
	AllowedActions  []string `json:"allowed_actions" gorm:"-"`
	RecentDevice    `gorm:"embedded"`
	VariantID       string     `json:"variant_id"`
	VariantTitle    *string    `json:"variant_title"`
	SourceID        string     `json:"source_id"`
	Bin             string     `json:"bin"`
	Revision        string     `json:"revision" gorm:"-"`
	HasSalesHistory bool       `json:"-"`
	SelfSource      bool       `json:"-"`
	UpdatedAt       time.Time  `json:"-"`
	DeletedAt       *time.Time `json:"-"`
}
type WarehouseDevices struct {
	LocationID string            `json:"location_id"`
	Category   string            `json:"category"`
	Devices    []WarehouseDevice `json:"devices"`
	Offset     int               `json:"offset"`
	Limit      int               `json:"limit"`
	HasMore    bool              `json:"has_more"`
}
type WarehouseDeviceCommand struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	Action           string `json:"action"`
}
type WarehouseDeviceResult struct {
	ID       string `json:"id"`
	State    string `json:"state"`
	Deleted  bool   `json:"deleted"`
	Replayed bool   `json:"replayed"`
}

func warehouseDeviceActions(category string, write bool) ([]authorization.Action, error) {
	var actions []authorization.Action
	switch category {
	case "sale":
		actions = []authorization.Action{authorization.WarehouseIntakeRead}
		if write {
			actions = append(actions, authorization.WarehouseIntakeWrite)
		}
	case "returned":
		actions = []authorization.Action{authorization.WarehouseReturnRead}
		if write {
			actions = append(actions, authorization.WarehouseReturnWrite)
		}
	default:
		return nil, ErrInput
	}
	return actions, nil
}
func warehouseDeviceQuery(tx *gorm.DB) *gorm.DB {
	return tx.Table("devices d").Select("d.id,d.serial_number,d.imei,d.imei2,d.eid,d.variant_id,v.title AS variant_title,d.source_id,d.location_id,d.bin,d.category,d.state,d.grade,d.battery_health,d.photos,d.created_at,d.updated_at,d.deleted_at,COALESCE(s.type='self' AND s.deleted_at IS NULL,FALSE) AS self_source,EXISTS(SELECT 1 FROM device_assignments a WHERE a.device_id=d.id AND a.deleted_at IS NULL AND a.status IN ('reserved','packed','shipped','returned')) AS has_sales_history").Joins("LEFT JOIN catalog_variants v ON v.id=d.variant_id AND v.deleted_at IS NULL").Joins("LEFT JOIN device_sources s ON s.id=d.source_id")
}
func projectWarehouseDevice(row *WarehouseDevice, revision uint64) error {
	if json.Unmarshal([]byte(row.PhotoJSON), &row.Photos) != nil || row.Photos == nil || len(row.Photos) > 8 {
		return ErrUnavailable
	}
	for _, p := range row.Photos {
		if !validPhotoRef(p) {
			return ErrUnavailable
		}
	}
	b, _ := json.Marshal([]any{row.ID, row.SerialNumber, row.IMEI, row.IMEI2, row.EID, row.VariantID, row.SourceID, row.LocationID, row.Bin, row.Category, row.State, row.Grade, row.BatteryHealth, row.Photos, row.UpdatedAt, row.DeletedAt, revision})
	sum := sha256.Sum256(b)
	row.Revision = hex.EncodeToString(sum[:])
	return nil
}
func normalizeWarehouseSearch(q string) (string, error) {
	q = strings.TrimSpace(q)
	if !intakeText(q, 100) {
		return "", ErrInput
	}
	return cases.Upper(language.Und).String(q), nil
}
func (h Handler) ReadWarehouseDevices(ctx context.Context, actor identity.User, locationID, category, search string, offset int) (WarehouseDevices, error) {
	out := WarehouseDevices{LocationID: locationID, Category: category, Offset: offset, Limit: 50, Devices: []WarehouseDevice{}}
	actions, e := warehouseDeviceActions(category, false)
	if e != nil || !validPathID(locationID) || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	q, e := normalizeWarehouseSearch(search)
	if e != nil {
		return out, e
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, locationID, actions...); e != nil {
			return e
		}
		revision, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		query := warehouseDeviceQuery(tx).Where("d.location_id=? AND d.category=? AND d.deleted_at IS NULL", locationID, category)
		if q != "" {
			query = query.Where("(d.serial_number=? OR d.imei=? OR d.imei2=? OR d.eid=?)", q, q, q, q)
		}
		if query.Order("d.id ASC").Offset(offset).Limit(51).Scan(&out.Devices).Error != nil {
			return ErrUnavailable
		}
		if len(out.Devices) > 50 {
			out.HasMore = true
			out.Devices = out.Devices[:50]
		}
		writeActions, _ := warehouseDeviceActions(category, true)
		writeErr := authorization.CheckWarehouseTx(ctx, tx, actor, locationID, writeActions...)
		if writeErr != nil && !errors.Is(writeErr, authorization.ErrDenied) {
			return writeErr
		}
		for i := range out.Devices {
			row := &out.Devices[i]
			if e := projectWarehouseDevice(row, revision); e != nil {
				return e
			}
			row.AllowedActions = []string{}
			if writeErr == nil && !row.HasSalesHistory {
				switch row.State {
				case "reserved", "packed", "shipped", "repair_ready":
				default:
					row.AllowedActions = append(row.AllowedActions, "retire", "delete")
				}
				if category == "sale" && row.SelfSource {
					if row.State == "inspection" {
						row.AllowedActions = append(row.AllowedActions, "activate")
					}
					if row.State == "pending" {
						row.AllowedActions = append(row.AllowedActions, "make_available")
					}
				}
			}
		}
		return nil
	})
	return out, e
}
func validWarehouseCommand(in WarehouseDeviceCommand) bool {
	if validateEnroll(EnrollInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision}) != nil {
		return false
	}
	switch in.Action {
	case "activate", "make_available", "retire", "delete":
		return true
	}
	return false
}
func (h Handler) CommandWarehouseDevice(ctx context.Context, actor identity.User, locationID, category, deviceID string, in WarehouseDeviceCommand) (WarehouseDeviceResult, error) {
	var out WarehouseDeviceResult
	actions, e := warehouseDeviceActions(category, true)
	if e != nil || !validPathID(locationID) || !validPathID(deviceID) || !validWarehouseCommand(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, locationID, actions...); e != nil {
			return e
		}
		revision, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		var row WarehouseDevice
		e = warehouseDeviceQuery(tx).Where("d.id=? AND d.location_id=? AND d.category=?", deviceID, locationID, category).Take(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if e != nil {
			return ErrUnavailable
		}
		// Current object binding is checked before replay too, including soft-deleted rows.
		var prior struct{ ActorID, LocationID, Category, DeviceID, RequestJSON, ResultJSON string }
		e = tx.Table("warehouse_device_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.LocationID != locationID || prior.Category != category || prior.DeviceID != deviceID || prior.RequestJSON != string(request) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.ID != deviceID {
				return ErrUnavailable
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if row.DeletedAt != nil {
			return ErrNotFound
		}
		if e := projectWarehouseDevice(&row, revision); e != nil {
			return e
		}
		if row.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		if row.HasSalesHistory {
			return ErrConflict
		}
		state := row.State
		switch in.Action {
		case "activate", "make_available":
			previous := "inspection"
			if in.Action == "make_available" {
				previous = "pending"
			}
			if category != "sale" || !row.SelfSource || row.State != previous {
				return ErrConflict
			}
			var productID string
			if tx.Table("catalog_variants").Where("id=? AND deleted_at IS NULL", row.VariantID).Select("product_id").Scan(&productID).Error != nil || productID == "" {
				return ErrConflict
			}
			sku, e := intakeSKU(tx, productID, row.VariantID)
			if e != nil {
				return e
			}
			id := Identity{SerialNumber: row.SerialNumber}
			if row.IMEI != nil {
				id.IMEI = *row.IMEI
			}
			if row.IMEI2 != nil {
				id.IMEI2 = *row.IMEI2
			}
			if row.EID != nil {
				id.EID = *row.EID
			}
			normalized, e := NormalizeIdentity(id, sku.RequiresRadio)
			if e != nil || normalized != id {
				return ErrInput
			}
			if e := validateStoredConfiguration(sku.Options, Configuration{row.Grade, row.BatteryHealth}); e != nil {
				return e
			}
			if e := ClaimIdentityTx(ctx, tx, row.ID, id, sku.RequiresRadio); e != nil {
				return e
			}
			state = "available"
		case "retire", "delete":
			switch row.State {
			case "reserved", "packed", "shipped", "repair_ready":
				return ErrConflict
			}
			if in.Action == "retire" {
				state = "retired"
			}
		}
		if in.Action == "delete" {
			if tx.Exec("UPDATE devices SET deleted_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", deviceID).Error != nil {
				return ErrUnavailable
			}
		} else if tx.Exec("UPDATE devices SET state=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", state, deviceID).Error != nil {
			return ErrUnavailable
		}
		data, _ := json.Marshal(map[string]any{"previous_state": row.State, "category": category})
		if tx.Exec("INSERT INTO device_events(id,device_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), deviceID, actor.ID, in.Action, string(data)).Error != nil {
			return ErrUnavailable
		}
		out = WarehouseDeviceResult{ID: deviceID, State: state, Deleted: in.Action == "delete"}
		result, _ := json.Marshal(out)
		if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO warehouse_device_changes(operation_id,actor_id,location_id,category,device_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, locationID, category, deviceID, string(request), string(result)).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
