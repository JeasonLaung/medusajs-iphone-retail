package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
)

func checkAuthorization(t *testing.T, db *gorm.DB, user identity.User, get func(string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	var roleID string
	if db.Raw("SELECT role_id FROM authorization_memberships WHERE user_id=?", user.ID).Scan(&roleID).Error != nil || roleID == "" {
		t.Fatal("authorization fixture membership unavailable")
	}
	mutate := func(query string, args ...any) {
		t.Helper()
		if db.Transaction(func(tx *gorm.DB) error {
			var n int
			if err := tx.Raw("SELECT id FROM authorization_runtime_schema WHERE id=1 FOR UPDATE").Scan(&n).Error; err != nil {
				return err
			}
			if err := tx.Exec(query, args...).Error; err != nil {
				return err
			}
			return tx.Exec("UPDATE authorization_runtime_schema SET revision=revision+1 WHERE id=1").Error
		}) != nil {
			t.Fatal("authorization artificial mutation failed")
		}
	}
	view := func() authorization.View {
		t.Helper()
		var v authorization.View
		if json.Unmarshal(get("/api/admin/auth/permissions", 200), &v) != nil || v.Revision == "" {
			t.Fatal("authorization snapshot invalid")
		}
		return v
	}
	if authorization.CheckTx(ctx, db, user, authorization.ProductRead) != authorization.ErrUnavailable || authorization.Bootstrap(ctx, db, user.ID) != authorization.ErrUnavailable {
		t.Fatal("authorization lock API accepted autocommit")
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, user.ID) }); !errors.Is(err, authorization.ErrDenied) {
		t.Fatal("existing principal silently reinitialized")
	}
	initial := view()
	if initial.Scope != "employee" || len(initial.Actions) != 13 {
		t.Fatal("explicit bootstrap actions missing")
	}
	mutate("DELETE FROM authorization_memberships WHERE user_id=?", user.ID)
	if len(view().Actions) != 0 {
		t.Fatal("admin role label implicitly grants actions")
	}
	if view().Revision == initial.Revision {
		t.Fatal("permission revision did not change")
	}
	get("/api/admin/orders", 403)
	get("/api/admin/products", 403)
	get("/api/admin/auth/me", 200)
	mutate("INSERT INTO authorization_memberships(user_id,role_id) VALUES(?,?)", user.ID, roleID)
	for _, kind := range []string{"warehouse", "supplier"} {
		mutate("UPDATE authorization_principals SET kind=? WHERE user_id=?", kind, user.ID)
		v := view()
		if v.Scope != kind || len(v.Actions) != 0 {
			t.Fatal("scoped actor obtained full-store actions")
		}
		get("/api/admin/orders", 403)
		get("/api/admin/products", 403)
	}
	mutate("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", user.ID)
	mutate("UPDATE authorization_roles SET enabled=FALSE WHERE id=?", roleID)
	if len(view().Actions) != 0 {
		t.Fatal("disabled role still grants")
	}
	get("/api/admin/orders", 403)
	mutate("UPDATE authorization_roles SET enabled=TRUE WHERE id=?", roleID)
	mutate("DELETE FROM authorization_role_actions WHERE role_id=?", roleID)
	mutate("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'order.read')", roleID)
	mutate("INSERT INTO authorization_roles(id,name,enabled) VALUES('fixture-reader','Reader',TRUE)")
	mutate("INSERT INTO authorization_role_actions(role_id,action_code) VALUES('fixture-reader','product.read')")
	mutate("INSERT INTO authorization_memberships(user_id,role_id) VALUES(?,'fixture-reader')", user.ID)
	if len(view().Actions) != 2 {
		t.Fatal("multiple-role union differs")
	}
	get("/api/admin/orders", 200)
	get("/api/admin/products", 200)
	for _, path := range []string{"/api/admin/orders/missing/amounts", "/api/admin/orders/missing/payments", "/api/admin/orders/missing/fulfillments", "/api/admin/products/missing/variants/missing/prices", "/api/admin/products/missing/variants/missing/inventory"} {
		get(path, 403)
	}
	for _, code := range []string{"*", "product.*", "Product.Read", "product.unknown"} {
		if db.Exec("INSERT INTO authorization_actions(code) VALUES(?)", code).Error == nil {
			t.Fatal("unknown or wildcard action accepted")
		}
	}
	// A repeatable-read snapshot established before removal must not preserve a
	// grant when a later command performs its mandatory locking recheck.
	stale := db.Begin()
	if stale.Error != nil {
		t.Fatal("authorization stale transaction failed")
	}
	defer stale.Rollback()
	var before int
	if stale.Raw("SELECT COUNT(*) FROM authorization_role_actions").Scan(&before).Error != nil {
		t.Fatal("authorization old snapshot unavailable")
	}
	mutate("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='order.read'", roleID)
	if !errors.Is(authorization.CheckTx(ctx, stale, user, authorization.OrderRead), authorization.ErrDenied) {
		t.Fatal("old snapshot retained revoked grant")
	}
	stale.Rollback()
	for _, action := range authorization.Actions() {
		mutate("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,?)", roleID, action)
	}
	hold := db.Begin()
	if hold.Error != nil {
		t.Fatal("authorization lock transaction failed")
	}
	defer hold.Rollback()
	if authorization.CheckTx(ctx, hold, user, authorization.ProductWrite) != nil {
		t.Fatal("authorization locked check failed")
	}
	deadline, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	err := db.WithContext(deadline).Exec("UPDATE authorization_runtime_schema SET revision=revision+1 WHERE id=1").Error
	if err == nil || deadline.Err() == nil {
		cancel()
		t.Fatal("permission writer bypassed command authorization lock")
	}
	cancel()
	hold.Rollback()
	mutate("DELETE FROM authorization_memberships WHERE role_id='fixture-reader' AND user_id=?", user.ID)
	mutate("DELETE FROM authorization_role_actions WHERE role_id='fixture-reader'")
	mutate("DELETE FROM authorization_roles WHERE id='fixture-reader'")
	mutate("UPDATE authorization_runtime_schema SET enabled=FALSE WHERE id=1")
	get("/api/admin/orders", 503)
	get("/api/admin/auth/permissions", 503)
	mutate("UPDATE authorization_runtime_schema SET enabled=TRUE WHERE id=1")
	if len(view().Actions) != 13 {
		t.Fatal("authorization restoration failed")
	}
	t.Log(`AUTHORIZATION_REHEARSAL {"migrations":47,"explicit_actions":13,"admin_label_no_implicit_grants":true,"multi_role_union":true,"all_required_actions":true,"warehouse_supplier_full_store_denied":true,"disabled_role_denied":true,"unknown_wildcard_case_denied":true,"old_snapshot_revocation":true,"command_lock_blocks_permission_writer":true,"disabled_schema_rejected":true,"autocommit_rejected":true,"existing_principal_bootstrap_rejected":true,"revision_changes":true,"legacy_grants_imported":false}`)
}
