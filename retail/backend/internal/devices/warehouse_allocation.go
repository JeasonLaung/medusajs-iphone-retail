package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime"
	"sort"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WarehouseAllocationDevice struct {
	ID           string `json:"id"`
	SerialNumber string `json:"serial_number"`
	AssignmentID string `json:"assignment_id"`
}
type WarehouseAllocation struct {
	LocationID    string                      `json:"location_id"`
	OrderID       string                      `json:"order_id"`
	FulfillmentID string                      `json:"fulfillment_id"`
	ItemID        string                      `json:"item_id"`
	Enabled       bool                        `json:"enabled"`
	Editable      bool                        `json:"editable"`
	Quantity      int                         `json:"quantity"`
	Assigned      []WarehouseAllocationDevice `json:"assigned"`
	Candidates    []WarehouseAllocationDevice `json:"candidates"`
	Revision      string                      `json:"revision"`
}
type warehouseAllocationFacts struct {
	Parcel     ParcelContext
	Allocation AllocationContext
}

func warehouseAllocationGate(ctx context.Context, tx *gorm.DB, actor identity.User, location string) (uint64, bool, error) {
	if e := authorization.CheckWarehouseTx(ctx, tx, actor, location, authorization.OrderFulfillmentRead, authorization.WarehouseAllocationWrite); e != nil {
		return 0, false, e
	}
	rev, enabled, e := allocationRuntimeGate(ctx, tx)
	if e != nil {
		return 0, false, e
	}
	if fulfillment.Ready(ctx, tx) != nil {
		return 0, false, ErrUnavailable
	}
	return rev, enabled, nil
}
func loadWarehouseAllocation(tx *gorm.DB, location, o, f, item string, rev uint64, enabled bool) (WarehouseAllocation, warehouseAllocationFacts, error) {
	out := WarehouseAllocation{LocationID: location, OrderID: o, FulfillmentID: f, ItemID: item, Enabled: enabled, Assigned: []WarehouseAllocationDevice{}, Candidates: []WarehouseAllocationDevice{}}
	var facts warehouseAllocationFacts
	p, _, e := loadParcel(tx, o, f, rev, enabled)
	if e != nil {
		return out, facts, e
	}
	if e = warehouseShipmentBinding(p, location); e != nil {
		return out, facts, e
	}
	facts.Parcel = p
	// Keep the raw decimal authoritative; device-backed parcel quantities must be integers.
	var rows []struct{ Quantity, RawQuantity string }
	if tx.Table("fulfillment_items").Select("CAST(quantity AS CHAR) AS quantity,raw_quantity").Where("fulfillment_id=? AND line_item_id=? AND deleted_at IS NULL", f, item).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Scan(&rows).Error != nil {
		return out, facts, ErrUnavailable
	}
	if len(rows) != 1 {
		return out, facts, gorm.ErrRecordNotFound
	}
	var raw totals.RawValue
	if json.Unmarshal([]byte(rows[0].RawQuantity), &raw) != nil {
		return out, facts, ErrUnavailable
	}
	value, e := totals.Resolve(totals.Number{Decimal: rows[0].Quantity, Raw: &raw})
	if e != nil {
		return out, facts, ErrConflict
	}
	out.Quantity, e = allocationUnits(value)
	if e != nil || out.Quantity < 1 {
		return out, facts, ErrConflict
	}
	a, e := loadAllocationAt(tx, o, item, rev, enabled, location)
	if e != nil {
		return out, facts, e
	}
	facts.Allocation = a
	if a.Fulfilled < out.Quantity {
		return out, facts, ErrConflict
	}
	out.Editable = p.OrderActive && a.Paid && (p.Status == "packed" || p.Status == "unpacked")
	for _, d := range p.Devices {
		if d.ItemID == item {
			if d.State != d.Status || d.VariantID != a.VariantID || d.SourceID != d.AssignmentSourceID {
				return out, facts, ErrConflict
			}
			out.Assigned = append(out.Assigned, WarehouseAllocationDevice{ID: d.DeviceID, SerialNumber: d.SerialNumber, AssignmentID: d.AssignmentID})
		}
	}
	if len(out.Assigned) > out.Quantity {
		return out, facts, ErrConflict
	}
	for _, d := range a.Devices {
		out.Candidates = append(out.Candidates, WarehouseAllocationDevice{ID: d.ID, SerialNumber: d.SerialNumber})
	}
	b, _ := json.Marshal([]any{out, p.Revision, a.Revision, rows})
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, facts, nil
}
func (h Handler) ReadWarehouseAllocation(ctx context.Context, actor identity.User, location, o, f, item string) (WarehouseAllocation, error) {
	var out WarehouseAllocation
	if !validPathID(location) || !validPathID(o) || !validPathID(f) || !validPathID(item) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := warehouseAllocationGate(ctx, tx, actor, location)
		if e != nil {
			return e
		}
		out, _, e = loadWarehouseAllocation(tx, location, o, f, item, rev, enabled)
		return e
	})
	return out, e
}

// IDs are the desired devices for this parcel line. Existing devices may be
// replaced, but reducing the assigned count requires the original parcel-cancel
// workflow and is deliberately rejected here; no quantity inventory is mutated.
func (h Handler) AllocateWarehouseParcel(ctx context.Context, actor identity.User, location, o, f, item string, in AllocationInput) (AllocationResult, error) {
	var out AllocationResult
	if !validPathID(location) || !validPathID(o) || !validPathID(f) || !validPathID(item) || !validAllocation(in) || in.Action != "reserve" {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal([]any{"warehouse-parcel-allocation", location, o, f, item, in})
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := warehouseAllocationGate(ctx, tx, actor, location)
		if e != nil {
			return e
		}
		if !enabled {
			return ErrUnavailable
		}
		before, facts, e := loadWarehouseAllocation(tx, location, o, f, item, rev, enabled)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, OrderID, ItemID, RequestJSON, ResultJSON string }
		e = tx.Table("order_device_allocations").Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.OrderID != o || receipt.ItemID != item || receipt.RequestJSON != string(request) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(receipt.ResultJSON), &out) != nil || len(out.AssignmentIDs) == 0 {
				return ErrUnavailable
			}
			for _, id := range out.AssignmentIDs {
				if !packingExactlyOne(tx.Table("device_assignments a").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL AND d.location_id=?", location).Where("a.id=? AND a.order_id=? AND a.item_id=? AND a.fulfillment_id=? AND a.deleted_at IS NULL", id, o, item, f), "a.id") {
					return gorm.ErrRecordNotFound
				}
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !before.Editable || before.Revision != in.ExpectedRevision || len(in.IDs) > before.Quantity || len(in.IDs) < len(before.Assigned) {
			return ErrConflict
		}
		selected := map[string]bool{}
		for _, id := range in.IDs {
			selected[id] = true
		}
		current := map[string]WarehouseAllocationDevice{}
		var replaced []WarehouseAllocationDevice
		for _, d := range before.Assigned {
			current[d.ID] = d
			if !selected[d.ID] {
				replaced = append(replaced, d)
			}
		}
		candidates := map[string]AllocationDevice{}
		for _, d := range facts.Allocation.Devices {
			candidates[d.ID] = d
		}
		active := 0
		for _, a := range facts.Allocation.Assignments {
			if a.Status == "reserved" || a.Status == "packed" || a.Status == "shipped" {
				active++
			}
		}
		if active+len(in.IDs)-len(before.Assigned) > facts.Allocation.Quantity {
			return ErrConflict
		}
		ids := append([]string{}, in.IDs...)
		sort.Strings(ids)
		replacement := 0
		out.AssignmentIDs = []string{}
		for _, id := range ids {
			if d, ok := current[id]; ok {
				out.AssignmentIDs = append(out.AssignmentIDs, d.AssignmentID)
				continue
			}
			d, ok := candidates[id]
			if !ok {
				return ErrConflict
			}
			var productID string
			if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", d.VariantID).Scan(&productID).Error != nil || productID == "" {
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
			if e = ClaimIdentityTx(ctx, tx, d.ID, ident, sku.RequiresRadio); e != nil {
				return e
			}
			assignmentID := "dasg_" + uuid.NewString()
			if replacement < len(replaced) {
				old := replaced[replacement]
				replacement++
				assignmentID = old.AssignmentID
				if tx.Exec("UPDATE devices SET state='available',updated_at=UTC_TIMESTAMP(6) WHERE id=? AND state='packed' AND location_id=? AND deleted_at IS NULL", old.ID, location).RowsAffected != 1 {
					return ErrConflict
				}
				if tx.Exec("UPDATE device_assignments SET device_id=?,source_id=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND status='packed' AND fulfillment_id=? AND deleted_at IS NULL", d.ID, d.SourceID, assignmentID, f).RowsAffected != 1 {
					return ErrConflict
				}
				event, _ := json.Marshal(map[string]string{"assignment_id": assignmentID, "previous_device_id": old.ID, "device_id": d.ID, "fulfillment_id": f})
				if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,'replace_allocation',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), old.ID, o, actor.ID, string(event)).Error != nil {
					return ErrUnavailable
				}
			} else if tx.Exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,fulfillment_id,created_at,updated_at) VALUES(?,?,?,?,?,'packed',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", assignmentID, d.ID, o, item, d.SourceID, f).Error != nil {
				return ErrUnavailable
			}
			if tx.Exec("UPDATE devices SET state='packed',updated_at=UTC_TIMESTAMP(6) WHERE id=? AND state='available' AND location_id=? AND deleted_at IS NULL", d.ID, location).RowsAffected != 1 {
				return ErrConflict
			}
			event, _ := json.Marshal(map[string]string{"assignment_id": assignmentID, "item_id": item, "fulfillment_id": f})
			if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,'assign',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), d.ID, o, actor.ID, string(event)).Error != nil {
				return ErrUnavailable
			}
			out.AssignmentIDs = append(out.AssignmentIDs, assignmentID)
		}
		if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		after, _, e := loadWarehouseAllocation(tx, location, o, f, item, rev+1, enabled)
		if e != nil {
			return e
		}
		out.Revision = after.Revision
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		result, _ := json.Marshal(out)
		return tx.Table("order_device_allocations").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "order_id": o, "item_id": item, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_json": string(result), "created_at": time.Now().UTC()}).Error
	})
	return out, e
}
func (h Handler) WarehouseAllocationHTTP(c *gin.Context) {
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
	location, o, f, item := c.Param("location_id"), c.Param("order_id"), c.Param("fulfillment_id"), c.Param("item_id")
	if c.Request.Method == "GET" {
		out, e := h.ReadWarehouseAllocation(ctx, actor, location, o, f, item)
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
	in, e := decodeAllocation(c.Request.Body)
	if e != nil {
		parcelFail(c, e)
		return
	}
	out, e := h.AllocateWarehouseParcel(ctx, actor, location, o, f, item, in)
	if e != nil {
		parcelFail(c, e)
		return
	}
	c.JSON(200, out)
}
