package identity_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/refunds"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Runs only in a restored, owned paid checkpoint. The external refund adapter
// is artificial; admission, claims, balances, ledger, stock and notices are real.
func TestWindowsOrderCancellation(t *testing.T) {
	if os.Getenv("WINDOWS_ORDER_CANCELLATION") != "1" {
		t.Skip("owned paid checkpoint required")
	}
	config, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs, order := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS"), os.Getenv("WINDOWS_CANCELLATION_ORDER")
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || config.Net != "tcp" || config.Addr != "127.0.0.1:13307" || config.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner || !regexp.MustCompile(`^order_[0-9a-f-]{36}$`).MatchString(order) {
		t.Fatal("owned loopback cancellation database required")
	}
	db, err := platform.Open()
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	ring, err := protected.ParseKeyring(os.Getenv("PAYMENT_KEY_ACTIVE"), os.Getenv("PAYMENT_KEYRING_JSON"))
	if err != nil {
		t.Fatal("payment keyring unavailable")
	}
	var actor identity.User
	if db.Where("role='admin' AND enabled=TRUE").Order("id").First(&actor).Error != nil {
		t.Fatal("owned admin unavailable")
	}
	channel := &refundChannel{db: db, t: t, status: "pending"}
	registry, _ := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": channel})
	service := refunds.Service{DB: db, StoreID: os.Getenv("STOREFRONT_STORE_ID"), Ring: ring, Adapters: registry}
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("owned SQL setup failed")
		}
	}
	fingerprint := func() string {
		t.Helper()
		out := []json.RawMessage{}
		for _, table := range []string{"orders", "order_items", "order_credit_lines", "order_shipping_versions", "order_line_adjustments", "order_shipping_adjustments", "order_summaries", "order_transactions", "order_cancellations", "order_refund_operations", "payments", "payment_captures", "payment_refunds", "payment_collections", "inventory_levels", "inventory_reservations", "inventory_reservation_operations", "devices", "device_assignments", "device_events", "customer_account_notices", "customer_event_mail", "device_runtime_schema"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("fingerprint unavailable")
			}
			raw, _ := json.Marshal(rows)
			out = append(out, raw)
		}
		raw, _ := json.Marshal(out)
		return string(raw)
	}
	phase := os.Getenv("WINDOWS_CANCELLATION_PHASE")
	var root struct{ OperationID string }
	if phase != "guards" && db.Table("order_cancellations").Select("operation_id").Where("order_id=?", order).Take(&root).Error != nil {
		t.Fatal("admitted root required")
	}
	var active struct{ OperationID string }
	if phase != "guards" {
		if db.Table("order_refund_operations").Select("operation_id").Where("cancellation_id=?", root.OperationID).Order("created_at DESC,operation_id DESC").First(&active).Error != nil {
			t.Fatal("refund child required")
		}
	}
	switch phase {
	case "guards":
		var version int
		if db.Table("orders").Select("version").Where("id=?", order).Scan(&version).Error != nil {
			t.Fatal("order missing")
		}
		baseline := fingerprint()
		rollback := errors.New("owned rollback")
		for _, kind := range []string{"stale", "completed", "gate", "disabled", "foreign-order", "active-parcel", "uncaptured"} {
			err := db.Transaction(func(tx *gorm.DB) error {
				probe := service
				probe.DB = tx
				in := refunds.CancellationInput{OperationID: uuid.NewString(), ExpectedVersion: strconv.Itoa(version), Reason: "Owned cancellation guard"}
				id := order
				switch kind {
				case "stale":
					in.ExpectedVersion = strconv.Itoa(version + 1)
				case "completed":
					if tx.Exec("UPDATE orders SET status='completed' WHERE id=?", order).Error != nil {
						return errors.New("setup failed")
					}
				case "gate":
					if tx.Exec("UPDATE order_cancellation_runtime SET enabled=FALSE WHERE id=1").Error != nil {
						return errors.New("setup failed")
					}
				case "disabled":
					if tx.Exec("UPDATE admin_users SET enabled=FALSE WHERE id=?", actor.ID).Error != nil {
						return errors.New("setup failed")
					}
				case "active-parcel":
					if tx.Exec("INSERT INTO fulfillments(id,location_id,requires_shipping,created_at,updated_at) VALUES('owned-cancel-parcel','owned-location',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))").Error != nil {
						return errors.New("parcel setup failed")
					}
					if tx.Exec("INSERT INTO order_fulfillments(id,order_id,fulfillment_id,created_at,updated_at) VALUES('owned-cancel-link',?,'owned-cancel-parcel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", order).Error != nil {
						return errors.New("parcel link setup failed")
					}
				case "uncaptured":
					if tx.Exec("UPDATE payments p JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id SET p.captured_at=NULL WHERE l.order_id=?", order).Error != nil {
						return errors.New("uncaptured setup failed")
					}
				case "foreign-order":
					id = "order_00000000-0000-4000-8000-000000000000"
				}
				if _, err := probe.CancelOrder(t.Context(), actor, id, in); err == nil {
					return errors.New("unsafe cancellation accepted")
				}
				return rollback
			})
			if !errors.Is(err, rollback) || fingerprint() != baseline {
				t.Fatal("guard rollback failed", kind, err)
			}
		}
	case "failed", "lost", "pending", "refunded", "apply":
		if phase != "failed" {
			exec("UPDATE order_refund_operations SET lease_until=UTC_TIMESTAMP(6) WHERE operation_id=? AND dispatched_at IS NOT NULL", active.OperationID)
		}
		if phase == "failed" {
			channel.status = "failed"
		}
		if phase == "lost" {
			channel.lost.Store(true)
		}
		if phase == "refunded" || phase == "apply" {
			channel.status = "refunded"
		}
		var calls sync.WaitGroup
		for i := 0; i < 6; i++ {
			calls.Add(1)
			go func() {
				defer calls.Done()
				_, err := service.Run(t.Context(), active.OperationID)
				if phase == "lost" || phase == "refunded" {
					return
				}
				if err != nil {
					t.Errorf("refund execution failed: %v", err)
				}
			}()
		}
		calls.Wait()
		if phase == "failed" || phase == "lost" {
			if channel.posts.Load() != 1 {
				t.Fatal("first dispatch must POST once", channel.posts.Load())
			}
		} else if channel.posts.Load() != 0 {
			t.Fatal("recovery repeated POST")
		}
		if phase == "pending" || phase == "refunded" {
			if channel.gets.Load() != 1 {
				t.Fatal("recovery must GET once", channel.gets.Load())
			}
		}
		if phase == "apply" && channel.gets.Load() != 0 {
			t.Fatal("cached verified observation must not require another network call")
		}
		var state struct {
			State   string
			Applied bool
		}
		if db.Table("order_refund_operations").Select("state,applied").Where("operation_id=?", active.OperationID).Take(&state).Error != nil {
			t.Fatal("state missing")
		}
		if phase == "refunded" && (state.State != "refunded" || state.Applied) {
			t.Fatal("late ledger failure must retain verified but unapplied refund")
		}
		if phase == "apply" && !state.Applied {
			t.Fatal("verified refund must apply from cache")
		}
		if phase == "pending" {
			before := fingerprint()
			if err := service.FinishCancellation(t.Context(), root.OperationID); err != nil || fingerprint() != before {
				t.Fatal("pending refund released cancellation")
			}
		}
		if phase == "lost" {
			before := fingerprint()
			var heldVersion int
			if db.Table("orders").Select("version").Where("id=?", order).Scan(&heldVersion).Error != nil {
				t.Fatal("order version missing")
			}
			tx := db.Begin()
			_, e := orders.AdvanceVersionTx(t.Context(), tx, order, heldVersion)
			_ = tx.Rollback()
			if e == nil || fingerprint() != before {
				t.Fatal("uncertain cancellation did not freeze amount changes")
			}
		}
	case "devices":
		baseline := fingerprint()
		rollback := errors.New("owned device rollback")
		for _, state := range []string{"reserved", "packed", "shipped"} {
			err := db.Transaction(func(tx *gorm.DB) error {
				var line struct{ ItemID, VariantID string }
				if tx.Table("order_items i").Select("i.item_id,l.variant_id").Joins("JOIN orders o ON o.id=i.order_id AND o.version=i.version").Joins("JOIN order_line_items l ON l.id=i.item_id").Where("i.order_id=? AND i.deleted_at IS NULL", order).Limit(1).Take(&line).Error != nil {
					return errors.New("device line missing")
				}
				if tx.Exec("INSERT INTO devices(id,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('owned-cancel-device',?,'owned-cancel-source','owned-cancel-location',0,'eur','','',?,'sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", line.VariantID, state).Error != nil {
					return errors.New("device setup failed")
				}
				if tx.Exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('owned-cancel-assignment','owned-cancel-device',?,?,'owned-cancel-source',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", order, line.ItemID, state).Error != nil {
					return errors.New("assignment setup failed")
				}
				probe := service
				probe.DB = tx
				finishErr := probe.FinishCancellation(t.Context(), root.OperationID)
				if state != "reserved" {
					if finishErr == nil {
						return errors.New("packed/shipped device released")
					}
					return rollback
				}
				if finishErr != nil {
					return finishErr
				}
				var deviceState, assignmentState string
				if tx.Table("devices").Select("state").Where("id='owned-cancel-device'").Scan(&deviceState).Error != nil || deviceState != "available" {
					return errors.New("reserved device not released")
				}
				if tx.Table("device_assignments").Select("status").Where("id='owned-cancel-assignment'").Scan(&assignmentState).Error != nil || assignmentState != "canceled" {
					return errors.New("assignment not canceled")
				}
				return rollback
			})
			if !errors.Is(err, rollback) || fingerprint() != baseline {
				t.Fatal("device release/rollback failed", state, err)
			}
		}
	case "finish-failure":
		before := fingerprint()
		if err := service.FinishCancellation(t.Context(), root.OperationID); err == nil || fingerprint() != before {
			t.Fatal("late cancellation failure was not atomic")
		}
	case "finish":
		var calls sync.WaitGroup
		for i := 0; i < 6; i++ {
			calls.Add(1)
			go func() {
				defer calls.Done()
				if err := service.FinishCancellation(t.Context(), root.OperationID); err != nil {
					t.Errorf("finish failed: %v", err)
				}
			}()
		}
		calls.Wait()
	default:
		t.Fatal("unknown owned cancellation phase")
	}
	raw, _ := json.Marshal(map[string]any{"passed": !t.Failed(), "phase": phase, "posts": channel.posts.Load(), "gets": channel.gets.Load(), "six_concurrent_executions": phase != "guards" && phase != "finish-failure" && phase != "devices"})
	if os.WriteFile(filepath.Join(logs, "cancellation-"+phase+".json"), raw, 0600) != nil {
		t.Fatal("evidence unavailable")
	}
}
