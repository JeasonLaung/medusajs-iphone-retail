package identity_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/warehouseusers"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkWarehouseUsers(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, string, string, []*http.Cookie, int) *httptest.ResponseRecorder, admin *http.Cookie, csrf string) {
	t.Helper()
	ctx := t.Context()
	const origin = "https://admin.example"
	const password = "warehouse-fixture-only-password"
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("warehouse user fixture", e)
		}
	}
	get := func(status int) []byte {
		return call("GET", "/api/admin/warehouse-users", "", origin, "", []*http.Cookie{admin}, status).Body.Bytes()
	}
	post := func(c warehouseusers.Command, status int) warehouseusers.Result {
		t.Helper()
		raw, _ := json.Marshal(c)
		body := call("POST", "/api/admin/warehouse-users", string(raw), origin, csrf, []*http.Cookie{admin}, status).Body.Bytes()
		if strings.Contains(string(body), password) || strings.Contains(string(body), "password_hash") || strings.Contains(string(body), "auth_version") {
			t.Fatal("credentials leaked")
		}
		var out warehouseusers.Result
		if status == 200 && json.Unmarshal(body, &out) != nil {
			t.Fatal("result invalid")
		}
		return out
	}
	read := func() warehouseusers.Directory {
		t.Helper()
		var d warehouseusers.Directory
		if json.Unmarshal(get(200), &d) != nil || d.Revision == "" {
			t.Fatal("directory invalid")
		}
		return d
	}
	get(503)
	exec("UPDATE warehouse_user_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE warehouse_user_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE warehouse_scope_runtime SET enabled=FALSE WHERE id=1")
	exec("INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('user-scope-a','A',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('user-scope-b','B',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	d := read()
	c := warehouseusers.Command{OperationID: uuid.NewString(), ExpectedRevision: d.Revision, Name: "仓库人员", Email: "warehouse-user@example.invalid", Password: password, Enabled: true, Mode: "selected", Locations: []string{"user-scope-a"}, Actions: []authorization.Action{authorization.InventoryRead}}
	h := warehouseusers.Handler{DB: db}
	var fresh atomic.Int32
	var failed atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := h.Save(ctx, actor, c)
			if e != nil {
				failed.Store(true)
			} else if !r.Replayed {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if failed.Load() || fresh.Load() != 1 {
		t.Fatal("concurrent account duplicate")
	}
	created := post(c, 200)
	if !created.Replayed {
		t.Fatal("successful receipt not replayed")
	}
	d = read()
	if len(d.Users) != 1 || d.Users[0].ID != created.ID || len(d.Users[0].Locations) != 1 {
		t.Fatal("created projection mismatch")
	}
	var target identity.User
	if db.Where("id=?", created.ID).Take(&target).Error != nil {
		t.Fatal("account missing")
	}
	if e := db.Transaction(func(tx *gorm.DB) error {
		return authorization.CheckWarehouseTx(ctx, tx, target, "user-scope-a", authorization.InventoryRead)
	}); e != nil {
		t.Fatal("selected authority missing", e)
	}
	if e := db.Transaction(func(tx *gorm.DB) error {
		return authorization.CheckWarehouseTx(ctx, tx, target, "user-scope-b", authorization.InventoryRead)
	}); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("cross warehouse granted")
	}
	cookie := func(r *httptest.ResponseRecorder, name string) *http.Cookie {
		t.Helper()
		for _, c := range r.Result().Cookies() {
			if c.Name == name {
				return c
			}
		}
		t.Fatal("cookie missing")
		return nil
	}
	login := func(pass string, status int) *http.Cookie {
		t.Helper()
		challenge := call("POST", "/api/admin/auth/challenge", "", origin, "", nil, 200)
		var v struct {
			CSRF string `json:"csrf_token"`
		}
		if json.Unmarshal(challenge.Body.Bytes(), &v) != nil {
			t.Fatal("challenge invalid")
		}
		raw, _ := json.Marshal(map[string]string{"email": c.Email, "password": pass})
		out := call("POST", "/api/admin/auth/login", string(raw), origin, v.CSRF, []*http.Cookie{cookie(challenge, "__Host-retail_admin_prelogin")}, status)
		if status != 200 {
			return nil
		}
		return cookie(out, "__Host-retail_admin_session")
	}
	session := login(password, 200)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 200)
	call("GET", "/api/admin/orders", "", origin, "", []*http.Cookie{session}, 403)
	call("GET", "/api/admin/warehouse-users", "", origin, "", []*http.Cookie{session}, 403)
	bad := c
	bad.OperationID = uuid.NewString()
	bad.Email = "other@example.invalid"
	post(bad, 409)
	bad = c
	bad.OperationID = uuid.NewString()
	bad.ExpectedRevision = read().Revision
	post(bad, 409) // Duplicate email at the current revision, not a stale version.
	edit := c
	edit.ID = created.ID
	edit.Password = ""
	edit.ExpectedRevision = d.Revision
	edit.OperationID = uuid.NewString()
	edit.Enabled = false
	post(edit, 200)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 401)
	login(password, 401)
	edit.OperationID = uuid.NewString()
	edit.ExpectedRevision = read().Revision
	edit.Enabled = true
	edit.Mode = "all"
	edit.Locations = []string{}
	post(edit, 200)
	session = login(password, 200)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 200)
	edit.OperationID = uuid.NewString()
	edit.ExpectedRevision = read().Revision
	edit.Mode = "selected"
	edit.Locations = []string{"user-scope-b"}
	post(edit, 200)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 401)
	session = login(password, 200)
	// Password reset and scope edit invalidate even the re-enabled session.
	edit.OperationID = uuid.NewString()
	edit.ExpectedRevision = read().Revision
	edit.Password = password + "-new"
	post(edit, 200)
	call("GET", "/api/admin/auth/me", "", origin, "", []*http.Cookie{session}, 401)
	login(password, 401)
	login(edit.Password, 200)
	changed := edit
	changed.Password = password + "-different"
	post(changed, 409)
	// Fault the final receipt after all account/role/scope writes.
	next := c
	next.OperationID = uuid.NewString()
	next.ExpectedRevision = read().Revision
	next.Email = "rollback-warehouse@example.invalid"
	before := read()
	exec("CREATE TRIGGER warehouse_user_receipt_fault BEFORE INSERT ON warehouse_user_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial receipt failure'")
	post(next, 503)
	exec("DROP TRIGGER warehouse_user_receipt_fault")
	after := read()
	var n int64
	if db.Table("admin_users").Where("email=?", next.Email).Count(&n).Error != nil || n != 0 || before.Revision != after.Revision || len(before.Users) != len(after.Users) {
		t.Fatal("late failure was not atomic")
	}
	post(next, 200)
	// Original operation remains bound to its request and current manager grants.
	var role string
	db.Raw("SELECT role_id FROM authorization_memberships WHERE user_id=? LIMIT 1", actor.ID).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.write'", role)
	post(c, 403)
	get(200)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.write')", role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='inventory.read'", role)
	post(c, 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'inventory.read')", role)
	invalid := c
	invalid.OperationID = uuid.NewString()
	invalid.ExpectedRevision = read().Revision
	invalid.Email = "bad-action@example.invalid"
	invalid.Actions = []authorization.Action{authorization.OrderRead}
	post(invalid, 400)
	invalid = c
	invalid.OperationID = uuid.NewString()
	invalid.ExpectedRevision = read().Revision
	invalid.Email = "missing-location@example.invalid"
	invalid.Locations = []string{"missing"}
	post(invalid, 404)
	// A retired location must not prevent disabling an existing warehouse user.
	exec("UPDATE inventory_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE id='user-scope-b'")
	edit.OperationID = uuid.NewString()
	edit.ExpectedRevision = read().Revision
	edit.Enabled = false
	edit.Password = ""
	post(edit, 200)
	edit.OperationID = uuid.NewString()
	edit.ExpectedRevision = read().Revision
	edit.Enabled = true
	post(edit, 404)
	exec("UPDATE inventory_locations SET deleted_at=NULL WHERE id='user-scope-b'")
	call("POST", "/api/admin/warehouse-users", "{}", origin, "", []*http.Cookie{admin}, 403)
	call("GET", "/api/admin/warehouse-users?all=1", "", origin, "", []*http.Cookie{admin}, 400)
	// No password is present in persisted canonical request/audit snapshots.
	var leaks int64
	if db.Table("warehouse_user_changes").Where("request_json LIKE ? OR before_json LIKE ? OR after_json LIKE ?", "%"+password+"%", "%"+password+"%", "%"+password+"%").Count(&leaks).Error != nil || leaks != 0 {
		t.Fatal("plaintext persisted")
	}
	exec("UPDATE warehouse_user_runtime SET enabled=FALSE WHERE id=1")
	get(503)
	post(c, 503)
	t.Log(`WAREHOUSE_USERS_REHEARSAL {"real_gin_mysql_redis":true,"new_account_login":true,"four_concurrent_one_creation":true,"immutable_password_bound_replay":true,"selected_scope_denies_cross_warehouse":true,"no_full_store_access":true,"disable_and_scope_change_revoke_sessions":true,"password_reset":true,"stale_revision":true,"late_receipt_failure_atomic":true,"revoked_manager_replay_denied":true,"password_not_in_projection_or_audit":true,"csrf_and_query":true,"default_disabled":true}`)
}
