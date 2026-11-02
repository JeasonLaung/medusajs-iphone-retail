package identity_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// This participant runs after the actual customer submission in an owned copy.
// It never posts a payment, refund, inventory or device command.
func TestWindowsAfterSalesRejectionGuards(t *testing.T) {
	if os.Getenv("WINDOWS_AFTER_SALES_REJECTION") != "1" {
		t.Skip("owned paid checkpoint required")
	}
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned loopback database required")
	}
	var fixture struct{ OrderID, RequestID string }
	raw, err := os.ReadFile(filepath.Join(logs, "rejection-guard-input.json"))
	if err != nil || json.Unmarshal(raw, &fixture) != nil || !regexp.MustCompile(`^order_[0-9a-f-]{36}$`).MatchString(fixture.OrderID) || !regexp.MustCompile(`^asr_[0-9a-f-]{36}$`).MatchString(fixture.RequestID) {
		t.Fatal("owned request identities required")
	}
	db, err := platform.Open()
	if err != nil {
		t.Fatal("owned database unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	var actor identity.User
	if db.Where("role='admin' AND enabled=TRUE").Order("id").First(&actor).Error != nil {
		t.Fatal("owned admin unavailable")
	}
	fingerprint := func() string {
		t.Helper()
		out := []json.RawMessage{}
		for _, table := range []string{"after_sales_requests", "after_sales_reviews", "customer_account_notices", "customer_event_mail", "orders", "order_summaries", "payments", "payment_captures", "payment_refunds", "order_transactions", "inventory_levels", "inventory_reservations", "device_assignments", "device_runtime_schema", "after_sales_review_runtime", "admin_users"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("guard fingerprint unavailable")
			}
			data, _ := json.Marshal(rows)
			out = append(out, data)
		}
		data, _ := json.Marshal(out)
		return string(data)
	}
	rollback := errors.New("owned guard rollback")
	checks := 0
	for _, name := range []string{"disabled gate", "disabled actor", "stale revision", "terminal request", "invalid status", "foreign request"} {
		t.Run(name, func(t *testing.T) {
			before := fingerprint()
			err := db.Transaction(func(tx *gorm.DB) error {
				if name == "disabled gate" && tx.Exec("UPDATE after_sales_review_runtime SET enabled=FALSE WHERE id=1").Error != nil {
					t.Fatal("gate setup")
				}
				if name == "disabled actor" && tx.Exec("UPDATE admin_users SET enabled=FALSE WHERE id=?", actor.ID).Error != nil {
					t.Fatal("actor setup")
				}
				if name == "terminal request" && tx.Exec("UPDATE after_sales_requests SET status='rejected' WHERE id=?", fixture.RequestID).Error != nil {
					t.Fatal("terminal setup")
				}
				var view customer.ReviewContext
				if name == "disabled actor" {
					_, e := customer.ReviewAfterSalesTx(t.Context(), tx, actor, fixture.OrderID, nil)
					if !errors.Is(e, authorization.ErrDenied) {
						t.Fatal("disabled actor accepted", e)
					}
					return rollback
				}
				view, e := customer.ReviewAfterSalesTx(t.Context(), tx, actor, fixture.OrderID, nil)
				if e != nil {
					t.Fatal("guard read", e)
				}
				in := customer.ReviewInput{OperationID: uuid.NewString(), ExpectedRevision: view.Revision, RequestID: fixture.RequestID, Status: "rejected"}
				want := customer.ErrAfterSalesConflict
				switch name {
				case "disabled gate":
					want = customer.ErrUnavailable
				case "stale revision":
					in.ExpectedRevision = strings.Repeat("0", 64)
				case "invalid status":
					in.Status = "canceled"
					want = customer.ErrInvalid
				case "foreign request":
					in.RequestID = "asr_" + uuid.NewString()
					want = gorm.ErrRecordNotFound
				}
				_, e = customer.ReviewAfterSalesTx(t.Context(), tx, actor, fixture.OrderID, &in)
				if !errors.Is(e, want) {
					t.Fatalf("guard %s: %v", name, e)
				}
				return rollback
			})
			if !errors.Is(err, rollback) || before != fingerprint() {
				t.Fatal("guard changed durable facts", err)
			}
			checks++
		})
	}
	before := fingerprint()
	err = db.Transaction(func(tx *gorm.DB) error {
		view, e := customer.ReviewAfterSalesTx(t.Context(), tx, actor, fixture.OrderID, nil)
		if e != nil {
			return e
		}
		_, e = customer.ReviewAfterSalesTx(t.Context(), tx, actor, fixture.OrderID, &customer.ReviewInput{OperationID: "00000000-0000-4000-8000-000000000001", ExpectedRevision: view.Revision, RequestID: fixture.RequestID, Status: "rejected"})
		return e
	})
	if !errors.Is(err, customer.ErrUnavailable) || before != fingerprint() {
		t.Fatal("late notice failure committed review/status/mail", err)
	}
	result, _ := json.MarshalIndent(map[string]any{"passed": true, "guard_cases": checks, "late_notice_failure_rolls_back_review_status_receipt_mail": true, "all_guard_transactions_rolled_back": true}, "", "  ")
	if os.WriteFile(filepath.Join(logs, "rejection-guards.json"), result, 0600) != nil {
		t.Fatal("guard evidence unavailable")
	}
}
