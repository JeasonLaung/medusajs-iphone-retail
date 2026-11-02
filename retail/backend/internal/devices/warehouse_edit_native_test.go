package devices

import (
	"encoding/json"
	"errors"
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

func TestWindowsWarehouseEditAllocation(t *testing.T) {
	if os.Getenv("WINDOWS_DEVICE_EDIT_TEST") != "1" {
		t.Skip("owned Windows edit fixture required")
	}
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Addr != "127.0.0.1:13307" || cfg.Net != "tcp" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned database required")
	}
	var browser struct {
		Device   string `json:"edit_device_id"`
		Location string `json:"edit_location_id"`
	}
	raw, e := os.ReadFile(filepath.Join(logs, "browser-result.json"))
	if e != nil || json.Unmarshal(raw, &browser) != nil || browser.Device == "" || browser.Location == "" {
		t.Fatal("browser fixture required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("database unavailable")
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
	if tx.Where("email=?", "local-editor-"+owner+"@example.invalid").Take(&actor).Error != nil {
		t.Fatal("fixture editor missing")
	}
	var role string
	if tx.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Scan(&role).Error != nil || role == "" {
		t.Fatal("role missing")
	}
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.intake.write')", role)
	h := Handler{DB: tx}
	ctx := t.Context()
	read := func() WarehouseEditContext {
		t.Helper()
		v, e := h.ReadWarehouseEdit(ctx, actor, browser.Location, "sale", browser.Device)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := read()
	supplier := "dsrc_" + uuid.NewString()
	supplierLocation := "sloc_" + uuid.NewString()
	exec("INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES(?,'Owned supplier location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", supplierLocation)
	exec("INSERT INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, supplierLocation)
	exec("INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES(?,'Owned supplier fixture','supplier',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", supplier, supplierLocation)
	for _, s := range read().Sources {
		if s.ID == supplier {
			t.Fatal("supplier exposed as editable warehouse source")
		}
	}
	denied := WarehouseEditInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Fields: before.Fields}
	denied.Fields.SourceID = supplier
	if _, e := h.EditWarehouseDevice(ctx, actor, browser.Location, "sale", browser.Device, denied); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("supplier source accepted", e)
	}
	var other string
	if tx.Table("inventory_locations").Select("id").Where("id<>? AND deleted_at IS NULL", browser.Location).Order("id").Limit(1).Scan(&other).Error != nil || other == "" {
		t.Fatal("other warehouse missing")
	}
	exec("INSERT IGNORE INTO authorization_warehouse_locations(user_id,location_id) VALUES(?,?)", actor.ID, other)
	const order = "order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d"
	var item string
	if tx.Table("order_items").Select("item_id").Where("order_id=? AND deleted_at IS NULL", order).Limit(1).Scan(&item).Error != nil || item == "" {
		t.Fatal("copied order item missing")
	}
	assignment := "dassign_" + uuid.NewString()
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,created_at,updated_at) VALUES(?,?,?,?,?,'reserved',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", assignment, browser.Device, order, item, before.Fields.SourceID)
	exec("UPDATE devices SET state='reserved' WHERE id=?", browser.Device)
	active := read()
	input := func() WarehouseEditInput {
		return WarehouseEditInput{OperationID: uuid.NewString(), ExpectedRevision: active.Revision, Fields: active.Fields}
	}
	for _, change := range []func(*WarehouseEditFields){func(f *WarehouseEditFields) { f.LocationID = other }, func(f *WarehouseEditFields) { f.State = "inspection" }, func(f *WarehouseEditFields) { f.Category = "returned" }} {
		in := input()
		change(&in.Fields)
		if _, e := h.EditWarehouseDevice(ctx, actor, browser.Location, "sale", browser.Device, in); !errors.Is(e, ErrConflict) {
			t.Fatal("allocated identity changed", e)
		}
	}
	in := input()
	in.Fields.Notes = "Allowed allocated metadata update"
	in.Fields.CostCents = 54321
	if _, e := h.EditWarehouseDevice(ctx, actor, browser.Location, "sale", browser.Device, in); e != nil {
		t.Fatal("allocated metadata edit rejected", e)
	}
	exec("DELETE FROM device_assignments WHERE id=?", assignment)
	exec("UPDATE devices SET state='available' WHERE id=?", browser.Device)
	free := read()
	in = WarehouseEditInput{OperationID: uuid.NewString(), ExpectedRevision: free.Revision, Fields: free.Fields}
	in.Fields.LocationID = other
	moved, e := h.EditWarehouseDevice(ctx, actor, browser.Location, "sale", browser.Device, in)
	if e != nil || moved.ID != browser.Device {
		t.Fatal("authorized move rejected", e)
	}
	if _, e := h.ReadWarehouseEdit(ctx, actor, browser.Location, "sale", browser.Device); !errors.Is(e, ErrNotFound) {
		t.Fatal("old location still reads moved device", e)
	}
	if replay, e := h.EditWarehouseDevice(ctx, actor, browser.Location, "sale", browser.Device, in); e != nil || !replay.Replayed {
		t.Fatal("move receipt failed", e)
	}
	exec("DELETE FROM authorization_warehouse_locations WHERE user_id=? AND location_id=?", actor.ID, other)
	if _, e := h.EditWarehouseDevice(ctx, actor, browser.Location, "sale", browser.Device, in); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("revoked destination receipt accepted", e)
	}
	t.Log("allocated structural guards / metadata edit / authorized move / source path invalidation / destination revocation verified; all rolled back")
}
