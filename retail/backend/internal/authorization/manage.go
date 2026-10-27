package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInput = errors.New("invalid authorization command")
var ErrConflict = errors.New("authorization revision conflict")
var ErrLastManager = errors.New("at least one enabled permission manager required")
var ErrNotFound = errors.New("authorization subject unavailable")

type Role struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Actions []Action `json:"actions"`
}
type Member struct {
	UserID  string   `json:"user_id"`
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Kind    string   `json:"kind"`
	Roles   []string `json:"roles"`
}
type Directory struct {
	Revision string   `json:"revision"`
	Actions  []Action `json:"actions"`
	Roles    []Role   `json:"roles"`
	Members  []Member `json:"members"`
}
type Command struct {
	OperationID      string   `json:"operation_id"`
	ExpectedRevision string   `json:"expected_revision"`
	Kind             string   `json:"kind"`
	Role             *Role    `json:"role"`
	UserID           *string  `json:"user_id"`
	Roles            []string `json:"roles"`
}
type ChangeResult struct {
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func normalize(in Command) (Command, error) {
	parsed, e := uuid.Parse(in.OperationID)
	if e != nil || parsed.String() != in.OperationID || parsed.Version() != 4 {
		return Command{}, ErrInput
	}
	revision, e := strconv.ParseUint(in.ExpectedRevision, 10, 64)
	if e != nil || revision == 0 || strconv.FormatUint(revision, 10) != in.ExpectedRevision {
		return Command{}, ErrInput
	}
	switch in.Kind {
	case "role.save":
		if in.Role == nil || in.UserID != nil || in.Roles != nil {
			return Command{}, ErrInput
		}
		r := *in.Role
		r.Actions = append([]Action{}, r.Actions...)
		if !identifier.MatchString(r.ID) || !utf8.ValidString(r.Name) || strings.TrimSpace(r.Name) == "" || utf8.RuneCountInString(r.Name) > 160 || strings.ContainsRune(r.Name, 0) || in.Role.Actions == nil || len(r.Actions) > len(Actions()) {
			return Command{}, ErrInput
		}
		sort.Slice(r.Actions, func(i, j int) bool { return r.Actions[i] < r.Actions[j] })
		for i, a := range r.Actions {
			if !known(a) || i > 0 && a == r.Actions[i-1] {
				return Command{}, ErrInput
			}
		}
		in.Role = &r
	case "member.roles":
		if in.Role != nil || in.UserID == nil || !identifier.MatchString(*in.UserID) || in.Roles == nil || len(in.Roles) > 100 {
			return Command{}, ErrInput
		}
		in.Roles = append([]string{}, in.Roles...)
		sort.Strings(in.Roles)
		for i, id := range in.Roles {
			if !identifier.MatchString(id) || i > 0 && id == in.Roles[i-1] {
				return Command{}, ErrInput
			}
		}
	default:
		return Command{}, ErrInput
	}
	return in, nil
}

// Save owns the complete permission transaction, global revision lock, current
// authority check, exact attempt receipt and before/after audit. Nothing commits
// if the caller lost its write grant or would remove the last active manager.
func (h Handler) Save(ctx context.Context, actor identity.User, input Command) (ChangeResult, error) {
	in, err := normalize(input)
	if err != nil {
		return ChangeResult{}, err
	}
	if h.DB == nil {
		return ChangeResult{}, ErrUnavailable
	}
	request, _ := json.Marshal(in)
	var result ChangeResult
	err = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m marker
		if tx.Table("authorization_runtime_schema").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&m).Error != nil || m.SchemaVersion != 1 || m.ModelSHA256 != modelSHA || !m.Enabled || m.Revision == 0 {
			return ErrUnavailable
		}
		if err := CheckTx(ctx, tx, actor, Write); err != nil {
			return err
		}
		var previous struct {
			ActorID, RequestJSON string
			ResultRevision       uint64
		}
		e := tx.Table("authorization_changes").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&previous).Error
		if e == nil {
			if previous.ActorID != actor.ID || previous.RequestJSON != string(request) {
				return ErrConflict
			}
			result = ChangeResult{strconv.FormatUint(previous.ResultRevision, 10), true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if strconv.FormatUint(m.Revision, 10) != in.ExpectedRevision {
			return ErrConflict
		}
		if m.Revision == math.MaxUint64 {
			return ErrUnavailable
		}
		// Granting a role cannot delegate an action the manager does not hold.
		view, e := inspect(ctx, tx, actor, true)
		if e != nil {
			return e
		}
		held := map[Action]bool{}
		for _, a := range view.Actions {
			held[a] = true
		}
		var before, after any
		switch in.Kind {
		case "role.save":
			for _, a := range in.Role.Actions {
				if !held[a] {
					return ErrDenied
				}
			}
			var old struct {
				ID, Name string
				Enabled  bool
			}
			e := tx.Table("authorization_roles").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", in.Role.ID).Take(&old).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrUnavailable
			}
			if e == nil {
				actions, err := roleActions(tx, old.ID)
				if err != nil {
					return err
				}
				before = Role{old.ID, old.Name, old.Enabled, actions}
				if tx.Exec("UPDATE authorization_roles SET name=?,enabled=? WHERE id=?", in.Role.Name, in.Role.Enabled, in.Role.ID).Error != nil {
					return ErrUnavailable
				}
			} else {
				var count int64
				if tx.Table("authorization_roles").Count(&count).Error != nil || count >= 100 {
					return ErrUnavailable
				}
				if tx.Exec("INSERT INTO authorization_roles(id,name,enabled) VALUES(?,?,?)", in.Role.ID, in.Role.Name, in.Role.Enabled).Error != nil {
					return ErrUnavailable
				}
			}
			if tx.Exec("DELETE FROM authorization_role_actions WHERE role_id=?", in.Role.ID).Error != nil {
				return ErrUnavailable
			}
			for _, a := range in.Role.Actions {
				if tx.Exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,?)", in.Role.ID, a).Error != nil {
					return ErrUnavailable
				}
			}
			after = in.Role
		case "member.roles":
			var target struct{ Kind string }
			if e := tx.Table("authorization_principals").Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=?", *in.UserID).Take(&target).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					return ErrNotFound
				}
				return ErrUnavailable
			}
			// Kind/source/warehouse bindings need their separate object contracts.
			if target.Kind != "employee" {
				return ErrDenied
			}
			var old []string
			if tx.Table("authorization_memberships").Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id=?", *in.UserID).Order("role_id ASC").Pluck("role_id", &old).Error != nil {
				return ErrUnavailable
			}
			if old == nil {
				old = []string{}
			}
			before = struct {
				UserID string   `json:"user_id"`
				Roles  []string `json:"roles"`
			}{*in.UserID, old}
			for _, id := range in.Roles {
				var role struct{ Enabled bool }
				if e := tx.Table("authorization_roles").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=?", id).Take(&role).Error; e != nil {
					if errors.Is(e, gorm.ErrRecordNotFound) {
						return ErrNotFound
					}
					return ErrUnavailable
				}
				if !role.Enabled {
					return ErrDenied
				}
				actions, e := roleActions(tx, id)
				if e != nil {
					return e
				}
				for _, a := range actions {
					if !held[a] {
						return ErrDenied
					}
				}
			}
			if tx.Exec("DELETE FROM authorization_memberships WHERE user_id=?", *in.UserID).Error != nil {
				return ErrUnavailable
			}
			for _, id := range in.Roles {
				if tx.Exec("INSERT INTO authorization_memberships(user_id,role_id) VALUES(?,?)", *in.UserID, id).Error != nil {
					return ErrUnavailable
				}
			}
			after = struct {
				UserID string   `json:"user_id"`
				Roles  []string `json:"roles"`
			}{*in.UserID, in.Roles}
		}
		var managers int64
		if tx.Table("authorization_principals p").Joins("JOIN admin_users u ON u.id=p.user_id AND u.enabled=TRUE AND u.role='admin' AND u.auth_version>0").Joins("JOIN authorization_memberships m ON m.user_id=p.user_id").Joins("JOIN authorization_roles r ON r.id=m.role_id AND r.enabled=TRUE").Joins("JOIN authorization_role_actions a ON a.role_id=r.id AND a.action_code=?", Write).Where("p.kind='employee'").Distinct("p.user_id").Count(&managers).Error != nil {
			return ErrUnavailable
		}
		if managers == 0 {
			return ErrLastManager
		}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		if tx.Exec("INSERT INTO authorization_changes(operation_id,actor_id,request_json,before_json,after_json,result_revision,created_at) VALUES(?,?,?,?,?,?,?)", in.OperationID, actor.ID, string(request), string(b), string(a), m.Revision+1, time.Now().UTC()).Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("UPDATE authorization_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
			return ErrUnavailable
		}
		result = ChangeResult{strconv.FormatUint(m.Revision+1, 10), false}
		return nil
	})
	if err != nil {
		return ChangeResult{}, err
	}
	return result, nil
}
func roleActions(tx *gorm.DB, id string) ([]Action, error) {
	var actions []Action
	if tx.Table("authorization_role_actions").Clauses(clause.Locking{Strength: "SHARE"}).Where("role_id=?", id).Order("action_code ASC").Limit(1001).Pluck("action_code", &actions).Error != nil || len(actions) > len(Actions()) {
		return nil, ErrUnavailable
	}
	if actions == nil {
		actions = []Action{}
	}
	for _, a := range actions {
		if !known(a) {
			return nil, ErrUnavailable
		}
	}
	return actions, nil
}
