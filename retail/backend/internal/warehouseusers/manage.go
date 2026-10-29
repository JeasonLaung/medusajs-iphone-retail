// Package warehouseusers manages independently created warehouse accounts only.
package warehouseusers

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInput = errors.New("invalid warehouse user request")
var ErrConflict = errors.New("warehouse users changed")
var ErrMissing = errors.New("warehouse user unavailable")
var ErrUnavailable = errors.New("warehouse user management unavailable")

type Handler struct{ DB *gorm.DB }
type User struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Email     string                 `json:"email"`
	Enabled   bool                   `json:"enabled"`
	Mode      string                 `json:"mode"`
	Locations []string               `json:"locations"`
	Actions   []authorization.Action `json:"actions"`
}
type Location struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Directory struct {
	Revision  string     `json:"revision"`
	Users     []User     `json:"users"`
	Locations []Location `json:"locations"`
}
type Command struct {
	OperationID      string                 `json:"operation_id"`
	ExpectedRevision string                 `json:"expected_revision"`
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Email            string                 `json:"email"`
	Password         string                 `json:"password"`
	Enabled          bool                   `json:"enabled"`
	Mode             string                 `json:"mode"`
	Locations        []string               `json:"locations"`
	Actions          []authorization.Action `json:"actions"`
}
type Result struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func allowed(a authorization.Action) bool {
	return a == authorization.InventoryRead || a == authorization.StockWrite || a == authorization.OrderFulfillmentRead
}
func safeText(s string, max int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func normalize(c Command) (Command, error) {
	id, e := uuid.Parse(c.OperationID)
	if e != nil || id.Version() != 4 || id.String() != c.OperationID {
		return c, ErrInput
	}
	rev, e := strconv.ParseUint(c.ExpectedRevision, 10, 64)
	if e != nil || rev == 0 || strconv.FormatUint(rev, 10) != c.ExpectedRevision {
		return c, ErrInput
	}
	if c.ID != "" {
		id, e = uuid.Parse(strings.TrimPrefix(c.ID, "usr_"))
		if e != nil || id.Version() != 4 || "usr_"+id.String() != c.ID {
			return c, ErrInput
		}
	}
	if c.Name == "" || !safeText(c.Name, 160) {
		return c, ErrInput
	}
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	a, e := mail.ParseAddress(c.Email)
	if e != nil || a.Address != c.Email || len(c.Email) > 254 || !safeText(c.Email, 254) {
		return c, ErrInput
	}
	for _, r := range c.Email {
		if r > 127 {
			return c, ErrInput
		}
	}
	if c.ID == "" && c.Password == "" || c.Password != "" && (len(c.Password) < 12 || len(c.Password) > 72 || !utf8.ValidString(c.Password)) {
		return c, ErrInput
	}
	if c.Mode != "all" && c.Mode != "selected" || c.Locations == nil || len(c.Locations) > 1000 || c.Mode == "all" && len(c.Locations) != 0 || c.Actions == nil || len(c.Actions) > 3 {
		return c, ErrInput
	}
	c.Locations = append([]string{}, c.Locations...)
	sort.Strings(c.Locations)
	for i, id := range c.Locations {
		if id == "" || !safeText(id, 191) || strings.ContainsAny(id, "/\\?#") || i > 0 && id == c.Locations[i-1] {
			return c, ErrInput
		}
	}
	c.Actions = append([]authorization.Action{}, c.Actions...)
	sort.Slice(c.Actions, func(i, j int) bool { return c.Actions[i] < c.Actions[j] })
	for i, a := range c.Actions {
		if !allowed(a) || i > 0 && a == c.Actions[i-1] {
			return c, ErrInput
		}
	}
	return c, nil
}
func gate(tx *gorm.DB) error {
	for _, table := range []string{"warehouse_user_runtime", "warehouse_scope_runtime"} {
		var g struct {
			Version int
			Enabled bool
		}
		if tx.Table(table).Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&g).Error != nil || g.Version != 1 || !g.Enabled {
			return ErrUnavailable
		}
	}
	return nil
}
func revision(tx *gorm.DB) (uint64, error) {
	var r struct{ Revision uint64 }
	if tx.Table("authorization_runtime_schema").Where("id=1").Take(&r).Error != nil || r.Revision == 0 {
		return 0, ErrUnavailable
	}
	return r.Revision, nil
}
func readUser(tx *gorm.DB, id string) (User, string, error) {
	u := User{Locations: []string{}, Actions: []authorization.Action{}}
	var row struct {
		ID, Name, Email, Kind, Mode, RoleID string
		Enabled                             bool
	}
	e := tx.Table("warehouse_user_accounts w").Select("u.id,u.name,u.email,u.enabled,p.kind,s.mode,w.role_id").
		Joins("JOIN admin_users u ON u.id=w.user_id").Joins("JOIN authorization_principals p ON p.user_id=w.user_id").Joins("JOIN authorization_warehouse_scopes s ON s.user_id=w.user_id").Where("w.user_id=?", id).Clauses(clause.Locking{Strength: "SHARE"}).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return u, "", ErrMissing
	}
	if e != nil || row.Kind != "warehouse" || row.Mode != "all" && row.Mode != "selected" {
		return u, "", ErrUnavailable
	}
	u.ID = row.ID
	u.Name = row.Name
	u.Email = row.Email
	u.Enabled = row.Enabled
	u.Mode = row.Mode
	if tx.Table("authorization_warehouse_locations").Where("user_id=?", id).Order("location_id ASC").Limit(1001).Pluck("location_id", &u.Locations).Error != nil || len(u.Locations) > 1000 {
		return u, "", ErrUnavailable
	}
	if tx.Table("authorization_role_actions").Where("role_id=?", row.RoleID).Order("action_code ASC").Limit(4).Pluck("action_code", &u.Actions).Error != nil || len(u.Actions) > 3 {
		return u, "", ErrUnavailable
	}
	for _, a := range u.Actions {
		if !allowed(a) {
			return u, "", ErrUnavailable
		}
	}
	if u.Mode == "all" && len(u.Locations) != 0 {
		return u, "", ErrUnavailable
	}
	return u, row.RoleID, nil
}
func (h Handler) Read(ctx context.Context, actor identity.User) (Directory, error) {
	d := Directory{Users: []User{}, Locations: []Location{}}
	if h.DB == nil {
		return d, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.Read, authorization.WarehouseRead); e != nil {
			return e
		}
		if e := gate(tx); e != nil {
			return e
		}
		r, e := revision(tx)
		if e != nil {
			return e
		}
		d.Revision = strconv.FormatUint(r, 10)
		var ids []string
		if tx.Table("warehouse_user_accounts").Order("user_id ASC").Limit(1001).Pluck("user_id", &ids).Error != nil || len(ids) > 1000 {
			return ErrUnavailable
		}
		for _, id := range ids {
			u, _, e := readUser(tx, id)
			if e != nil {
				return e
			}
			d.Users = append(d.Users, u)
		}
		if tx.Table("inventory_locations").Select("id,name").Where("deleted_at IS NULL").Order("id ASC").Limit(1001).Find(&d.Locations).Error != nil || len(d.Locations) > 1000 {
			return ErrUnavailable
		}
		return nil
	})
	return d, e
}
func (h Handler) Save(ctx context.Context, actor identity.User, input Command) (Result, error) {
	c, e := normalize(input)
	if e != nil {
		return Result{}, e
	}
	if h.DB == nil {
		return Result{}, ErrUnavailable
	}
	canonical := c
	canonical.Password = ""
	raw, _ := json.Marshal(canonical)
	var out Result
	e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lock struct{ ID int }
		if tx.Table("authorization_runtime_schema").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&lock).Error != nil {
			return ErrUnavailable
		}
		if e := authorization.CheckTx(ctx, tx, actor, authorization.Read, authorization.Write, authorization.WarehouseRead, authorization.WarehouseWrite); e != nil {
			return e
		}
		if e := gate(tx); e != nil {
			return e
		}
		if len(c.Actions) > 0 {
			if e := authorization.CheckTx(ctx, tx, actor, c.Actions...); e != nil {
				return e
			}
		}
		var receipt struct {
			ActorID, UserID, RequestJSON, PasswordHash string
			ResultRevision                             uint64
		}
		e := tx.Table("warehouse_user_changes").Where("operation_id=?", c.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.RequestJSON != string(raw) || c.Password == "" && receipt.PasswordHash != "" || c.Password != "" && bcrypt.CompareHashAndPassword([]byte(receipt.PasswordHash), []byte(c.Password)) != nil {
				return ErrConflict
			}
			out = Result{receipt.UserID, strconv.FormatUint(receipt.ResultRevision, 10), true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		r, e := revision(tx)
		if e != nil {
			return e
		}
		if strconv.FormatUint(r, 10) != c.ExpectedRevision {
			return ErrConflict
		}
		if r == math.MaxUint64 {
			return ErrUnavailable
		}
		for _, id := range c.Locations {
			var location struct{ ID string }
			q := tx.Table("inventory_locations").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=?", id)
			if c.Enabled {
				q = q.Where("deleted_at IS NULL")
			}
			e := q.Take(&location).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrMissing
			}
			if e != nil {
				return ErrUnavailable
			}
		}
		hash := ""
		if c.Password != "" {
			encoded, e := bcrypt.GenerateFromPassword([]byte(c.Password), bcrypt.DefaultCost)
			if e != nil {
				return ErrUnavailable
			}
			hash = string(encoded)
		}
		now := time.Now().UTC()
		id := c.ID
		roleID := ""
		var before any
		if id == "" {
			var count int64
			if tx.Table("authorization_roles").Count(&count).Error != nil || count >= 100 {
				return ErrUnavailable
			}
			id = "usr_" + uuid.NewString()
			roleID = "role_" + uuid.NewString()
			u := identity.User{ID: id, Email: c.Email, Name: c.Name, Enabled: c.Enabled, Role: "admin", AuthVersion: 1, PasswordHash: hash, CreatedAt: now, UpdatedAt: now}
			if e := tx.Create(&u).Error; e != nil {
				var duplicate *driver.MySQLError
				if errors.As(e, &duplicate) && duplicate.Number == 1062 {
					return ErrConflict
				}
				return ErrUnavailable
			}
			for _, q := range []struct {
				SQL  string
				Args []any
			}{
				{"INSERT INTO authorization_principals(user_id,kind) VALUES(?,'warehouse')", []any{id}},
				{"INSERT INTO authorization_roles(id,name,enabled) VALUES(?,?,TRUE)", []any{roleID, "Warehouse " + id}},
				{"INSERT INTO authorization_memberships(user_id,role_id) VALUES(?,?)", []any{id, roleID}},
				{"INSERT INTO authorization_warehouse_scopes(user_id,mode) VALUES(?,?)", []any{id, c.Mode}},
				{"INSERT INTO warehouse_user_accounts(user_id,role_id) VALUES(?,?)", []any{id, roleID}},
			} {
				if tx.Exec(q.SQL, q.Args...).Error != nil {
					return ErrUnavailable
				}
			}
		} else {
			u, managedRole, e := readUser(tx, id)
			if e != nil {
				return e
			}
			roleID = managedRole
			before = u
			// Identity generations are always rotated, including scope-only edits.
			var current identity.User
			if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", id).Take(&current).Error != nil || current.AuthVersion == 0 || current.AuthVersion == math.MaxUint64 || current.Role != "admin" {
				return ErrUnavailable
			}
			if c.Email != current.Email {
				return ErrInput
			}
			updates := map[string]any{"name": c.Name, "enabled": c.Enabled, "auth_version": current.AuthVersion + 1, "updated_at": now}
			if hash != "" {
				updates["password_hash"] = hash
			}
			if tx.Model(&identity.User{}).Where("id=?", id).Updates(updates).Error != nil {
				return ErrUnavailable
			}
			if tx.Exec("UPDATE authorization_warehouse_scopes SET mode=? WHERE user_id=?", c.Mode, id).Error != nil {
				return ErrUnavailable
			}
			if tx.Exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", id).Error != nil || tx.Exec("DELETE FROM authorization_role_actions WHERE role_id=?", roleID).Error != nil {
				return ErrUnavailable
			}
		}
		for _, location := range c.Locations {
			if tx.Exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", id, location).Error != nil {
				return ErrUnavailable
			}
		}
		for _, a := range c.Actions {
			if tx.Exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,?)", roleID, a).Error != nil {
				return ErrUnavailable
			}
		}
		after := User{id, c.Name, c.Email, c.Enabled, c.Mode, c.Locations, c.Actions}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		if tx.Exec("INSERT INTO warehouse_user_changes(operation_id,actor_id,user_id,request_json,password_hash,before_json,after_json,result_revision,created_at) VALUES(?,?,?,?,?,?,?,?,?)", c.OperationID, actor.ID, id, string(raw), hash, string(b), string(a), r+1, now).Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("UPDATE authorization_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		out = Result{id, strconv.FormatUint(r+1, 10), false}
		return nil
	})
	return out, e
}
