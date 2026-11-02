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

func verifyQuantityPrices(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := context.Background()
	h := Handler{DB: db}
	p, v := "tier-price-product", "tier-price-v"
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('cad','$','$',2,0,'0','Artificial dollar',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('tier-region','Tier fixture','cad',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('tier-other-region','Other tier fixture','cad',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at) VALUES(?,'Tier fixture',?,'draft',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", p, p)
	exec("INSERT INTO catalog_variants(id,product_id,title,allow_backorder,manage_inventory,created_at,updated_at) VALUES(?,?,'Tier fixture',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('tier-price-v2',?,'Shared tier',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", v, p, p)
	read := func() PriceCreateContext {
		t.Helper()
		d, e := h.ReadPriceCreate(ctx, actor, p, v)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	save := func(in PriceCreateInput) (PriceCreateResult, error) { return h.CreateBasePrice(ctx, actor, p, v, in) }
	input := func(min, max, amount, region string) PriceCreateInput {
		return PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, CurrencyCode: "cad", Amount: amount, RegionID: region, MinQuantity: min, MaxQuantity: max}
	}
	in := input("5", "10", "80.123456789012345678", "")
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("tier gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	if _, e := save(input("", "", "100.000000000000000000", "")); e != nil {
		t.Fatal("tier base", e)
	}
	in.ExpectedRevision = read().Revision
	exec("CREATE TRIGGER tier_receipt_failure BEFORE INSERT ON catalog_price_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial tier receipt failure'")
	_, failed := save(in)
	exec("DROP TRIGGER tier_receipt_failure")
	if failed == nil || read().Revision != in.ExpectedRevision {
		t.Fatal("tier rollback", failed)
	}
	results := make([]PriceCreateResult, 6)
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
			t.Fatal("tier concurrent", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("tier receipt", fresh)
	}
	var row struct{ Min, Max, RawMin, RawMax string }
	if e := db.Raw("SELECT min_quantity AS min,max_quantity AS max,JSON_UNQUOTE(JSON_EXTRACT(raw_min_quantity,'$.value')) AS raw_min,JSON_UNQUOTE(JSON_EXTRACT(raw_max_quantity,'$.value')) AS raw_max FROM pricing_prices WHERE id=?", results[0].ID).Scan(&row).Error; e != nil || row.Min != "5.000000000000000000" || row.Max != "10.000000000000000000" || row.RawMin != "5" || row.RawMax != "10" {
		t.Fatal("tier exact mirrors", row, e)
	}
	duplicate := input("5", "10", in.Amount, "")
	before := read().Revision
	if _, e := save(duplicate); !errors.Is(e, ErrEditConflict) || read().Revision != before {
		t.Fatal("duplicate tier", e)
	}
	changed := in
	changed.MaxQuantity = "11"
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed retry tier", e)
	}
	exec("INSERT INTO catalog_variant_price_sets(id,variant_id,price_set_id,created_at,updated_at) VALUES('tier-shared-link','tier-price-v2',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", *read().PriceSetID)
	quote := func(region string, wants map[string]string) {
		t.Helper()
		for qty, want := range wants {
			if e := db.Transaction(func(tx *gorm.DB) error {
				quotes, e := pricing.Read(tx, []string{v, "tier-price-v2"}, pricing.Context{Currency: "cad", Region: region, Quantity: qty}, time.Now().UTC())
				if e != nil {
					return e
				}
				for _, id := range []string{v, "tier-price-v2"} {
					if quotes[id] == nil || quotes[id].Amount != want {
						t.Fatalf("tier boundary %s/%s/%s: %+v want %s", region, qty, id, quotes[id], want)
					}
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
		}
	}
	quote("tier-region", map[string]string{"1": "100", "4": "100", "5": in.Amount, "10": in.Amount, "11": "100"})
	for _, in := range []PriceCreateInput{input("", "3", "90.000000000000000000", ""), input("11", "", "70.000000000000000000", ""), input("5", "10", "75.000000000000000000", "tier-region")} {
		in.ExpectedRevision = read().Revision
		if _, e := save(in); e != nil {
			t.Fatal("one-sided/regional tier", e)
		}
	}
	quote("tier-region", map[string]string{"1": "90", "3": "90", "4": "100", "5": "75", "10": "75", "11": "70"})
	quote("tier-other-region", map[string]string{"5": "80.123456789012345678", "10": "80.123456789012345678"})
	if _, e := save(input("6", "9", "60.000000000000000000", "")); e != nil {
		t.Fatal("source overlapping tiers", e)
	}
	quote("tier-other-region", map[string]string{"6": "60", "9": "60", "10": in.Amount})
	quote("tier-region", map[string]string{"6": "75"})
	large := input("18446744073709551615", "99999999999999999999", "50.000000000000000000", "")
	if _, e := save(large); e != nil {
		t.Fatal("large integer tier", e)
	}
	quote("tier-other-region", map[string]string{"18446744073709551615": "50"})
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(in)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("revoked tier retry", denied)
	}
	verifyPriceQuantityEdit(t, db, actor)
	t.Log(`QUANTITY_PRICE_NATIVE {"default_closed":true,"base_preserved":true,"late_receipt_rollback":true,"concurrent_attempts":6,"single_price":true,"exact_bound_mirrors":true,"duplicate_rejected":true,"changed_retry_rejected":true,"inclusive_boundaries":true,"one_sided_bounds":true,"region_specificity_preserved":true,"shared_sku_quotes":true,"source_overlap_selection":true,"large_integer_exact":true,"revoked_replay_denied":true}`)
}
