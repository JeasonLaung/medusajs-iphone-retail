package catalog

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

func verifyPriceListCreate(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := context.Background()
	h := Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at) VALUES('list-create-product','List create fixture','list-create-fixture','draft',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variants(id,product_id,title,manage_inventory,allow_backorder,created_at,updated_at) VALUES('list-create-variant','list-create-product','Fresh SKU',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	ids := []string{"tier-price-product", "list-create-product"}
	read := func() PriceListCreateContext {
		t.Helper()
		out, e := h.ReadPriceListCreate(ctx, actor, ids)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	in := PriceListCreateInput{PriceListInput: PriceListInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, Title: "Native initial list", Description: "Native two products three prices", Status: "active", Type: "sale"}, ProductIDs: ids, CustomerGroupIDs: []string{}, Prices: []InitialListPrice{{ProductID: ids[0], VariantID: "tier-price-v", CurrencyCode: "cad", Amount: "30.000000000000000000", RegionID: "tier-region"}, {ProductID: ids[0], VariantID: "tier-price-v", CurrencyCode: "cad", Amount: "20.123456789012345678", RegionID: "tier-region", MinQuantity: "2", MaxQuantity: "3"}, {ProductID: ids[1], VariantID: "list-create-variant", CurrencyCode: "cad", Amount: "55.000000000000000000"}}}
	save := func(in PriceListCreateInput) (PriceListCreateResult, error) { return h.CreatePriceList(ctx, actor, in) }
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("create list gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	stale := in
	exec("UPDATE catalog_products SET title='Changed create fixture' WHERE id='list-create-product'")
	if _, e := save(stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("create product revision", e)
	}
	in.ExpectedRevision = read().Revision
	exec("CREATE TRIGGER fail_list_create_receipt BEFORE INSERT ON catalog_price_list_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned list create receipt failure'")
	_, failed := save(in)
	exec("DROP TRIGGER fail_list_create_receipt")
	if failed == nil || read().Revision != in.ExpectedRevision {
		t.Fatal("full create rollback", failed)
	}
	var count int64
	db.Table("catalog_variant_price_sets").Where("variant_id='list-create-variant'").Count(&count)
	if count != 0 {
		t.Fatal("rolled back new set membership", count)
	}
	results := make([]PriceListCreateResult, 6)
	errs := make([]error, 6)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = save(in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.ID != results[0].ID {
			t.Fatal("create concurrency", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("create single list", fresh)
	}
	id := results[0].ID
	detail, e := h.ReadPriceList(ctx, actor, id)
	if e != nil || detail.PriceCount != 3 || detail.Title != in.Title {
		t.Fatal("created list detail", detail, e)
	}
	db.Table("catalog_price_list_edits").Where("operation_id=?", in.OperationID).Count(&count)
	if count != 1 {
		t.Fatal("one creation receipt", count)
	}
	var exact string
	if e := db.Raw("SELECT JSON_UNQUOTE(JSON_EXTRACT(raw_amount,'$.value')) FROM pricing_prices WHERE price_list_id=? AND min_quantity=2", id).Scan(&exact).Error; e != nil || exact != "20.123456789012345678" {
		t.Fatal("exact initial amount", exact, e)
	}
	quote := func(variant, quantity, want string, groups []string) {
		t.Helper()
		e := db.Transaction(func(tx *gorm.DB) error {
			quotes, e := pricing.Read(tx, []string{variant}, pricing.Context{Currency: "cad", Region: "tier-region", Quantity: quantity, CustomerGroups: groups}, time.Now().UTC())
			if e != nil {
				return e
			}
			if quotes[variant] == nil || quotes[variant].Amount != want {
				t.Fatalf("create quote %s/%s: %+v want %s", variant, quantity, quotes[variant], want)
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	quote("tier-price-v", "1", "30", nil)
	quote("tier-price-v2", "2", in.Prices[1].Amount, nil)
	quote("tier-price-v2", "4", "30", nil)
	quote("list-create-variant", "1", "55", nil)
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = read().Revision
	duplicate.Prices = append(append([]InitialListPrice{}, in.Prices...), InitialListPrice{ProductID: ids[0], VariantID: "tier-price-v2", CurrencyCode: "cad", Amount: "29.000000000000000000", RegionID: "tier-region"})
	before := read().Revision
	if _, e := save(duplicate); !errors.Is(e, ErrEditConflict) || read().Revision != before {
		t.Fatal("shared duplicate rollback", e)
	}
	changed := in
	changed.Title = "Changed replay"
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed create replay", e)
	}
	group := in
	group.OperationID = uuid.NewString()
	group.ExpectedRevision = read().Revision
	group.Title = "Group fixture"
	group.CustomerGroupIDs = []string{"native-vip"}
	group.Prices = []InitialListPrice{{ProductID: ids[0], VariantID: "tier-price-v", CurrencyCode: "cad", Amount: "12.000000000000000000", RegionID: "tier-region"}}
	if _, e := save(group); e != nil {
		t.Fatal("group initial price", e)
	}
	quote("tier-price-v", "1", "30", nil)
	quote("tier-price-v", "1", "12", []string{"native-vip"})
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(in)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("revoked creation replay", denied)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("closed replay", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE pricing_price_lists SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", id)
	if _, e := save(in); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("deleted creation replay", e)
	}
	t.Log(`PRICE_LIST_CREATE_NATIVE {"default_closed":true,"selected_product_revision":true,"late_receipt_full_rollback":true,"new_price_set_rollback":true,"concurrent_attempts":6,"single_list_and_receipt":true,"two_products_three_prices":true,"exact_initial_amount":true,"shared_sku_boundary_quotes":true,"shared_duplicate_batch_rollback":true,"changed_replay_denied":true,"group_rule_anonymous_excluded":true,"current_group_context_matches":true,"revoked_closed_deleted_replay_denied":true}`)
}
