package devices

import (
	"bytes"
	"context"
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
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WarehouseBatchItem struct {
	ID               string `json:"id"`
	ExpectedRevision string `json:"expected_revision"`
}
type WarehouseBatchInput struct {
	OperationID string               `json:"operation_id"`
	Devices     []WarehouseBatchItem `json:"devices"`
}
type WarehouseBatchResult struct {
	IDs      []string `json:"ids"`
	State    string   `json:"state"`
	Replayed bool     `json:"replayed"`
}

func validWarehouseBatch(in WarehouseBatchInput) bool {
	id, e := uuid.Parse(in.OperationID)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || len(in.Devices) < 1 || len(in.Devices) > 200 {
		return false
	}
	seen := map[string]bool{}
	for _, d := range in.Devices {
		if !validPathID(d.ID) || seen[d.ID] || validateEnroll(EnrollInput{OperationID: in.OperationID, ExpectedRevision: d.ExpectedRevision}) != nil {
			return false
		}
		seen[d.ID] = true
	}
	return true
}

func decodeWarehouseBatch(r io.Reader) (WarehouseBatchInput, error) {
	var in WarehouseBatchInput
	raw, e := io.ReadAll(io.LimitReader(r, 65537))
	if e != nil || len(raw) > 65536 || !utf8.Valid(raw) {
		return in, ErrInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if uniqueJSON(decoder) != nil || decoder.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || !validWarehouseBatch(in) {
		return in, ErrInput
	}
	// Require every member; null and omitted fields are not commands.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 2 {
		return in, ErrInput
	}
	var rows []map[string]json.RawMessage
	if json.Unmarshal(fields["devices"], &rows) != nil {
		return in, ErrInput
	}
	for _, row := range rows {
		if len(row) != 2 {
			return in, ErrInput
		}
	}
	return in, nil
}

// One outer transaction owns the complete batch. Existing single-device rules,
// identity claims, events and receipts are reused, including their row locks.
func (h Handler) MakeWarehouseBatchAvailable(ctx context.Context, actor identity.User, location string, in WarehouseBatchInput) (WarehouseBatchResult, error) {
	var out WarehouseBatchResult
	if !validPathID(location) || !validWarehouseBatch(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	raw, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckWarehouseTx(ctx, tx, actor, location, authorization.WarehouseIntakeRead, authorization.WarehouseIntakeWrite); e != nil {
			return e
		}
		revision, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		rows := make([]WarehouseDevice, len(in.Devices))
		for i, d := range in.Devices {
			e := warehouseDeviceQuery(tx).Where("d.id=? AND d.location_id=? AND d.category='sale' AND d.deleted_at IS NULL", d.ID, location).Take(&rows[i]).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			if e != nil {
				return ErrUnavailable
			}
		}
		// Current scope and objects remain authoritative even for a committed retry.
		var prior struct{ ActorID, LocationID, Category, DeviceID, RequestJSON, ResultJSON string }
		e = tx.Table("warehouse_device_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.LocationID != location || prior.Category != "sale" || prior.DeviceID != in.Devices[0].ID || prior.RequestJSON != string(raw) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || len(out.IDs) != len(in.Devices) || out.State != "available" {
				return ErrUnavailable
			}
			for i, d := range in.Devices {
				if out.IDs[i] != d.ID {
					return ErrUnavailable
				}
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		for i := range rows {
			if e := projectWarehouseDevice(&rows[i], revision); e != nil {
				return e
			}
			if rows[i].Revision != in.Devices[i].ExpectedRevision {
				return ErrConflict
			}
		}
		out = WarehouseBatchResult{IDs: []string{}, State: "available"}
		scoped := Handler{DB: tx}
		for _, d := range in.Devices {
			// Every original revision was checked before the first mutation. The shared
			// device revision advances within this transaction after each existing command.
			current, e := sourceGate(ctx, tx)
			if e != nil {
				return e
			}
			var row WarehouseDevice
			if warehouseDeviceQuery(tx).Where("d.id=?", d.ID).Take(&row).Error != nil {
				return ErrUnavailable
			}
			if e := projectWarehouseDevice(&row, current); e != nil {
				return e
			}
			if _, e := scoped.CommandWarehouseDevice(ctx, actor, location, "sale", d.ID, WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: row.Revision, Action: "make_available"}); e != nil {
				return e
			}
			out.IDs = append(out.IDs, d.ID)
		}
		result, _ := json.Marshal(out)
		if tx.Exec("INSERT INTO warehouse_device_changes(operation_id,actor_id,location_id,category,device_id,request_json,result_json,created_at) VALUES(?,?,?,'sale',?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, location, in.Devices[0].ID, string(raw), string(result)).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}

func (h Handler) WarehouseBatchAvailableHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "device.content_type", "JSON required")
		return
	}
	if c.Request.URL.RawQuery != "" {
		trackingFail(c, ErrInput)
		return
	}
	in, e := decodeWarehouseBatch(c.Request.Body)
	if e != nil {
		trackingFail(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	out, e := h.MakeWarehouseBatchAvailable(ctx, actor, c.Param("location_id"), in)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, out)
}
