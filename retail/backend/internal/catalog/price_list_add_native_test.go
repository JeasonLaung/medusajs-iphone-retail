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

func verifyPriceListAdd(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := context.Background()
	h := Handler{DB: db}
	id := "native-list-add"
	ids := []string{"tier-price-product", "list-create-product"}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO pricing_price_lists(id,title,description,status,type,starts_at,rules_count,metadata,created_at,updated_at) VALUES(?,'Existing add fixture','Keep details','active','sale','2000-01-01 00:00:00.123456',1,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, `{"exact":9007199254740993}`)
	exec("INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES('native-add-rule',?,'region_id',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, `"tier-region"`)
	var set string
	db.Table("catalog_variant_price_sets").Select("price_set_id").Where("variant_id='tier-price-v' AND deleted_at IS NULL").Scan(&set)
	exec("INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,price_list_id,created_at,updated_at) VALUES('native-add-existing',?,'cad',40,?,0,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", set, `{"value":"40.123456789012345678","precision":20}`, id)
	read := func() PriceListCreateContext {
		t.Helper()
		out, e := h.ReadPriceListAdd(ctx, actor, id, ids)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	fingerprint := func() string {
		t.Helper()
		var out string
		e := db.Raw("SELECT SHA2(CONCAT(l.title,l.description,l.status,l.type,l.starts_at,l.metadata,l.updated_at,r.attribute,r.value,r.updated_at,p.amount,p.raw_amount,p.updated_at),256) FROM pricing_price_lists l JOIN pricing_price_list_rules r ON r.price_list_id=l.id JOIN pricing_prices p ON p.price_list_id=l.id WHERE l.id=? AND p.id='native-add-existing'", id).Scan(&out).Error
		if e != nil || out == "" {
			t.Fatal(e)
		}
		return out
	}
	in := PriceListAddInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, ProductIDs: ids, Prices: []InitialListPrice{{ProductID: ids[0], VariantID: "tier-price-v", CurrencyCode: "cad", RegionID: "tier-region", Amount: "15.123456789012345678", MinQuantity: "2", MaxQuantity: "3"}, {ProductID: ids[1], VariantID: "list-create-variant", CurrencyCode: "cad", RegionID: "tier-region", Amount: "21.000000000000000000"}}}
	save := func(in PriceListAddInput) (PriceListCreateResult, error) {
		return h.AddPriceListPrices(ctx, actor, id, in)
	}
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("batch gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	exec("UPDATE pricing_price_list_rules SET value=? WHERE id='native-add-rule'", `["tier-region"]`)
	if _, e := save(in); !errors.Is(e, ErrEditConflict) {
		t.Fatal("batch list rule stale", e)
	}
	exec("UPDATE pricing_price_list_rules SET value=? WHERE id='native-add-rule'", `"tier-region"`)
	in.ExpectedRevision = read().Revision
	before := fingerprint()
	exec("CREATE TRIGGER fail_native_list_add BEFORE INSERT ON catalog_price_list_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned list add receipt failure'")
	_, failed := save(in)
	exec("DROP TRIGGER fail_native_list_add")
	if failed == nil || read().Revision != in.ExpectedRevision || fingerprint() != before {
		t.Fatal("batch late rollback", failed)
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
		if errs[i] != nil || r.ID != id {
			t.Fatal("batch concurrency", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("batch one receipt", fresh)
	}
	var count int64
	db.Table("catalog_price_list_edits").Where("operation_id=?", in.OperationID).Count(&count)
	if count != 1 {
		t.Fatal("batch unique receipt", count)
	}
	db.Table("pricing_prices").Where("price_list_id=? AND deleted_at IS NULL", id).Count(&count)
	if count != 3 || fingerprint() != before {
		t.Fatal("batch preserves list and old price", count)
	}
	var exact string
	db.Raw("SELECT JSON_UNQUOTE(JSON_EXTRACT(raw_amount,'$.value')) FROM pricing_prices WHERE price_list_id=? AND min_quantity=2", id).Scan(&exact)
	if exact != in.Prices[0].Amount {
		t.Fatal("batch exact amount", exact)
	}
	quote := func(v, qty, want string) {
		t.Helper()
		e := db.Transaction(func(tx *gorm.DB) error {
			quotes, e := pricing.Read(tx, []string{v}, pricing.Context{Currency: "cad", Region: "tier-region", Quantity: qty}, time.Now().UTC())
			if e != nil {
				return e
			}
			if quotes[v] == nil || quotes[v].Amount != want {
				t.Fatalf("batch quote %s/%s %+v want %s", v, qty, quotes[v], want)
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	quote("tier-price-v2", "2", in.Prices[0].Amount)
	quote("tier-price-v2", "4", "40.123456789012345678")
	quote("list-create-variant", "1", "21")
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = read().Revision
	duplicate.Prices = []InitialListPrice{{ProductID: ids[0], VariantID: "tier-price-v", CurrencyCode: "cad", Amount: "11.000000000000000000", MinQuantity: "4", MaxQuantity: "5"}, in.Prices[0]}
	revision := read().Revision
	if _, e := save(duplicate); !errors.Is(e, ErrEditConflict) || read().Revision != revision {
		t.Fatal("late duplicate all-or-none", e)
	}
	changed := in
	changed.Prices = append([]InitialListPrice{}, in.Prices...)
	changed.Prices[0].Amount = "14.000000000000000000"
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed batch replay", e)
	}
	cross := PriceListInput{OperationID: in.OperationID, ExpectedRevision: read().Revision, Title: "Forged", Description: "Forged", Status: "active", Type: "sale"}
	if _, e := h.EditPriceList(ctx, actor, id, cross); !errors.Is(e, ErrEditConflict) {
		t.Fatal("cross-action batch replay", e)
	}
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(in)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("batch revoke", denied)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("batch closed replay", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE pricing_price_lists SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", id)
	if _, e := save(in); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("batch deleted replay", e)
	}
	t.Log(`PRICE_LIST_ADD_NATIVE {"default_closed":true,"list_rule_revision":true,"late_receipt_rollback":true,"concurrent_attempts":6,"unique_batch_receipt":true,"two_product_prices":true,"exact_raw_amount":true,"settings_rules_metadata_old_price_preserved":true,"shared_sku_boundary_quotes":true,"late_duplicate_full_rollback":true,"changed_cross_action_denied":true,"current_permission_gate_deleted_rechecked":true}`)
}
