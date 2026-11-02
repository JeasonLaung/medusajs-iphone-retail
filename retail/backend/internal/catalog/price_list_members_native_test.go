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

func verifyPriceListMembers(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := context.Background()
	h := Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	read := func() PriceCreateContext {
		t.Helper()
		d, e := h.ReadPriceCreate(ctx, actor, "tier-price-product", "tier-price-v")
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	save := func(in PriceCreateInput) (PriceCreateResult, error) {
		return h.CreateBasePrice(ctx, actor, "tier-price-product", "tier-price-v", in)
	}
	input := func(list, region, min, max, amount string) PriceCreateInput {
		return PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: read().Revision, CurrencyCode: "cad", Amount: amount, PriceListID: list, RegionID: region, MinQuantity: min, MaxQuantity: max}
	}
	exec("INSERT INTO pricing_price_lists(id,title,description,status,type,rules_count,created_at,updated_at) VALUES('member-list','Member fixture','','active','sale',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('member-other','Other fixture','','draft','sale',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES('member-rule','member-list','region_id',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", `"tier-region"`)
	in := input("member-list", "tier-region", "2", "3", "37.123456789012345678")
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("member gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	missing := input("missing-list", "", "", "", "20.000000000000000000")
	if _, e := save(missing); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("missing list", e)
	}
	original := read().Revision
	exec("UPDATE pricing_price_list_rules SET value=? WHERE id='member-rule'", `["tier-region"]`)
	if read().Revision == original {
		t.Fatal("list rule absent from revision")
	}
	if _, e := save(in); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale list rule", e)
	}
	exec("UPDATE pricing_price_list_rules SET value=? WHERE id='member-rule'", `"tier-region"`)
	in.ExpectedRevision = read().Revision
	exec("CREATE TRIGGER member_receipt_failure BEFORE INSERT ON catalog_price_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned member failure'")
	_, failed := save(in)
	exec("DROP TRIGGER member_receipt_failure")
	if failed == nil || read().Revision != in.ExpectedRevision {
		t.Fatal("member late rollback", failed)
	}
	var count int64
	db.Table("pricing_prices").Where("price_list_id='member-list'").Count(&count)
	if count != 0 {
		t.Fatal("member partial price", count)
	}
	var wg sync.WaitGroup
	results := make([]PriceCreateResult, 6)
	errs := make([]error, 6)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = save(in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.ID != results[0].ID {
			t.Fatal("member concurrent", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("member single receipt", fresh)
	}
	var row struct{ List, Region, Amount, Min, Max string }
	if e := db.Raw("SELECT p.price_list_id AS list,r.value AS region,JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value')) AS amount,JSON_UNQUOTE(JSON_EXTRACT(p.raw_min_quantity,'$.value')) AS min,JSON_UNQUOTE(JSON_EXTRACT(p.raw_max_quantity,'$.value')) AS max FROM pricing_prices p JOIN pricing_price_rules r ON r.price_id=p.id WHERE p.id=?", results[0].ID).Scan(&row).Error; e != nil || row.List != in.PriceListID || row.Region != in.RegionID || row.Amount != in.Amount || row.Min != "2" || row.Max != "3" {
		t.Fatal("member exact record", row, e)
	}
	db.Table("catalog_price_edits").Where("operation_id=?", in.OperationID).Count(&count)
	if count != 1 {
		t.Fatal("member receipt count", count)
	}
	duplicate := input(in.PriceListID, in.RegionID, in.MinQuantity, in.MaxQuantity, in.Amount)
	if _, e := save(duplicate); !errors.Is(e, ErrEditConflict) {
		t.Fatal("duplicate member", e)
	}
	changed := in
	changed.PriceListID = "member-other"
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed list retry", e)
	}
	// Same identity is valid in another list and in a draft list.
	if _, e := save(input("member-other", "tier-region", "2", "3", "10.000000000000000000")); e != nil {
		t.Fatal("other draft list", e)
	}
	quote := func(region, qty, want string) {
		t.Helper()
		e := db.Transaction(func(tx *gorm.DB) error {
			quotes, e := pricing.Read(tx, []string{"tier-price-v", "tier-price-v2"}, pricing.Context{Currency: "cad", Region: region, Quantity: qty}, time.Now().UTC())
			if e != nil {
				return e
			}
			for _, id := range []string{"tier-price-v", "tier-price-v2"} {
				if quotes[id] == nil || quotes[id].Amount != want {
					t.Fatalf("member quote %s/%s %s: %+v want %s", region, qty, id, quotes[id], want)
				}
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	quote("tier-region", "1", "75")
	quote("tier-region", "2", in.Amount)
	quote("tier-region", "3", in.Amount)
	quote("tier-region", "4", "75")
	quote("tier-other-region", "2", "90")
	exec("UPDATE pricing_price_lists SET starts_at='2099-01-01' WHERE id='member-list'")
	quote("tier-region", "2", "75")
	exec("UPDATE pricing_price_lists SET starts_at=NULL WHERE id='member-list'")
	// A non-tier member coexists with the existing regional base price.
	if _, e := save(input("member-list", "tier-region", "", "", "45.000000000000000000")); e != nil {
		t.Fatal("member regional base coexistence", e)
	}
	quote("tier-region", "1", "45")
	quote("tier-region", "2", in.Amount)
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(in)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("member revoke replay", denied)
	}
	exec("UPDATE pricing_price_lists SET deleted_at=UTC_TIMESTAMP(6) WHERE id='member-list'")
	if _, e := save(in); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("deleted member replay", e)
	}
	quote("tier-region", "2", "75")
	t.Log(`PRICE_LIST_MEMBER_NATIVE {"default_closed":true,"missing_list_denied":true,"list_rule_revision":true,"late_rollback":true,"concurrent_attempts":6,"single_price_and_receipt":true,"exact_amount_region_bounds":true,"duplicate_scoped_to_list":true,"changed_list_retry_denied":true,"draft_ignored":true,"scheduled_ignored":true,"shared_sku_quotes":true,"base_preserved":true,"revoked_replay_denied":true,"deleted_list_replay_denied":true}`)
}
