package identity_test

import (
	"encoding/json"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func seedPriceEdit(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(q string) {
		t.Helper()
		if e := db.Exec(q).Error; e != nil {
			t.Fatal("price edit fixture", e)
		}
	}
	exec(`INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('price-edit-channel','Price edit channel',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES('price-edit-store','Price edit store','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT IGNORE INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('eur','EUR','EUR',2,0,'{"value":"0","precision":20}','Euro',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('price-edit-region','Europe','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_store_currencies(id,store_id,currency_code,is_default,created_at,updated_at) VALUES('price-edit-currency','price-edit-store','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('price-edit-product','Price edit product','price-edit-product',FALSE,'published',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES('price-edit-channel-link','price-edit-product','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variants(id,product_id,title,sku,allow_backorder,manage_inventory,created_at,updated_at) VALUES('price-edit-v','price-edit-product','128 GB','price-edit-128',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('price-edit-shared','price-edit-product','256 GB','price-edit-256',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('price-edit-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variant_price_sets(id,variant_id,price_set_id,created_at,updated_at) VALUES('price-edit-link','price-edit-v','price-edit-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('price-edit-link-shared','price-edit-shared','price-edit-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES('price-edit-base','price-edit-set','eur',100,'{"value":"100","precision":20}',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
}
func checkPriceAmountEdit(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	seedPriceEdit(t, db)
	ctx := t.Context()
	h := catalog.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	const path = "/api/admin/products/price-edit-product/variants/price-edit-v/prices/price-edit-base"
	read := func() catalog.PriceEditContext {
		t.Helper()
		var d catalog.PriceEditContext
		raw := call("GET", path, "", 200)
		if json.Unmarshal(raw, &d) != nil || d.SharedVariants != 2 || strings.Contains(string(raw), "raw_amount") {
			t.Fatal("price edit context")
		}
		return d
	}
	body := func(in catalog.PriceAmountInput) string { b, _ := json.Marshal(in); return string(b) }
	exec(`UPDATE pricing_prices SET raw_amount='{"value":"100.00000000000000000001","precision":20}' WHERE id='price-edit-base'`)
	old := read()
	if old.EffectiveAmount != "100.00000000000000000001" {
		t.Fatal("rounded scalar substituted for authoritative raw amount")
	}
	if old.Enabled {
		t.Fatal("price edit default must be disabled")
	}
	in := catalog.PriceAmountInput{OperationID: uuid.NewString(), ExpectedRevision: old.Revision, Amount: "0.000000000000000000"}
	call("POST", path, body(in), 503)
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.EditPriceAmount(ctx, actor, "price-edit-product", "price-edit-v", "price-edit-base", in)
			if e != nil {
				failed.Store(true)
			} else if out.Replayed {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if failed.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("price edit concurrent operation")
	}
	call("POST", path, body(in), 200)
	var raw struct{ Amount, RawAmount string }
	if db.Table("pricing_prices").Where("id='price-edit-base'").Take(&raw).Error != nil || raw.Amount != in.Amount || !strings.Contains(raw.RawAmount, in.Amount) {
		t.Fatal("scalar and raw amount must agree")
	}
	public := gin.New()
	public.GET("/products/:id", (catalog.StoreHandler{DB: db, StoreID: "price-edit-store"}).Product)
	quote := func(expected string) {
		t.Helper()
		w := httptest.NewRecorder()
		public.ServeHTTP(w, httptest.NewRequest("GET", "/products/price-edit-product?region_id=price-edit-region", nil))
		var d struct{ Product catalog.StoreProduct }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &d) != nil || len(d.Product.Variants) != 2 {
			t.Fatalf("public edit quote HTTP %d", w.Code)
		}
		for _, v := range d.Product.Variants {
			if v.CalculatedPrice == nil || v.CalculatedPrice.Amount != expected {
				t.Fatal("public quote differs from edited price")
			}
		}
	}
	quote("0")
	altered := in
	altered.Amount = "1.000000000000000000"
	call("POST", path, body(altered), 409)
	call("POST", strings.Replace(path, "price-edit-v", "price-edit-shared", 1), body(in), 409)
	call("GET", strings.Replace(path, "price-edit-product", "other-product", 1), "", 404)
	next := catalog.PriceAmountInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Amount: "12345678901234567890.123456789012345678"}
	exec("CREATE TRIGGER price_edit_receipt_fault BEFORE INSERT ON catalog_price_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial price edit fault'")
	call("POST", path, body(next), 503)
	exec("DROP TRIGGER price_edit_receipt_fault")
	if now := read(); now.Amount != in.Amount || now.Revision != next.ExpectedRevision {
		t.Fatal("price edit late failure left change")
	}
	quote("0")
	call("POST", path, body(next), 200)
	quote(next.Amount)
	// Changes in eligibility must invalidate an open editor, even if amount did not change.
	stale := catalog.PriceAmountInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Amount: "101.000000000000000001"}
	exec("INSERT INTO pricing_price_rules(id,price_id,attribute,operator,value,priority,created_at,updated_at) VALUES('price-edit-rule','price-edit-base','region_id','eq','price-edit-region',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE pricing_prices SET rules_count=1 WHERE id='price-edit-base'")
	call("POST", path, body(stale), 409)
	fresh := stale
	fresh.ExpectedRevision = read().Revision
	call("POST", path, body(fresh), 200)
	quote(fresh.Amount)
	var count int64
	if db.Table("pricing_price_rules").Where("id='price-edit-rule' AND attribute='region_id' AND value='price-edit-region'").Count(&count).Error != nil || count != 1 {
		t.Fatal("price rules changed")
	}
	var role string
	if db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role).Error != nil || role == "" {
		t.Fatal("price edit grant fixture")
	}
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='price.write'", role)
	call("POST", path, body(fresh), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'price.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("POST", path, body(fresh), 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("UPDATE catalog_variant_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id='price-edit-v' AND price_set_id='price-edit-set'")
	call("POST", path, body(fresh), 404)
	exec("UPDATE catalog_variant_price_sets SET deleted_at=NULL WHERE variant_id='price-edit-v' AND price_set_id='price-edit-set'")
	call("POST", path, `{"amount":1}`, 400)
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	checkPriceCreate(t, db, actor, call)
	checkVariantCreate(t, db, actor, call)
	checkInventoryLevelCreate(t, db, actor, call)
	checkOrderDeviceAllocation(t, db, actor, call)
	checkOrderDevicePacking(t, db, actor, call)
	checkOrderDeviceParcels(t, db, actor, call)
	t.Log(`PRICE_AMOUNT_EDIT_REHEARSAL {"default_disabled":true,"explicit_price_write":true,"canonical_decimal_38_18":true,"raw_and_scalar_atomic":true,"concurrent_attempts":4,"single_mutation":true,"immutable_replay":true,"variant_receipt_binding":true,"stale_rule_context_denied":true,"rules_preserved":true,"late_receipt_failure_atomic":true,"shared_variants_public_quotes":true,"zero_price":true,"revoked_and_warehouse_replay_denied":true,"unlinked_replay_denied":true}`)
}

func TestPreparePriceEditBrowserFixture(t *testing.T) {
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
		t.Fatal(e)
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	seedPriceEdit(t, db)
	if os.Getenv("ORDER_ALLOCATION_BROWSER") == "1" {
		seedOrderDeviceAllocation(t, db)
		for _, q := range []string{"UPDATE device_runtime_schema SET identity_enabled=TRUE,commands_enabled=TRUE WHERE id=1", "UPDATE order_device_allocation_runtime SET enabled=TRUE WHERE id=1", "UPDATE payment_collections SET status='completed' WHERE id='allocation-payment'"} {
			if db.Exec(q).Error != nil {
				t.Fatal("allocation browser prep")
			}
		}
	}
	if os.Getenv("ORDER_PACKING_BROWSER") == "1" {
		seedOrderDevicePacking(t, db)
		if db.Exec("UPDATE order_device_packing_runtime SET enabled=TRUE WHERE id=1").Error != nil {
			t.Fatal("packing browser gate")
		}
	}
	if os.Getenv("ORDER_PARCELS_BROWSER") == "1" {
		if db.Exec("UPDATE order_device_parcel_runtime SET enabled=TRUE WHERE id=1").Error != nil {
			t.Fatal("parcel browser gate")
		}
	}
	if os.Getenv("VARIANT_CREATE_BROWSER") == "1" {
		seedVariantCreate(t, db)
		if db.Exec("UPDATE catalog_variant_create_runtime SET enabled=TRUE WHERE id=1").Error != nil {
			t.Fatal("variant gate")
		}
	}
	if os.Getenv("LEVEL_CREATE_BROWSER") == "1" {
		seedLevelCreateLocations(t, db)
		if db.Exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.create_level'").Error != nil {
			t.Fatal("level gate")
		}
	}
	if os.Getenv("PRICE_CREATE_BROWSER") == "1" && db.Exec(`INSERT INTO catalog_variants(id,product_id,title,sku,allow_backorder,manage_inventory,created_at,updated_at) VALUES('price-create-v','price-edit-product','Unpriced SKU','UNPRICED',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`).Error != nil {
		t.Fatal("unpriced browser fixture")
	}
	if db.Exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1").Error != nil {
		t.Fatal("price edit fixture enable")
	}
	t.Log("OWNED_PRICE_EDIT_READY")
}
