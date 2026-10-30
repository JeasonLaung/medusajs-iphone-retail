package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/google/uuid"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type ReturnInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	AssignmentID     string `json:"assignment_id"`
	SourceID         string `json:"source_id"`
	Scan             string `json:"scan"`
}
type ReturnDevice struct {
	AssignmentID     string    `json:"assignment_id"`
	DeviceID         string    `json:"device_id"`
	ItemID           string    `json:"item_id"`
	SerialNumber     string    `json:"serial_number"`
	Status           string    `json:"status"`
	State            string    `json:"state"`
	LocationID       string    `json:"location_id"`
	IMEI, IMEI2, EID *string   `json:"-"`
	RequestID        *string   `json:"-" gorm:"column:after_sales_request_id"`
	UpdatedAt        time.Time `json:"-"`
}
type ReturnContext struct {
	OrderID   string         `json:"order_id"`
	RequestID string         `json:"request_id"`
	Type      string         `json:"type"`
	Status    string         `json:"status"`
	Revision  string         `json:"revision"`
	Enabled   bool           `json:"enabled"`
	Devices   []ReturnDevice `json:"devices"`
	Sources   []Source       `json:"sources"`
}
type ReturnResult struct {
	AssignmentID string `json:"assignment_id"`
	DeviceID     string `json:"device_id"`
	ReturnID     string `json:"return_id"`
	State        string `json:"state"`
	Replayed     bool   `json:"replayed"`
}
type returnLine struct {
	ItemID    string   `json:"item_id"`
	Quantity  int      `json:"quantity"`
	DeviceIDs []string `json:"device_ids"`
}
type returnFacts struct {
	Items          string
	Version        int
	DeviceRevision uint64
	Lines          []returnLine
}

func validReturn(in ReturnInput) bool {
	return validateEnroll(EnrollInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision}) == nil && validPathID(in.AssignmentID) && validPathID(in.SourceID) && in.Scan != "" && intakeText(in.Scan, 191) && strings.TrimSpace(in.Scan) == in.Scan
}
func returnAuthority(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	actions := []authorization.Action{authorization.OrderRead, authorization.DeviceRead, authorization.WarehouseRead}
	if write {
		actions = append(actions, authorization.DeviceIntake, authorization.OrderAfterSalesWrite)
	}
	return authorization.CheckTx(ctx, tx, actor, actions...)
}
func loadReturn(ctx context.Context, tx *gorm.DB, o, r string) (ReturnContext, returnFacts, error) {
	out := ReturnContext{OrderID: o, RequestID: r, Devices: []ReturnDevice{}, Sources: []Source{}}
	facts := returnFacts{}
	rev, e := sourceGate(ctx, tx)
	if e != nil {
		return out, facts, e
	}
	facts.DeviceRevision = rev
	var cap struct {
		Version int
		Enabled bool
	}
	if tx.Table("device_return_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&cap).Error != nil || cap.Version != 1 || orders.Ready(ctx, tx) != nil {
		return out, facts, ErrUnavailable
	}
	out.Enabled = cap.Enabled
	var order struct {
		Version   int
		Status    string
		UpdatedAt time.Time
	}
	if e = tx.Table("orders").Select("version,status,updated_at").Where("id=? AND deleted_at IS NULL", o).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error; e != nil {
		return out, facts, e
	}
	facts.Version = order.Version

	var request struct {
		Type, Status, Items string
		UpdatedAt           time.Time
	}
	if e = tx.Table("after_sales_requests").Select("type,status,items,updated_at").Where("id=? AND order_id=? AND deleted_at IS NULL", r, o).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&request).Error; e != nil {
		return out, facts, e
	}
	out.Type = request.Type
	out.Status = request.Status
	facts.Items = request.Items
	var parsed struct {
		Lines []returnLine `json:"lines"`
	}
	if json.Unmarshal([]byte(request.Items), &parsed) != nil || len(parsed.Lines) == 0 || len(parsed.Lines) > 100 {
		return out, facts, ErrUnavailable
	}
	facts.Lines = parsed.Lines
	all := []ReturnDevice{}
	if tx.Table("device_assignments a").Select("a.id AS assignment_id,a.device_id,a.item_id,a.status,a.after_sales_request_id,d.serial_number,d.imei,d.imei2,d.eid,d.state,d.location_id,d.updated_at").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Where("a.order_id=? AND a.deleted_at IS NULL", o).Order("a.id").Limit(1001).Scan(&all).Error != nil || len(all) > 1000 {
		return out, facts, ErrUnavailable
	}
	for _, a := range all {
		for _, l := range facts.Lines {
			if !validPathID(l.ItemID) || l.Quantity < 1 || l.Quantity > 1000000000 {
				return out, facts, ErrUnavailable
			}
			if a.ItemID != l.ItemID {
				continue
			}
			match := len(l.DeviceIDs) == 0
			for _, id := range l.DeviceIDs {
				if id == a.DeviceID {
					match = true
				}
			}
			if match && (a.RequestID != nil && *a.RequestID == r || a.Status == "shipped" && a.State == "shipped") {
				out.Devices = append(out.Devices, a)
			}
			break
		}
	}
	if tx.Table("device_sources s").Select("s.id,s.name,s.type,s.location_id,l.name AS location_name").Joins("JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.deleted_at IS NULL").Order("s.id").Limit(1001).Scan(&out.Sources).Error != nil || len(out.Sources) > 1000 {
		return out, facts, ErrUnavailable
	}
	b, _ := json.Marshal([]any{order, request, all, out.Sources, rev})
	sum := sha256.Sum256(b)
	out.Revision = hex.EncodeToString(sum[:])
	return out, facts, nil
}
func (h Handler) ReadReturn(ctx context.Context, actor identity.User, o, r string) (ReturnContext, error) {
	var out ReturnContext
	if !validPathID(o) || !validPathID(r) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := returnAuthority(ctx, tx, actor, false); e != nil {
			return e
		}
		var e error
		out, _, e = loadReturn(ctx, tx, o, r)
		return e
	})
	return out, e
}
func returnScan(scan string, d ReturnDevice) bool {
	s := cases.Upper(language.Und).String(strings.TrimSpace(scan))
	if s == cases.Upper(language.Und).String(d.SerialNumber) {
		return true
	}
	for _, v := range []*string{d.IMEI, d.IMEI2, d.EID} {
		if v != nil && *v != "" && s == *v {
			return true
		}
	}
	return false
}

// ReceiveReturn quarantines a single shipped device. It never increments quantity stock.
func (h Handler) ReceiveReturn(ctx context.Context, actor identity.User, o, r string, in ReturnInput) (ReturnResult, error) {
	var out ReturnResult
	if !validPathID(o) || !validPathID(r) || !validReturn(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	wire, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := returnAuthority(ctx, tx, actor, true); e != nil {
			return e
		}
		before, facts, e := loadReturn(ctx, tx, o, r)
		if e != nil {
			return e
		}
		var d *ReturnDevice
		for i := range before.Devices {
			if before.Devices[i].AssignmentID == in.AssignmentID {
				d = &before.Devices[i]
			}
		}
		if d == nil {
			return gorm.ErrRecordNotFound
		}
		var dest *Source
		for i := range before.Sources {
			if before.Sources[i].ID == in.SourceID {
				dest = &before.Sources[i]
			}
		}
		if dest == nil {
			return gorm.ErrRecordNotFound
		}
		var prior struct{ ActorID, OrderID, RequestID, RequestJSON, ResultJSON string }
		e = tx.Table("device_return_receipts").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.OrderID != o || prior.RequestID != r || prior.RequestJSON != string(wire) {
				return ErrConflict
			}
			if d.RequestID == nil || *d.RequestID != r {
				return gorm.ErrRecordNotFound
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.AssignmentID != in.AssignmentID {
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
		if before.Revision != in.ExpectedRevision || before.Status != "processing" || d.Status != "shipped" || d.State != "shipped" || d.RequestID != nil && *d.RequestID == r || !returnScan(in.Scan, *d) {
			return ErrConflict
		}
		if d.RequestID != nil && *d.RequestID != r {
			var previous struct{ Status string }
			if tx.Table("after_sales_requests").Select("status").Where("id=? AND order_id=? AND deleted_at IS NULL", *d.RequestID, o).Take(&previous).Error != nil || (previous.Status != "completed" && previous.Status != "rejected") {
				return ErrConflict
			}
		}
		var currentOrder struct{ Status string }
		if tx.Table("orders").Select("status").Where("id=?", o).Take(&currentOrder).Error != nil {
			return ErrUnavailable
		}
		if currentOrder.Status == "canceled" {
			return ErrConflict
		}
		var quantity int
		for _, l := range facts.Lines {
			if l.ItemID == d.ItemID {
				quantity = l.Quantity
				break
			}
		}
		var received int64
		if tx.Table("device_assignments").Where("after_sales_request_id=? AND item_id=? AND deleted_at IS NULL", r, d.ItemID).Count(&received).Error != nil {
			return ErrUnavailable
		}
		if quantity < 1 || received >= int64(quantity) {
			return ErrConflict
		}
		out = ReturnResult{AssignmentID: d.AssignmentID, DeviceID: d.DeviceID, State: "inspection"}
		if before.Type == "repair" {
			out.State = "repair"
		} else if before.Type != "return" && before.Type != "exchange" {
			return ErrUnavailable
		}
		if before.Type == "return" {
			id, e := quarantineReturn(tx, o, d.ItemID, dest.LocationID, facts.Version, r)
			if e != nil {
				return e
			}
			out.ReturnID = id
		}
		if tx.Exec("UPDATE devices SET location_id=?,state=?,category='returned',updated_at=UTC_TIMESTAMP(6) WHERE id=?", dest.LocationID, out.State, d.DeviceID).Error != nil || tx.Exec("UPDATE device_assignments SET status='returned',after_sales_request_id=?,return_id=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", r, nullableReturn(out.ReturnID), d.AssignmentID).Error != nil {
			return ErrUnavailable
		}
		data, _ := json.Marshal(map[string]any{"assignment_id": d.AssignmentID, "request_id": r, "type": before.Type, "destination": in.SourceID})
		if tx.Exec("INSERT INTO device_events(id,device_id,order_id,actor_id,action,data,created_at,updated_at) VALUES(?,?,?,?,'receive',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "devent_"+uuid.NewString(), d.DeviceID, o, actor.ID, string(data)).Error != nil {
			return ErrUnavailable
		}
		result, _ := json.Marshal(out)
		if tx.Exec("INSERT INTO device_return_receipts(operation_id,actor_id,order_id,request_id,assignment_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, o, r, d.AssignmentID, string(wire), string(result)).Error != nil || tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
func nullableReturn(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Final facts of the original RETURN_ITEM then RECEIVE_DAMAGED_RETURN_ITEM:
// requested is unchanged; dismissed and written-off each rise by one; received stays zero.
// Operational quantities keep the current Gin order snapshot, as shipment does.
func quarantineReturn(tx *gorm.DB, o, item, location string, version int, request string) (string, error) {
	var line struct{ ID, Shipped, Dismissed, WrittenOff, Received, Requested string }
	var lineCount int64
	if tx.Table("order_items").Where("order_id=? AND item_id=? AND version=? AND deleted_at IS NULL", o, item, version).Count(&lineCount).Error != nil || lineCount != 1 {
		return "", ErrConflict
	}
	if tx.Table("order_items").Select("id,CAST(shipped_quantity AS CHAR) AS shipped,CAST(return_dismissed_quantity AS CHAR) AS dismissed,CAST(written_off_quantity AS CHAR) AS written_off,CAST(return_received_quantity AS CHAR) AS received,CAST(return_requested_quantity AS CHAR) AS requested").Where("order_id=? AND item_id=? AND version=? AND deleted_at IS NULL", o, item, version).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&line).Error != nil {
		return "", ErrConflict
	}
	remaining, e := inventory.Difference(line.Shipped, line.Dismissed)
	if e != nil {
		return "", ErrConflict
	}
	remaining, e = inventory.Difference(remaining, line.Received)
	if e != nil {
		return "", ErrConflict
	}
	remaining, e = inventory.Difference(remaining, line.Requested)
	if e != nil {
		return "", ErrConflict
	}
	remaining, e = inventory.Difference(remaining, "1")
	if e != nil || !nonnegative(remaining) {
		return "", ErrConflict
	}
	dismissed, e := inventory.Difference(line.Dismissed, "-1")
	if e != nil {
		return "", e
	}
	written, e := inventory.Difference(line.WrittenOff, "-1")
	if e != nil {
		return "", e
	}
	id := "return_" + uuid.NewString()
	var schemaID int
	if tx.Table("order_runtime_schema").Select("id").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&schemaID).Error != nil || schemaID != 1 {
		return "", ErrUnavailable
	}
	var display int64
	if tx.Table("returns").Select("COALESCE(MAX(display_id),0)+1").Scan(&display).Error != nil || display > 2147483647 {
		return "", ErrUnavailable
	}
	metadata, _ := json.Marshal(map[string]string{"after_sales_request_id": request, "quarantine": "true"})
	if tx.Exec("INSERT INTO returns(id,order_id,order_version,display_id,status,location_id,metadata,received_at,created_at,updated_at) VALUES(?,?,?,?,'received',?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, o, version, display, location, string(metadata)).Error != nil {
		return "", ErrUnavailable
	}
	if tx.Exec("INSERT INTO return_items(id,return_id,item_id,quantity,raw_quantity,received_quantity,raw_received_quantity,damaged_quantity,raw_damaged_quantity,created_at,updated_at) VALUES(?,?,?,1,?,1,?,1,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "retitem_"+uuid.NewString(), id, item, rawCount("1"), rawCount("1"), rawCount("1")).Error != nil {
		return "", ErrUnavailable
	}
	if tx.Exec("UPDATE order_items SET return_dismissed_quantity=?,raw_return_dismissed_quantity=?,written_off_quantity=?,raw_written_off_quantity=?,updated_at=UTC_TIMESTAMP(6) WHERE id=?", dismissed, rawCount(dismissed), written, rawCount(written), line.ID).Error != nil {
		return "", ErrUnavailable
	}
	return id, nil
}
