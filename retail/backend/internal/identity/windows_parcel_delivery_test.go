package identity_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestWindowsParcelDelivery(t *testing.T) {
	if os.Getenv("WINDOWS_PARCEL_DELIVERY") != "1" {
		t.Skip("owned shipped checkpoint required")
	}
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs, order := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS"), os.Getenv("WINDOWS_DELIVERY_ORDER")
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner || !regexp.MustCompile(`^order_[0-9a-f-]{36}$`).MatchString(order) {
		t.Fatal("owned loopback database required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	var actor identity.User
	if db.Where("role='admin' AND enabled=TRUE").Order("id").First(&actor).Error != nil {
		t.Fatal("owned admin missing")
	}
	var parcels []string
	if db.Table("fulfillments f").Select("f.id").Joins("JOIN order_fulfillments l ON l.fulfillment_id=f.id AND l.deleted_at IS NULL").Where("l.order_id=? AND f.shipped_at IS NOT NULL AND f.delivered_at IS NULL AND f.canceled_at IS NULL AND f.deleted_at IS NULL", order).Pluck("f.id", &parcels).Error != nil || len(parcels) != 1 {
		t.Fatal("one shipped parcel required")
	}
	h := devices.Handler{DB: db}
	parcel := parcels[0]
	read := func(h devices.Handler) devices.ParcelContext {
		t.Helper()
		v, e := h.ReadParcel(t.Context(), actor, order, parcel)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	input := func(v devices.ParcelContext) devices.ParcelInput {
		return devices.ParcelInput{OperationID: uuid.NewString(), ExpectedRevision: v.Revision, Action: "deliver"}
	}
	fingerprint := func() string {
		t.Helper()
		all := []json.RawMessage{}
		for _, table := range []string{"fulfillments", "order_items", "device_assignments", "devices", "inventory_levels", "inventory_reservations", "payments", "payment_captures", "payment_refunds", "customer_order_ownership", "order_device_parcel_operations", "customer_account_notices", "customer_event_mail", "device_runtime_schema"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	baseline := fingerprint()
	before := read(h)
	if before.Status != "shipped" || len(before.Devices) != 1 {
		t.Fatal("shipped device required")
	}
	stale := input(before)
	stale.ExpectedRevision = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, e = h.ChangeParcel(t.Context(), actor, order, parcel, stale); e == nil {
		t.Fatal("stale delivery accepted")
	}
	bad := input(before)
	bad.TrackingNumber = "rewrite"
	if _, e = h.ChangeParcel(t.Context(), actor, order, parcel, bad); e == nil {
		t.Fatal("delivery changed tracking")
	}
	if _, e = h.ChangeParcel(t.Context(), actor, "missing-order", parcel, input(before)); e == nil {
		t.Fatal("foreign order accepted")
	}
	fault := input(before)
	fault.OperationID = "00000000-0000-4000-8000-000000000001"
	_, e = h.ChangeParcel(t.Context(), actor, order, parcel, fault)
	if e == nil || fingerprint() != baseline {
		t.Fatal("late failure did not roll back delivery, counts and notices")
	}
	rollback := errors.New("owned rollback")
	for _, mutation := range []string{
		"UPDATE order_items SET delivered_quantity=shipped_quantity,raw_delivered_quantity=raw_shipped_quantity WHERE order_id=?",
		"UPDATE fulfillments SET canceled_at=UTC_TIMESTAMP(6) WHERE id=?",
		"UPDATE devices SET state='available' WHERE id=?",
		"UPDATE admin_users SET enabled=FALSE WHERE id=?",
		"UPDATE order_device_parcel_runtime SET enabled=FALSE WHERE id=1",
	} {
		e = db.Transaction(func(tx *gorm.DB) error {
			id := order
			if mutation == "UPDATE fulfillments SET canceled_at=UTC_TIMESTAMP(6) WHERE id=?" {
				id = parcel
			}
			if mutation == "UPDATE devices SET state='available' WHERE id=?" {
				id = before.Devices[0].DeviceID
			}
			if mutation == "UPDATE admin_users SET enabled=FALSE WHERE id=?" {
				id = actor.ID
			}
			var err error
			if mutation == "UPDATE order_device_parcel_runtime SET enabled=FALSE WHERE id=1" {
				err = tx.Exec(mutation).Error
			} else {
				err = tx.Exec(mutation, id).Error
			}
			if err != nil {
				return err
			}
			probe := devices.Handler{DB: tx}
			v, err := probe.ReadParcel(t.Context(), actor, order, parcel)
			if err != nil {
				v = before
			}
			if _, err = probe.ChangeParcel(t.Context(), actor, order, parcel, input(v)); err == nil {
				return errors.New("unsafe delivery accepted")
			}
			return rollback
		})
		if !errors.Is(e, rollback) || fingerprint() != baseline {
			t.Fatal("delivery guard or rollback failed", e)
		}
	}
	b, _ := json.Marshal(map[string]any{"passed": true, "stale_tracking_foreign_order_denied": true, "late_receipt_failure_atomic": true, "over_delivery_canceled_returned_disabled_account_and_gate_denied": true, "protected_facts_unchanged": true, "new_delivery_effects": 0})
	if os.WriteFile(filepath.Join(logs, "parcel-delivery-guards.json"), b, 0600) != nil {
		t.Fatal("guard evidence unavailable")
	}
}
