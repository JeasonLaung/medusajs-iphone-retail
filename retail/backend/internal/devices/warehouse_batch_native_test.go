package devices

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func TestWindowsWarehouseBatchLimit(t *testing.T) {
	if os.Getenv("WINDOWS_DEVICE_BATCH_TEST") != "1" {
		t.Skip("owned Windows edit fixture required")
	}
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Addr != "127.0.0.1:13307" || cfg.Net != "tcp" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned database required")
	}
	var browser struct {
		Device   string `json:"batch_device_id"`
		Location string `json:"batch_location_id"`
	}
	raw, e := os.ReadFile(filepath.Join(logs, "browser-result.json"))
	if e != nil || json.Unmarshal(raw, &browser) != nil || browser.Device == "" || browser.Location == "" {
		t.Fatal("browser fixture required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := tx.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	var actor identity.User
	if tx.Where("email=?", "local-batch-"+owner+"@example.invalid").Take(&actor).Error != nil {
		t.Fatal("fixture operator missing")
	}
	var role string
	if tx.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Scan(&role).Error != nil || role == "" {
		t.Fatal("role missing")
	}
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.intake.write')", role)
	h := Handler{DB: tx}
	ctx := t.Context()
	revision, e := sourceGate(ctx, tx)
	if e != nil {
		t.Fatal(e)
	}
	input := WarehouseBatchInput{OperationID: uuid.NewString(), Devices: []WarehouseBatchItem{}}
	for i := 0; i < 200; i++ {
		id := "dev_" + uuid.NewString()
		exec("INSERT INTO devices(id,serial_number,imei,imei2,eid,variant_id,source_id,location_id,grade,battery_health,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) SELECT ?,?,?,NULL,NULL,variant_id,source_id,location_id,grade,battery_health,cost_cents,currency_code,bin,notes,'pending','sale',photos,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM devices WHERE id=?", id, fmt.Sprintf("BATCH-LIMIT-%03d", i), fmt.Sprintf("%015d", 900000000000000+i), browser.Device)
		var row WarehouseDevice
		if warehouseDeviceQuery(tx).Where("d.id=?", id).Take(&row).Error != nil {
			t.Fatal("owned row missing")
		}
		if e := projectWarehouseDevice(&row, revision); e != nil {
			t.Fatal(e)
		}
		input.Devices = append(input.Devices, WarehouseBatchItem{ID: id, ExpectedRevision: row.Revision})
	}
	second := input.Devices[1].ID
	exec("UPDATE devices SET state='inspection' WHERE id=?", second)
	// Update the second snapshot so eligibility, rather than a stale revision, fails.
	var row WarehouseDevice
	if warehouseDeviceQuery(tx).Where("d.id=?", second).Take(&row).Error != nil {
		t.Fatal("row missing")
	}
	if e := projectWarehouseDevice(&row, revision); e != nil {
		t.Fatal(e)
	}
	invalid := WarehouseBatchInput{OperationID: uuid.NewString(), Devices: append([]WarehouseBatchItem{}, input.Devices[:2]...)}
	invalid.Devices[1].ExpectedRevision = row.Revision
	if _, e := h.MakeWarehouseBatchAvailable(ctx, actor, browser.Location, invalid); !errors.Is(e, ErrConflict) {
		t.Fatalf("ineligible second row: %v", e)
	}
	var firstState string
	if tx.Table("devices").Select("state").Where("id=?", input.Devices[0].ID).Scan(&firstState).Error != nil || firstState != "pending" {
		t.Fatal("partial update escaped rollback")
	}
	exec("UPDATE devices SET state='pending' WHERE id=?", second)
	// The manual fixture state update can alter a fractional timestamp: read all
	// snapshots immediately before the actual 200-member command.
	for i := range input.Devices {
		row = WarehouseDevice{}
		if warehouseDeviceQuery(tx).Where("d.id=?", input.Devices[i].ID).Take(&row).Error != nil {
			t.Fatal("row missing")
		}
		if e := projectWarehouseDevice(&row, revision); e != nil {
			t.Fatal(e)
		}
		input.Devices[i].ExpectedRevision = row.Revision
	}
	result, e := h.MakeWarehouseBatchAvailable(ctx, actor, browser.Location, input)
	if e != nil || len(result.IDs) != 200 || result.Replayed {
		t.Fatalf("200-device command: %v", e)
	}
	retry, e := h.MakeWarehouseBatchAvailable(ctx, actor, browser.Location, input)
	if e != nil || !retry.Replayed {
		t.Fatalf("200-device replay: %v", e)
	}
	var available, events int64
	if tx.Table("devices").Where("serial_number LIKE 'BATCH-LIMIT-%' AND state='available'").Count(&available).Error != nil || available != 200 {
		t.Fatal("missing batch state")
	}
	if tx.Table("device_events e").Joins("JOIN devices d ON d.id=e.device_id").Where("d.serial_number LIKE 'BATCH-LIMIT-%' AND e.action='make_available'").Count(&events).Error != nil || events != 200 {
		t.Fatal("non-unique batch events")
	}
	exec("UPDATE devices SET location_id=(SELECT id FROM inventory_locations WHERE id<>? AND deleted_at IS NULL LIMIT 1) WHERE id=?", browser.Location, input.Devices[199].ID)
	if _, e := h.MakeWarehouseBatchAvailable(ctx, actor, browser.Location, input); !errors.Is(e, ErrNotFound) {
		t.Fatalf("moved member replay: %v", e)
	}
	exec("UPDATE devices SET location_id=? WHERE id=?", browser.Location, input.Devices[199].ID)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='warehouse.intake.write'", role)
	if _, e := h.MakeWarehouseBatchAvailable(ctx, actor, browser.Location, input); !errors.Is(e, authorization.ErrDenied) {
		t.Fatalf("revoked replay: %v", e)
	}
}
