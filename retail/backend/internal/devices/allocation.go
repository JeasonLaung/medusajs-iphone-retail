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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AllocationInput struct {
	OperationID      string   `json:"operation_id"`
	ExpectedRevision string   `json:"expected_revision"`
	Action           string   `json:"action"`
	IDs              []string `json:"ids"`
}
type AllocationDevice struct {
	ID           string    `json:"id"`
	SerialNumber string    `json:"serial_number"`
	IMEI         *string   `json:"imei"`
	IMEI2        *string   `json:"imei2"`
	EID          *string   `json:"eid"`
	VariantID    string    `json:"-"`
	SourceID     string    `json:"source_id"`
	SourceName   string    `json:"source_name"`
	LocationID   string    `json:"location_id"`
	LocationName string    `json:"location_name"`
	State        string    `json:"state"`
	Category     string    `json:"-"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type AllocationAssignment struct {
	ID            string    `json:"id"`
	DeviceID      string    `json:"device_id"`
	OrderID       string    `json:"-"`
	ItemID        string    `json:"-"`
	SourceID      string    `json:"-"`
	Status        string    `json:"status"`
	FulfillmentID *string   `json:"fulfillment_id"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type AllocationContext struct {
	OrderID     string                 `json:"order_id"`
	ItemID      string                 `json:"item_id"`
	VariantID   string                 `json:"variant_id"`
	Version     int                    `json:"version"`
	Paid        bool                   `json:"paid"`
	Enabled     bool                   `json:"enabled"`
	OrderActive bool                   `json:"order_active"`
	Quantity    int                    `json:"quantity"`
	Fulfilled   int                    `json:"fulfilled"`
	Remaining   int                    `json:"remaining"`
	Devices     []AllocationDevice     `json:"devices"`
	Assignments []AllocationAssignment `json:"assignments"`
	Revision    string                 `json:"revision"`
}
type AllocationResult struct {
	AssignmentIDs []string `json:"assignment_ids"`
	Revision      string   `json:"revision"`
	Replayed      bool     `json:"replayed"`
}

func validAllocation(in AllocationInput) bool {
	id, e := uuid.Parse(in.OperationID)
	rev, e2 := hex.DecodeString(in.ExpectedRevision)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || e2 != nil || len(rev) != 32 || hex.EncodeToString(rev) != in.ExpectedRevision || (in.Action != "reserve" && in.Action != "release") || len(in.IDs) < 1 || len(in.IDs) > 200 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range in.IDs {
		if !validPathID(v) || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func decodeAllocation(r io.Reader) (AllocationInput, error) {
	var in AllocationInput
	b, e := io.ReadAll(io.LimitReader(r, 65537))
	if e != nil || len(b) > 65536 || !utf8.Valid(b) {
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
		case "operation_id", "expected_revision", "action", "ids":
		default:
			return in, ErrInput
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return in, ErrInput
		}
		fields[k] = raw
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') || len(fields) != 4 || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	b, _ = json.Marshal(fields)
	if json.Unmarshal(b, &in) != nil || !validAllocation(in) {
		return in, ErrInput
	}
	return in, nil
}

// Source orderItemQuantities reads the persisted numeric quantity before raw
// fallback. Allocation of serialized devices requires an exact integral count.
func allocationUnits(s string) (int, error) {
	parts := strings.Split(s, ".")
	if len(parts) > 2 || len(parts) == 2 && strings.Trim(parts[1], "0") != "" {
		return 0, ErrConflict
	}
	n, e := strconv.Atoi(parts[0])
	if e != nil || n < 0 || n > 1000000000 {
		return 0, ErrConflict
	}
	return n, nil
}
func allocationGate(ctx context.Context, tx *gorm.DB, actor identity.User) (uint64, bool, error) {
	if e := authorization.CheckTx(ctx, tx, actor, authorization.OrderRead, authorization.OrderPaymentRead, authorization.DeviceRead, authorization.OrderAllocationWrite); e != nil {
		return 0, false, e
	}
	return allocationRuntimeGate(ctx, tx)
}
func allocationRuntimeGate(ctx context.Context, tx *gorm.DB) (uint64, bool, error) {
	revision, e := sourceGate(ctx, tx)
	if e != nil {
		return 0, false, e
	}
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("order_device_allocation_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&cap).Error != nil || cap.Version != 1 || orders.Ready(ctx, tx) != nil || payments.Ready(ctx, tx) != nil {
		return 0, false, ErrUnavailable
	}
	return revision, cap.Enabled, nil
}
func loadAllocation(tx *gorm.DB, orderID, itemID string, revision uint64, enabled bool) (AllocationContext, error) {
	out := AllocationContext{OrderID: orderID, ItemID: itemID, Enabled: enabled, Devices: []AllocationDevice{}, Assignments: []AllocationAssignment{}}
	var order struct {
		ID, Status     string
		SalesChannelID *string
		Version        int
		IsDraftOrder   bool
		CanceledAt     *time.Time
		UpdatedAt      time.Time
	}
	if e := tx.Table("orders").Select("id,status,sales_channel_id,version,is_draft_order,canceled_at,updated_at").Where("id=? AND deleted_at IS NULL", orderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error; e != nil {
		return out, e
	}
	out.Version = order.Version
	out.OrderActive = !order.IsDraftOrder && order.Status != "draft" && order.Status != "canceled" && order.CanceledAt == nil
	var line struct {
		ID, VariantID, Quantity, FulfilledQuantity string
		UpdatedAt                                  time.Time
	}
	if e := tx.Table("order_items i").Select("i.id,l.variant_id,i.quantity,i.fulfilled_quantity,i.updated_at").Joins("JOIN order_line_items l ON l.id=i.item_id AND l.deleted_at IS NULL").Where("i.order_id=? AND i.version=? AND i.item_id=? AND i.deleted_at IS NULL", orderID, order.Version, itemID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&line).Error; e != nil {
		return out, e
	}
	out.VariantID = line.VariantID
	var e error
	out.Quantity, e = allocationUnits(line.Quantity)
	if e != nil {
		return out, e
	}
	out.Fulfilled, e = allocationUnits(line.FulfilledQuantity)
	if e != nil || out.Fulfilled > out.Quantity {
		return out, ErrConflict
	}
	var collections []struct {
		ID, PaymentCollectionID string
		Status                  *string
		UpdatedAt               *time.Time
	}
	if tx.Table("order_payment_collections l").Select("l.id,l.payment_collection_id,p.status,p.updated_at").Joins("LEFT JOIN payment_collections p ON p.id=l.payment_collection_id AND p.deleted_at IS NULL").Where("l.order_id=? AND l.deleted_at IS NULL", orderID).Order("l.id").Limit(101).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&collections).Error != nil || len(collections) > 100 {
		return out, ErrUnavailable
	}
	out.Paid = len(collections) > 0
	for _, p := range collections {
		out.Paid = out.Paid && p.Status != nil && *p.Status == "completed"
	}
	if tx.Table("device_assignments").Select("id,device_id,order_id,item_id,source_id,status,fulfillment_id,updated_at").Where("order_id=? AND item_id=? AND deleted_at IS NULL", orderID, itemID).Order("id").Limit(10001).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&out.Assignments).Error != nil || len(out.Assignments) > 10000 {
		return out, ErrUnavailable
	}
	active, reserved := 0, 0
	for _, a := range out.Assignments {
		if a.Status == "reserved" || a.Status == "packed" || a.Status == "shipped" {
			active++
		}
		if a.Status == "reserved" {
			reserved++
		}
	}
	out.Remaining = min(out.Quantity-active, out.Quantity-out.Fulfilled-reserved)
	if out.Remaining < 0 {
		out.Remaining = 0
	}
	if order.SalesChannelID != nil {
		if tx.Table("devices d").Select("d.id,d.serial_number,d.imei,d.imei2,d.eid,d.variant_id,d.source_id,s.name AS source_name,d.location_id,l.name AS location_name,d.state,d.category,d.updated_at").Joins("JOIN device_sources s ON s.id=d.source_id AND s.deleted_at IS NULL AND s.location_id=d.location_id").Joins("JOIN inventory_locations l ON l.id=d.location_id AND l.deleted_at IS NULL").Where("d.variant_id=? AND d.deleted_at IS NULL AND d.state='available' AND d.category='sale'", line.VariantID).Where("EXISTS (SELECT 1 FROM commerce_channel_locations cl WHERE cl.stock_location_id=d.location_id AND cl.sales_channel_id=? AND cl.deleted_at IS NULL)", *order.SalesChannelID).Where("NOT EXISTS (SELECT 1 FROM device_assignments a WHERE a.device_id=d.id AND a.status IN ('reserved','packed','shipped') AND a.deleted_at IS NULL)").Order("d.location_id,d.id").Limit(10001).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&out.Devices).Error != nil || len(out.Devices) > 10000 {
			return out, ErrUnavailable
		}
	}
	// Include hidden binding facts explicitly; response omits order/source internals.
	b, _ := json.Marshal([]any{order, line, collections, out.Assignments, out.Devices, revision})
	h := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(h[:])
	return out, nil
}
func (h Handler) ReadAllocation(ctx context.Context, actor identity.User, o, item string) (AllocationContext, error) {
	var out AllocationContext
	if !validPathID(o) || !validPathID(item) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := allocationGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		out, e = loadAllocation(tx, o, item, rev, enabled)
		return e
	})
	return out, e
}
func (h Handler) Allocate(ctx context.Context, actor identity.User, o, item string, in AllocationInput) (AllocationResult, error) {
	var out AllocationResult
	if !validPathID(o) || !validPathID(item) || !validAllocation(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rev, enabled, e := allocationGate(ctx, tx, actor)
		if e != nil {
			return e
		}
		if !enabled {
			return ErrUnavailable
		}
		before, e := loadAllocation(tx, o, item, rev, enabled)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, OrderID, ItemID, RequestJSON, ResultJSON string }
		e = tx.Table("order_device_allocations").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.OrderID != o || receipt.ItemID != item || receipt.RequestJSON != string(request) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(receipt.ResultJSON), &out) != nil {
				return ErrUnavailable
			}
			for _, id := range out.AssignmentIDs {
				var n int64
				if tx.Table("device_assignments a").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Where("a.id=? AND a.order_id=? AND a.item_id=? AND a.deleted_at IS NULL", id, o, item).Count(&n).Error != nil || n != 1 {
					return gorm.ErrRecordNotFound
				}
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !before.OrderActive || before.Revision != in.ExpectedRevision {
			return ErrConflict
		}
		if in.Action == "reserve" && (!before.Paid || len(in.IDs) > before.Remaining) {
			return ErrConflict
		}
		out.AssignmentIDs = []string{}
		candidates := map[string]AllocationDevice{}
		for _, d := range before.Devices {
			candidates[d.ID] = d
		}
		assignments := map[string]AllocationAssignment{}
		for _, a := range before.Assignments {
			assignments[a.ID] = a
		}
		for _, id := range in.IDs {
			var deviceID, assignmentID string
			if in.Action == "reserve" {
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
				identity := Identity{SerialNumber: d.SerialNumber}
				if d.IMEI != nil {
					identity.IMEI = *d.IMEI
				}
				if d.IMEI2 != nil {
					identity.IMEI2 = *d.IMEI2
				}
				if d.EID != nil {
					identity.EID = *d.EID
				}
				normalized, e := NormalizeIdentity(identity, sku.RequiresRadio)
				if e != nil || normalized != identity {
					return ErrIdentity
				}
				if e = ClaimIdentityTx(ctx, tx, d.ID, identity, sku.RequiresRadio); e != nil {
					return e
				}
				assignmentID = "dasg_" + uuid.NewString()
				deviceID = d.ID
				if tx.Exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES(?,?,?,?,?,'reserved',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", assignmentID, d.ID, o, item, d.SourceID).Error != nil {
					return ErrUnavailable
				}
				if tx.Exec("UPDATE devices SET state='reserved',updated_at=UTC_TIMESTAMP(6) WHERE id=? AND state='available' AND deleted_at IS NULL", d.ID).RowsAffected != 1 {
					return ErrConflict
				}
			} else {
				a, ok := assignments[id]
				if !ok || a.Status != "reserved" || a.FulfillmentID != nil {
					return ErrConflict
				}
				var d struct{ ID, State string }
				if tx.Table("devices").Select("id,state").Where("id=? AND deleted_at IS NULL", a.DeviceID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error != nil || d.State != "reserved" {
					return ErrConflict
				}
				assignmentID = a.ID
				deviceID = a.DeviceID
				if tx.Exec("UPDATE device_assignments SET status='canceled',updated_at=UTC_TIMESTAMP(6) WHERE id=?", a.ID).Error != nil || tx.Exec("UPDATE devices SET state='available',updated_at=UTC_TIMESTAMP(6) WHERE id=?", a.DeviceID).Error != nil {
					return ErrUnavailable
				}
			}
			out.AssignmentIDs = append(out.AssignmentIDs, assignmentID)
			data, _ := json.Marshal(map[string]any{"assignment_id": assignmentID, "item_id": item})
			if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), deviceID, o, actor.ID, map[bool]string{true: "assign", false: "release"}[in.Action == "reserve"], string(data)).Error != nil {
				return ErrUnavailable
			}
		}
		if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		after, e := loadAllocation(tx, o, item, rev+1, enabled)
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
func (h Handler) AllocationHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	fail := func(e error) {
		switch {
		case errors.Is(e, ErrInput), errors.Is(e, ErrIdentity):
			platform.Fail(c, 400, "devices.invalid_allocation", "Invalid allocation input or device identity")
		case errors.Is(e, authorization.ErrDenied):
			platform.Fail(c, 403, "authorization.denied", "Current allocation permission required")
		case errors.Is(e, gorm.ErrRecordNotFound):
			platform.Fail(c, 404, "devices.not_found", "Current order or item not found")
		case errors.Is(e, ErrConflict), errors.Is(e, ErrDuplicate):
			platform.Fail(c, 409, "devices.allocation_conflict", "Payment, order or devices changed; reload")
		default:
			platform.Fail(c, 503, "devices.unavailable", "Allocation unavailable; retry the same operation")
		}
	}
	if c.Request.URL.RawQuery != "" {
		fail(ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	o, item := c.Param("id"), c.Param("item_id")
	if c.Request.Method == "GET" {
		out, e := h.ReadAllocation(ctx, actor, o, item)
		if e != nil {
			fail(e)
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
		fail(e)
		return
	}
	out, e := h.Allocate(ctx, actor, o, item, in)
	if e != nil {
		fail(e)
		return
	}
	c.JSON(200, out)
}
