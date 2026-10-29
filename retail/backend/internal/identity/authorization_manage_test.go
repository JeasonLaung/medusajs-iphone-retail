package identity_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkAuthorizationManagement(t *testing.T, db *gorm.DB, actor identity.User, get func(string, int) []byte, post func(string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := authorization.Handler{DB: db}
	directory := func() authorization.Directory {
		t.Helper()
		var d authorization.Directory
		if json.Unmarshal(get("/api/admin/authorization", 200), &d) != nil || len(d.Actions) != 19 || len(d.Members) == 0 {
			t.Fatal("permission directory invalid")
		}
		return d
	}
	d := directory()
	var ownRole string
	for _, member := range d.Members {
		if member.UserID == actor.ID && len(member.Roles) == 1 {
			ownRole = member.Roles[0]
		}
	}
	if ownRole == "" {
		t.Fatal("permission manager role unavailable")
	}
	next := func(role authorization.Role) authorization.Command {
		return authorization.Command{OperationID: uuid.NewString(), ExpectedRevision: directory().Revision, Kind: "role.save", Role: &role}
	}
	call := func(command authorization.Command, status int) authorization.ChangeResult {
		t.Helper()
		b, _ := json.Marshal(command)
		var r authorization.ChangeResult
		body := post(string(b), status)
		if status == 200 && json.Unmarshal(body, &r) != nil {
			t.Fatal("permission write response invalid")
		}
		return r
	}
	role := authorization.Role{ID: "managed-reader", Name: "Read only 中文", Enabled: true, Actions: []authorization.Action{authorization.ProductRead, authorization.OrderRead}}
	in := next(role)
	original, _ := json.Marshal(in)
	saved := call(in, 200)
	if saved.Replayed || saved.Revision == in.ExpectedRevision {
		t.Fatal("permission revision unchanged")
	}
	if replay := call(in, 200); !replay.Replayed || replay.Revision != saved.Revision {
		t.Fatal("permission immutable replay differs")
	}
	changed := in
	copy := *in.Role
	copy.Name = "Different attempt"
	changed.Role = &copy
	call(changed, 409)
	stale := in
	stale.OperationID = uuid.NewString()
	call(stale, 409)
	role.Name = "Concurrent role"
	concurrent := next(role)
	var wg sync.WaitGroup
	results := make(chan authorization.ChangeResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Go(func() { r, e := h.Save(ctx, actor, concurrent); results <- r; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent permission change failed")
		}
	}
	fresh, replayed := 0, 0
	for r := range results {
		if r.Replayed {
			replayed++
		} else {
			fresh++
		}
	}
	if fresh != 1 || replayed != 7 {
		t.Fatal("permission duplicate execution")
	}
	if b, _ := json.Marshal(in); string(b) != string(original) {
		t.Fatal("permission input mutated")
	}
	if db.Exec("INSERT INTO admin_users(id,email,password_hash,name,role,enabled,auth_version,created_at,updated_at) VALUES('managed-subject','managed@example.invalid','artificial-only','Managed employee','admin',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))").Error != nil || db.Exec("INSERT INTO authorization_principals(user_id,kind) VALUES('managed-subject','employee')").Error != nil {
		t.Fatal("managed subject setup failed")
	}
	userID := "managed-subject"
	membership := authorization.Command{OperationID: uuid.NewString(), ExpectedRevision: directory().Revision, Kind: "member.roles", UserID: &userID, Roles: []string{role.ID}}
	call(membership, 200)
	var count int64
	if db.Table("authorization_memberships").Where("user_id=? AND role_id=?", userID, role.ID).Count(&count).Error != nil || count != 1 {
		t.Fatal("permission membership not saved")
	}
	missing := membership
	missing.OperationID = uuid.NewString()
	missing.ExpectedRevision = directory().Revision
	missing.Roles = []string{"missing-role"}
	call(missing, 404)
	last := authorization.Role{ID: ownRole, Name: "Last manager", Enabled: false, Actions: authorization.Actions()}
	call(next(last), 409)
	last.Enabled = true
	last.Actions = []authorization.Action{authorization.ProductRead}
	call(next(last), 409)
	// A failure at the final receipt insert must roll back role and revision.
	before := directory()
	if db.Exec("CREATE TRIGGER reject_permission_receipt BEFORE INSERT ON authorization_changes FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial receipt failure'").Error != nil {
		t.Fatal("permission receipt failure setup failed")
	}
	role.Name = "Must roll back"
	failure := next(role)
	call(failure, 503)
	if db.Exec("DROP TRIGGER reject_permission_receipt").Error != nil {
		t.Fatal("permission receipt trigger restoration failed")
	}
	after := directory()
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("permission failure left partial state")
	}
	// The real command waits behind the same authority lock held by business
	// checks; timeout rolls back and creates no receipt or revision change.
	hold := db.Begin()
	if hold.Error != nil || authorization.CheckTx(ctx, hold, actor, authorization.ProductWrite) != nil {
		t.Fatal("permission business lock setup failed")
	}
	deadline, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	r, e := h.Save(deadline, actor, failure)
	if e == nil || deadline.Err() == nil || r != (authorization.ChangeResult{}) {
		cancel()
		hold.Rollback()
		t.Fatal("permission change bypassed active business command")
	}
	cancel()
	hold.Rollback()
	if directory().Revision != before.Revision {
		t.Fatal("timed out permission change altered revision")
	}
	if db.Exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code=?", ownRole, authorization.Write).Error != nil {
		t.Fatal("permission manager revocation setup failed")
	}
	call(in, 403)
	if db.Exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,?)", ownRole, authorization.Write).Error != nil {
		t.Fatal("permission manager restoration failed")
	}
	// A manager without product.write cannot delegate it, directly or through
	// assigning an existing more powerful role.
	if db.Exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code=?", ownRole, authorization.ProductWrite).Error != nil {
		t.Fatal("limited manager setup failed")
	}
	role.Actions = []authorization.Action{authorization.ProductWrite}
	call(next(role), 403)
	if db.Exec("INSERT INTO authorization_roles(id,name,enabled) VALUES('power-role','Power',TRUE)").Error != nil || db.Exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES('power-role',?)", authorization.ProductWrite).Error != nil {
		t.Fatal("delegation fixture failed")
	}
	missing.OperationID = uuid.NewString()
	missing.Roles = []string{"power-role"}
	missing.ExpectedRevision = directory().Revision
	call(missing, 403)
	if db.Exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,?)", ownRole, authorization.ProductWrite).Error != nil {
		t.Fatal("limited manager restoration failed")
	}
	if db.Exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", userID).Error != nil {
		t.Fatal("scoped membership setup failed")
	}
	missing.OperationID = uuid.NewString()
	missing.ExpectedRevision = directory().Revision
	missing.Roles = []string{role.ID}
	call(missing, 403)
	if db.Exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", userID).Error != nil {
		t.Fatal("scoped membership restoration failed")
	}
	get("/api/admin/authorization?fields=*", 400)
	// Malformed wire bodies never change revision, role or member rows.
	encoded, _ := json.Marshal(next(role))
	raw := string(encoded)
	for _, bad := range []string{`null`, `{}`, raw + `{}`, strings.Replace(raw, `"enabled":true`, `"enabled":null`, 1), strings.Replace(raw, `"kind":"role.save"`, `"kind":"role.save","kind":"member.roles"`, 1), strings.Replace(raw, `"enabled":true`, `"enabled":true,"enabled":false`, 1), strings.Replace(raw, `"name":"Must roll back"`, `"name":"Must roll back","unknown":true`, 1)} {
		post(bad, 400)
	}
	var audit struct{ BeforeJSON, AfterJSON string }
	if db.Table("authorization_changes").Where("operation_id=?", in.OperationID).Take(&audit).Error != nil || audit.BeforeJSON != "null" || !strings.Contains(audit.AfterJSON, "Read only 中文") {
		t.Fatal("permission before/after audit missing")
	}
	t.Log(`AUTHORIZATION_MANAGEMENT_REHEARSAL {"migrations":58,"explicit_actions":19,"role_and_membership_save":true,"revision_conflict":true,"immutable_replay":true,"concurrent_attempts":8,"single_fresh_attempt":1,"duplicate_replays":7,"input_unchanged":true,"receipt_failure_atomic":true,"last_manager_preserved":true,"revoked_replay_denied":true,"undelegated_action_denied":true,"scoped_member_change_denied":true,"business_lock_serialization":true,"wire_duplicate_null_unknown_rejected":true,"before_after_audit":true}`)
}
