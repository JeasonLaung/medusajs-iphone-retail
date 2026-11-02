package catalog

import (
	"context"
	"encoding/json"
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

func verifyPriceQuantityEdit(t *testing.T, db *gorm.DB, actor identity.User) {
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
	var row struct{ ID string }
	if e := db.Table("pricing_prices p").Select("p.id").Joins("JOIN pricing_price_rules r ON r.price_id=p.id AND r.deleted_at IS NULL").Where("r.value='tier-region' AND p.deleted_at IS NULL").Take(&row).Error; e != nil {
		t.Fatal(e)
	}
	read := func() PriceEditContext {
		t.Helper()
		d, e := h.ReadPriceEdit(ctx, actor, p, v, row.ID)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	fingerprint := func() string {
		t.Helper()
		var price struct {
			ID, Amount, RawAmount, CurrencyCode, PriceSetID string
			PriceListID                                     *string
			RulesCount                                      *int
		}
		var rules []struct {
			ID, PriceID, Attribute, Operator, Value string
			Priority                                int
			UpdatedAt                               time.Time
		}
		if e := db.Table("pricing_prices").Where("id=?", row.ID).Take(&price).Error; e != nil {
			t.Fatal(e)
		}
		if e := db.Table("pricing_price_rules").Where("price_id=?", row.ID).Order("id").Scan(&rules).Error; e != nil || len(rules) != 1 {
			t.Fatal("rules", e)
		}
		b, _ := json.Marshal([]any{price, rules})
		return string(b)
	}
	original := fingerprint()
	before := read()
	min, max := "2", "3"
	in := PriceQuantityInput{uuid.NewString(), before.Revision, &min, &max}
	save := func(in PriceQuantityInput) (EditResult, error) {
		return h.EditPriceQuantity(ctx, actor, p, v, row.ID, in)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("quantity edit gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	exec("CREATE TRIGGER quantity_edit_failure BEFORE INSERT ON catalog_price_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial quantity edit receipt failure'")
	_, failed := save(in)
	exec("DROP TRIGGER quantity_edit_failure")
	if failed == nil || read().Revision != before.Revision || fingerprint() != original {
		t.Fatal("quantity edit rollback", failed)
	}
	results := make([]EditResult, 6)
	errs := make([]error, 6)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = save(in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.Revision != results[0].Revision {
			t.Fatal("quantity edit concurrent", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("quantity edit receipt", fresh)
	}
	after := read()
	if after.EffectiveMinQuantity == nil || *after.EffectiveMinQuantity != "2" || after.EffectiveMaxQuantity == nil || *after.EffectiveMaxQuantity != "3" || fingerprint() != original {
		t.Fatal("quantity edit preserved context")
	}
	quote := func(wants map[string]string) {
		t.Helper()
		for qty, want := range wants {
			if e := db.Transaction(func(tx *gorm.DB) error {
				qs, e := pricing.Read(tx, []string{v, "tier-price-v2"}, pricing.Context{Currency: "cad", Region: "tier-region", Quantity: qty}, time.Now().UTC())
				if e != nil {
					return e
				}
				for _, id := range []string{v, "tier-price-v2"} {
					if qs[id] == nil || qs[id].Amount != want {
						t.Fatalf("edited shared quote %s %s: %+v want %s", qty, id, qs[id], want)
					}
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
		}
	}
	quote(map[string]string{"2": "75", "3": "75", "4": "100", "5": "80.123456789012345678"})
	stale := in
	stale.OperationID = uuid.NewString()
	if _, e := save(stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale edit", e)
	}
	changed := in
	changed.MaxQuantity = nil
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed edit retry", e)
	}
	clear := PriceQuantityInput{uuid.NewString(), after.Revision, nil, nil}
	if _, e := save(clear); e != nil {
		t.Fatal("clear quantities", e)
	}
	var nulls int64
	if e := db.Table("pricing_prices").Where("id=? AND min_quantity IS NULL AND raw_min_quantity IS NULL AND max_quantity IS NULL AND raw_max_quantity IS NULL", row.ID).Count(&nulls).Error; e != nil || nulls != 1 || fingerprint() != original {
		t.Fatal("exact nulls", e)
	}
	quote(map[string]string{"1": "75", "4": "75"})
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(clear)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("revoked edit retry", denied)
	}
	exec("UPDATE catalog_variant_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id=?", v)
	if _, e := save(clear); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("unlinked edit retry", e)
	}
	exec("UPDATE catalog_variant_price_sets SET deleted_at=NULL WHERE variant_id=?", v)
	t.Log(`QUANTITY_EDIT_NATIVE {"default_closed":true,"late_receipt_rollback":true,"concurrent_attempts":6,"single_receipt":true,"price_id_amount_currency_rules_preserved":true,"shared_sku_boundary_quotes":true,"stale_rejected":true,"changed_retry_rejected":true,"clear_both_exact_sql_nulls":true,"cleared_quote":true,"revoked_replay_denied":true,"unlinked_replay_denied":true}`)
}
