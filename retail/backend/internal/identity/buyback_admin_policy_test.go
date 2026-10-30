package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestVerifyBuybackAdminPolicies(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned browser fixture required")
	}
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("fixture unavailable")
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal("fixture pool unavailable")
	}
	defer pool.Close()

	ctx := context.Background()
	var actor identity.User
	if db.Where("id='new-browser-fixture'").Take(&actor).Error != nil {
		t.Fatal("actor fixture")
	}
	var row struct{ ID string }
	if db.Table("buyback_requests").Select("id").Where("status='received'").Take(&row).Error != nil {
		t.Fatal("completed browser fixture required")
	}
	sentinel := errors.New("rollback fixture")
	test := func(name string, work func(*testing.T, *gorm.DB)) {
		t.Run(name, func(t *testing.T) {
			e := db.Transaction(func(tx *gorm.DB) error { work(t, tx); return sentinel })
			if !errors.Is(e, sentinel) {
				t.Fatal("fixture rollback")
			}
		})
	}
	test("cross-store-and-ungranted", func(t *testing.T, tx *gorm.DB) {
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "other-store", row.ID, nil); !errors.Is(e, buyback.ErrMissing) {
			t.Fatal("cross store")
		}
		no := actor
		no.ID = "missing-new-user"
		if _, e := buyback.AdminRequestTx(ctx, tx, no, "fixture-store", row.ID, nil); !errors.Is(e, authorization.ErrDenied) {
			t.Fatal("ungranted actor")
		}
		if tx.Exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID).Error != nil {
			t.Fatal("scope fixture")
		}
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, nil); !errors.Is(e, authorization.ErrDenied) {
			t.Fatal("warehouse allowed")
		}
	})
	test("write-grant-current", func(t *testing.T, tx *gorm.DB) {
		if tx.Exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='buyback.write'", actor.ID).Error != nil {
			t.Fatal("grant fixture")
		}
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, nil); e != nil {
			t.Fatal("read grant")
		}
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, &buyback.AdminCommand{Action: "mark_received"}); !errors.Is(e, authorization.ErrDenied) {
			t.Fatal("missing write grant")
		}
	})
	test("disabled-gate", func(t *testing.T, tx *gorm.DB) {
		if tx.Exec("UPDATE buyback_admin_runtime SET enabled=FALSE").Error != nil {
			t.Fatal("gate fixture")
		}
		v, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, nil)
		if e != nil || v.Enabled {
			t.Fatal("read disabled gate")
		}
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, &buyback.AdminCommand{Action: "mark_received"}); !errors.Is(e, buyback.ErrUnavailable) {
			t.Fatal("disabled write")
		}
	})
	for _, state := range []string{"awaiting_photos", "photo_review", "ready_to_ship", "in_transit", "received", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled"} {
		for _, action := range []string{"review_photo", "approve_shipping", "mark_received", "cancel"} {
			test(state+"-"+action, func(t *testing.T, tx *gorm.DB) {
				if tx.Exec("UPDATE buyback_requests SET status=? WHERE id=?", state, row.ID).Error != nil {
					t.Fatal("state fixture")
				}
				current, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, nil)
				if e != nil || len(current.Request.Photos) != 2 {
					t.Fatal("current fixture")
				}
				input := buyback.AdminCommand{Action: action}
				switch action {
				case "review_photo":
					input.PhotoID = current.Request.Photos[0].ID
					input.ReviewStatus = "approved"
				case "approve_shipping":
					input.ShippingInstructions = "Send carefully to warehouse"
				case "cancel":
					input.Note = "Fixture canceled"
				}
				_, e = buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, &input)
				allowed := action == "review_photo" && (state == "awaiting_photos" || state == "photo_review") || action == "approve_shipping" && state == "photo_review" || action == "mark_received" && (state == "ready_to_ship" || state == "in_transit") || action == "cancel" && (state == "awaiting_photos" || state == "photo_review" || state == "ready_to_ship")
				if (e == nil) != allowed {
					t.Fatalf("state guard %s/%s %v", state, action, e)
				}
			})
		}
	}
	test("latest-photo-only", func(t *testing.T, tx *gorm.DB) {
		if tx.Exec("UPDATE buyback_requests SET status='photo_review' WHERE id=?", row.ID).Error != nil {
			t.Fatal("state fixture")
		}
		var old struct{ ID string }
		if tx.Table("buyback_photos").Select("id").Where("request_id=? AND review_status='retake'", row.ID).Take(&old).Error != nil {
			t.Fatal("old photo fixture")
		}
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, &buyback.AdminCommand{Action: "review_photo", PhotoID: old.ID, ReviewStatus: "approved"}); !errors.Is(e, buyback.ErrPhotoMissing) {
			t.Fatal("old version review accepted")
		}
	})
	test("retake-note", func(t *testing.T, tx *gorm.DB) {
		tx.Exec("UPDATE buyback_requests SET status='photo_review' WHERE id=?", row.ID)
		current, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, &buyback.AdminCommand{Action: "review_photo", PhotoID: current.Request.Photos[0].ID, ReviewStatus: "retake", Note: "12"}); !errors.Is(e, buyback.ErrReviewNote) {
			t.Fatal("short retake note")
		}
	})
	// A post-command failure rolls back photo review, status, notices and audit.
	var before struct{ History, Status string }
	db.Table("buyback_requests").Select("history,status").Where("id=?", row.ID).Take(&before)
	var notices, audits int64
	db.Table("customer_account_notices").Count(&notices)
	db.Table("audit_events").Where("action='buyback.command'").Count(&audits)
	test("late-failure", func(t *testing.T, tx *gorm.DB) {
		tx.Exec("UPDATE buyback_requests SET status='photo_review' WHERE id=?", row.ID)
		current, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, &buyback.AdminCommand{Action: "review_photo", PhotoID: current.Request.Photos[0].ID, ReviewStatus: "retake", Note: "Please retake fixture"}); e != nil {
			t.Fatal(e)
		}
	})
	var after struct{ History, Status string }
	db.Table("buyback_requests").Select("history,status").Where("id=?", row.ID).Take(&after)
	var n, a int64
	db.Table("customer_account_notices").Count(&n)
	db.Table("audit_events").Where("action='buyback.command'").Count(&a)
	if before != after || n != notices || a != audits {
		t.Fatal("late failure left effects")
	}
	// Competing staff transitions serialize on the request, one state change wins.
	var stateBefore string
	db.Table("buyback_requests").Select("status").Where("id=?", row.ID).Scan(&stateBefore)
	if db.Exec("UPDATE buyback_requests SET status='photo_review' WHERE id=?", row.ID).Error != nil {
		t.Fatal("race fixture")
	}
	var wg sync.WaitGroup
	result := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result <- db.Transaction(func(tx *gorm.DB) error {
				_, e := buyback.AdminRequestTx(ctx, tx, actor, "fixture-store", row.ID, &buyback.AdminCommand{Action: "approve_shipping", ShippingInstructions: "Approved race fixture warehouse"})
				return e
			})
		}()
	}
	wg.Wait()
	close(result)
	wins, denied := 0, 0
	for e := range result {
		if e == nil {
			wins++
		} else if errors.Is(e, buyback.ErrCommandState) {
			denied++
		} else {
			t.Fatal("race error", e)
		}
	}
	if wins != 1 || denied != 1 {
		t.Fatal("race duplicate transitions")
	}
	// Remove only this explicitly artificial final race effect before durable browser checks.
	if db.Exec("UPDATE buyback_requests SET status=?,history=?,shipping_instructions='Pack carefully. Send to fixture warehouse.' WHERE id=?", stateBefore, before.History, row.ID).Error != nil {
		t.Fatal("race restore")
	}
	if db.Exec("DELETE FROM customer_account_notices WHERE resource_id=? AND action='approve_shipping' AND id NOT IN (SELECT id FROM (SELECT id FROM customer_account_notices WHERE resource_id=? AND action='approve_shipping' ORDER BY created_at,id LIMIT 1) keep_notice)", row.ID, row.ID).Error != nil {
		t.Fatal("race notice restore")
	}
	// Production handler query/input errors reveal no private data.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("admin_user", actor) })
	(buyback.AdminHandler{DB: db, StoreID: "fixture-store"}).Register(r.Group("/api/admin"))
	for _, path := range []string{"/api/admin/buyback?private=1", "/api/admin/buyback?status=nope", "/api/admin/buyback?status=photo_review%7Cready_to_ship", "/api/admin/buyback?status=", "/api/admin/buyback?offset=01", "/api/admin/buyback/" + row.ID + "?q=1"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatal("query accepted", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/admin/buyback/"+row.ID, strings.NewReader(`{"action":"mark_received","action":"mark_received"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal("duplicate accepted")
	}
	body, _ := json.Marshal(map[string]any{"state_action_cases": 48, "competing_transition_single_winner": true, "warehouse_denied": true, "grants_rechecked": true, "disabled_write_denied": true, "late_failure_atomic": true, "strict_http": true})
	t.Log("BUYBACK_ADMIN_POLICIES " + string(body))
}
