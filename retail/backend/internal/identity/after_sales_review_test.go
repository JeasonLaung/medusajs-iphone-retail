package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/customer"
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

func checkAfterSalesReview(t *testing.T, ctx context.Context, db *gorm.DB, orderID, requestID string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("review fixture SQL")
		}
	}
	actor := identity.User{ID: "review-fixture-admin", Email: "review@example.invalid", Name: "Reviewer", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: "fixture-not-used-for-login", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if db.Create(&actor).Error != nil {
		t.Fatal("review artificial user")
	}
	if db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, actor.ID) }) != nil {
		t.Fatal("review explicit grants")
	}
	exec("UPDATE after_sales_review_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE after_sales_review_runtime SET enabled=FALSE WHERE id=1")
	defer exec("DELETE FROM after_sales_reviews WHERE actor_id=?", actor.ID)
	read := func() customer.ReviewContext {
		t.Helper()
		var out customer.ReviewContext
		e := db.Transaction(func(tx *gorm.DB) error {
			var e error
			out, e = customer.ReviewAfterSalesTx(ctx, tx, actor, orderID, nil)
			return e
		})
		if e != nil {
			t.Fatal("review read", e)
		}
		return out
	}
	change := func(in customer.ReviewInput) error {
		return db.Transaction(func(tx *gorm.DB) error { _, e := customer.ReviewAfterSalesTx(ctx, tx, actor, orderID, &in); return e })
	}
	before := read()
	if len(before.Requests) != 1 || before.Requests[0].ID != requestID {
		t.Fatal("review request projection")
	}

	// Real HTTP handler over the owned MySQL fixture; session/CSRF routing is tested separately.
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("admin_user", actor); c.Next() })
	router.GET("/orders/:id/after-sales", customer.ReviewHandler{DB: db}.HTTP)
	router.POST("/orders/:id/after-sales", customer.ReviewHandler{DB: db}.HTTP)
	httpCall := func(method, body string, want int) {
		t.Helper()
		req := httptest.NewRequest(method, "/orders/"+orderID+"/after-sales", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("review HTTP status got %d want %d", w.Code, want)
		}
	}
	httpCall("GET", "", 200)
	httpCall("POST", `{"status":"processing","status":"completed"}`, 400)
	httpCall("POST", `{"operation_id":null}`, 400)
	input := customer.ReviewInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, RequestID: requestID, Status: "processing"}
	exec("CREATE TRIGGER review_late_failure BEFORE INSERT ON after_sales_reviews FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial review receipt failure'")
	if change(input) == nil {
		t.Fatal("late receipt failure accepted")
	}
	exec("DROP TRIGGER review_late_failure")
	if read().Requests[0].Status != "pending" {
		t.Fatal("late review failure committed status")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- change(input) }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("review concurrent replay", e)
		}
	}
	var count int64
	if db.Table("after_sales_reviews").Where("actor_id=?", actor.ID).Count(&count).Error != nil || count != 1 {
		t.Fatal("duplicate review")
	}
	exec("UPDATE after_sales_requests SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", requestID)
	if !errors.Is(change(input), gorm.ErrRecordNotFound) {
		t.Fatal("soft deleted target replay accepted")
	}
	exec("UPDATE after_sales_requests SET deleted_at=NULL WHERE id=?", requestID)
	encoded, _ := json.Marshal(input)
	httpCall("POST", string(encoded), 200)
	after := read()
	if after.Requests[0].Status != "processing" || after.Revision == before.Revision {
		t.Fatal("review status/revision")
	}
	stale := input
	stale.OperationID = uuid.NewString()
	stale.Status = "completed"
	if !errors.Is(change(stale), customer.ErrAfterSalesConflict) {
		t.Fatal("stale review accepted")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id IN (SELECT role_id FROM authorization_memberships WHERE user_id=?) AND action_code='order.after_sales.write'", actor.ID)
	if !errors.Is(change(input), authorization.ErrDenied) {
		t.Fatal("revoked grant replay accepted")
	}
	httpCall("POST", string(encoded), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) SELECT role_id,'order.after_sales.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	next := customer.ReviewInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, RequestID: requestID, Status: "completed"}
	if e := change(next); e != nil {
		t.Fatal("unshipped original completion", e)
	}
	closed := read()
	next.OperationID = uuid.NewString()
	next.ExpectedRevision = closed.Revision
	next.Status = "processing"
	if !errors.Is(change(next), customer.ErrAfterSalesConflict) {
		t.Fatal("closed request reopened")
	}
	exec("UPDATE after_sales_requests SET status='pending' WHERE id=?", requestID)
	t.Log(`AFTER_SALES_REVIEW_REHEARSAL {"real_mysql":true,"explicit_grants":true,"late_receipt_failure_atomic":true,"concurrent_replay_once":true,"stale_revision_denied":true,"revoked_grant_replay_denied":true,"unshipped_original_completion":true,"closed_request_not_reopened":true,"notifications_sent":false}`)
}
