package warehouse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeliverySet struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Linked bool   `json:"linked"`
}
type DeliveryDirectory struct {
	Revision   string        `json:"revision"`
	LocationID string        `json:"location_id"`
	Sets       []DeliverySet `json:"sets"`
}
type DeliveryCommand struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	LocationID       string `json:"location_id"`
	SetID            string `json:"set_id"`
}

func deliverySets(tx *gorm.DB, location string) ([]DeliverySet, error) {
	sets := []DeliverySet{}
	e := tx.Table("fulfillment_sets s").Select("s.id,s.name,EXISTS(SELECT 1 FROM fulfillment_location_sets l WHERE l.fulfillment_set_id=s.id AND l.stock_location_id=? AND l.deleted_at IS NULL) AS linked", location).Where("s.type='shipping' AND s.deleted_at IS NULL").Order("s.id").Limit(1001).Scan(&sets).Error
	if e != nil || len(sets) > 1000 {
		return nil, ErrUnavailable
	}
	return sets, nil
}
func (h Handler) ReadDelivery(ctx context.Context, actor identity.User, location string) (DeliveryDirectory, error) {
	out := DeliveryDirectory{LocationID: location}
	if !validID(location) || h.DB == nil {
		return out, ErrInput
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.WarehouseRead); e != nil {
			return e
		}
		r, e := gate(tx, "SHARE")
		if e != nil {
			return e
		}
		if _, e = readLocation(tx, location); e != nil {
			return e
		}
		out.Revision = strconv.FormatUint(r.Revision, 10)
		out.Sets, e = deliverySets(tx, location)
		return e
	})
	return out, e
}
func (h Handler) LinkDelivery(ctx context.Context, actor identity.User, in DeliveryCommand) (Result, error) {
	var out Result
	op, e := uuid.Parse(in.OperationID)
	if e != nil || op.Version() != 4 || op.String() != in.OperationID || !validID(in.LocationID) || !validID(in.SetID) {
		return out, ErrInput
	}
	if revision, e := strconv.ParseUint(in.ExpectedRevision, 10, 64); e != nil || revision == 0 || strconv.FormatUint(revision, 10) != in.ExpectedRevision {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.WarehouseRead, authorization.WarehouseWrite); e != nil {
			return e
		}
		r, e := gate(tx, "UPDATE")
		if e != nil {
			return e
		}
		if _, e = readLocation(tx, in.LocationID); e != nil {
			return e
		}
		var old struct {
			ActorID, RequestJSON string
			ResultRevision       uint64
		}
		e = tx.Table("warehouse_changes").Where("operation_id=?", in.OperationID).Take(&old).Error
		if e == nil {
			if old.ActorID != actor.ID || old.RequestJSON != string(request) {
				return ErrConflict
			}
			out = Result{in.LocationID, strconv.FormatUint(old.ResultRevision, 10), true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if strconv.FormatUint(r.Revision, 10) != in.ExpectedRevision {
			return ErrConflict
		}
		var set struct{ ID string }
		if e = tx.Table("fulfillment_sets").Select("id").Where("id=? AND type='shipping' AND deleted_at IS NULL", in.SetID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&set).Error; e != nil {
			return ErrNotFound
		}
		before, e := deliverySets(tx, in.LocationID)
		if e != nil {
			return e
		}
		linked := false
		for _, s := range before {
			if s.ID == in.SetID {
				linked = s.Linked
			}
		}
		if r.Revision == math.MaxUint64 {
			return ErrUnavailable
		}
		if !linked {
			if e = tx.Table("fulfillment_location_sets").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "stock_location_id"}, {Name: "fulfillment_set_id"}}, DoUpdates: clause.Assignments(map[string]any{"deleted_at": nil, "updated_at": time.Now().UTC()})}).Create(map[string]any{"id": "slfuset_" + uuid.NewString(), "stock_location_id": in.LocationID, "fulfillment_set_id": in.SetID, "created_at": time.Now().UTC(), "updated_at": time.Now().UTC()}).Error; e != nil {
				return ErrUnavailable
			}
		}
		after, e := deliverySets(tx, in.LocationID)
		if e != nil {
			return e
		}
		a, _ := json.Marshal(after)
		b, _ := json.Marshal(before)
		if e = tx.Table("warehouse_changes").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "location_id": in.LocationID, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": r.Revision + 1, "created_at": time.Now().UTC()}).Error; e != nil {
			return ErrUnavailable
		}
		if e = tx.Exec("UPDATE warehouse_management_runtime SET revision=revision+1 WHERE id=1").Error; e != nil {
			return ErrUnavailable
		}
		out = Result{in.LocationID, strconv.FormatUint(r.Revision+1, 10), false}
		return nil
	})
	return out, e
}
func (h Handler) DeliveryHTTP(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		fail(c, authorization.ErrDenied)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if c.Request.Method == "GET" {
		out, e := h.ReadDelivery(ctx, actor, c.Param("id"))
		if e != nil {
			fail(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		c.Status(415)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 16385))
	if e != nil || len(raw) > 16384 || !utf8.Valid(raw) {
		fail(c, ErrInput)
		return
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	fields, e := object(d, map[string]bool{"operation_id": true, "expected_revision": true, "location_id": true, "set_id": true}, false)
	if e != nil {
		fail(c, ErrInput)
		return
	}
	raw, _ = json.Marshal(fields)
	var in DeliveryCommand
	if json.Unmarshal(raw, &in) != nil || in.LocationID != c.Param("id") {
		fail(c, ErrInput)
		return
	}
	out, e := h.LinkDelivery(ctx, actor, in)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
