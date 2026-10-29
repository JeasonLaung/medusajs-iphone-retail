package authorization

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CheckWarehouseTx is for dedicated, field-projected warehouse operations only.
// It does not make a warehouse actor eligible for any full-store endpoint.
// Call before business locks and receipt replay. Permission/scope mutations must
// first lock authorization_runtime_schema FOR UPDATE, in the same transaction.
func CheckWarehouseTx(ctx context.Context, tx *gorm.DB, actor identity.User, locationID string, actions ...Action) error {
	if tx == nil || tx.Statement == nil {
		return ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrUnavailable
	}
	if !warehouseLocationID(locationID) || len(actions) == 0 {
		return ErrDenied
	}
	for _, action := range actions {
		switch action {
		case InventoryRead, StockWrite, OrderFulfillmentRead:
		default:
			return ErrDenied
		}
	}
	view, err := inspect(ctx, tx, actor, true)
	if err != nil {
		return err
	}
	if view.Scope != "warehouse" {
		return ErrDenied
	}
	var grants []struct{ ActionCode Action }
	if tx.WithContext(ctx).Table("authorization_memberships AS m").Select("a.action_code").
		Joins("JOIN authorization_roles r ON r.id=m.role_id AND r.enabled=TRUE").
		Joins("JOIN authorization_role_actions a ON a.role_id=r.id").Where("m.user_id=?", actor.ID).
		Order("a.action_code ASC").Limit(1001).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&grants).Error != nil || len(grants) > 1000 {
		return ErrUnavailable
	}
	held := map[Action]bool{}
	for _, row := range grants {
		if !known(row.ActionCode) {
			return ErrUnavailable
		}
		held[row.ActionCode] = true
	}
	for _, action := range actions {
		if !held[action] {
			return ErrDenied
		}
	}
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.WithContext(ctx).Table("warehouse_scope_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || !gate.Enabled {
		return ErrUnavailable
	}
	var scope struct{ Mode string }
	err = tx.WithContext(ctx).Table("authorization_warehouse_scopes").Clauses(clause.Locking{Strength: "SHARE"}).Where("user_id=?", actor.ID).Take(&scope).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrDenied
	}
	if err != nil {
		return ErrUnavailable
	}
	switch scope.Mode {
	case "selected":
		var binding struct{ LocationID string }
		err = tx.WithContext(ctx).Table("authorization_warehouse_locations").Clauses(clause.Locking{Strength: "SHARE"}).Where("user_id=? AND location_id=?", actor.ID, locationID).Take(&binding).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDenied
		}
		if err != nil {
			return ErrUnavailable
		}
	case "all":
	default:
		return ErrUnavailable
	}
	var location struct{ ID string }
	err = tx.WithContext(ctx).Table("inventory_locations").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=? AND deleted_at IS NULL", locationID).Take(&location).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrDenied
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func warehouseLocationID(id string) bool {
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 || strings.ContainsAny(id, "/\\?#") {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return strings.TrimSpace(id) == id
}
