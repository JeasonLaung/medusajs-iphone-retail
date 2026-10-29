// Package warehouse manages locations without granting inventory or order rights.
package warehouse

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInput = errors.New("invalid warehouse input")
var ErrConflict = errors.New("warehouse revision conflict")
var ErrUnavailable = errors.New("warehouse management unavailable")
var ErrNotFound = errors.New("warehouse unavailable")

type Address struct {
	Address1    string `json:"address_1" gorm:"column:address_1"`
	Address2    string `json:"address_2" gorm:"column:address_2"`
	Company     string `json:"company"`
	City        string `json:"city"`
	CountryCode string `json:"country_code"`
	Phone       string `json:"phone"`
	Province    string `json:"province"`
	PostalCode  string `json:"postal_code"`
}
type Location struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Address *Address `json:"address"`
}
type Directory struct {
	Revision  string     `json:"revision"`
	Locations []Location `json:"locations"`
}
type Command struct {
	OperationID      string  `json:"operation_id"`
	ExpectedRevision string  `json:"expected_revision"`
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Address          Address `json:"address"`
}
type Result struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}
type Handler struct{ DB *gorm.DB }
type runtime struct {
	Version  int
	Revision uint64
	Enabled  bool
}

func gate(tx *gorm.DB, strength string) (runtime, error) {
	var r runtime
	if tx.Table("warehouse_management_runtime").Where("id=1").Clauses(clause.Locking{Strength: strength}).Take(&r).Error != nil || r.Version != 1 || r.Revision == 0 || !r.Enabled {
		return r, ErrUnavailable
	}
	return r, nil
}
func text(v string, max int, required bool) bool {
	return utf8.ValidString(v) && utf8.RuneCountInString(v) <= max && (!required || strings.TrimSpace(v) != "") && !strings.ContainsFunc(v, func(r rune) bool { return r < 32 || r == 127 })
}
func validID(v string) bool { return text(v, 191, true) && !strings.ContainsAny(v, "/\\?#") }
func validate(in Command) error {
	id, e := uuid.Parse(in.OperationID)
	r, e2 := strconv.ParseUint(in.ExpectedRevision, 10, 64)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || e2 != nil || r == 0 || strconv.FormatUint(r, 10) != in.ExpectedRevision || in.ID != "" && !validID(in.ID) || !text(in.Name, 160, true) {
		return ErrInput
	}
	a := in.Address
	if len(a.CountryCode) != 2 || a.CountryCode[0] < 'a' || a.CountryCode[0] > 'z' || a.CountryCode[1] < 'a' || a.CountryCode[1] > 'z' {
		return ErrInput
	}
	for _, f := range []struct {
		v        string
		max      int
		required bool
	}{{a.Address1, 500, true}, {a.Address2, 500, false}, {a.Company, 160, false}, {a.City, 160, false}, {a.Phone, 80, false}, {a.Province, 160, false}, {a.PostalCode, 40, false}} {
		if !text(f.v, f.max, f.required) {
			return ErrInput
		}
	}
	return nil
}
func readLocation(tx *gorm.DB, id string) (Location, error) {
	var row struct {
		ID, Name  string
		AddressID *string
	}
	e := tx.Table("inventory_locations").Select("id,name,address_id").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "SHARE"}).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return Location{}, ErrNotFound
	}
	if e != nil {
		return Location{}, ErrUnavailable
	}
	out := Location{ID: row.ID, Name: row.Name}
	if row.AddressID != nil {
		var a Address
		q := tx.Table("inventory_location_addresses").Select("address_1,COALESCE(address_2,'') AS address_2,COALESCE(company,'') AS company,COALESCE(city,'') AS city,country_code,COALESCE(phone,'') AS phone,COALESCE(province,'') AS province,COALESCE(postal_code,'') AS postal_code").Where("id=? AND deleted_at IS NULL", *row.AddressID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&a)
		if q.Error == nil {
			out.Address = &a
		} else if !errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return Location{}, ErrUnavailable
		}
	}
	return out, nil
}
func (h Handler) Read(ctx context.Context, actor identity.User) (Directory, error) {
	out := Directory{Locations: []Location{}}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.WarehouseRead); e != nil {
			return e
		}
		r, e := gate(tx, "SHARE")
		if e != nil {
			return e
		}
		var ids []string
		if tx.Table("inventory_locations").Select("id").Where("deleted_at IS NULL").Order("id").Limit(1001).Scan(&ids).Error != nil || len(ids) > 1000 {
			return ErrUnavailable
		}
		for _, id := range ids {
			location, e := readLocation(tx, id)
			if e != nil {
				return e
			}
			out.Locations = append(out.Locations, location)
		}
		out.Revision = strconv.FormatUint(r.Revision, 10)
		return nil
	})
	return out, err
}
func (h Handler) Save(ctx context.Context, actor identity.User, in Command) (Result, error) {
	if validate(in) != nil {
		return Result{}, ErrInput
	}
	if h.DB == nil {
		return Result{}, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	var out Result
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.WarehouseRead, authorization.WarehouseWrite); e != nil {
			return e
		}
		r, e := gate(tx, "UPDATE")
		if e != nil {
			return e
		}
		var previous struct {
			ActorID, LocationID, RequestJSON string
			ResultRevision                   uint64
		}
		e = tx.Table("warehouse_changes").Where("operation_id=?", in.OperationID).Take(&previous).Error
		if e == nil {
			if previous.ActorID != actor.ID || previous.RequestJSON != string(request) {
				return ErrConflict
			}
			if _, e = readLocation(tx, previous.LocationID); e != nil {
				return e
			}
			out = Result{previous.LocationID, strconv.FormatUint(previous.ResultRevision, 10), true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if strconv.FormatUint(r.Revision, 10) != in.ExpectedRevision || r.Revision == math.MaxUint64 {
			return ErrConflict
		}
		// A real registered country is required; syntax alone is insufficient.
		var count int64
		if tx.Table("commerce_countries").Where("iso_2=? AND deleted_at IS NULL", in.Address.CountryCode).Count(&count).Error != nil {
			return ErrUnavailable
		}
		if count != 1 {
			return ErrInput
		}
		var before *Location
		id := in.ID
		if id != "" {
			v, e := readLocation(tx, id)
			if e != nil {
				return e
			}
			before = &v
		} else {
			id = "sloc_" + uuid.NewString()
		}
		a := in.Address
		addressID := "sladdr_" + uuid.NewString()
		// Copy on write: imported addresses may be shared by other locations.
		if tx.Exec("INSERT INTO inventory_location_addresses(id,address_1,address_2,company,city,country_code,phone,province,postal_code,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", addressID, a.Address1, a.Address2, a.Company, a.City, a.CountryCode, a.Phone, a.Province, a.PostalCode).Error != nil {
			return ErrUnavailable
		}
		if before == nil {
			if tx.Exec("INSERT INTO inventory_locations(id,name,address_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, in.Name, addressID).Error != nil {
				return ErrUnavailable
			}
		} else {
			q := tx.Exec("UPDATE inventory_locations SET name=?,address_id=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", in.Name, addressID, id)
			if q.Error != nil || q.RowsAffected != 1 {
				return ErrUnavailable
			}
		}
		after := Location{ID: id, Name: in.Name, Address: &a}
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		if tx.Exec("INSERT INTO warehouse_changes(operation_id,actor_id,location_id,request_json,before_json,after_json,result_revision,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", in.OperationID, actor.ID, id, string(request), string(oldJSON), string(newJSON), r.Revision+1).Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("UPDATE warehouse_management_runtime SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		out = Result{id, strconv.FormatUint(r.Revision+1, 10), false}
		return nil
	})
	return out, err
}
