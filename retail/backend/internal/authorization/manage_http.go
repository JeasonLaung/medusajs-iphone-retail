package authorization

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// decodeCommand rejects duplicate keys (including nested roles), unknown keys,
// missing nullable selectors and noncanonical member/role command shapes.
func decodeCommand(reader io.Reader) (Command, error) {
	d := json.NewDecoder(reader)
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 4 {
			return nil, ErrInput
		}
		token, e := d.Token()
		if e != nil {
			return nil, ErrInput
		}
		switch token {
		case json.Delim('{'):
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				key, ok := k.(string)
				if e != nil || !ok {
					return nil, ErrInput
				}
				if _, ok := m[key]; ok {
					return nil, ErrInput
				}
				value, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				m[key] = value
			}
			if _, e := d.Token(); e != nil {
				return nil, ErrInput
			}
			return m, nil
		case json.Delim('['):
			values := []any{}
			for d.More() {
				v, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				values = append(values, v)
				if len(values) > 100 {
					return nil, ErrInput
				}
			}
			if _, e := d.Token(); e != nil {
				return nil, ErrInput
			}
			return values, nil
		default:
			if _, bad := token.(json.Delim); bad {
				return nil, ErrInput
			}
			return token, nil
		}
	}
	value, err := read(0)
	if err != nil {
		return Command{}, ErrInput
	}
	if _, err := d.Token(); err != io.EOF {
		return Command{}, ErrInput
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 6 {
		return Command{}, ErrInput
	}
	for _, key := range []string{"operation_id", "expected_revision", "kind", "role", "user_id", "roles"} {
		if _, ok := object[key]; !ok {
			return Command{}, ErrInput
		}
	}
	if r, ok := object["role"].(map[string]any); ok {
		if _, ok := r["enabled"].(bool); !ok {
			return Command{}, ErrInput
		}
		if len(r) != 4 {
			return Command{}, ErrInput
		}
		for _, key := range []string{"id", "name", "enabled", "actions"} {
			if _, ok := r[key]; !ok {
				return Command{}, ErrInput
			}
		}
	}
	encoded, _ := json.Marshal(value)
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var in Command
	if decoder.Decode(&in) != nil {
		return Command{}, ErrInput
	}
	return normalize(in)
}

func (h Handler) Command(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "authorization.invalid_input", "Invalid authorization command")
		return
	}
	content, _, err := mime.ParseMediaType(c.ContentType())
	if err != nil || content != "application/json" {
		platform.Fail(c, 400, "authorization.invalid_input", "Invalid authorization command")
		return
	}
	c.Request.Body = io.NopCloser(io.LimitReader(c.Request.Body, 32769))
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) > 32768 {
		platform.Fail(c, 400, "authorization.invalid_input", "Invalid authorization command")
		return
	}
	in, err := decodeCommand(bytes.NewReader(body))
	if err != nil {
		platform.Fail(c, 400, "authorization.invalid_input", "Invalid authorization command")
		return
	}
	actor, ok := c.Get("admin_user")
	user, valid := actor.(identity.User)
	if !ok || !valid {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	result, err := h.Save(c.Request.Context(), user, in)
	switch {
	case err == nil:
		c.JSON(200, result)
	case errors.Is(err, ErrInput):
		platform.Fail(c, 400, "authorization.invalid_input", "Invalid authorization command")
	case errors.Is(err, ErrDenied):
		fail(c, err)
	case errors.Is(err, ErrNotFound):
		platform.Fail(c, 404, "authorization.not_found", "Authorization subject unavailable")
	case errors.Is(err, ErrConflict):
		platform.Fail(c, 409, "authorization.conflict", "Authorization changed; reload before saving")
	case errors.Is(err, ErrLastManager):
		platform.Fail(c, 409, "authorization.last_manager", "Keep an enabled permission manager")
	default:
		fail(c, ErrUnavailable)
	}
}
func (h Handler) Directory(c *gin.Context) {
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
	if h.DB == nil {
		fail(c, ErrUnavailable)
		return
	}
	var result Directory
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		view, e := inspect(c.Request.Context(), tx, user, false)
		if e != nil {
			return e
		}
		allowed := false
		for _, a := range view.Actions {
			allowed = allowed || a == Read
		}
		if !allowed {
			return ErrDenied
		}
		result = Directory{Revision: view.Revision, Actions: Actions(), Roles: []Role{}, Members: []Member{}}
		var roles []struct {
			ID, Name string
			Enabled  bool
		}
		if tx.Table("authorization_roles").Select("id,name,enabled").Order("id ASC").Limit(101).Scan(&roles).Error != nil || len(roles) > 100 {
			return ErrUnavailable
		}
		var grants []struct {
			RoleID     string
			ActionCode Action
		}
		if tx.Table("authorization_role_actions").Select("role_id,action_code").Order("role_id ASC,action_code ASC").Limit(1101).Scan(&grants).Error != nil || len(grants) > 1100 {
			return ErrUnavailable
		}
		for _, r := range roles {
			role := Role{r.ID, r.Name, r.Enabled, []Action{}}
			for _, g := range grants {
				if !known(g.ActionCode) {
					return ErrUnavailable
				}
				if g.RoleID == r.ID {
					role.Actions = append(role.Actions, g.ActionCode)
				}
			}
			result.Roles = append(result.Roles, role)
		}
		var members []struct {
			UserID, Name, Kind string
			Enabled            bool
		}
		if tx.Table("authorization_principals p").Select("p.user_id,u.name,u.enabled,p.kind").Joins("JOIN admin_users u ON u.id=p.user_id").Order("p.user_id ASC").Limit(1001).Scan(&members).Error != nil || len(members) > 1000 {
			return ErrUnavailable
		}
		var memberships []struct{ UserID, RoleID string }
		if tx.Table("authorization_memberships").Select("user_id,role_id").Order("user_id ASC,role_id ASC").Limit(100001).Scan(&memberships).Error != nil || len(memberships) > 100000 {
			return ErrUnavailable
		}
		for _, m := range members {
			member := Member{m.UserID, m.Name, m.Enabled, m.Kind, []string{}}
			for _, r := range memberships {
				if r.UserID == m.UserID {
					member.Roles = append(member.Roles, r.RoleID)
				}
			}
			result.Members = append(result.Members, member)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, result)
}
