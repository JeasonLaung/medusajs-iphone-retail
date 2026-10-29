package identity_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/devices"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func checkDeviceIdentity(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := t.Context()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("device identity fixture", e)
		}
	}
	insert := func(tx *gorm.DB, id string, in devices.Identity) error {
		nullable := func(s string) any {
			if s == "" {
				return nil
			}
			return s
		}
		return tx.Exec("INSERT INTO devices(id,serial_number,imei,imei2,eid,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,?,?,?,?,'artificial-sku','artificial-source','artificial-location',0,'eur','','','available','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, in.SerialNumber, nullable(in.IMEI), nullable(in.IMEI2), nullable(in.EID)).Error
	}
	claim := func(id string, in devices.Identity) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if e := devices.LockIdentityTx(ctx, tx); e != nil {
				return e
			}
			if e := insert(tx, id, in); e != nil {
				return e
			}
			return devices.ClaimIdentityTx(ctx, tx, id, in, true)
		})
	}
	base := devices.Identity{SerialNumber: "DEVICE-SCHEMA-ONE", IMEI: "000000000000001", EID: "00000000000000000000000000000001"}
	if devices.LockIdentityTx(ctx, db) != devices.ErrUnavailable || devices.ClaimIdentityTx(ctx, db, "invalid", base, true) != devices.ErrUnavailable {
		t.Fatal("autocommit accepted")
	}
	if e := claim("device-one", base); !errors.Is(e, devices.ErrUnavailable) {
		t.Fatal("default disabled capability accepted", e)
	}
	exec("UPDATE device_runtime_schema SET identity_enabled=TRUE WHERE id=1")
	defer exec("UPDATE device_runtime_schema SET identity_enabled=FALSE WHERE id=1")
	if e := claim("device-one", base); e != nil {
		t.Fatal("canonical claim failed", e)
	}
	var n int64
	if db.Table("device_identity_claims").Where("device_id='device-one'").Count(&n).Error != nil || n != 3 {
		t.Fatal("identity claims missing")
	}
	var eid string
	if db.Table("devices").Where("id='device-one'").Pluck("eid", &eid).Error != nil || eid != base.EID {
		t.Fatal("leading zero EID lost")
	}
	// Cross-column radio collision must roll back the newly inserted device.
	other := devices.Identity{SerialNumber: "DEVICE-SCHEMA-TWO", IMEI2: base.IMEI}
	if e := claim("device-cross-column", other); !errors.Is(e, devices.ErrDuplicate) {
		t.Fatal("cross-column collision accepted", e)
	}
	if db.Table("devices").Where("id='device-cross-column'").Count(&n).Error != nil || n != 0 {
		t.Fatal("conflicting device committed")
	}
	// Existing source rows without a claim still participate in the uniqueness check.
	imported := devices.Identity{SerialNumber: "DEVICE-UNCLAIMED", IMEI: "000000000000002"}
	if e := insert(db, "device-unclaimed", imported); e != nil {
		t.Fatal(e)
	}
	if e := claim("device-import-collision", devices.Identity{SerialNumber: "DEVICE-NEW", IMEI2: imported.IMEI}); !errors.Is(e, devices.ErrDuplicate) {
		t.Fatal("unclaimed source collision ignored", e)
	}
	// Distinct serials, same radio in alternating IMEI columns: one commit only.
	var wins atomic.Int32
	var unexpected atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := devices.Identity{SerialNumber: fmt.Sprintf("DEVICE-RACE-%d", i)}
			if i%2 == 0 {
				in.IMEI = "000000000000003"
			} else {
				in.IMEI2 = "000000000000003"
			}
			e := claim(fmt.Sprintf("device-race-%d", i), in)
			var duplicate *driver.MySQLError
			if e == nil {
				wins.Add(1)
			} else if !errors.Is(e, devices.ErrDuplicate) && !(errors.As(e, &duplicate) && duplicate.Number == 1062) {
				unexpected.Store(true)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 || unexpected.Load() {
		t.Fatal("concurrent cross-column claim did not serialize")
	}
	// Two devices in one batch: second failure rolls back the first too.
	e := db.Transaction(func(tx *gorm.DB) error {
		if e := devices.LockIdentityTx(ctx, tx); e != nil {
			return e
		}
		first := devices.Identity{SerialNumber: "BATCH-FIRST", IMEI: "000000000000004"}
		if e := insert(tx, "device-batch-first", first); e != nil {
			return e
		}
		if e := devices.ClaimIdentityTx(ctx, tx, "device-batch-first", first, true); e != nil {
			return e
		}
		second := devices.Identity{SerialNumber: "BATCH-SECOND", IMEI2: base.IMEI}
		if e := insert(tx, "device-batch-second", second); e != nil {
			return e
		}
		return devices.ClaimIdentityTx(ctx, tx, "device-batch-second", second, true)
	})
	if !errors.Is(e, devices.ErrDuplicate) {
		t.Fatal("batch collision accepted", e)
	}
	if db.Table("devices").Where("id LIKE 'device-batch-%'").Count(&n).Error != nil || n != 0 {
		t.Fatal("partial batch committed")
	}
	exec("CREATE TRIGGER device_claim_fault BEFORE INSERT ON device_identity_claims FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial claim fault'")
	if e := claim("device-fault", devices.Identity{SerialNumber: "FAULT", IMEI: "000000000000005"}); !errors.Is(e, devices.ErrUnavailable) {
		t.Fatal("late claim failure not observed", e)
	}
	exec("DROP TRIGGER device_claim_fault")
	if db.Table("devices").Where("id='device-fault'").Count(&n).Error != nil || n != 0 {
		t.Fatal("late claim failure left device")
	}
	// Soft deletion releases source uniqueness; a new claim replaces only the
	// deleted holder while the archived device facts remain unchanged.
	exec("UPDATE devices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='device-one'")
	if e := claim("device-reused", base); e != nil {
		t.Fatal("deleted holder prevented source-compatible reuse", e)
	}
	if db.Table("devices").Where("id='device-one' AND deleted_at IS NOT NULL").Count(&n).Error != nil || n != 1 {
		t.Fatal("deleted historical device lost")
	}
	// Assignment/source/tracking constraints preserve reviewed source behavior.
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('assignment-one','device-one','order-one','line-one','source-one','reserved',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	if db.Exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('assignment-two','device-one','order-two','line-two','source-one','packed',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))").Error == nil {
		t.Fatal("two active assignments accepted")
	}
	exec("UPDATE device_assignments SET status='canceled' WHERE id='assignment-one'")
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES('assignment-two','device-one','order-two','line-two','source-one','packed',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	if db.Exec("UPDATE devices SET state='invalid' WHERE id='device-one'").Error == nil || db.Exec("UPDATE devices SET photos='invalid' WHERE id='device-one'").Error == nil {
		t.Fatal("device schema constraint bypassed")
	}
	t.Log(`DEVICE_IDENTITY_REHEARSAL {"real_mysql":true,"reviewed_device_tables":6,"default_disabled":true,"leading_zero_eid":true,"cross_column_radio_conflict":true,"unclaimed_source_rows_checked":true,"concurrent_attempts":6,"single_winner":true,"batch_failure_atomic":true,"late_claim_failure_atomic":true,"single_active_assignment":true,"enum_and_json_rejected":true,"autocommit_rejected":true,"stock_changed":false,"intake_http_or_ui_integrated":false}`)
}
