package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func verifyPolicyCommands(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("policy fixture", e)
		}
	}
	exec(`INSERT INTO commerce_stores(id,name,metadata,created_at,updated_at) VALUES('policy-store','Policy fixture','{"private":{"integer":9007199254740993123},"other":"preserve"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	h := StoreHandler{DB: db, StoreID: "policy-store"}
	current, e := h.ReadPolicySettings(ctx, actor)
	if e != nil || current.Enabled || current.Policies != defaultBuyPolicies() {
		t.Fatal("policy initial", e)
	}
	in := PolicyCommand{uuid.NewString(), current.Revision, BuyPolicies{12, 30, PolicyNote{" English ", " 中文 "}, PolicyNote{"Return", "退货"}}}
	if _, e := h.SavePolicySettings(ctx, actor, in); !errors.Is(e, ErrSchema) {
		t.Fatal("policy gate allowed write", e)
	}
	exec("UPDATE catalog_command_capabilities SET enabled=TRUE WHERE command='store.buy-policies'")
	results := make([]PolicyResult, 6)
	errs := make([]error, 6)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = h.SavePolicySettings(ctx, actor, in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil {
			t.Fatal("policy concurrent write", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("policy receipt not unique")
	}
	var count int64
	if db.Table("store_policy_edits").Where("store_id='policy-store'").Count(&count).Error != nil || count != 1 {
		t.Fatal("policy receipt count")
	}
	current, e = h.ReadPolicySettings(ctx, actor)
	if e != nil || current.Policies.WarrantyNote.EN != "English" || current.Policies.WarrantyNote.ZH != "中文" {
		t.Fatal("policy normalized persistence", e)
	}
	var metadata string
	if db.Raw("SELECT metadata FROM commerce_stores WHERE id='policy-store'").Scan(&metadata).Error != nil || !strings.Contains(metadata, "9007199254740993123") || !strings.Contains(metadata, `"other":"preserve"`) {
		t.Fatal("policy metadata changed unrelated facts")
	}
	stale := in
	stale.OperationID = uuid.NewString()
	if _, e := h.SavePolicySettings(ctx, actor, stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("policy stale accepted", e)
	}
	altered := in
	altered.Policies.ReturnDays++
	if _, e := h.SavePolicySettings(ctx, actor, altered); !errors.Is(e, ErrEditConflict) {
		t.Fatal("policy UUID changed request accepted", e)
	}
	next := in
	next.OperationID = uuid.NewString()
	next.ExpectedRevision = current.Revision
	next.Policies.WarrantyMonths = 0
	next.Policies.ReturnDays = 0
	exec(`CREATE TRIGGER policy_receipt_test_failure BEFORE INSERT ON store_policy_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial receipt failure'`)
	_, failure := h.SavePolicySettings(ctx, actor, next)
	exec("DROP TRIGGER policy_receipt_test_failure")
	if failure == nil {
		t.Fatal("policy receipt failure accepted")
	}
	after, e := h.ReadPolicySettings(ctx, actor)
	if e != nil || after != current {
		t.Fatal("policy receipt failure not atomic", e)
	}
	if _, e = h.SavePolicySettings(ctx, actor, next); e != nil {
		t.Fatal("policy retry after rollback", e)
	}
	after, e = h.ReadPolicySettings(ctx, actor)
	if e != nil || after.Policies.WarrantyMonths != 0 || after.Policies.ReturnDays != 0 {
		t.Fatal("policy explicit zero lost")
	}
	exec("UPDATE catalog_command_capabilities SET enabled=FALSE WHERE command='store.buy-policies'")
	if r, e := h.SavePolicySettings(ctx, actor, next); e != nil || !r.Replayed {
		t.Fatal("policy gate closed recovery", e)
	}
	exec("UPDATE admin_users SET enabled=FALSE WHERE id=?", actor.ID)
	if _, e := h.SavePolicySettings(ctx, actor, next); e == nil {
		t.Fatal("policy revoked retry accepted")
	}
	exec("UPDATE admin_users SET enabled=TRUE WHERE id=?", actor.ID)
	verifyPolicyPermissionHTTP(t, db, actor, h)
	t.Log(`STORE_POLICIES_NATIVE {"default_disabled":true,"concurrent_attempts":6,"single_receipt":true,"unrelated_large_integer_preserved":true,"stale_revision_rejected":true,"changed_uuid_payload_rejected":true,"late_receipt_failure_atomic":true,"zero_periods_preserved":true,"closed_gate_recovery":true,"revoked_retry_denied":true}`)
}

// Exercise real authorization rows and handler responses, independently of the
// browser transport's session/Origin/CSRF checks.
func verifyPolicyPermissionHTTP(t *testing.T, db *gorm.DB, owner identity.User, h StoreHandler) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	actor := owner
	actor.ID, actor.Email = "policy-limited-actor", "policy-limited@example.test"
	if e := db.Create(&actor).Error; e != nil {
		t.Fatal(e)
	}
	exec("INSERT INTO authorization_principals(user_id,kind) VALUES(?,'employee')", actor.ID)
	exec("INSERT INTO authorization_roles(id,name,enabled) VALUES('policy-limited-role','Policy limited',TRUE)")
	exec("INSERT INTO authorization_memberships(user_id,role_id) VALUES(?,'policy-limited-role')", actor.ID)
	call := func(method, path, body string, want int, code string) {
		t.Helper()
		w := httptest.NewRecorder()
		// A fresh router keeps its actual status/body serialization intact.
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("admin_user", actor) })
		router.GET("/policies", h.PolicySettingsHTTP)
		router.POST("/policies", h.PolicySettingsHTTP)
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		if w.Code != want || code != "" && !strings.Contains(w.Body.String(), code) {
			t.Fatalf("policy HTTP %s: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	ctx := context.Background()
	current, e := h.ReadPolicySettings(ctx, owner)
	if e != nil {
		t.Fatal(e)
	}
	in := PolicyCommand{uuid.NewString(), current.Revision, BuyPolicies{18, 28, PolicyNote{"Limited", "权限测试"}, PolicyNote{}}}
	raw, _ := json.Marshal(in)
	body := string(raw)
	call("GET", "/policies", "", 403, "authorization.denied")
	call("POST", "/policies", body, 403, "authorization.denied")
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES('policy-limited-role','store.policies.read')")
	call("GET", "/policies", "", 200, "")
	call("POST", "/policies", body, 403, "authorization.denied")
	exec("DELETE FROM authorization_role_actions WHERE role_id='policy-limited-role'")
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES('policy-limited-role','store.policies.write')")
	call("GET", "/policies", "", 403, "authorization.denied")
	call("POST", "/policies", body, 403, "authorization.denied")
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES('policy-limited-role','store.policies.read')")
	call("POST", "/policies", body, 503, "policies.unavailable")
	call("GET", "/policies?store_id=other", "", 400, "policies.invalid_input")
	call("POST", "/policies?", body, 400, "policies.invalid_input")
	exec("UPDATE catalog_command_capabilities SET enabled=TRUE WHERE command='store.buy-policies'")
	call("POST", "/policies", body, 200, "")
	exec("DELETE FROM authorization_role_actions WHERE role_id='policy-limited-role' AND action_code='store.policies.write'")
	call("POST", "/policies", body, 403, "authorization.denied")
	var n int64
	if e := db.Table("store_policy_edits").Where("operation_id=?", in.OperationID).Count(&n).Error; e != nil || n != 1 {
		t.Fatal("revoked HTTP retry changed receipts", e, n)
	}
	exec("UPDATE commerce_stores SET deleted_at=UTC_TIMESTAMP(6) WHERE id='policy-store'")
	call("GET", "/policies", "", 404, "policies.not_found")
	exec("UPDATE commerce_stores SET deleted_at=NULL WHERE id='policy-store'")
	exec("UPDATE catalog_command_capabilities SET enabled=FALSE WHERE command='store.buy-policies'")
	t.Log(`STORE_POLICIES_HTTP_NATIVE {"legacy_role_not_implicit":true,"read_only":true,"write_without_read_denied":true,"closed_gate_503":true,"store_override_query_rejected":true,"revoked_replay_denied":true,"single_receipt":true,"deleted_store_hidden":true}`)
}
