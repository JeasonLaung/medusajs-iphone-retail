package refunds

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Continue a real paid checkpoint and browser-created return/refund instruction.
// Only the external refund response is artificial; no real refund is sent.
type nonDeviceChannel struct {
	t       *testing.T
	db      *gorm.DB
	posts   atomic.Int32
	gets    atomic.Int32
	command Result
}

func (a *nonDeviceChannel) CreateSession(context.Context, payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
	a.t.Error("this continuation must never create a payment")
	return payments.CreateUncertain, payments.CreatedSession{}, payments.ErrCreateExecution
}
func (a *nonDeviceChannel) observation(in payments.RefundRequest) payments.RefundObservation {
	var count int64
	if in.OperationID != a.command.OperationID || in.OrderID != a.command.OrderID || in.RequestID != a.command.RequestID || in.Amount != "789.00" || strings.ToLower(in.Session.Currency) != "eur" || !strings.HasPrefix(in.RemotePaymentID, "tr_") || len(in.Session.Data) == 0 {
		a.t.Error("original encrypted payment provenance changed")
	}
	if a.db.Table("order_refund_operations").Where("operation_id=? AND dispatched_at IS NOT NULL", in.OperationID).Count(&count).Error != nil || count != 1 {
		a.t.Error("external refund before durable dispatch")
	}
	return payments.RefundObservation{RefundID: "re_owned_non_device", PaymentID: in.RemotePaymentID, Status: "refunded", Amount: in.Amount, Currency: in.Session.Currency, CreatedAt: time.Now().UTC()}
}
func (a *nonDeviceChannel) CreateRefund(_ context.Context, in payments.RefundRequest) (payments.RefundObservation, error) {
	a.posts.Add(1)
	return a.observation(in), nil
}
func (a *nonDeviceChannel) ObserveRefund(_ context.Context, in payments.RefundRequest, _ string) (payments.RefundObservation, error) {
	a.gets.Add(1)
	return a.observation(in), nil
}
func (a *nonDeviceChannel) DiscoverRefund(ctx context.Context, in payments.RefundRequest) (payments.RefundObservation, error) {
	return a.ObserveRefund(ctx, in, "")
}

func TestWindowsNonDeviceRefund(t *testing.T) {
	if os.Getenv("WINDOWS_NON_DEVICE_REFUND") != "1" {
		t.Skip("explicit owned Windows continuation required")
	}
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned loopback database required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	ctx := context.Background()
	wire, e := os.ReadFile(filepath.Join(logs, "non-device-command.json"))
	if e != nil {
		t.Fatal(e)
	}
	var command Result
	if json.Unmarshal(wire, &command) != nil || !validUUID(command.OperationID) || command.Amount != "789.00" {
		t.Fatal("owned browser command required")
	}
	var saved operation
	if db.Where("operation_id=? AND dispatched_at IS NULL AND applied=FALSE", command.OperationID).Take(&saved).Error != nil || saved.OrderID != command.OrderID || saved.RequestID != command.RequestID {
		t.Fatal("fresh browser instruction missing")
	}
	exec := func(tx *gorm.DB, q string, args ...any) {
		t.Helper()
		if e := tx.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	var line struct{ ItemID, VariantID string }
	if db.Table("order_items i").Select("i.item_id,l.variant_id").Joins("JOIN order_line_items l ON l.id=i.item_id").Joins("JOIN orders o ON o.id=i.order_id AND o.version=i.version").Where("i.order_id=?", command.OrderID).Take(&line).Error != nil {
		t.Fatal("current line missing")
	}
	setLines := func(tx *gorm.DB, typ string, lines []map[string]any) {
		b, _ := json.Marshal(map[string]any{"lines": lines})
		exec(tx, "UPDATE after_sales_requests SET type=?,items=? WHERE id=?", typ, string(b), command.RequestID)
	}
	item := func(qty int, ids ...string) map[string]any {
		return map[string]any{"item_id": line.ItemID, "quantity": qty, "device_ids": ids}
	}
	tracking := func(tx *gorm.DB) {
		exec(tx, "INSERT INTO device_variant_tracking(id,variant_id,inventory_item_id,created_at,updated_at) VALUES('owned-non-device-tracking',?,'owned-item',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", line.VariantID)
	}
	shipment := func(tx *gorm.DB) {
		exec(tx, "INSERT INTO fulfillments(id,location_id,requires_shipping,shipped_at,created_at,updated_at) VALUES('owned-non-device-parcel','owned-location',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		exec(tx, "INSERT INTO order_fulfillments(id,order_id,fulfillment_id,created_at,updated_at) VALUES('owned-non-device-link',?,'owned-non-device-parcel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", command.OrderID)
		exec(tx, `INSERT INTO fulfillment_items(id,title,sku,barcode,quantity,raw_quantity,line_item_id,fulfillment_id,created_at,updated_at) VALUES('owned-non-device-item','','','',1,'{"value":"1","precision":100}',?,'owned-non-device-parcel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, line.ItemID)
	}
	assignment := func(tx *gorm.DB) {
		exec(tx, "INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('owned-non-device-assignment','owned-device',?,?,'owned-source','shipped',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", command.OrderID, line.ItemID)
	}
	cases := []struct {
		name   string
		change func(*gorm.DB)
		want   error
	}{
		{"nontracked", func(*gorm.DB) {}, nil},
		{"tracked_unshipped", tracking, nil},
		{"tracked_shipped_requires_devices", func(tx *gorm.DB) { tracking(tx); shipment(tx) }, ErrConflict},
		{"canceled_parcel_not_shipped", func(tx *gorm.DB) {
			tracking(tx)
			shipment(tx)
			exec(tx, "UPDATE fulfillments SET canceled_at=UTC_TIMESTAMP(6) WHERE id='owned-non-device-parcel'")
		}, nil},
		{"nontracked_shipped_native_refund", shipment, nil},
		{"tracking_disabled_existing_assignment", func(tx *gorm.DB) { shipment(tx); assignment(tx) }, ErrConflict},
		{"assignment_without_parcel", assignment, ErrConflict},
		{"quantity_above_current", func(tx *gorm.DB) { setLines(tx, "return", []map[string]any{item(2)}) }, ErrConflict},
		{"zero_quantity", func(tx *gorm.DB) { setLines(tx, "return", []map[string]any{item(0)}) }, ErrConflict},
		{"duplicate_line", func(tx *gorm.DB) { setLines(tx, "return", []map[string]any{item(1), item(1)}) }, ErrConflict},
		{"wrong_line", func(tx *gorm.DB) {
			v := item(1)
			v["item_id"] = "foreign-line"
			setLines(tx, "return", []map[string]any{v})
		}, ErrConflict},
		{"exchange_requires_devices", func(tx *gorm.DB) { setLines(tx, "exchange", []map[string]any{item(1)}) }, ErrConflict},
		{"selected_device_must_be_received", func(tx *gorm.DB) { setLines(tx, "return", []map[string]any{item(1, "missing-device")}) }, ErrConflict},
		{"stale_request_status", func(tx *gorm.DB) {
			exec(tx, "UPDATE after_sales_requests SET status='completed' WHERE id=?", command.RequestID)
		}, ErrConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx := db.Begin()
			defer tx.Rollback()
			exec(tx, "UPDATE device_variant_tracking SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id=? AND deleted_at IS NULL", line.VariantID)
			tc.change(tx)
			if got := returned(ctx, tx, command.OrderID, command.RequestID); !errors.Is(got, tc.want) {
				t.Fatalf("eligibility got %v want %v", got, tc.want)
			}
		})
	}
	ring, e := protected.ParseKeyring(os.Getenv("PAYMENT_KEY_ACTIVE"), os.Getenv("PAYMENT_KEYRING_JSON"))
	if e != nil {
		t.Fatal("payment keyring unavailable")
	}
	channel := &nonDeviceChannel{t: t, db: db, command: command}
	adapters, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{saved.ProviderID: channel})
	if e != nil {
		t.Fatal(e)
	}
	s := Service{DB: db, StoreID: os.Getenv("STOREFRONT_STORE_ID"), Ring: ring, Adapters: adapters}
	// A parcel shipped after preparation must block the first external dispatch.
	shipment(db)
	if _, e = s.Run(ctx, command.OperationID); !errors.Is(e, ErrConflict) || channel.posts.Load() != 0 {
		t.Fatal("late shipment was not rejected", e)
	}
	var count int64
	db.Table("order_refund_operations").Where("operation_id=? AND dispatched_at IS NOT NULL", command.OperationID).Count(&count)
	if count != 0 {
		t.Fatal("blocked refund dispatched")
	}
	exec(db, "DELETE FROM fulfillment_items WHERE id='owned-non-device-item'")
	exec(db, "DELETE FROM order_fulfillments WHERE id='owned-non-device-link'")
	exec(db, "DELETE FROM fulfillments WHERE id='owned-non-device-parcel'")
	exec(db, "DELETE FROM device_variant_tracking WHERE id='owned-non-device-tracking'")
	// Fail after refund/transaction writes: all accounting must roll back while
	// the separately committed remote observation remains recoverable locally.
	_, e = s.Run(ctx, command.OperationID)
	if e == nil || channel.posts.Load() != 1 {
		t.Fatal("late refund failure not exercised", e)
	}
	for _, table := range []string{"payment_refunds", "order_transactions"} {
		query := db.Table(table)
		if table == "payment_refunds" {
			query = query.Where("id=?", "ref_"+command.OperationID)
		} else {
			query = query.Where("reference_id=?", "ref_"+command.OperationID)
		}
		if query.Count(&count).Error != nil || count != 0 {
			t.Fatal("late accounting leaked", table)
		}
	}
	exec(db, "DROP TRIGGER owned_non_device_ledger_failure")
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Run(ctx, command.OperationID)
			if e == nil && !r.Applied {
				e = errors.New("recovery not applied")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if channel.posts.Load() != 1 || channel.gets.Load() != 0 {
		t.Fatal("local recovery repeated remote calls")
	}
	if db.Table("payment_refunds").Where("id=? AND amount=789", "ref_"+command.OperationID).Count(&count).Error != nil || count != 1 {
		t.Fatal("unique exact refund missing")
	}
	result := map[string]any{"eligibility_cases": len(cases), "late_shipment_blocked": true, "late_ledger_failure_rolled_back": true, "concurrent_recoveries": 6, "channel_posts": channel.posts.Load(), "channel_gets": channel.gets.Load(), "real_mollie_refund": false}
	b, _ := json.Marshal(result)
	if os.WriteFile(filepath.Join(logs, "non-device-fixture-result.json"), b, 0600) != nil {
		t.Fatal("fixture evidence unavailable")
	}
}
