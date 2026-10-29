package authorization

import (
	"context"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WarehouseLocation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type WarehouseView struct {
	Revision  string              `json:"revision"`
	Actions   []Action            `json:"actions"`
	Locations []WarehouseLocation `json:"locations"`
}

func (h Handler) WarehouseAccess(ctx context.Context, actor identity.User) (WarehouseView, error) {
	out := WarehouseView{Actions: []Action{}, Locations: []WarehouseLocation{}}
	if h.DB == nil {
		return out, ErrUnavailable
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		held, e := warehouseAuthority(ctx, tx, actor)
		if e != nil {
			return e
		}
		m, e := schemaState(ctx, tx, true)
		if e != nil {
			return e
		}
		out.Revision = strconv.FormatUint(m.Revision, 10)
		for _, a := range WarehouseActions() {
			if held[a] {
				out.Actions = append(out.Actions, a)
			}
		}
		// No operational actions means no location directory, even if a scope exists.
		if len(out.Actions) == 0 {
			return nil
		}
		var scope struct{ Mode string }
		if e := tx.Table("authorization_warehouse_scopes").Clauses(clause.Locking{Strength: "SHARE"}).Where("user_id=?", actor.ID).Take(&scope).Error; e != nil {
			if e == gorm.ErrRecordNotFound {
				return nil
			}
			return ErrUnavailable
		}
		q := tx.Table("inventory_locations l").Select("l.id,l.name").Where("l.deleted_at IS NULL").Order("l.id ASC").Limit(1001).Clauses(clause.Locking{Strength: "SHARE"})
		switch scope.Mode {
		case "selected":
			q = q.Joins("JOIN authorization_warehouse_locations w ON w.location_id=l.id AND w.user_id=?", actor.ID)
		case "all":
		default:
			return ErrUnavailable
		}
		if q.Scan(&out.Locations).Error != nil || len(out.Locations) > 1000 {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
func (h Handler) WarehouseSnapshot(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "warehouse.invalid_query", "Invalid warehouse access request")
		return
	}
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.WarehouseAccess(ctx, actor)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
