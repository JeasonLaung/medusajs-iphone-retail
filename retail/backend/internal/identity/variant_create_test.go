package identity_test

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedVariantCreate(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('variant-create-product','iPhone fixture','variant-create-product',FALSE,'published',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES('variant-create-channel','variant-create-product','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('vc-storage','Storage',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-color','Color',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES('vc-128','128 GB','vc-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-256','256 GB','vc-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-black','Black','vc-color',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-white','White','vc-color',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_product_options(id,product_id,product_option_id,created_at,updated_at) VALUES('vc-po-storage','variant-create-product','vc-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-po-color','variant-create-product','vc-color',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES('vc-pv-128','vc-po-storage','vc-128',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-pv-256','vc-po-storage','vc-256',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-pv-black','vc-po-color','vc-black',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('vc-pv-white','vc-po-color','vc-white',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	} {
		if e := db.Exec(q).Error; e != nil {
			t.Fatal("SKU create fixture", e)
		}
	}
}
func checkVariantCreate(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	seedVariantCreate(t, db)
	h := catalog.Handler{DB: db}
	ctx := t.Context()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	const path = "/api/admin/products/variant-create-product"
	read := func() catalog.VariantCreateContext {
		t.Helper()
		var d catalog.VariantCreateContext
		if json.Unmarshal(call("GET", path+"/variant-creation", "", 200), &d) != nil {
			t.Fatal("context")
		}
		return d
	}
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	old := read()
	if old.Enabled || len(old.Options) != 2 {
		t.Fatal("default gate/options")
	}
	in := catalog.VariantCreateInput{OperationID: uuid.NewString(), ExpectedRevision: old.Revision, Title: "128 GB / Black", SKU: "", OptionValueIDs: []string{"vc-128", "vc-black"}}
	call("POST", path+"/variants", body(in), 503)
	exec("UPDATE catalog_variant_create_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_variant_create_runtime SET enabled=FALSE WHERE id=1")
	exec("CREATE TRIGGER variant_creation_fault BEFORE INSERT ON catalog_variant_creations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial variant create fault'")
	call("POST", path+"/variants", body(in), 503)
	exec("DROP TRIGGER variant_creation_fault")
	var n int64
	db.Table("catalog_variants").Where("product_id='variant-create-product'").Count(&n)
	if n != 0 || read().Revision != old.Revision {
		t.Fatal("variant rollback")
	}
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	results := make(chan catalog.VariantCreateResult, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.CreateVariant(ctx, actor, "variant-create-product", in)
			if e != nil {
				failed.Store(true)
				return
			}
			results <- out
			if out.Replayed {
				replays.Add(1)
			} else {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	close(results)
	if failed.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("concurrent variant creation")
	}
	var first catalog.VariantCreateResult
	for r := range results {
		if first.ID == "" {
			first = r
		}
		if r.ID != first.ID || r.InventoryItemID != first.InventoryItemID || r.SKU != first.SKU {
			t.Fatal("replay identity")
		}
	}
	if reserved := first.SKU; reserved == "" {
		t.Fatal("ERP missing")
	}
	var facts struct {
		SKU, Title, RequiredQuantity                      string
		ManageInventory, AllowBackorder, RequiresShipping bool
	}
	if e := db.Raw("SELECT i.sku,i.title,vi.required_quantity,v.manage_inventory,v.allow_backorder,i.requires_shipping FROM catalog_variants v JOIN catalog_variant_inventory_items vi ON vi.variant_id=v.id JOIN inventory_items i ON i.id=vi.inventory_item_id WHERE v.id=?", first.ID).Scan(&facts).Error; e != nil || facts.SKU != first.SKU || facts.Title != "iPhone fixture / 128 GB / Black" || facts.RequiredQuantity != "1.000000000000000000" || !facts.ManageInventory || facts.AllowBackorder || !facts.RequiresShipping {
		t.Fatal("created inventory facts", facts, e)
	}
	db.Table("inventory_levels").Where("inventory_item_id=?", first.InventoryItemID).Count(&n)
	if n != 0 {
		t.Fatal("invented stock")
	}
	db.Table("catalog_variant_price_sets").Where("variant_id=?", first.ID).Count(&n)
	if n != 0 {
		t.Fatal("invented price")
	}
	// Duplicate combination, reused receipt with different content, partial and foreign options.
	fresh := in
	fresh.OperationID = uuid.NewString()
	fresh.ExpectedRevision = read().Revision
	fresh.SKU = "CUSTOM-UNIQUE"
	call("POST", path+"/variants", body(fresh), 409)
	changed := in
	changed.Title = "changed"
	call("POST", path+"/variants", body(changed), 409)
	fresh.OptionValueIDs = []string{"vc-256"}
	call("POST", path+"/variants", body(fresh), 400)
	fresh.OptionValueIDs = []string{"vc-128", "foreign"}
	call("POST", path+"/variants", body(fresh), 400)
	fresh.OptionValueIDs = []string{"vc-128", "vc-256"}
	call("POST", path+"/variants", body(fresh), 400)
	fresh.OptionValueIDs = []string{"vc-256", "vc-black"}
	fresh.SKU = first.SKU
	call("POST", path+"/variants", body(fresh), 409)
	// Explicit ERP codes advance numbering, and deleted imported codes stay reserved.
	fresh.SKU = "ERP900000"
	var custom catalog.VariantCreateResult
	json.Unmarshal(call("POST", path+"/variants", body(fresh), 200), &custom)
	if custom.SKU != "ERP900000" {
		t.Fatal("custom SKU lost")
	}
	next := fresh
	next.OperationID = uuid.NewString()
	next.ExpectedRevision = read().Revision
	next.SKU = ""
	next.OptionValueIDs = []string{"vc-128", "vc-white"}
	exec("UPDATE catalog_variants SET sku='ERP950000',deleted_at=UTC_TIMESTAMP(6) WHERE id=?", custom.ID)
	// Deletion changed the relation revision; an open create form must refresh.
	call("POST", path+"/variants", body(next), 409)
	next.ExpectedRevision = read().Revision
	var numbered catalog.VariantCreateResult
	json.Unmarshal(call("POST", path+"/variants", body(next), 200), &numbered)
	if numbered.SKU != "ERP950001" {
		t.Fatal("imported/deleted ERP high water", numbered.SKU)
	}
	// Current rights and live membership are required even for a receipt replay.
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='product.write'", role)
	call("POST", path+"/variants", body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'product.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("POST", path+"/variants", body(in), 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	exec("UPDATE catalog_variants SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", first.ID)
	call("POST", path+"/variants", body(in), 404)
	exec("UPDATE catalog_variants SET deleted_at=NULL WHERE id=?", first.ID)
	call("POST", "/api/admin/products/price-edit-product/variants", body(in), 409)
	if _, e := h.CreateVariant(ctx, actor, "missing-product", in); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("missing product")
	}
	t.Log(`VARIANT_CREATE_REHEARSAL {"default_disabled":true,"complete_owned_options":true,"global_sku_unique":true,"duplicate_combination_denied":true,"concurrent_attempts":4,"one_variant_inventory_receipt":true,"zero_stock_no_price":true,"erp_custom_and_imported_deleted_high_water":true,"immutable_replay":true,"late_receipt_failure_atomic":true,"current_rights_rechecked":true,"deleted_receipt_target_denied":true}`)
}
