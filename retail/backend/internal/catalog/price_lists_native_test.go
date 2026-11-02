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

func verifyPriceLists(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := context.Background()
	h := Handler{DB: db}
	id := "native-price-list"
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO pricing_price_lists(id,title,description,status,type,starts_at,rules_count,metadata,created_at,updated_at) VALUES(?,'Original list','Original description','draft','sale','2000-01-01 00:00:00.123456',1,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, `{"exact":9007199254740993}`)
	exec("INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES('native-list-rule',?,'region_id',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, `"tier-region"`)
	var set string
	if e := db.Table("catalog_variant_price_sets").Select("price_set_id").Where("variant_id='tier-price-v' AND deleted_at IS NULL").Scan(&set).Error; e != nil || set == "" {
		t.Fatal(e)
	}
	exec("INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,price_list_id,created_at,updated_at) VALUES('native-list-price',?,'cad',42,?,0,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", set, `{"value":"42.123456789012345678","precision":20}`, id)
	read := func() PriceListDetail {
		t.Helper()
		d, e := h.ReadPriceList(ctx, actor, id)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	input := func() PriceListInput {
		d := read()
		var start *string
		if d.StartsAt != nil {
			value := d.StartsAt.Format(time.RFC3339Nano)
			start = &value
		}
		return PriceListInput{uuid.NewString(), d.Revision, "Edited list", "Edited description", "active", "sale", start, nil}
	}
	save := func(in PriceListInput) (EditResult, error) { return h.EditPriceList(ctx, actor, id, in) }
	fingerprint := func() string {
		t.Helper()
		var s string
		e := db.Raw("SELECT SHA2(CONCAT(COALESCE(l.metadata,''),r.id,r.attribute,r.value,r.updated_at,p.id,p.amount,p.raw_amount,p.price_set_id,p.rules_count,p.updated_at),256) FROM pricing_price_lists l JOIN pricing_price_list_rules r ON r.price_list_id=l.id JOIN pricing_prices p ON p.price_list_id=l.id WHERE l.id=?", id).Scan(&s).Error
		if e != nil || s == "" {
			t.Fatal(e)
		}
		return s
	}
	original := fingerprint()
	in := input()
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("closed price-list gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	exec("CREATE TRIGGER fail_native_list_receipt BEFORE INSERT ON catalog_price_list_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned late list receipt failure'")
	if _, e := save(in); e == nil {
		t.Fatal("late receipt failure missing")
	}
	if read().Revision != in.ExpectedRevision || fingerprint() != original {
		t.Fatal("list transaction did not roll back")
	}
	exec("DROP TRIGGER fail_native_list_receipt")
	var wg sync.WaitGroup
	var mu sync.Mutex
	fresh := 0
	revisions := map[string]bool{}
	errs := []error{}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := save(in)
			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				errs = append(errs, e)
				return
			}
			if !r.Replayed {
				fresh++
			}
			revisions[r.Revision] = true
		}()
	}
	wg.Wait()
	if fresh != 1 || len(revisions) != 1 || len(errs) > 0 {
		t.Fatal("list concurrent receipt", fresh, revisions, errs)
	}
	if fingerprint() != original || read().StartsAt.Format(time.RFC3339Nano) != "2000-01-01T00:00:00.123456Z" {
		t.Fatal("rules, metadata, exact price or microseconds changed")
	}
	quote := func(now time.Time, region, want string) {
		t.Helper()
		e := db.Transaction(func(tx *gorm.DB) error {
			q, e := pricing.Read(tx, []string{"tier-price-v", "tier-price-v2"}, pricing.Context{Currency: "cad", Region: region, Quantity: "1"}, now)
			if e != nil {
				return e
			}
			for _, id := range []string{"tier-price-v", "tier-price-v2"} {
				if q[id] == nil || q[id].Amount != want {
					t.Fatalf("list quote %s/%s: %+v want %s", region, id, q[id], want)
				}
			}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
	}
	quote(time.Now().UTC(), "tier-region", "42.123456789012345678")
	quote(time.Now().UTC(), "tier-other-region", "90")
	stale := in
	stale.OperationID = uuid.NewString()
	if _, e := save(stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale list", e)
	}
	changed := in
	changed.Status = "draft"
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed list retry", e)
	}
	draft := input()
	draft.Status = "draft"
	if _, e := save(draft); e != nil {
		t.Fatal(e)
	}
	quote(time.Now().UTC(), "tier-region", "75")
	future := "2099-01-01T00:00:00.123456Z"
	scheduled := input()
	scheduled.StartsAt = &future
	if _, e := save(scheduled); e != nil {
		t.Fatal(e)
	}
	quote(time.Now().UTC(), "tier-region", "75")
	boundary, _ := time.Parse(time.RFC3339Nano, future)
	quote(boundary, "tier-region", "42.123456789012345678")
	ended := input()
	ended.StartsAt = nil
	past := "2001-01-01T00:00:00.123456Z"
	ended.EndsAt = &past
	if _, e := save(ended); e != nil {
		t.Fatal(e)
	}
	quote(time.Now().UTC(), "tier-region", "75")
	end, _ := time.Parse(time.RFC3339Nano, past)
	quote(end, "tier-region", "42.123456789012345678")
	quote(end.Add(time.Microsecond), "tier-region", "75")
	// Source sale keeps the lower base, while override replaces it even if higher.
	exec("UPDATE pricing_prices SET amount=200,raw_amount=? WHERE id='native-list-price'", `{"value":"200.123456789012345678","precision":20}`)
	sale := input()
	sale.StartsAt = nil
	sale.EndsAt = nil
	if _, e := save(sale); e != nil {
		t.Fatal(e)
	}
	quote(time.Now().UTC(), "tier-region", "75")
	override := input()
	override.StartsAt = nil
	override.Type = "override"
	if _, e := save(override); e != nil {
		t.Fatal(e)
	}
	quote(time.Now().UTC(), "tier-region", "200.123456789012345678")
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(override)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("revoked list replay", denied)
	}
	verifyPriceListRemoval(t, db, actor, override)
	if _, e := save(override); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("deleted list replay", e)
	}
	t.Log(`PRICE_LIST_NATIVE {"default_closed":true,"concurrent_attempts":6,"unique_receipt":true,"late_failure_rollback":true,"metadata_rules_prices_preserved":true,"microseconds_preserved":true,"shared_sku_quotes":true,"region_rule_preserved":true,"draft_scheduled_expired":true,"inclusive_time_boundaries":true,"sale_override_semantics":true,"stale_changed_retry_denied":true,"revoked_deleted_replay_denied":true}`)
}

func seedPriceListBrowser(t *testing.T, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at) VALUES('list-browser-product','Price list browser product','list-browser-product','published',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES('list-browser-channel','list-browser-product','option-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variants(id,product_id,title,allow_backorder,manage_inventory,created_at,updated_at) VALUES('list-browser-variant','list-browser-product','Default',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('list-browser-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variant_price_sets(id,variant_id,price_set_id,created_at,updated_at) VALUES('list-browser-link','list-browser-variant','list-browser-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO pricing_price_lists(id,title,description,status,type,starts_at,rules_count,metadata,created_at,updated_at) VALUES('list-browser-sale','Browser list draft','Original browser description','draft','sale','2000-01-01 00:00:00.123456',1,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", `{"exact":9007199254740993}`)
	exec("INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES('list-browser-rule','list-browser-sale','sales_channel_id',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", `"option-channel"`)
	exec("INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,price_list_id,created_at,updated_at) VALUES('list-browser-base','list-browser-set','eur',100,?,0,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('list-browser-price','list-browser-set','eur',42.75,?,0,'list-browser-sale',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", `{"value":"100","precision":20}`, `{"value":"42.75","precision":20}`)
	exec("INSERT INTO pricing_price_rules(id,price_id,attribute,operator,value,priority,created_at,updated_at) VALUES('list-browser-price-rule','list-browser-price','region_id','eq','option-region',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE pricing_prices SET rules_count=1 WHERE id='list-browser-price'")
	exec("INSERT INTO pricing_price_lists(id,title,description,status,type,rules_count,created_at,updated_at) VALUES('list-browser-member','Browser member list','','active','sale',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES('list-browser-member-rule','list-browser-member','sales_channel_id',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", `"option-channel"`)

}
