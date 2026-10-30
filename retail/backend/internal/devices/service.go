package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type ServiceInput struct {
	OperationID      string  `json:"operation_id"`
	ExpectedRevision string  `json:"expected_revision"`
	Action           string  `json:"action"`
	Result           string  `json:"result"`
	VariantID        string  `json:"variant_id"`
	Grade            string  `json:"grade"`
	BatteryHealth    int     `json:"battery_health"`
	Notes            string  `json:"notes"`
	Photos           []Photo `json:"photos"`
	TrackingNumber   string  `json:"tracking_number"`
}
type ServiceContext struct {
	OrderID        string  `json:"order_id"`
	RequestID      string  `json:"request_id"`
	AssignmentID   string  `json:"assignment_id"`
	DeviceID       string  `json:"device_id"`
	SerialNumber   string  `json:"serial_number"`
	Type           string  `json:"type"`
	RequestStatus  string  `json:"request_status"`
	Status         string  `json:"status"`
	State          string  `json:"state"`
	Category       string  `json:"category"`
	ProductID      string  `json:"product_id"`
	VariantID      string  `json:"variant_id"`
	Grade          *string `json:"grade"`
	BatteryHealth  *int    `json:"battery_health"`
	TrackingNumber *string `json:"tracking_number"`
	Revision       string  `json:"revision"`
	Enabled        bool    `json:"enabled"`
}
type ServiceResult struct {
	DeviceID     string `json:"device_id"`
	AssignmentID string `json:"assignment_id"`
	State        string `json:"state"`
	Category     string `json:"category"`
	Replayed     bool   `json:"replayed"`
}
type serviceDevice struct {
	ID, VariantID, SourceID, LocationID, State, Category, SerialNumber, Notes string
	Grade                                                                     *string
	BatteryHealth                                                             *int
	UpdatedAt                                                                 time.Time
}

func validService(in ServiceInput) bool {
	if validateEnroll(EnrollInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision}) != nil {
		return false
	}
	if in.Action == "repair_ship" {
		return in.Result == "" && in.VariantID == "" && in.Grade == "" && in.BatteryHealth == 0 && in.Notes == "" && len(in.Photos) == 0 && in.TrackingNumber != "" && intakeText(in.TrackingNumber, 200) && strings.TrimSpace(in.TrackingNumber) == in.TrackingNumber
	}
	if in.Action != "inspect" || !validPathID(in.VariantID) || in.TrackingNumber != "" || in.Notes == "" || !intakeText(in.Notes, 2000) || strings.TrimSpace(in.Notes) != in.Notes || in.BatteryHealth < 0 || in.BatteryHealth > 100 || in.Photos == nil || len(in.Photos) > 8 {
		return false
	}
	switch in.Grade {
	case "S", "A", "B", "C":
	default:
		return false
	}
	switch in.Result {
	case "pass", "repair", "retire", "supplier_return":
	default:
		return false
	}
	seen := map[string]bool{}
	for _, p := range in.Photos {
		if !validPhotoRef(p) || seen[p.ID] {
			return false
		}
		seen[p.ID] = true
	}
	return true
}
func serviceAuthority(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	if e := returnAuthority(ctx, tx, actor, write); e != nil {
		return e
	}
	return authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.InventoryRead)
}
func loadService(ctx context.Context, tx *gorm.DB, o, r, a string) (ServiceContext, serviceDevice, error) {
	var out ServiceContext
	var d serviceDevice
	base, _, e := loadReturn(ctx, tx, o, r)
	if e != nil {
		return out, d, e
	}
	var assignment *ReturnDevice
	for i := range base.Devices {
		v := &base.Devices[i]
		if v.AssignmentID == a && v.RequestID != nil && *v.RequestID == r {
			assignment = v
		}
	}
	if assignment == nil {
		return out, d, gorm.ErrRecordNotFound
	}
	if e = tx.Table("devices").Where("id=? AND deleted_at IS NULL", assignment.DeviceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error; e != nil {
		return out, d, e
	}
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("device_service_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&gate).Error != nil || gate.Version != 1 {
		return out, d, ErrUnavailable
	}
	var productID string
	if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", d.VariantID).Scan(&productID).Error != nil {
		return out, d, ErrUnavailable
	}
	var assignmentInfo struct{ TrackingNumber *string }
	if tx.Table("device_assignments").Select("tracking_number").Where("id=?", a).Scan(&assignmentInfo).Error != nil {
		return out, d, ErrUnavailable
	}
	out = ServiceContext{OrderID: o, RequestID: r, AssignmentID: a, DeviceID: d.ID, SerialNumber: d.SerialNumber, Type: base.Type, RequestStatus: base.Status, Status: assignment.Status, State: d.State, Category: d.Category, ProductID: productID, VariantID: d.VariantID, Grade: d.Grade, BatteryHealth: d.BatteryHealth, TrackingNumber: assignmentInfo.TrackingNumber, Enabled: gate.Enabled}
	b, _ := json.Marshal([]any{base.Revision, d, assignmentInfo.TrackingNumber})
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, d, nil
}
func (h Handler) ReadService(ctx context.Context, actor identity.User, o, r, a string) (ServiceContext, error) {
	var out ServiceContext
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
		var e error
		out, _, e = loadService(ctx, tx, o, r, a)
		return e
	})
	return out, e
}
func serviceState(result string, repair bool) (string, string) {
	switch result {
	case "pass":
		if repair {
			return "repair_ready", "returned"
		}
		return "available", "sale"
	case "repair":
		return "repair", ""
	case "retire":
		return "retired", ""
	default:
		return "supplier_returned", ""
	}
}

// ServiceDevice changes physical inspection/repair facts only; quantity inventory is independent.
func (h Handler) ServiceDevice(ctx context.Context, actor identity.User, o, r, a string, in ServiceInput) (ServiceResult, error) {
	var out ServiceResult
	if !validPathID(o) || !validPathID(r) || !validPathID(a) || !validService(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	wire, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := serviceAuthority(ctx, tx, actor, true); e != nil {
			return e
		}
		before, d, e := loadService(ctx, tx, o, r, a)
		if e != nil {
			return e
		}
		var prior struct{ ActorID, OrderID, RequestID, AssignmentID, RequestJSON, ResultJSON string }
		e = tx.Table("device_service_receipts").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.OrderID != o || prior.RequestID != r || prior.AssignmentID != a || prior.RequestJSON != string(wire) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.DeviceID != d.ID || out.AssignmentID != a {
				return ErrUnavailable
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if !before.Enabled {
			return ErrUnavailable
		}
		if before.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		state, category := d.State, d.Category
		event := map[string]any{"assignment_id": a, "request_id": r}
		if in.Action == "repair_ship" {
			if before.Status != "returned" || d.State != "repair_ready" || before.Type != "repair" || before.RequestStatus != "processing" {
				return ErrConflict
			}
			var order struct{ Status string }
			if tx.Table("orders").Select("status").Where("id=? AND deleted_at IS NULL", o).Take(&order).Error != nil || order.Status == "canceled" {
				return ErrConflict
			}
			var active int64
			if tx.Table("device_assignments").Where("device_id=? AND deleted_at IS NULL AND status IN ('reserved','packed','shipped')", d.ID).Count(&active).Error != nil {
				return ErrUnavailable
			}
			if active != 0 {
				return ErrConflict
			}
			state = "shipped"
			event["tracking_number"] = in.TrackingNumber
			if tx.Exec("UPDATE device_assignments SET status='shipped',tracking_number=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", in.TrackingNumber, a).Error != nil {
				return ErrUnavailable
			}
			if tx.Exec("UPDATE devices SET state='shipped',updated_at=UTC_TIMESTAMP(6) WHERE id=?", d.ID).Error != nil {
				return ErrUnavailable
			}
		} else {
			if before.Status != "returned" || (d.Category != "returned" && d.Category != "recycled") || (d.State != "inspection" && d.State != "repair") {
				return ErrConflict
			}
			var productID string
			if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", in.VariantID).Scan(&productID).Error != nil || productID == "" {
				return ErrConflict
			}
			sku, e := intakeSKU(tx, productID, in.VariantID)
			if e != nil {
				return e
			}
			var sourceCount int64
			if tx.Table("device_sources").Where("location_id=? AND deleted_at IS NULL", d.LocationID).Count(&sourceCount).Error != nil || sourceCount < 1 {
				return ErrConflict
			}
			if e := verifyPhotosTx(tx, in.Photos); e != nil {
				return e
			}
			repair := before.Type == "repair" && before.RequestStatus == "processing"
			var nextCategory string
			state, nextCategory = serviceState(in.Result, repair)
			if nextCategory != "" {
				category = nextCategory
			}
			if in.Result == "pass" {
				if validateStoredConfiguration(sku.Options, Configuration{Grade: &in.Grade, BatteryHealth: &in.BatteryHealth}) != nil || repair && in.VariantID != d.VariantID {
					return ErrConflict
				}
			}
			if in.Result == "supplier_return" {
				var source struct{ Type string }
				if tx.Table("device_sources").Select("type").Where("id=? AND deleted_at IS NULL", d.SourceID).Take(&source).Error != nil || source.Type != "supplier" {
					return ErrConflict
				}
			}
			event["result"] = in.Result
			event["notes"] = in.Notes
			event["photos"] = in.Photos
			event["before"] = map[string]any{"state": d.State, "variant_id": d.VariantID, "grade": d.Grade, "battery_health": d.BatteryHealth}
			event["after"] = map[string]any{"state": state, "variant_id": in.VariantID, "grade": in.Grade, "battery_health": in.BatteryHealth}
			if tx.Exec("UPDATE devices SET variant_id=?,state=?,category=?,grade=?,battery_health=?,notes=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", in.VariantID, state, category, in.Grade, in.BatteryHealth, in.Notes, d.ID).Error != nil {
				return ErrUnavailable
			}
		}
		payload, _ := json.Marshal(event)
		if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), d.ID, o, actor.ID, in.Action, string(payload)).Error != nil {
			return ErrUnavailable
		}
		out = ServiceResult{DeviceID: d.ID, AssignmentID: a, State: state, Category: category}
		result, _ := json.Marshal(out)
		if tx.Exec("INSERT INTO device_service_receipts(operation_id,actor_id,order_id,request_id,assignment_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, o, r, a, string(wire), string(result)).Error != nil || tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
