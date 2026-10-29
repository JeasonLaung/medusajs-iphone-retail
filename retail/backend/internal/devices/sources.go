package devices

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SourceHandler struct {
	DB      *gorm.DB
	StoreID string
}
type Source struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	LocationID   string  `json:"location_id"`
	LocationName *string `json:"location_name"`
}
type Sources struct {
	Revision string   `json:"revision"`
	Sources  []Source `json:"sources"`
}
type SourceInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	Name             string `json:"name"`
	Type             string `json:"type"`
}
type SourceResult struct {
	Source   Source `json:"source"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func sourceAuthority(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	actions := []authorization.Action{authorization.DeviceRead, authorization.WarehouseRead}
	if write {
		actions = append(actions, authorization.DeviceSourceWrite, authorization.WarehouseWrite)
	}
	return authorization.CheckTx(ctx, tx, actor, actions...)
}
func sourceGate(ctx context.Context, tx *gorm.DB) (uint64, error) {
	if e := LockIdentityTx(ctx, tx); e != nil {
		return 0, e
	}
	var r struct {
		Revision        uint64
		CommandsEnabled bool
	}
	if tx.Table("device_runtime_schema").Where("id=1").Take(&r).Error != nil || !r.CommandsEnabled || r.Revision == 0 || r.Revision == math.MaxUint64 {
		return 0, ErrUnavailable
	}
	return r.Revision, nil
}
func validateSource(in SourceInput) error {
	id, e := uuid.Parse(in.OperationID)
	rev, e2 := strconv.ParseUint(in.ExpectedRevision, 10, 64)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || e2 != nil || rev == 0 || strconv.FormatUint(rev, 10) != in.ExpectedRevision || !utf8.ValidString(in.Name) || strings.TrimSpace(in.Name) != in.Name || in.Name == "" || utf8.RuneCountInString(in.Name) > 100 || strings.ContainsFunc(in.Name, unicode.IsControl) || (in.Type != "self" && in.Type != "supplier") {
		return ErrInput
	}
	return nil
}
func (h SourceHandler) Read(ctx context.Context, actor identity.User) (Sources, error) {
	out := Sources{Sources: []Source{}}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := sourceAuthority(ctx, tx, actor, false); e != nil {
			return e
		}
		revision, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		if tx.Table("device_sources s").Select("s.id,s.name,s.type,s.location_id,l.name AS location_name").Joins("LEFT JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.deleted_at IS NULL").Order("s.id").Limit(1001).Scan(&out.Sources).Error != nil || len(out.Sources) > 1000 {
			return ErrUnavailable
		}
		out.Revision = strconv.FormatUint(revision, 10)
		return nil
	})
	return out, e
}
func (h SourceHandler) Save(ctx context.Context, actor identity.User, in SourceInput) (SourceResult, error) {
	var out SourceResult
	if validateSource(in) != nil {
		return out, ErrInput
	}
	if h.DB == nil || !validPathID(h.StoreID) {
		return out, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := sourceAuthority(ctx, tx, actor, true); e != nil {
			return e
		}
		revision, e := sourceGate(ctx, tx)
		if e != nil {
			return e
		}
		// Coordinate the existing warehouse editor's revision and preserve its lock
		// through source, location and relationship creation and receipt persistence.
		var warehouse struct {
			Version  int
			Revision uint64
			Enabled  bool
		}
		if tx.Table("warehouse_management_runtime").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&warehouse).Error != nil || warehouse.Version != 1 || !warehouse.Enabled || warehouse.Revision == 0 || warehouse.Revision == math.MaxUint64 {
			return ErrUnavailable
		}
		var prior struct{ ActorID, StoreID, RequestJSON, ResultJSON string }
		e = tx.Table("device_source_changes").Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || prior.StoreID != h.StoreID || prior.RequestJSON != string(request) {
				return ErrConflict
			}
			if json.Unmarshal([]byte(prior.ResultJSON), &out) != nil || out.Source.ID == "" || out.Source.LocationID == "" {
				return ErrUnavailable
			}
			var count int64
			if tx.Table("device_sources s").Joins("JOIN inventory_locations l ON l.id=s.location_id AND l.deleted_at IS NULL").Where("s.id=? AND s.location_id=? AND s.deleted_at IS NULL", out.Source.ID, out.Source.LocationID).Count(&count).Error != nil {
				return ErrUnavailable
			}
			if count != 1 {
				return ErrNotFound
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if strconv.FormatUint(revision, 10) != in.ExpectedRevision {
			return ErrConflict
		}
		var duplicate struct{ ID string }
		e = tx.Table("device_sources").Select("id").Where("name=? AND deleted_at IS NULL", in.Name).Clauses(clause.Locking{Strength: "SHARE"}).Take(&duplicate).Error
		if e == nil {
			return ErrConflict
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		// Use the configured store, never the first row of an unordered store list.
		var store struct{ DefaultSalesChannelID *string }
		if tx.Table("commerce_stores").Select("default_sales_channel_id").Where("id=? AND deleted_at IS NULL", h.StoreID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&store).Error != nil || store.DefaultSalesChannelID == nil {
			return ErrUnavailable
		}
		var channel struct{ ID string }
		if tx.Table("commerce_sales_channels").Select("id").Where("id=? AND is_disabled=FALSE AND deleted_at IS NULL", *store.DefaultSalesChannelID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&channel).Error != nil {
			return ErrUnavailable
		}
		var provider struct{ ID string }
		if tx.Table("fulfillment_providers").Select("id").Where("id='manual_manual' AND is_enabled=TRUE AND deleted_at IS NULL").Clauses(clause.Locking{Strength: "SHARE"}).Take(&provider).Error != nil {
			return ErrUnavailable
		}
		sourceID, locationID := "dsrc_"+uuid.NewString(), "sloc_"+uuid.NewString()
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{"INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES(?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", []any{locationID, in.Name}},
			{"INSERT INTO commerce_channel_locations(id,sales_channel_id,stock_location_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", []any{"scloc_" + uuid.NewString(), channel.ID, locationID}},
			{"INSERT INTO fulfillment_location_providers(id,stock_location_id,fulfillment_provider_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", []any{"slprov_" + uuid.NewString(), locationID, provider.ID}},
			{"INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES(?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", []any{sourceID, in.Name, in.Type, locationID}},
		} {
			if tx.Exec(q.sql, q.args...).Error != nil {
				return ErrUnavailable
			}
		}
		if tx.Exec("UPDATE warehouse_management_runtime SET revision=revision+1 WHERE id=1").Error != nil || tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		out = SourceResult{Source: Source{ID: sourceID, Name: in.Name, Type: in.Type, LocationID: locationID, LocationName: &in.Name}, Revision: strconv.FormatUint(revision+1, 10)}
		result, _ := json.Marshal(out)
		if tx.Exec("INSERT INTO device_source_changes(operation_id,actor_id,store_id,request_json,result_json,created_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, h.StoreID, string(request), string(result)).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
