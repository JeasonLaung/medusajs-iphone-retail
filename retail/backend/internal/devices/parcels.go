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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/customernotices"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ParcelInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	Action           string `json:"action"`
	TrackingNumber   string `json:"tracking_number"`
	TrackingURL      string `json:"tracking_url"`
}
type ParcelRecord struct {
	ID                                           string     `json:"id"`
	LocationID                                   string     `json:"location_id"`
	ProviderID                                   *string    `json:"provider_id"`
	PackedAt, ShippedAt, DeliveredAt, CanceledAt *time.Time `json:"-"`
	UpdatedAt                                    time.Time  `json:"-"`
}
type ParcelItem struct {
	ID              string  `json:"id"`
	ItemID          *string `json:"item_id" gorm:"column:line_item_id"`
	InventoryItemID *string `json:"-"`
	Title           string  `json:"title"`
	Quantity        string  `json:"quantity"`
}
type ParcelDevice struct {
	AssignmentID       string    `json:"assignment_id"`
	DeviceID           string    `json:"device_id"`
	SerialNumber       string    `json:"serial_number"`
	ItemID             string    `json:"item_id"`
	Status             string    `json:"status"`
	State              string    `json:"-"`
	VariantID          string    `json:"-"`
	LocationID         string    `json:"-"`
	SourceID           string    `json:"-"`
	AssignmentSourceID string    `json:"-"`
	IMEI, IMEI2, EID   *string   `json:"-"`
	UpdatedAt          time.Time `json:"-"`
}
type ParcelLabel struct {
	ID             string `json:"id"`
	TrackingNumber string `json:"tracking_number"`
	TrackingURL    string `json:"tracking_url"`
	LabelURL       string `json:"label_url"`
}
type ParcelContext struct {
	OrderID       string         `json:"order_id"`
	FulfillmentID string         `json:"fulfillment_id"`
	Enabled       bool           `json:"enabled"`
	OrderActive   bool           `json:"order_active"`
	Supported     bool           `json:"supported"`
	Status        string         `json:"status"`
	LocationID    string         `json:"location_id"`
	Devices       []ParcelDevice `json:"devices"`
	Items         []ParcelItem   `json:"items"`
	Labels        []ParcelLabel  `json:"labels"`
	Revision      string         `json:"revision"`
}
type ParcelResult struct {
	FulfillmentID string   `json:"fulfillment_id"`
	Action        string   `json:"action"`
	AssignmentIDs []string `json:"assignment_ids"`
	Replayed      bool     `json:"replayed"`
}
type parcelLine struct{ ID, ItemID, VariantID, Quantity, FulfilledQuantity, ShippedQuantity string }
type parcelFacts struct {
	Packing packingFacts
	Record  ParcelRecord
	Lines   map[string]parcelLine
}

func validParcel(in ParcelInput) bool {
	id, e := uuid.Parse(in.OperationID)
	rev, e2 := hex.DecodeString(in.ExpectedRevision)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || e2 != nil || len(rev) != 32 || hex.EncodeToString(rev) != in.ExpectedRevision {
		return false
	}
	if in.Action == "cancel" {
		return in.TrackingNumber == "" && in.TrackingURL == ""
	}
	if in.Action != "ship" || !utf8.ValidString(in.TrackingNumber) || utf8.RuneCountInString(in.TrackingNumber) < 1 || utf8.RuneCountInString(in.TrackingNumber) > 200 || strings.TrimSpace(in.TrackingNumber) != in.TrackingNumber || strings.IndexFunc(in.TrackingNumber, unicode.IsControl) >= 0 || len(in.TrackingURL) > 2048 || !utf8.ValidString(in.TrackingURL) || strings.IndexFunc(in.TrackingURL, unicode.IsControl) >= 0 {
		return false
	}
	if in.TrackingURL != "" {
		u, e := url.Parse(in.TrackingURL)
		if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
			return false
		}
	}
	return true
}
func decodeParcel(reader io.Reader) (ParcelInput, error) {
	var in ParcelInput
	b, e := io.ReadAll(io.LimitReader(reader, 16385))
	if e != nil || len(b) > 16384 || !utf8.Valid(b) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(b))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return in, ErrInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, e = d.Token()
		k, ok := tok.(string)
		if e != nil || !ok || fields[k] != nil {
			return in, ErrInput
		}
		switch k {
		case "operation_id", "expected_revision", "action", "tracking_number", "tracking_url":
		default:
			return in, ErrInput
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return in, ErrInput
		}
		fields[k] = raw
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != 5 || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	b, _ = json.Marshal(fields)
	if json.Unmarshal(b, &in) != nil {
		return in, ErrInput
	}
	in.TrackingNumber = strings.TrimSpace(in.TrackingNumber)
	if !validParcel(in) {
		return in, ErrInput
	}
	return in, nil
}
func parcelGate(ctx context.Context, tx *gorm.DB, actor identity.User) (uint64, bool, error) {
	// Allocation's enabled flag does not disable cancellation/recovery of an
	// existing parcel. Its current identity/permission/schema checks still apply.
	rev, _, e := allocationGate(ctx, tx, actor)
	if e != nil {
		return 0, false, e
	}
	if e = authorization.CheckTx(ctx, tx, actor, authorization.OrderFulfillmentRead); e != nil {
		return 0, false, e
	}
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("order_device_parcel_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&cap).Error != nil || cap.Version != 1 || fulfillment.Ready(ctx, tx) != nil {
		return 0, false, ErrUnavailable
	}
	var marker struct{ ID int }
	if tx.Table("inventory_runtime_schema").Select("id").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&marker).Error != nil || inventory.Ready(ctx, tx) != nil {
		return 0, false, ErrUnavailable
	}
	return rev, cap.Enabled, nil
}
func loadParcel(tx *gorm.DB, o, f string, rev uint64, enabled bool) (ParcelContext, parcelFacts, error) {
	out := ParcelContext{OrderID: o, FulfillmentID: f, Enabled: enabled, Devices: []ParcelDevice{}, Items: []ParcelItem{}, Labels: []ParcelLabel{}}
	facts := parcelFacts{Lines: map[string]parcelLine{}}
	packing, p, e := loadPacking(tx, o, rev, enabled)
	if e != nil {
		return out, facts, e
	}
	facts.Packing = p
	out.OrderActive = packing.OrderActive
	if e = tx.Table("fulfillments f").Select("f.id,f.location_id,f.provider_id,f.packed_at,f.shipped_at,f.delivered_at,f.canceled_at,f.updated_at").Joins("JOIN order_fulfillments l ON l.fulfillment_id=f.id AND l.deleted_at IS NULL AND l.order_id=?", o).Where("f.id=? AND f.deleted_at IS NULL", f).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&facts.Record).Error; e != nil {
		return out, facts, e
	}
	out.LocationID = facts.Record.LocationID
	out.Supported = facts.Record.ProviderID != nil && *facts.Record.ProviderID == "manual_manual"
	out.Status = "packed"
	if facts.Record.PackedAt == nil {
		out.Status = "unpacked"
	}
	if facts.Record.ShippedAt != nil {
		out.Status = "shipped"
	}
	if facts.Record.DeliveredAt != nil {
		out.Status = "delivered"
	}
	if facts.Record.CanceledAt != nil {
		out.Status = "canceled"
	}
	if tx.Table("fulfillment_items").Select("id,line_item_id,inventory_item_id,title,quantity").Where("fulfillment_id=? AND deleted_at IS NULL", f).Order("id").Limit(1001).Clauses(clause.Locking{Strength: "SHARE"}).Find(&out.Items).Error != nil || len(out.Items) > 1000 {
		return out, facts, ErrUnavailable
	}
	if tx.Table("device_assignments a").Select("a.id AS assignment_id,d.id AS device_id,d.serial_number,a.item_id,a.status,d.state,d.variant_id,d.location_id,d.source_id,a.source_id AS assignment_source_id,d.imei,d.imei2,d.eid,d.updated_at").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Where("a.fulfillment_id=? AND a.order_id=? AND a.deleted_at IS NULL", f, o).Order("a.id").Limit(1001).Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&out.Devices).Error != nil || len(out.Devices) > 1000 {
		return out, facts, ErrUnavailable
	}
	if tx.Table("fulfillment_labels").Select("id,tracking_number,tracking_url,label_url").Where("fulfillment_id=? AND deleted_at IS NULL", f).Order("id").Limit(101).Clauses(clause.Locking{Strength: "SHARE"}).Find(&out.Labels).Error != nil || len(out.Labels) > 100 {
		return out, facts, ErrUnavailable
	}
	for _, item := range out.Items {
		if item.ItemID == nil {
			out.Supported = false
			continue
		}
		var line parcelLine
		e = tx.Table("order_items i").Select("i.id,i.item_id,l.variant_id,i.quantity,i.fulfilled_quantity,i.shipped_quantity").Joins("JOIN order_line_items l ON l.id=i.item_id AND l.deleted_at IS NULL").Where("i.order_id=? AND i.item_id=? AND i.version=? AND i.deleted_at IS NULL", o, *item.ItemID, p.Version).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&line).Error
		if e != nil {
			out.Supported = false
			continue
		}
		facts.Lines[*item.ItemID] = line
	}
	// Include hidden binding fields explicitly: the public projection has no
	// inventory identities, radio identifiers or mutable stock information.
	deviceFacts := []any{}
	for _, d := range out.Devices {
		deviceFacts = append(deviceFacts, []any{d.AssignmentID, d.State, d.VariantID, d.SourceID, d.AssignmentSourceID, d.LocationID, d.IMEI, d.IMEI2, d.EID, d.UpdatedAt})
	}
	itemFacts := []any{}
	for _, i := range out.Items {
		itemFacts = append(itemFacts, []any{i.ID, i.InventoryItemID})
	}
	data, e := json.Marshal([]any{packing.Revision, out, facts.Record.PackedAt, facts.Record.ShippedAt, facts.Record.DeliveredAt, facts.Record.CanceledAt, facts.Record.UpdatedAt, facts.Lines, deviceFacts, itemFacts, rev})
	if e != nil {
		return out, facts, ErrUnavailable
	}
	hash := sha256.Sum256(data)
	out.Revision = hex.EncodeToString(hash[:])
	return out, facts, nil
}
func (h Handler) ReadParcel(ctx context.Context, actor identity.User, o, f string) (ParcelContext, error) {
	var out ParcelContext
	if !validPathID(o) || !validPathID(f) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := parcelGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		out, _, e = loadParcel(tx, o, f, rev, enabled)
		return e
	})
	return out, e
}

func restoreParcelStock(tx *gorm.DB, actor, item, stockID, location, quantity string) error {
	var reservations []packingReservation
	if tx.Table("inventory_reservations").Where("line_item_id=? AND deleted_at IS NULL", item).Order("id").Limit(3).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&reservations).Error != nil || len(reservations) > 1 {
		return ErrConflict
	}
	reservationLocation := location
	reservationQuantity := quantity
	if len(reservations) == 1 {
		r := reservations[0]
		if r.InventoryItemID != stockID {
			return ErrConflict
		}
		reservationLocation = r.LocationID
		var e error
		reservationQuantity, e = inventory.Difference(r.Quantity, "-"+quantity)
		if e != nil {
			return e
		}
	}
	locations := []string{location}
	if reservationLocation != location {
		locations = append(locations, reservationLocation)
	}
	sort.Strings(locations)
	for _, loc := range locations {
		var level packingStock
		if tx.Table("inventory_levels").Where("inventory_item_id=? AND location_id=? AND deleted_at IS NULL", stockID, loc).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&level).Error != nil {
			return ErrConflict
		}
		if !packingExactlyOne(tx.Table("inventory_locations").Where("id=? AND deleted_at IS NULL", loc), "id") {
			return ErrConflict
		}
		var holds []struct{ Reason string }
		if tx.Table("inventory_migration_holds").Select("reason").Where("inventory_item_id=? AND location_id=?", stockID, loc).Limit(4).Clauses(clause.Locking{Strength: "SHARE"}).Find(&holds).Error != nil || len(holds) != 0 {
			return inventory.ErrStockHeld
		}
		var all []struct{ Quantity string }
		if tx.Table("inventory_reservations").Select("quantity").Where("inventory_item_id=? AND location_id=? AND deleted_at IS NULL", stockID, loc).Limit(10001).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&all).Error != nil || len(all) > 10000 {
			return ErrUnavailable
		}
		total := "0"
		for _, r := range all {
			if !nonnegative(r.Quantity) {
				return inventory.ErrStockHeld
			}
			neg, e := inventory.Difference("0", r.Quantity)
			if e != nil {
				return e
			}
			total, e = inventory.Difference(total, neg)
			if e != nil {
				return e
			}
		}
		same, e := inventory.Equal(total, level.ReservedQuantity)
		if e != nil || !same {
			return inventory.ErrStockHeld
		}
		stocked, reserved := level.StockedQuantity, level.ReservedQuantity
		if loc == location {
			stocked, e = inventory.Difference(stocked, "-"+quantity)
			if e != nil {
				return e
			}
		}
		if loc == reservationLocation {
			reserved, e = inventory.Difference(reserved, "-"+quantity)
			if e != nil {
				return e
			}
		}
		free, e := inventory.Difference(stocked, reserved)
		if e != nil || !nonnegative(free) || !nonnegative(stocked) || !nonnegative(reserved) {
			return inventory.ErrInsufficientStock
		}
		if tx.Exec("UPDATE inventory_levels SET stocked_quantity=?,raw_stocked_quantity=?,reserved_quantity=?,raw_reserved_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", stocked, rawCount(stocked), reserved, rawCount(reserved), level.ID).Error != nil {
			return ErrUnavailable
		}
	}
	if len(reservations) == 0 {
		if tx.Exec("INSERT INTO inventory_reservations(id,inventory_item_id,location_id,line_item_id,quantity,raw_quantity,allow_backorder,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,FALSE,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "res_"+uuid.NewString(), stockID, reservationLocation, item, reservationQuantity, rawCount(reservationQuantity), actor).Error != nil {
			return ErrUnavailable
		}
	} else if tx.Exec("UPDATE inventory_reservations SET quantity=?,raw_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", reservationQuantity, rawCount(reservationQuantity), reservations[0].ID).Error != nil {
		return ErrUnavailable
	}
	return nil
}
func (h Handler) ChangeParcel(ctx context.Context, actor identity.User, o, f string, in ParcelInput) (ParcelResult, error) {
	var result ParcelResult
	if !validPathID(o) || !validPathID(f) || !validParcel(in) {
		return result, ErrInput
	}
	if h.DB == nil {
		return result, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := parcelGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		if !enabled {
			return ErrUnavailable
		}
		before, facts, e := loadParcel(tx, o, f, rev, enabled)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, OrderID, FulfillmentID, RequestJSON, ResultJSON string }
		e = tx.Table("order_device_parcel_operations").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.OrderID != o || receipt.FulfillmentID != f || receipt.RequestJSON != string(request) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(receipt.ResultJSON), &result) != nil || result.FulfillmentID != f || result.Action != in.Action || len(result.AssignmentIDs) == 0 {
				return ErrUnavailable
			}
			for _, id := range result.AssignmentIDs {
				if !packingExactlyOne(tx.Table("device_assignments a").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Joins("JOIN order_items i ON i.order_id=a.order_id AND i.item_id=a.item_id AND i.version=? AND i.deleted_at IS NULL", facts.Packing.Version).Joins("JOIN order_line_items l ON l.id=a.item_id AND l.deleted_at IS NULL").Where("a.id=? AND a.order_id=? AND a.deleted_at IS NULL", id, o), "a.id") {
					return gorm.ErrRecordNotFound
				}
			}
			result.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !before.OrderActive || !before.Supported || before.Status != "packed" || before.Revision != in.ExpectedRevision || len(before.Items) == 0 || len(before.Devices) == 0 {
			return ErrConflict
		}
		var allAssignments []struct{ ID, OrderID string }
		if tx.Table("device_assignments").Select("id,order_id").Where("fulfillment_id=? AND deleted_at IS NULL", f).Order("id").Limit(1001).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&allAssignments).Error != nil || len(allAssignments) != len(before.Devices) {
			return ErrConflict
		}
		for _, a := range allAssignments {
			if a.OrderID != o {
				return ErrConflict
			}
		}
		counts := map[string]int{}
		source := ""
		for _, d := range before.Devices {
			if d.Status != "packed" || d.State != "packed" || d.LocationID != before.LocationID || d.SourceID != d.AssignmentSourceID {
				return ErrConflict
			}
			if source == "" {
				source = d.SourceID
			}
			if source != d.SourceID {
				return ErrConflict
			}
			line, ok := facts.Lines[d.ItemID]
			if !ok || line.VariantID != d.VariantID {
				return ErrConflict
			}
			if in.Action == "ship" {
				var productID string
				if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", d.VariantID).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&productID).Error != nil || productID == "" {
					return ErrConflict
				}
				sku, e := intakeSKU(tx, productID, d.VariantID)
				if e != nil {
					return e
				}
				ident := Identity{SerialNumber: d.SerialNumber}
				if d.IMEI != nil {
					ident.IMEI = *d.IMEI
				}
				if d.IMEI2 != nil {
					ident.IMEI2 = *d.IMEI2
				}
				if d.EID != nil {
					ident.EID = *d.EID
				}
				normalized, e := NormalizeIdentity(ident, sku.RequiresRadio)
				if e != nil || normalized != ident {
					return ErrIdentity
				}
				if e = ClaimIdentityTx(ctx, tx, d.DeviceID, ident, sku.RequiresRadio); e != nil {
					return e
				}
			}

			counts[d.ItemID]++
		}
		if len(counts) != len(before.Items) {
			return ErrConflict
		}
		seen := map[string]bool{}
		for _, item := range before.Items {
			if item.ItemID == nil || item.InventoryItemID == nil || seen[*item.ItemID] {
				return ErrConflict
			}
			seen[*item.ItemID] = true
			quantity, e := allocationUnits(item.Quantity)
			if e != nil || quantity < 1 || quantity != counts[*item.ItemID] {
				return ErrConflict
			}
			line := facts.Lines[*item.ItemID]
			ordered, e := allocationUnits(line.Quantity)
			if e != nil {
				return ErrConflict
			}
			fulfilled, e := allocationUnits(line.FulfilledQuantity)
			if e != nil || fulfilled > ordered {
				return ErrConflict
			}
			shipped, e := allocationUnits(line.ShippedQuantity)
			if e != nil || shipped > fulfilled {
				return ErrConflict
			}
			if !packingExactlyOne(tx.Table("catalog_variant_inventory_items v").Joins("JOIN inventory_items i ON i.id=v.inventory_item_id AND i.deleted_at IS NULL").Where("v.variant_id=? AND v.inventory_item_id=? AND v.required_quantity=1 AND v.deleted_at IS NULL", line.VariantID, *item.InventoryItemID), "v.id") {
				return ErrConflict
			}
			amount := strconv.Itoa(quantity)
			if in.Action == "cancel" {
				remaining, e := inventory.Difference(line.FulfilledQuantity, amount)
				if e != nil {
					return e
				}
				margin, e := inventory.Difference(remaining, line.ShippedQuantity)
				if e != nil || !nonnegative(margin) {
					return ErrConflict
				}
				if e = restoreParcelStock(tx, actor.ID, *item.ItemID, *item.InventoryItemID, before.LocationID, amount); e != nil {
					return e
				}
				if tx.Exec("UPDATE order_items SET fulfilled_quantity=?,raw_fulfilled_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", remaining, rawCount(remaining), line.ID).Error != nil {
					return ErrUnavailable
				}
			} else {
				next, e := inventory.Difference(line.ShippedQuantity, "-"+amount)
				if e != nil {
					return e
				}
				margin, e := inventory.Difference(line.FulfilledQuantity, next)
				if e != nil || !nonnegative(margin) {
					return ErrConflict
				}
				if tx.Exec("UPDATE order_items SET shipped_quantity=?,raw_shipped_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", next, rawCount(next), line.ID).Error != nil {
					return ErrUnavailable
				}
			}
		}
		result = ParcelResult{FulfillmentID: f, Action: in.Action, AssignmentIDs: []string{}}
		for _, d := range before.Devices {
			eventAction := "ship"
			if in.Action == "cancel" {
				eventAction = "cancel_fulfillment"
				if tx.Exec("UPDATE device_assignments SET status='reserved',fulfillment_id=NULL,updated_at=UTC_TIMESTAMP(6) WHERE id=?", d.AssignmentID).Error != nil || tx.Exec("UPDATE devices SET state='reserved',updated_at=UTC_TIMESTAMP(6) WHERE id=?", d.DeviceID).Error != nil {
					return ErrUnavailable
				}
			} else {
				if tx.Exec("UPDATE device_assignments SET status='shipped',tracking_number=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", in.TrackingNumber, d.AssignmentID).Error != nil || tx.Exec("UPDATE devices SET state='shipped',updated_at=UTC_TIMESTAMP(6) WHERE id=?", d.DeviceID).Error != nil {
					return ErrUnavailable
				}
			}
			data, _ := json.Marshal(map[string]any{"assignment_id": d.AssignmentID, "fulfillment_id": f, "tracking_number": in.TrackingNumber})
			if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), d.DeviceID, o, actor.ID, eventAction, string(data)).Error != nil {
				return ErrUnavailable
			}
			result.AssignmentIDs = append(result.AssignmentIDs, d.AssignmentID)
		}
		if in.Action == "cancel" {
			if tx.Exec("UPDATE fulfillments SET canceled_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE id=?", f).Error != nil {
				return ErrUnavailable
			}
		} else {
			if tx.Exec("UPDATE fulfillments SET shipped_at=UTC_TIMESTAMP(6),marked_shipped_by=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", actor.ID, f).Error != nil {
				return ErrUnavailable
			}
			if tx.Exec("INSERT INTO fulfillment_labels(id,fulfillment_id,tracking_number,tracking_url,label_url,created_at,updated_at) VALUES(?,?,?,?,'',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "fullabel_"+uuid.NewString(), f, in.TrackingNumber, in.TrackingURL).Error != nil {
				return ErrUnavailable
			}
			if e := customernotices.EmitOrderTx(tx, o, "shipment.created", f); e != nil {
				return ErrUnavailable
			}
		}
		if tx.Exec("UPDATE orders SET updated_at=UTC_TIMESTAMP(6) WHERE id=?", o).Error != nil || tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		after, afterFacts, e := loadParcel(tx, o, f, rev+1, enabled)
		if e != nil {
			return e
		}
		audit := func(c ParcelContext, f parcelFacts) string {
			b, _ := json.Marshal(map[string]any{"context": c, "stock": f.Packing.Stock, "reservations": f.Packing.Reservations, "lines": f.Lines})
			return string(b)
		}
		encoded, _ := json.Marshal(result)
		return tx.Table("order_device_parcel_operations").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "order_id": o, "fulfillment_id": f, "request_json": string(request), "before_json": audit(before, facts), "after_json": audit(after, afterFacts), "result_json": string(encoded), "created_at": time.Now().UTC()}).Error
	})
	return result, e
}
func (h Handler) ParcelHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if c.Request.Method != "GET" {
		media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if e != nil || media != "application/json" {
			platform.Fail(c, 415, "devices.content_type", "JSON required")
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	var e error
	if c.Request.URL.RawQuery != "" {
		e = ErrInput
	} else if c.Request.Method == "GET" {
		var out ParcelContext
		out, e = h.ReadParcel(ctx, actor, c.Param("id"), c.Param("fulfillment_id"))
		if e == nil {
			c.JSON(200, out)
			return
		}
	} else {
		var in ParcelInput
		in, e = decodeParcel(c.Request.Body)
		if e == nil {
			var out ParcelResult
			out, e = h.ChangeParcel(ctx, actor, c.Param("id"), c.Param("fulfillment_id"), in)
			if e == nil {
				c.JSON(200, out)
				return
			}
		}
	}
	switch {
	case errors.Is(e, ErrInput), errors.Is(e, ErrIdentity):
		platform.Fail(c, 400, "devices.invalid_parcel", "Invalid parcel command or identity")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Current allocation and fulfillment permissions required")
	case errors.Is(e, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "orders.not_found", "Current order or parcel not found")
	case errors.Is(e, ErrConflict), errors.Is(e, ErrDuplicate), errors.Is(e, inventory.ErrStockHeld), errors.Is(e, inventory.ErrInsufficientStock):
		platform.Fail(c, 409, "devices.parcel_conflict", "Order, parcel, devices or inventory changed; reload")
	default:
		platform.Fail(c, 503, "devices.parcel_unavailable", "Parcel operation unavailable; retry the same operation")
	}
}
