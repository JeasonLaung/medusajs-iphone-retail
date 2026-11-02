package devices

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WarehouseEditFields struct {
	VariantID  string  `json:"variant_id"`
	SourceID   string  `json:"source_id"`
	LocationID string  `json:"location_id"`
	IMEI       *string `json:"imei" gorm:"column:imei"`
	IMEI2      *string `json:"imei2" gorm:"column:imei2"`
	EID        *string `json:"eid"`
	Category   string  `json:"category"`
	State      string  `json:"state"`
	Configuration
	CostCents    int64   `json:"cost_cents"`
	CurrencyCode string  `json:"currency_code"`
	Bin          string  `json:"bin"`
	Notes        string  `json:"notes"`
	Photos       []Photo `json:"photos" gorm:"-"`
}
type WarehouseEditInput struct {
	OperationID      string              `json:"operation_id"`
	ExpectedRevision string              `json:"expected_revision"`
	Fields           WarehouseEditFields `json:"fields"`
}
type WarehouseEditContext struct {
	ID           string                            `json:"id"`
	SerialNumber string                            `json:"serial_number"`
	Revision     string                            `json:"revision"`
	Fields       WarehouseEditFields               `json:"fields"`
	Sources      []Source                          `json:"sources"`
	Locations    []authorization.WarehouseLocation `json:"locations"`
	Categories   []string                          `json:"categories"`
}

func editState(state string) bool {
	switch state {
	case "pending", "inspection", "available", "reserved", "packed", "shipped", "repair", "repair_ready", "retired", "supplier_returned":
		return true
	}
	return false
}
func radio(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func validWarehouseEdit(in WarehouseEditInput) bool {
	f := in.Fields
	if !validPathID(f.VariantID) || !validPathID(f.LocationID) || !editState(f.State) || (f.Category != "sale" && f.Category != "returned") {
		return false
	}
	for _, v := range []*string{f.IMEI, f.IMEI2, f.EID} {
		if v != nil && *v == "" {
			return false
		}
	}
	return validateIntake(IntakeInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, Devices: []IntakeDevice{{Identity: Identity{SerialNumber: "EDIT", IMEI: radio(f.IMEI), IMEI2: radio(f.IMEI2), EID: radio(f.EID)}, SourceID: f.SourceID, Category: f.Category, Configuration: f.Configuration, CostCents: f.CostCents, CurrencyCode: f.CurrencyCode, Bin: f.Bin, Notes: f.Notes, Photos: f.Photos}}}) == nil
}
func decodeWarehouseEdit(r io.Reader) (WarehouseEditInput, error) {
	var in WarehouseEditInput
	raw, e := io.ReadAll(io.LimitReader(r, 24577))
	if e != nil || len(raw) > 24576 || !utf8.Valid(raw) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if uniqueJSON(d) != nil || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	var root map[string]json.RawMessage
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil || len(root) != 3 || json.Unmarshal(root["fields"], &fields) != nil || len(fields) != 15 {
		return in, ErrInput
	}
	for k, v := range fields {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) && k != "imei" && k != "imei2" && k != "eid" && k != "grade" && k != "battery_health" {
			return in, ErrInput
		}
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || !validWarehouseEdit(in) {
		return in, ErrInput
	}
	return in, nil
}
func editAuthority(ctx context.Context, tx *gorm.DB, actor identity.User, location, category string) error {
	actions, e := warehouseDeviceActions(category, true)
	if e != nil {
		return e
	}
	return authorization.CheckWarehouseTx(ctx, tx, actor, location, actions...)
}
func loadWarehouseEdit(tx *gorm.DB, id string, revision uint64) (WarehouseEditContext, WarehouseDevice, error) {
	var out WarehouseEditContext
	var row WarehouseDevice
	e := warehouseDeviceQuery(tx).Where("d.id=? AND d.deleted_at IS NULL", id).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, row, ErrNotFound
	}
	if e != nil {
		return out, row, ErrUnavailable
	}
	var f WarehouseEditFields
	if tx.Table("devices").Select("variant_id,source_id,location_id,imei,imei2,eid,category,state,grade,battery_health,cost_cents,currency_code,bin,notes").Where("id=?", id).Take(&f).Error != nil {
		return out, row, ErrUnavailable
	}
	if e := projectWarehouseDevice(&row, revision); e != nil {
		return out, row, e
	}
	f.Photos = row.Photos
	b, _ := json.Marshal([]any{id, row.SerialNumber, f, row.UpdatedAt, revision, row.HasActiveAssignment})
	sum := sha256.Sum256(b)
	out = WarehouseEditContext{ID: id, SerialNumber: row.SerialNumber, Revision: hex.EncodeToString(sum[:]), Fields: f, Sources: []Source{}, Locations: []authorization.WarehouseLocation{}, Categories: []string{}}
	return out, row, nil
}
func (h Handler) ReadWarehouseEdit(ctx context.Context, actor identity.User, location, category, id string) (WarehouseEditContext, error) {
	var out WarehouseEditContext
	if !validPathID(location) || !validPathID(id) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := editAuthority(ctx, tx, actor, location, category); e != nil {
			return e
		}
		access, e := (authorization.Handler{DB: tx}).WarehouseAccess(ctx, actor)
		if e != nil {
			return e
		}
		rev, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		var row WarehouseDevice
		out, row, e = loadWarehouseEdit(tx, id, rev)
		if e != nil {
			return e
		}
		if row.LocationID != location || row.Category != category {
			return ErrNotFound
		}
		out.Locations = access.Locations
		ids := []string{}
		for _, l := range access.Locations {
			ids = append(ids, l.ID)
		}
		for _, cat := range []string{"sale", "returned"} {
			if e := editAuthority(ctx, tx, actor, location, cat); e == nil {
				out.Categories = append(out.Categories, cat)
			} else if !errors.Is(e, authorization.ErrDenied) {
				return e
			}
		}
		if tx.Table("device_sources s").Select("s.id,s.name,s.type,s.location_id,l.name AS location_name").Joins("JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.type='self' AND s.deleted_at IS NULL AND s.location_id IN ?", ids).Order("s.name,s.id").Scan(&out.Sources).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
func (h Handler) EditWarehouseDevice(ctx context.Context, actor identity.User, location, category, id string, in WarehouseEditInput) (WarehouseDeviceResult, error) {
	var out WarehouseDeviceResult
	if !validPathID(location) || !validPathID(id) || !validWarehouseEdit(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	wire, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := editAuthority(ctx, tx, actor, location, category); e != nil {
			return e
		}
		f := in.Fields
		if e := editAuthority(ctx, tx, actor, f.LocationID, f.Category); e != nil {
			return e
		}
		rev, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		before, row, e := loadWarehouseEdit(tx, id, rev)
		if e != nil {
			return e
		}
		if row.LocationID != location && row.LocationID != f.LocationID {
			return ErrNotFound
		}
		if e := editAuthority(ctx, tx, actor, row.LocationID, row.Category); e != nil {
			return e
		}
		var source struct{ LocationID, Type string }
		if tx.Table("device_sources s").Select("s.location_id,s.type").Joins("JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.id=? AND s.deleted_at IS NULL", f.SourceID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&source).Error != nil {
			return ErrConflict
		}
		if source.Type != "self" {
			return authorization.ErrDenied
		}
		if e := editAuthority(ctx, tx, actor, source.LocationID, f.Category); e != nil {
			return e
		}
		var prior struct{ ActorID, LocationID, Category, DeviceID, RequestJSON, ResultJSON string }
		e = tx.Table("warehouse_device_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.LocationID != location || prior.Category != category || prior.DeviceID != id || prior.RequestJSON != string(wire) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.ID != id {
				return ErrUnavailable
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if row.LocationID != location || row.Category != category || before.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		if row.HasActiveAssignment && (f.VariantID != row.VariantID || f.SourceID != row.SourceID || f.LocationID != row.LocationID || f.State != row.State || f.Category != row.Category) {
			return ErrConflict
		}
		if f.State != row.State && (f.State == "reserved" || f.State == "packed" || f.State == "shipped" || f.State == "repair_ready") {
			return ErrConflict
		}
		if (f.State == "available" || f.State == "pending") && f.Category != "sale" {
			return ErrConflict
		}
		var product string
		if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", f.VariantID).Scan(&product).Error != nil || product == "" {
			return ErrConflict
		}
		sku, e := intakeSKU(tx, product, f.VariantID)
		if e != nil {
			return e
		}
		if e := validateStoredConfiguration(sku.Options, f.Configuration); e != nil {
			return e
		}
		identity := Identity{SerialNumber: row.SerialNumber, IMEI: radio(f.IMEI), IMEI2: radio(f.IMEI2), EID: radio(f.EID)}
		canonical, e := NormalizeIdentity(identity, sku.RequiresRadio)
		if e != nil || canonical != identity {
			return ErrIdentity
		}
		if e := verifyPhotosTx(tx, f.Photos); e != nil {
			return e
		}
		photos, _ := json.Marshal(f.Photos)
		if err := tx.Exec("UPDATE devices SET variant_id=?,source_id=?,location_id=?,imei=?,imei2=?,eid=?,category=?,state=?,grade=?,battery_health=?,cost_cents=?,currency_code=?,bin=?,notes=?,photos=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", f.VariantID, f.SourceID, f.LocationID, f.IMEI, f.IMEI2, f.EID, f.Category, f.State, f.Grade, f.BatteryHealth, f.CostCents, f.CurrencyCode, f.Bin, f.Notes, string(photos), id).Error; err != nil {
			var duplicate *driver.MySQLError
			if errors.As(err, &duplicate) && duplicate.Number == 1062 {
				return ErrDuplicate
			}
			return ErrUnavailable
		}
		// Remove this device's old claims inside the same identity-locked transaction;
		// cross-field duplicate detection still checks every other active device.
		if tx.Exec("DELETE FROM device_identity_claims WHERE device_id=?", id).Error != nil {
			return ErrUnavailable
		}
		if e := ClaimIdentityTx(ctx, tx, id, identity, sku.RequiresRadio); e != nil {
			return e
		}
		data, _ := json.Marshal(map[string]any{"before": before.Fields, "after": f})
		if tx.Exec("INSERT INTO device_events(id,device_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), id, actor.ID, "update", string(data)).Error != nil {
			return ErrUnavailable
		}
		out = WarehouseDeviceResult{ID: id, State: f.State}
		result, _ := json.Marshal(out)
		if tx.Exec("INSERT INTO warehouse_device_changes(operation_id,actor_id,location_id,category,device_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, location, category, id, string(wire), string(result)).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
func (h Handler) WarehouseEditHTTP(c *gin.Context) {
	v, _ := c.Get("admin_user")
	actor, ok := v.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.required", "Sign in required")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	var e error
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		e = ErrInput
	} else if c.Request.Method == "GET" {
		var out WarehouseEditContext
		out, e = h.ReadWarehouseEdit(ctx, actor, c.Param("location_id"), c.Param("category"), c.Param("device_id"))
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
		var in WarehouseEditInput
		in, e = decodeWarehouseEdit(c.Request.Body)
		if e == nil {
			var out WarehouseDeviceResult
			out, e = h.EditWarehouseDevice(ctx, actor, c.Param("location_id"), c.Param("category"), c.Param("device_id"), in)
			if e == nil {
				c.JSON(200, out)
				return
			}
		}
	}
	trackingFail(c, e)
}
