// Package customergroups owns explicitly assigned groups for new accounts.
package customergroups

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInput = errors.New("invalid customer group command")
var ErrConflict = errors.New("customer group revision changed")
var ErrUnavailable = errors.New("customer groups unavailable")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`)

type Group struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Members int64  `json:"members"`
}
type Account struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
type Directory struct {
	Revision string  `json:"revision"`
	Enabled  bool    `json:"enabled"`
	Groups   []Group `json:"groups"`
}
type AccountPage struct {
	Accounts []Account `json:"accounts"`
	Count    int64     `json:"count"`
}
type Command struct {
	OperationID      string   `json:"operation_id"`
	ExpectedRevision string   `json:"expected_revision"`
	Kind             string   `json:"kind"`
	GroupID          string   `json:"group_id"`
	Name             string   `json:"name"`
	CustomerIDs      []string `json:"customer_ids"`
}
type Result struct {
	GroupID  string `json:"group_id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}
type runtime struct {
	Version  int
	Revision uint64
	Enabled  bool
}
type Handler struct{ DB *gorm.DB }

func installed(tx *gorm.DB) (bool, error) {
	var n int64
	if tx == nil || tx.Table("schema_migrations").Where("version=?", "0107_customer_groups.sql").Count(&n).Error != nil || n > 1 {
		return false, ErrUnavailable
	}
	return n == 1, nil
}
func lock(tx *gorm.DB, strength string) (runtime, error) {
	var r runtime
	if tx.Table("customer_group_runtime").Where("id=1").Clauses(clause.Locking{Strength: strength}).Take(&r).Error != nil || r.Version != 1 || r.Revision == 0 {
		return r, ErrUnavailable
	}
	return r, nil
}

// ActiveTx exposes names only for a pricing operator, never member identities.
// Older snapshots can still quote anonymously before this extension is installed.
func ActiveTx(tx *gorm.DB) ([]Group, error) {
	out := []Group{}
	ok, e := installed(tx)
	if e != nil || !ok {
		return out, e
	}
	if _, e = lock(tx, "SHARE"); e != nil {
		return out, e
	}
	if tx.Table("customer_groups g").Select("g.id,g.name,(SELECT COUNT(*) FROM customer_group_members m WHERE m.group_id=g.id) AS members").Where("g.deleted_at IS NULL").Order("g.name,g.id").Limit(1001).Scan(&out).Error != nil || len(out) > 1000 {
		return nil, ErrUnavailable
	}
	return out, nil
}

// ForAccountTx consumes durable membership even when new management writes are
// disabled. Identity is server-resolved; it is never an email or query argument.
// Lock accounts before the group runtime, matching membership command admission.
func ForAccountTx(tx *gorm.DB, account string) ([]string, error) {
	out := []string{}
	ok, e := installed(tx)
	if e != nil || !ok {
		return out, e
	}
	if account == "" {
		return out, nil
	}
	if !idPattern.MatchString(account) || len(account) > 80 {
		return nil, ErrInput
	}
	var a struct{ ID string }
	if tx.Table("customer_accounts").Select("id").Where("id=? AND enabled=TRUE AND verified_at IS NOT NULL", account).Clauses(clause.Locking{Strength: "SHARE"}).Take(&a).Error != nil {
		return nil, ErrConflict
	}
	if _, e = lock(tx, "SHARE"); e != nil {
		return nil, e
	}
	if tx.Table("customer_group_members m").Select("m.group_id").Joins("JOIN customer_groups g ON g.id=m.group_id AND g.deleted_at IS NULL").Where("m.customer_id=?", account).Order("m.group_id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(101).Scan(&out).Error != nil || len(out) > 100 {
		return nil, ErrUnavailable
	}
	return out, nil
}
func Valid(in Command) bool {
	u, e := uuid.Parse(in.OperationID)
	v, ve := strconv.ParseUint(in.ExpectedRevision, 10, 64)
	if e != nil || u.Version() != 4 || u.String() != in.OperationID || ve != nil || v == 0 || v == ^uint64(0) || strconv.FormatUint(v, 10) != in.ExpectedRevision || in.CustomerIDs == nil || len(in.CustomerIDs) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range in.CustomerIDs {
		if !idPattern.MatchString(id) || len(id) > 80 || seen[id] {
			return false
		}
		seen[id] = true
	}
	name := utf8.ValidString(in.Name) && strings.TrimSpace(in.Name) == in.Name && utf8.RuneCountInString(in.Name) > 0 && utf8.RuneCountInString(in.Name) <= 255 && !strings.ContainsAny(in.Name, "\x00\r\n")
	switch in.Kind {
	case "create":
		return in.GroupID == "" && name && len(in.CustomerIDs) == 0
	case "rename":
		return idPattern.MatchString(in.GroupID) && name && len(in.CustomerIDs) == 0
	case "delete":
		return idPattern.MatchString(in.GroupID) && in.Name == "" && len(in.CustomerIDs) == 0
	case "add_members", "remove_members":
		return idPattern.MatchString(in.GroupID) && in.Name == "" && len(in.CustomerIDs) > 0
	}
	return false
}
func authorize(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	actions := []authorization.Action{authorization.PriceRead, authorization.Read}
	if write {
		actions = append(actions, authorization.PriceWrite, authorization.Write)
	}
	return authorization.CheckTx(ctx, tx, actor, actions...)
}
func (h Handler) Read(ctx context.Context, actor identity.User) (Directory, error) {
	out := Directory{Groups: []Group{}}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorize(ctx, tx, actor, false); e != nil {
			return e
		}
		r, e := lock(tx, "SHARE")
		if e != nil {
			return e
		}
		out.Revision = strconv.FormatUint(r.Revision, 10)
		out.Enabled = r.Enabled
		out.Groups, e = ActiveTx(tx)
		return e
	})
	return out, e
}
func (h Handler) Accounts(ctx context.Context, actor identity.User, q, group string, offset int) (AccountPage, error) {
	out := AccountPage{Accounts: []Account{}}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	if !utf8.ValidString(q) || utf8.RuneCountInString(q) > 254 || strings.ContainsAny(q, "\x00\r\n") || group != "" && !idPattern.MatchString(group) || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorize(ctx, tx, actor, false); e != nil {
			return e
		}
		if _, e := lock(tx, "SHARE"); e != nil {
			return e
		}
		query := tx.Table("customer_accounts a").Where("a.enabled=TRUE AND a.verified_at IS NOT NULL")
		if group != "" {
			query = query.Joins("JOIN customer_group_members m ON m.customer_id=a.id AND m.group_id=?", group).Joins("JOIN customer_groups g ON g.id=m.group_id AND g.deleted_at IS NULL")
		}
		if q != "" {
			query = query.Where("LOCATE(?,a.email)>0 OR a.id=?", q, q)
		}
		if query.Count(&out.Count).Error != nil || query.Select("a.id,a.email").Order("a.email,a.id").Offset(offset).Limit(20).Scan(&out.Accounts).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
func (h Handler) Change(ctx context.Context, actor identity.User, in Command) (Result, error) {
	var out Result
	if !Valid(in) {
		return out, ErrInput
	}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	encoded, _ := json.Marshal(in)
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorize(ctx, tx, actor, true); e != nil {
			return e
		}
		// Avoid account -> runtime / runtime -> account inversion with login refresh.
		ids := append([]string{}, in.CustomerIDs...)
		sort.Strings(ids)
		for _, id := range ids {
			var a struct{ ID string }
			where := "id=?"
			if in.Kind == "add_members" {
				where += " AND enabled=TRUE AND verified_at IS NOT NULL"
			}
			if tx.Table("customer_accounts").Select("id").Where(where, id).Clauses(clause.Locking{Strength: "SHARE"}).Take(&a).Error != nil {
				return ErrConflict
			}
		}
		r, e := lock(tx, "UPDATE")
		if e != nil {
			return e
		}
		var old struct{ ActorID, RequestJSON, ResultJSON string }
		e = tx.Table("customer_group_commands").Where("operation_id=?", in.OperationID).Take(&old).Error
		if e == nil {
			var want Command
			if old.ActorID != actor.ID || json.Unmarshal([]byte(old.RequestJSON), &want) != nil {
				return ErrConflict
			}
			saved, _ := json.Marshal(want)
			if string(saved) != string(encoded) || json.Unmarshal([]byte(old.ResultJSON), &out) != nil {
				return ErrConflict
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if !r.Enabled {
			return ErrUnavailable
		}
		if strconv.FormatUint(r.Revision, 10) != in.ExpectedRevision {
			return ErrConflict
		}
		id := in.GroupID
		if in.Kind == "create" {
			id = "cgrp_" + uuid.NewString()
			if tx.Table("customer_groups").Create(map[string]any{"id": id, "name": in.Name, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
				return ErrUnavailable
			}
		} else {
			var group struct{ ID string }
			if tx.Table("customer_groups").Select("id").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&group).Error != nil {
				return ErrConflict
			}
			switch in.Kind {
			case "rename":
				e = tx.Table("customer_groups").Where("id=?", id).Updates(map[string]any{"name": in.Name, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error
			case "delete":
				e = tx.Table("customer_groups").Where("id=?", id).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error
			case "add_members":
				for _, account := range ids {
					var count int64
					if tx.Table("customer_group_members m").Joins("JOIN customer_groups g ON g.id=m.group_id AND g.deleted_at IS NULL").Where("m.customer_id=?", account).Count(&count).Error != nil || count >= 100 {
						return ErrConflict
					}
					if e = tx.Table("customer_group_members").Create(map[string]any{"group_id": id, "customer_id": account, "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error; e != nil {
						return ErrConflict
					}
				}
			case "remove_members":
				result := tx.Table("customer_group_members").Where("group_id=? AND customer_id IN ?", id, ids).Delete(map[string]any{})
				e = result.Error
				if e == nil && result.RowsAffected != int64(len(ids)) {
					return ErrConflict
				}
			}
			if e != nil {
				return ErrUnavailable
			}
		}
		if tx.Exec("UPDATE customer_group_runtime SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		out = Result{GroupID: id, Revision: strconv.FormatUint(r.Revision+1, 10)}
		saved, _ := json.Marshal(out)
		return tx.Table("customer_group_commands").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "group_id": id, "request_json": string(encoded), "result_json": string(saved), "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error
	})
	return out, e
}
