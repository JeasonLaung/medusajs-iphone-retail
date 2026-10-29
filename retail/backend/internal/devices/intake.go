package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Photo struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}
type IntakeDevice struct {
	Identity
	SourceID string `json:"source_id"`
	Category string `json:"category"`
	Configuration
	CostCents    int64   `json:"cost_cents"`
	CurrencyCode string  `json:"currency_code"`
	Bin          string  `json:"bin"`
	Notes        string  `json:"notes"`
	Photos       []Photo `json:"photos"`
}
type IntakeInput struct {
	OperationID      string         `json:"operation_id"`
	ExpectedRevision string         `json:"expected_revision"`
	Devices          []IntakeDevice `json:"devices"`
}
type IntakeRecord struct {
	ID         string `json:"id"`
	VariantID  string `json:"variant_id"`
	LocationID string `json:"location_id"`
	State      string `json:"state"`
	IntakeDevice
}
type IntakeResult struct {
	Devices  []IntakeRecord `json:"devices"`
	Replayed bool           `json:"replayed"`
}
type RecentDevice struct {
	Photos        []Photo   `json:"photos" gorm:"-"`
	PhotoJSON     string    `json:"-" gorm:"column:photos"`
	ID            string    `json:"id"`
	SerialNumber  string    `json:"serial_number"`
	IMEI          *string   `json:"imei" gorm:"column:imei"`
	IMEI2         *string   `json:"imei2" gorm:"column:imei2"`
	EID           *string   `json:"eid" gorm:"column:eid"`
	Category      string    `json:"category"`
	State         string    `json:"state"`
	LocationID    string    `json:"location_id"`
	Grade         *string   `json:"grade"`
	BatteryHealth *int      `json:"battery_health"`
	CreatedAt     time.Time `json:"created_at"`
}
type IntakeContext struct {
	RecentDevices []RecentDevice `json:"recent_devices"`
	RecentLimit   int            `json:"recent_limit"`

	Revision      string          `json:"revision"`
	RequiresRadio bool            `json:"requires_radio"`
	Options       []VariantOption `json:"options"`
	Sources       []Source        `json:"sources"`
}

func intakeAuthority(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	actions := []authorization.Action{authorization.DeviceRead, authorization.ProductRead, authorization.InventoryRead, authorization.WarehouseRead}
	if write {
		actions = append(actions, authorization.DeviceIntake)
	}
	return authorization.CheckTx(ctx, tx, actor, actions...)
}
func intakeSKU(tx *gorm.DB, productID, variantID string) (IntakeContext, error) {
	out := IntakeContext{Options: []VariantOption{}, Sources: []Source{}}
	tracking, e := inspectTracking(tx, productID, variantID)
	if e != nil {
		return out, e
	}
	var v struct{ ManageInventory, AllowBackorder bool }
	if tx.Table("catalog_variants").Select("manage_inventory,allow_backorder").Where("id=? AND product_id=? AND deleted_at IS NULL", variantID, productID).Take(&v).Error != nil || !v.ManageInventory || v.AllowBackorder {
		return out, ErrConflict
	}
	var links []struct{ InventoryItemID, Quantity string }
	if tx.Table("catalog_variant_inventory_items l").Select("l.inventory_item_id,CAST(l.required_quantity AS CHAR) AS quantity").Joins("JOIN inventory_items i ON i.id=l.inventory_item_id AND i.deleted_at IS NULL").Where("l.variant_id=? AND l.deleted_at IS NULL", variantID).Limit(2).Clauses(clause.Locking{Strength: "SHARE"}).Find(&links).Error != nil {
		return out, ErrUnavailable
	}
	// Original managed() falls back to a valid inventory SKU when tracking has
	// not been explicitly enrolled. Do not impose a new enrollment prerequisite.
	if len(links) != 1 || links[0].Quantity != "1.000000000000000000" {
		return out, ErrConflict
	}
	var product struct {
		Title, Handle string
		Metadata      *string
		TypeValue     *string
	}
	if tx.Table("catalog_products p").Select("p.title,p.handle,p.metadata,t.value AS type_value").Joins("LEFT JOIN catalog_product_types t ON t.id=p.type_id AND t.deleted_at IS NULL").Where("p.id=? AND p.deleted_at IS NULL", productID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&product).Error != nil {
		return out, ErrUnavailable
	}
	var metadata map[string]any
	if product.Metadata != nil && json.Unmarshal([]byte(*product.Metadata), &metadata) != nil {
		return out, ErrUnavailable
	}
	kind := ""
	if product.TypeValue != nil {
		kind = *product.TypeValue
	}
	out.RequiresRadio = RequiresRadio(metadata, product.Title, product.Handle, kind)
	if tx.Table("catalog_variant_options l").Select("o.title,v.value").Joins("JOIN catalog_option_values v ON v.id=l.option_value_id AND v.deleted_at IS NULL").Joins("JOIN catalog_options o ON o.id=v.option_id AND o.deleted_at IS NULL").Where("l.variant_id=?", variantID).Order("o.id,v.id").Limit(101).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&out.Options).Error != nil || len(out.Options) > 100 {
		return out, ErrUnavailable
	}
	if tx.Table("device_sources s").Select("s.id,s.name,s.type,s.location_id,l.name AS location_name").Joins("JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.type='self' AND s.deleted_at IS NULL").Order("s.id").Limit(1001).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&out.Sources).Error != nil || len(out.Sources) > 1000 {
		return out, ErrUnavailable
	}
	b, _ := json.Marshal([]any{tracking.Revision, out.RequiresRadio, out.Options, out.Sources})
	digest := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(digest[:])
	return out, nil
}
func (h Handler) ReadIntake(ctx context.Context, actor identity.User, productID, variantID string) (IntakeContext, error) {
	var out IntakeContext
	if !validPathID(productID) || !validPathID(variantID) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := h.intakeAccess(ctx, tx, actor, false); e != nil {
			return e
		}
		if _, e := sourceGate(ctx, tx); e != nil {
			return e
		}
		var e error
		out, e = h.intakeConfiguration(tx, productID, variantID)
		if e != nil {
			return e
		}
		out.RecentDevices = []RecentDevice{}
		out.RecentLimit = 50
		query := tx.Table("devices").Select("id,serial_number,imei,imei2,eid,category,state,location_id,grade,battery_health,photos,created_at").Where("variant_id=? AND deleted_at IS NULL", variantID)
		if h.WarehouseScope != nil {
			query = query.Where("location_id=? AND category=?", h.WarehouseScope.LocationID, h.WarehouseScope.Category)
		}
		if query.Order("created_at DESC,id DESC").Limit(50).Scan(&out.RecentDevices).Error != nil {
			return ErrUnavailable
		}
		for i := range out.RecentDevices {
			row := &out.RecentDevices[i]
			if json.Unmarshal([]byte(row.PhotoJSON), &row.Photos) != nil || row.Photos == nil || len(row.Photos) > 8 {
				return ErrUnavailable
			}
			for _, p := range row.Photos {
				if !validPhotoRef(p) {
					return ErrUnavailable
				}
			}
		}
		return nil
	})
	return out, e
}
func intakeText(v string, max int) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= max && !strings.ContainsFunc(v, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) && strings.TrimSpace(v) == v
}
func validateIntake(in IntakeInput) error {
	if validateEnroll(EnrollInput{in.OperationID, in.ExpectedRevision}) != nil || len(in.Devices) < 1 || len(in.Devices) > 200 {
		return ErrInput
	}
	for _, d := range in.Devices {
		if !validPathID(d.SourceID) || (d.Category != "sale" && d.Category != "returned" && d.Category != "recycled") || d.CostCents < 0 || d.CostCents > 100000000 || len(d.CurrencyCode) != 3 || !intakeText(d.Bin, 200) || !intakeText(d.Notes, 2000) || d.Photos == nil || len(d.Photos) > 8 {
			return ErrInput
		}
		for _, c := range d.CurrencyCode {
			if c < 'a' || c > 'z' {
				return ErrInput
			}
		}
		if _, e := NormalizeIdentity(d.Identity, false); e != nil {
			return ErrInput
		}
		if d.Grade != nil && *d.Grade != "S" && *d.Grade != "A" && *d.Grade != "B" && *d.Grade != "C" {
			return ErrInput
		}
		if d.BatteryHealth != nil && (*d.BatteryHealth < 0 || *d.BatteryHealth > 100) {
			return ErrInput
		}
		seen := map[string]bool{}
		for _, p := range d.Photos {
			if !validPhotoRef(p) || seen[p.ID] {
				return ErrInput
			}
			seen[p.ID] = true
		}
	}
	return nil
}
func (h Handler) Intake(ctx context.Context, actor identity.User, productID, variantID string, in IntakeInput) (IntakeResult, error) {
	var out IntakeResult
	if !validPathID(productID) || !validPathID(variantID) {
		return out, ErrInput
	}
	if e := validateIntake(in); e != nil {
		return out, e
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := h.intakeAccess(ctx, tx, actor, true); e != nil {
			return e
		}
		if _, e := sourceGate(ctx, tx); e != nil {
			return e
		}
		if e := h.intakeSources(tx, in.Devices); e != nil {
			return e
		}
		var prior struct{ ActorID, ProductID, VariantID, RequestJSON, ResultJSON, ContextKey string }
		e := tx.Table("device_intake_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ContextKey != h.intakeContextKey() || prior.ActorID != actor.ID || prior.ProductID != productID || prior.VariantID != variantID || prior.RequestJSON != string(request) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || len(out.Devices) != len(in.Devices) {
				return ErrUnavailable
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		sku, e := h.intakeConfiguration(tx, productID, variantID)
		if e != nil {
			return e
		}
		if sku.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		out.Devices = []IntakeRecord{}
		for _, input := range in.Devices {
			d := input
			d.Identity, e = NormalizeIdentity(input.Identity, sku.RequiresRadio)
			if e != nil {
				return ErrInput
			}
			d.Configuration, e = ResolveConfiguration(sku.Options, input.Configuration)
			if e != nil {
				return e
			}
			var source struct{ LocationID string }
			if e := tx.Table("device_sources s").Select("s.location_id").Joins("JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.id=? AND s.type='self' AND s.deleted_at IS NULL", d.SourceID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&source).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					return ErrConflict
				}
				return ErrUnavailable
			}
			if e := verifyPhotosTx(tx, d.Photos); e != nil {
				return e
			}
			photos, _ := json.Marshal(d.Photos)
			state := "available"
			if d.Category != "sale" {
				state = "inspection"
			}
			id := "dev_" + uuid.NewString()
			nullable := func(v string) any {
				if v == "" {
					return nil
				}
				return v
			}
			if e := tx.Exec("INSERT INTO devices(id,serial_number,imei,imei2,eid,variant_id,source_id,location_id,grade,battery_health,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, d.SerialNumber, nullable(d.IMEI), nullable(d.IMEI2), nullable(d.EID), variantID, d.SourceID, source.LocationID, d.Grade, d.BatteryHealth, d.CostCents, d.CurrencyCode, d.Bin, d.Notes, state, d.Category, string(photos)).Error; e != nil {
				var duplicate *driver.MySQLError
				if errors.As(e, &duplicate) && duplicate.Number == 1062 {
					return ErrDuplicate
				}
				return ErrUnavailable
			}
			if e := ClaimIdentityTx(ctx, tx, id, d.Identity, sku.RequiresRadio); e != nil {
				return e
			}
			record := IntakeRecord{ID: id, VariantID: variantID, LocationID: source.LocationID, State: state, IntakeDevice: d}
			data, _ := json.Marshal(map[string]any{"device": record})
			if tx.Exec("INSERT INTO device_events(id,device_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,'intake',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), id, actor.ID, string(data)).Error != nil {
				return ErrUnavailable
			}
			out.Devices = append(out.Devices, record)
		}
		result, _ := json.Marshal(out)
		if tx.Exec("INSERT INTO device_intake_changes(operation_id,actor_id,product_id,variant_id,request_json,result_json,context_key,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, productID, variantID, string(request), string(result), h.intakeContextKey()).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
