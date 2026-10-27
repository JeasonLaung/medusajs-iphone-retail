package authorization

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrDenied = errors.New("explicit action grant required")
var ErrUnavailable = errors.New("authorization unavailable")

type marker struct {
	SchemaVersion int
	ModelSHA256   string
	Revision      uint64
	Enabled       bool
}

func schemaState(ctx context.Context, tx *gorm.DB, lock bool) (marker, error) {
	var m marker
	if tx == nil {
		return m, ErrUnavailable
	}
	q := tx.WithContext(ctx).Table("authorization_runtime_schema").Where("id=1")
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if q.Take(&m).Error != nil || m.SchemaVersion != 1 || m.ModelSHA256 != modelSHA || m.Revision == 0 || !m.Enabled {
		return m, ErrUnavailable
	}
	return m, nil
}
func Ready(ctx context.Context, db *gorm.DB) error {
	if _, err := schemaState(ctx, db, false); err != nil {
		return err
	}
	var rows []struct{ Code Action }
	if db.WithContext(ctx).Table("authorization_actions").Select("code").Order("code ASC").Scan(&rows).Error != nil || len(rows) != len(Actions()) {
		return ErrUnavailable
	}
	for i, a := range Actions() {
		if rows[i].Code != a {
			return ErrUnavailable
		}
	}
	return nil
}

type View struct {
	Revision string   `json:"revision"`
	Scope    string   `json:"scope"`
	Actions  []Action `json:"actions"`
}

func inspect(ctx context.Context, tx *gorm.DB, actor identity.User, lock bool) (View, error) {
	view := View{Scope: "none", Actions: []Action{}}
	m, err := schemaState(ctx, tx, lock)
	if err != nil {
		return view, err
	}
	view.Revision = strconv.FormatUint(m.Revision, 10)
	var current identity.User
	q := tx.WithContext(ctx).Where("id=?", actor.ID)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if err := q.Take(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return view, ErrDenied
		}
		return view, ErrUnavailable
	}
	if !identity.SameAuthority(current, actor) {
		return view, ErrDenied
	}
	var principal struct{ Kind string }
	q = tx.WithContext(ctx).Table("authorization_principals").Select("kind").Where("user_id=?", actor.ID)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if err := q.Take(&principal).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return view, nil
		}
		return view, ErrUnavailable
	}
	view.Scope = principal.Kind
	// Scoped actors need dedicated field/object policies. Existing full-store
	// endpoints cannot serve a warehouse or supplier, even with matching grants.
	if principal.Kind != "employee" {
		if principal.Kind != "warehouse" && principal.Kind != "supplier" {
			return view, ErrUnavailable
		}
		return view, nil
	}
	var rows []struct{ ActionCode Action }
	q = tx.WithContext(ctx).Table("authorization_memberships AS m").Select("a.action_code").Joins("JOIN authorization_roles r ON r.id=m.role_id AND r.enabled=TRUE").Joins("JOIN authorization_role_actions a ON a.role_id=r.id").Where("m.user_id=?", actor.ID).Order("a.action_code ASC").Limit(1001)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if q.Scan(&rows).Error != nil || len(rows) > 1000 {
		return view, ErrUnavailable
	}
	seen := map[Action]bool{}
	for _, row := range rows {
		if !known(row.ActionCode) {
			return view, ErrUnavailable
		}
		if !seen[row.ActionCode] {
			view.Actions = append(view.Actions, row.ActionCode)
			seen[row.ActionCode] = true
		}
	}
	return view, nil
}

// CheckTx MUST run before business locks and idempotency replay, in the caller's
// real transaction. SHARE locks remain held through its commit/rollback.
// Permission mutations acquire the runtime row FOR UPDATE before grant rows.
func CheckTx(ctx context.Context, tx *gorm.DB, actor identity.User, actions ...Action) error {
	if tx == nil || tx.Statement == nil {
		return ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrUnavailable
	}
	if len(actions) == 0 {
		return ErrDenied
	}
	for _, a := range actions {
		if !known(a) {
			return ErrDenied
		}
	}
	view, err := inspect(ctx, tx, actor, true)
	if err != nil {
		return err
	}
	for _, required := range actions {
		found := false
		for _, a := range view.Actions {
			found = found || a == required
		}
		if !found {
			return ErrDenied
		}
	}
	return nil
}

type Handler struct{ DB *gorm.DB }

func (h Handler) view(ctx context.Context, actor identity.User) (View, error) {
	var v View
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if h.DB == nil {
		return v, ErrUnavailable
	}
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; v, err = inspect(ctx, tx, actor, false); return err }, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return v, err
}
func (h Handler) Snapshot(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "authorization.invalid_query", "Invalid authorization request")
		return
	}
	actor, ok := c.Get("admin_user")
	user, valid := actor.(identity.User)
	if !ok || !valid {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	v, err := h.view(c.Request.Context(), user)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, v)
}
func fail(c *gin.Context, err error) {
	if errors.Is(err, ErrDenied) {
		platform.Fail(c, 403, "authorization.denied", "Action not permitted")
	} else {
		platform.Fail(c, 503, "authorization.unavailable", "Authorization unavailable")
	}
}
func (h Handler) Require(actions ...Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := c.Get("admin_user")
		user, valid := actor.(identity.User)
		if !ok || !valid {
			platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
			return
		}
		if len(actions) == 0 {
			fail(c, ErrDenied)
			return
		}
		for _, a := range actions {
			if !known(a) {
				fail(c, ErrDenied)
				return
			}
		}
		v, err := h.view(c.Request.Context(), user)
		if err != nil {
			fail(c, err)
			return
		}
		for _, required := range actions {
			found := false
			for _, a := range v.Actions {
				found = found || a == required
			}
			if !found {
				fail(c, ErrDenied)
				return
			}
		}
		c.Next()
	}
}

// Bootstrap is only for a newly inserted account in a caller-owned transaction
// (create-admin). Existing accounts/grants are never silently adopted or reset.
func Bootstrap(ctx context.Context, tx *gorm.DB, userID string) error {
	if tx == nil || userID == "" {
		return ErrDenied
	}
	if tx.Statement == nil {
		return ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrUnavailable
	}
	var m marker
	if tx.WithContext(ctx).Table("authorization_runtime_schema").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&m).Error != nil || m.ModelSHA256 != modelSHA || m.SchemaVersion != 1 || !m.Enabled || m.Revision == 0 {
		return ErrUnavailable
	}
	var account identity.User
	if tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Where("id=?", userID).Take(&account).Error != nil || !account.Enabled || account.Role != "admin" || account.AuthVersion == 0 {
		return ErrDenied
	}
	roleID := "role_" + uuid.NewString()
	if tx.Exec("INSERT INTO authorization_principals(user_id,kind) VALUES(?,'employee')", userID).Error != nil {
		return ErrDenied
	}
	if tx.Exec("INSERT INTO authorization_roles(id,name,enabled) VALUES(?,'Administrator',TRUE)", roleID).Error != nil {
		return ErrUnavailable
	}
	for _, a := range Actions() {
		if tx.Exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,?)", roleID, a).Error != nil {
			return ErrUnavailable
		}
	}
	if tx.Exec("INSERT INTO authorization_memberships(user_id,role_id) VALUES(?,?)", userID, roleID).Error != nil {
		return ErrUnavailable
	}
	if tx.Exec("UPDATE authorization_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
		return ErrUnavailable
	}
	data, _ := json.Marshal(map[string]any{"role_id": roleID, "actions": Actions(), "kind": "employee"})
	if tx.Exec("INSERT INTO audit_events(id,actor_id,action,resource_id,data,created_at) VALUES(?,?,'authorization.bootstrap',?,?,?)", "audit_"+uuid.NewString(), userID, userID, string(data), time.Now().UTC()).Error != nil {
		return ErrUnavailable
	}
	return nil
}
