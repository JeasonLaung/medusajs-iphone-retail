package catalog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func verifyPriceListBatch(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := context.Background()
	h := Handler{DB: db}
	ids := []string{"tier-price-product", "list-create-product"}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	scope, e := h.ReadPriceListCreate(ctx, actor, ids)
	if e != nil {
		t.Fatal(e)
	}
	initial := []InitialListPrice{{ids[0], "tier-price-v", "cad", "40.000000000000000000", "tier-region", "", ""}, {ids[0], "tier-price-v", "cad", "15.000000000000000000", "tier-region", "2", "3"}, {ids[0], "tier-price-v", "cad", "16.000000000000000000", "tier-region", "4", "5"}, {ids[1], "list-create-variant", "cad", "21.000000000000000000", "tier-region", "", ""}}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	created, e := h.CreatePriceList(ctx, actor, PriceListCreateInput{PriceListInput: PriceListInput{OperationID: uuid.NewString(), ExpectedRevision: scope.Revision, Title: "Batch source fixture", Description: "Preserved batch settings", Status: "active", Type: "sale"}, ProductIDs: ids, Prices: initial, CustomerGroupIDs: []string{}})
	if e != nil {
		t.Fatal(e)
	}
	id := created.ID
	defer exec("UPDATE pricing_price_lists SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", id)
	read := func() PriceListCreateContext {
		t.Helper()
		c, e := h.ReadPriceListAdd(ctx, actor, id, ids)
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	priceID := func(amount string) string {
		t.Helper()
		var v string
		if e := db.Table("pricing_prices").Select("id").Where("price_list_id=? AND amount=? AND deleted_at IS NULL", id, amount).Scan(&v).Error; e != nil || v == "" {
			t.Fatal("find batch fixture price", e)
		}
		return v
	}

	emptyContext, e := h.ReadPriceListBatch(ctx, actor, id, []string{})
	if e != nil || len(emptyContext.Prices) != 0 {
		t.Fatal("empty batch selection", e)
	}
	batchContext, e := h.ReadPriceListBatch(ctx, actor, id, ids)
	if e != nil || len(batchContext.Prices) != 4 {
		t.Fatal("batch context deduplicated shared sets", e, len(batchContext.Prices))
	}
	seenContext := map[string]bool{}
	for _, p := range batchContext.Prices {
		if seenContext[p.ID] || len(p.Rules) != 1 || p.CurrencyCode != "cad" || p.Rules[0].Attribute != "region_id" || p.Rules[0].Value != "tier-region" {
			t.Fatal("batch current price rules", p)
		}
		seenContext[p.ID] = true
	}
	base, tierA, tierB, other := priceID("40"), priceID("15"), priceID("16"), priceID("21")
	in := PriceListBatchInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, ProductIDs: ids, Create: []InitialListPrice{{ids[0], "tier-price-v", "cad", "24.000000000000000000", "tier-region", "", ""}}, Update: []PriceListPriceUpdate{{PriceListPriceRef{tierA, ids[0], "tier-price-v"}, "12.123456789012345678", "4", "5"}, {PriceListPriceRef{tierB, ids[0], "tier-price-v"}, "13.000000000000000000", "2", "3"}, {PriceListPriceRef{other, ids[1], "list-create-variant"}, "17.750000000000000000", "", ""}}, Delete: []PriceListPriceRef{{base, ids[0], "tier-price-v"}}}
	save := func(body PriceListBatchInput) (PriceListCreateResult, error) {
		return h.BatchPriceListPrices(ctx, actor, id, body)
	}
	fingerprint := func() string {
		t.Helper()
		var v string
		if e := db.Raw("SELECT SHA2(CONCAT(title,description,status,type,COALESCE(metadata,''),created_at,updated_at),256) FROM pricing_price_lists WHERE id=?", id).Scan(&v).Error; e != nil || v == "" {
			t.Fatal(e)
		}
		return v
	}
	settings := fingerprint()
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("closed batch", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	wrong := in
	wrong.Update = append([]PriceListPriceUpdate{}, in.Update...)
	wrong.Update[0].ProductID = ids[1]
	if _, e := save(wrong); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("cross-product price denied", e)
	}
	exec("CREATE TRIGGER fail_native_batch BEFORE INSERT ON catalog_price_list_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned batch receipt failure'")
	_, failed := save(in)
	exec("DROP TRIGGER fail_native_batch")
	if failed == nil || read().Revision != in.ExpectedRevision || priceID("40") != base || priceID("15") != tierA || fingerprint() != settings {
		t.Fatal("mixed batch rollback", failed)
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
		t.Fatal("one fresh batch", fresh)
	}
	var count int64
	db.Table("catalog_price_list_edits").Where("operation_id=?", in.OperationID).Count(&count)
	if count != 1 {
		t.Fatal("one batch receipt", count)
	}
	db.Table("pricing_prices").Where("price_list_id=? AND deleted_at IS NULL", id).Count(&count)
	if count != 4 || fingerprint() != settings {
		t.Fatal("active replacement and settings", count)
	}
	var retained int64
	db.Raw("SELECT COUNT(*) FROM pricing_prices p JOIN pricing_price_rules r ON r.price_id=p.id WHERE p.id=? AND p.deleted_at IS NOT NULL AND r.deleted_at=p.deleted_at", base).Scan(&retained)
	if retained != 1 {
		t.Fatal("retained deleted price/rule", retained)
	}
	var raw string
	db.Raw("SELECT JSON_UNQUOTE(JSON_EXTRACT(raw_amount,'$.value')) FROM pricing_prices WHERE id=?", tierA).Scan(&raw)
	if raw != in.Update[0].Amount || priceID("17.75") != other {
		t.Fatal("preserve updated IDs and raw precision", raw)
	}
	for _, q := range []struct{ variant, quantity, want string }{{"tier-price-v2", "1", "24"}, {"tier-price-v2", "2", "13"}, {"tier-price-v2", "3", "13"}, {"tier-price-v2", "4", in.Update[0].Amount}, {"tier-price-v2", "5", in.Update[0].Amount}, {"tier-price-v2", "6", "24"}, {"list-create-variant", "1", "17.75"}} {
		if e := db.Transaction(func(tx *gorm.DB) error {
			quotes, e := pricing.Read(tx, []string{q.variant}, pricing.Context{Currency: "cad", Region: "tier-region", Quantity: q.quantity}, time.Now().UTC())
			if e != nil {
				return e
			}
			if quotes[q.variant] == nil || quotes[q.variant].Amount != q.want {
				t.Fatalf("batch quote %s/%s got %+v want %s", q.variant, q.quantity, quotes[q.variant], q.want)
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = read().Revision
	duplicate.Delete = []PriceListPriceRef{}
	duplicate.Create = []InitialListPrice{}
	duplicate.Update = []PriceListPriceUpdate{{PriceListPriceRef{tierA, ids[0], "tier-price-v"}, "11.000000000000000000", "2", "3"}}
	before := read().Revision
	if _, e := save(duplicate); !errors.Is(e, ErrEditConflict) || read().Revision != before {
		t.Fatal("final duplicate full rollback", e)
	}

	// Imported duplicate conditions are not rewritten or made uneditable merely
	// because an amount changes. This command must not invent a new uniqueness
	// migration for existing historical facts.
	exec("INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,price_list_id,rules_count,created_at,updated_at) SELECT 'batch-historical-duplicate',price_set_id,currency_code,amount,raw_amount,price_list_id,rules_count,created_at,updated_at FROM pricing_prices WHERE id=?", other)
	exec("INSERT INTO pricing_price_rules(id,price_id,attribute,operator,value,priority,created_at,updated_at) SELECT 'batch-historical-rule','batch-historical-duplicate',attribute,operator,value,priority,created_at,updated_at FROM pricing_price_rules WHERE price_id=? AND deleted_at IS NULL", other)
	amountOnly := PriceListBatchInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, ProductIDs: ids, Create: []InitialListPrice{}, Update: []PriceListPriceUpdate{{PriceListPriceRef{other, ids[1], "list-create-variant"}, "17.500000000000000000", "", ""}}, Delete: []PriceListPriceRef{}}
	if _, e := save(amountOnly); e != nil {
		t.Fatal("preserve existing duplicate amount edit", e)
	}
	cross := in
	cross.OperationID = created.ID // Not a UUID; normal input rejection remains strict.
	if _, e := save(cross); !errors.Is(e, ErrPriceInput) {
		t.Fatal("invalid operation identity", e)
	}
	stale := duplicate
	stale.ExpectedRevision = in.ExpectedRevision
	if _, e := save(stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale batch", e)
	}
	changed := in
	changed.Update = append([]PriceListPriceUpdate{}, in.Update...)
	changed.Update[0].Amount = "11.000000000000000000"
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed replay", e)
	}
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(in)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("revoked batch replay", denied)
	}
	exec("UPDATE catalog_variants SET deleted_at=UTC_TIMESTAMP(6) WHERE id='tier-price-v'")
	_, deleted := save(in)
	exec("UPDATE catalog_variants SET deleted_at=NULL WHERE id='tier-price-v'")
	if !errors.Is(deleted, gorm.ErrRecordNotFound) {
		t.Fatal("deleted SKU replay", deleted)
	}
	exec("UPDATE pricing_price_lists SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", id)
	if _, e := save(in); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("deleted list replay", e)
	}
	t.Log(`PRICE_LIST_BATCH_NATIVE {"mixed_create_update_delete":true,"read_current_shared_prices_rules":true,"default_closed":true,"late_full_rollback":true,"concurrent_attempts":6,"one_receipt":true,"same_condition_replacement":true,"bounds_swap":true,"exact_raw_and_updated_ids":true,"deleted_rule_retained":true,"list_settings_preserved":true,"shared_sku_boundary_quotes":true,"final_duplicate_rollback":true,"historical_duplicate_amount_edit":true,"stale_changed_membership_revoked_deleted_denied":true}`)
}
