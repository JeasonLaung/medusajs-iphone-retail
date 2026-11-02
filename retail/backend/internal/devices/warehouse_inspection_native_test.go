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

// Runs only inside the owned Windows browser fixture. All additional repair
// facts and commands live in an outer rollback transaction.
func TestWindowsWarehouseInspectionRepair(t *testing.T) {
	if os.Getenv("WINDOWS_INSPECTION_TEST") != "1" {
		t.Skip("owned Windows inspection fixture required")
	}
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	cfg, e := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	if e != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Addr != "127.0.0.1:13307" || cfg.Net != "tcp" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned database required")
	}
	var browser struct {
		Device    string `json:"inspection_device_id"`
		Location  string `json:"inspection_location_id"`
		Variant   string `json:"variant_id"`
		Alternate string `json:"inspection_alternate_variant_id"`
	}
	raw, e := os.ReadFile(filepath.Join(logs, "browser-result.json"))
	if e != nil || json.Unmarshal(raw, &browser) != nil || browser.Device == "" || browser.Location == "" || browser.Variant == "" {
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
	if tx.Where("email=?", "local-inspector-"+owner+"@example.invalid").Take(&actor).Error != nil {
		t.Fatal("fixture inspector missing")
	}
	// Browser verification revoked this grant; restore it only in this rollback
	// transaction to independently exercise the original-repair business rule.
	var role string
	if tx.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Scan(&role).Error != nil || role == "" {
		t.Fatal("inspector role missing")
	}
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) VALUES(?,'warehouse.return.write')", role)
	exec("UPDATE devices SET state='inspection',category='returned' WHERE id=?", browser.Device)
	var source string
	if tx.Table("devices").Select("source_id").Where("id=?", browser.Device).Scan(&source).Error != nil {
		t.Fatal("source missing")
	}
	const order = "order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d"
	var item string
	if tx.Table("order_items").Select("item_id").Where("order_id=? AND deleted_at IS NULL", order).Limit(1).Scan(&item).Error != nil || item == "" {
		t.Fatal("copied order item missing")
	}
	request, assignment := "asr_"+uuid.NewString(), "dassign_"+uuid.NewString()
	exec("INSERT INTO after_sales_requests(id,order_id,customer_id,type,reason,items,status,created_at,updated_at) VALUES(?,?,?,'repair','owned fixture','[]','processing',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", request, order, "owned-inspection-repair")
	exec("INSERT INTO device_assignments(id,device_id,order_id,item_id,source_id,status,after_sales_request_id,created_at,updated_at) VALUES(?,?,?,?,?,'returned',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", assignment, browser.Device, order, item, source, request)
	h := Handler{DB: tx}
	ctx := t.Context()
	read := func() WarehouseInspection {
		t.Helper()
		v, e := h.ReadWarehouseInspection(ctx, actor, browser.Location, browser.Device)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := read()
	if !before.Repair {
		t.Fatal("original repair not recognized")
	}
	other := browser.Alternate
	var product string
	if tx.Table("catalog_variants").Select("product_id").Where("id=? AND deleted_at IS NULL", other).Scan(&product).Error != nil || product == "" {
		t.Fatal("alternate SKU missing")
	}
	sku, e := intakeSKU(tx, product, other)
	if e != nil {
		t.Fatal("alternate SKU invalid", e)
	}
	in := ServiceInput{OperationID: uuid.NewString(), ExpectedRevision: before.Device.Revision, Action: "inspect", Result: "pass", VariantID: other, Grade: "B", BatteryHealth: 85, Notes: "Owned original repair inspection", Photos: []Photo{}}
	matched := false
	for _, grade := range []string{"S", "A", "B", "C"} {
		for battery := 0; battery <= 100; battery++ {
			if validateStoredConfiguration(sku.Options, Configuration{Grade: &grade, BatteryHealth: &battery}) == nil {
				in.Grade = grade
				in.BatteryHealth = battery
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched {
		t.Fatal("compatible alternate configuration missing")
	}
	if _, e := h.InspectWarehouseDevice(ctx, actor, browser.Location, browser.Device, in); !errors.Is(e, ErrConflict) {
		t.Fatal("original repair allowed different SKU", e)
	}
	in.VariantID = browser.Variant
	in.Grade = "B"
	in.BatteryHealth = 85
	exec("UPDATE after_sales_requests SET status='completed' WHERE id=?", request)
	if read().Device.Revision == before.Device.Revision {
		t.Fatal("repair status absent from revision")
	}
	if _, e := h.InspectWarehouseDevice(ctx, actor, browser.Location, browser.Device, in); !errors.Is(e, ErrConflict) {
		t.Fatal("stale repair state accepted", e)
	}
	exec("UPDATE after_sales_requests SET status='processing' WHERE id=?", request)
	out, e := h.InspectWarehouseDevice(ctx, actor, browser.Location, browser.Device, in)
	if e != nil || out.State != "repair_ready" {
		t.Fatal("original repair should become repair_ready", e)
	}
	again, e := h.InspectWarehouseDevice(ctx, actor, browser.Location, browser.Device, in)
	if e != nil || !again.Replayed {
		t.Fatal("original repair replay", e)
	}
	// Current warehouse/category and authority must precede an existing receipt.
	exec("UPDATE devices SET location_id=? WHERE id=?", "sloc_01M4FBP3NDPZHJKVV3VY682QTN", browser.Device)
	if _, e := h.InspectWarehouseDevice(ctx, actor, browser.Location, browser.Device, in); e == nil {
		t.Fatal("moved device receipt accepted")
	}
	exec("UPDATE devices SET location_id=? WHERE id=?", browser.Location, browser.Device)
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='warehouse.return.write'", actor.ID)
	if _, e := h.InspectWarehouseDevice(ctx, actor, browser.Location, browser.Device, in); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("revoked receipt accepted", e)
	}
	t.Log("original repair SKU guard / repair_ready / same-operation recovery / moved-device and revoked-receipt denial verified; outer transaction rolled back")
}
