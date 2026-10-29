package identity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
)

func checkWarehouseScope(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := t.Context()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("warehouse scope fixture", err)
		}
	}
	const a = "scope-fixture-a"
	const b = "scope-fixture-b"
	exec("INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES(?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),(?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", a, "A", b, "B")
	defer exec("DELETE FROM inventory_locations WHERE id IN (?,?)", a, b)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	defer exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	defer exec("UPDATE warehouse_scope_runtime SET enabled=FALSE WHERE id=1")
	exec("INSERT INTO authorization_warehouse_scopes(user_id,mode) VALUES(?,'selected')", actor.ID)
	defer exec("DELETE FROM authorization_warehouse_scopes WHERE user_id=?", actor.ID)
	defer exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	check := func(user identity.User, location string, want error, actions ...authorization.Action) {
		t.Helper()
		got := db.Transaction(func(tx *gorm.DB) error { return authorization.CheckWarehouseTx(ctx, tx, user, location, actions...) })
		if !errors.Is(got, want) {
			t.Fatalf("warehouse scope got %v want %v", got, want)
		}
	}
	if authorization.CheckWarehouseTx(ctx, db, actor, a, authorization.InventoryRead) != authorization.ErrUnavailable {
		t.Fatal("autocommit accepted")
	}
	check(actor, a, authorization.ErrUnavailable, authorization.InventoryRead)
	exec("UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1")
	check(actor, a, authorization.ErrDenied, authorization.InventoryRead)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, a)
	check(actor, a, nil, authorization.InventoryRead, authorization.StockWrite, authorization.OrderFulfillmentRead)
	if err := db.Transaction(func(tx *gorm.DB) error { return authorization.CheckTx(ctx, tx, actor, authorization.InventoryRead) }); !errors.Is(err, authorization.ErrDenied) {
		t.Fatal("scoped checker widened full-store policy")
	}
	check(actor, b, authorization.ErrDenied, authorization.InventoryRead)
	check(actor, a, authorization.ErrDenied)
	for _, action := range []authorization.Action{authorization.ProductRead, authorization.OrderRead, authorization.OrderAmountRead, authorization.OrderPaymentRead, authorization.WarehouseWrite, "*"} {
		check(actor, a, authorization.ErrDenied, action)
	}
	check(actor, "", authorization.ErrDenied, authorization.InventoryRead)
	check(actor, "../"+a, authorization.ErrDenied, authorization.InventoryRead)
	stale := actor
	stale.AuthVersion++
	check(stale, a, authorization.ErrDenied, authorization.InventoryRead)
	exec("UPDATE admin_users SET enabled=FALSE WHERE id=?", actor.ID)
	check(actor, a, authorization.ErrDenied, authorization.InventoryRead)
	exec("UPDATE admin_users SET enabled=TRUE WHERE id=?", actor.ID)
	var role string
	if db.Raw("SELECT role_id FROM authorization_memberships WHERE user_id=? LIMIT 1", actor.ID).Scan(&role).Error != nil || role == "" {
		t.Fatal("role missing")
	}
	exec("UPDATE authorization_roles SET enabled=FALSE WHERE id=?", role)
	check(actor, a, authorization.ErrDenied, authorization.InventoryRead)
	exec("UPDATE authorization_roles SET enabled=TRUE WHERE id=?", role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='inventory.stock.write'", role)
	check(actor, a, authorization.ErrDenied, authorization.InventoryRead, authorization.StockWrite)
	check(actor, a, nil, authorization.InventoryRead)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'inventory.stock.write')", role)
	exec("UPDATE authorization_warehouse_scopes SET mode='all' WHERE user_id=?", actor.ID)
	check(actor, b, nil, authorization.InventoryRead)
	check(actor, "missing", authorization.ErrDenied, authorization.InventoryRead)
	exec("UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", b)
	check(actor, b, authorization.ErrDenied, authorization.InventoryRead)
	exec("UPDATE inventory_locations SET deleted_at=NULL WHERE id=?", b)
	for _, kind := range []string{"employee", "supplier"} {
		exec("UPDATE authorization_principals SET kind=? WHERE user_id=?", kind, actor.ID)
		check(actor, a, authorization.ErrDenied, authorization.InventoryRead)
	}
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	exec("UPDATE authorization_warehouse_scopes SET mode='selected' WHERE user_id=?", actor.ID)
	// A preexisting RR snapshot must not retain an old binding after revocation.
	old := db.Begin()
	if old.Error != nil {
		t.Fatal(old.Error)
	}
	defer old.Rollback()
	var n int64
	if old.Table("authorization_warehouse_locations").Where("user_id=?", actor.ID).Count(&n).Error != nil || n != 1 {
		t.Fatal("snapshot missing")
	}
	exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	if err := authorization.CheckWarehouseTx(ctx, old, actor, a, authorization.InventoryRead); !errors.Is(err, authorization.ErrDenied) {
		t.Fatal("old snapshot retained revoked location", err)
	}
	if old.Rollback().Error != nil {
		t.Fatal("rollback")
	}
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, a)
	held := db.Begin()
	if held.Error != nil {
		t.Fatal(held.Error)
	}
	defer held.Rollback()
	if err := authorization.CheckWarehouseTx(ctx, held, actor, a, authorization.InventoryRead); err != nil {
		t.Fatal(err)
	}
	// A valid scope command takes the common authority write lock first.
	writer := db.Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	defer writer.Rollback()
	if writer.Exec("SET SESSION innodb_lock_wait_timeout=1").Error != nil {
		t.Fatal("lock timeout setup")
	}
	started := time.Now()
	if err := writer.Exec("SELECT id FROM authorization_runtime_schema WHERE id=1 FOR UPDATE").Error; err == nil || time.Since(started) < 500*time.Millisecond {
		t.Fatal("authority writer bypassed checked business transaction")
	}
	if writer.Exec("SET SESSION innodb_lock_wait_timeout=DEFAULT").Error != nil {
		t.Fatal("lock timeout restoration")
	}
	if writer.Rollback().Error != nil || held.Rollback().Error != nil {
		t.Fatal("lock test cleanup")
	}
	// A missing scope record grants nothing, including a previously all actor.
	exec("DELETE FROM authorization_warehouse_locations WHERE user_id=?", actor.ID)
	exec("DELETE FROM authorization_warehouse_scopes WHERE user_id=?", actor.ID)
	check(actor, a, authorization.ErrDenied, authorization.InventoryRead)
	t.Log(`WAREHOUSE_SCOPE_REHEARSAL {"real_mysql":true,"empty_is_none":true,"selected_location_only":true,"all_still_requires_active_location":true,"explicit_actions_required":true,"all_actions_required":true,"no_full_store_grants":true,"current_actor_required":true,"disabled_role_denied":true,"old_snapshot_revocation":true,"writer_blocked_until_business_end":true,"autocommit_rejected":true,"default_disabled":true,"http_or_ui_integrated":false}`)
}
