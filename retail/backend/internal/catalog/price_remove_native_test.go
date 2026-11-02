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

// Runs on the preceding region fixture in the owned native database only.
func verifyPriceRemoval(t *testing.T, db *gorm.DB, actor identity.User, priceID string) {
	t.Helper()
	ctx := context.Background()
	h := Handler{DB: db}
	p, v := "regional-price-product", "regional-price-v"
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	read := func() PriceEditContext {
		t.Helper()
		d, e := h.ReadPriceEdit(ctx, actor, p, v, priceID)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	before := read()
	if before.SharedVariants != 2 {
		t.Fatal("missing shared fixture")
	}
	in := PriceRemoveInput{uuid.NewString(), before.Revision}
	save := func(in PriceRemoveInput) (EditResult, error) { return h.RemovePrice(ctx, actor, p, v, priceID, in) }
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("removal gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	stale := in
	stale.ExpectedRevision = uuid.NewString() + uuid.NewString() // invalid shape is not a version conflict.
	if _, e := save(stale); !errors.Is(e, ErrPriceInput) {
		t.Fatal("invalid removal", e)
	}
	stale.ExpectedRevision = before.Revision[:63] + "0"
	if stale.ExpectedRevision == before.Revision {
		stale.ExpectedRevision = before.Revision[:63] + "1"
	}
	if _, e := save(stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale removal", e)
	}
	// A creation receipt with a colliding UUID cannot be repurposed.
	var created struct{ OperationID string }
	if e := db.Table("catalog_price_edits").Where("price_id=?", priceID).Take(&created).Error; e != nil {
		t.Fatal(e)
	}
	collision := in
	collision.OperationID = created.OperationID
	if _, e := save(collision); !errors.Is(e, ErrEditConflict) {
		t.Fatal("cross action receipt", e)
	}
	exec("CREATE TRIGGER remove_receipt_failure BEFORE INSERT ON catalog_price_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial remove receipt failure'")
	_, failed := save(in)
	exec("DROP TRIGGER remove_receipt_failure")
	if failed == nil || read().Revision != before.Revision {
		t.Fatal("removal rollback", failed)
	}
	var activeRules int64
	if e := db.Table("pricing_price_rules").Where("price_id=? AND deleted_at IS NULL", priceID).Count(&activeRules).Error; e != nil || activeRules != 1 {
		t.Fatal("rules rollback", e, activeRules)
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
			t.Fatal("removal concurrency", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("removal replay", fresh)
	}
	var n int64
	if e := db.Table("pricing_price_rules").Where("price_id=? AND deleted_at IS NOT NULL", priceID).Count(&n).Error; e != nil || n != 1 {
		t.Fatal("rule retained", e, n)
	}
	if _, e := h.ReadPriceEdit(ctx, actor, p, v, priceID); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("deleted detail visible", e)
	}
	checkQuote := func(want string) {
		t.Helper()
		if e := db.Transaction(func(tx *gorm.DB) error {
			quotes, e := pricing.Read(tx, []string{v, "regional-price-v2"}, pricing.Context{Currency: "zar", Region: "regional-price-region", Quantity: "1"}, time.Now().UTC())
			if e != nil {
				return e
			}
			for _, id := range []string{v, "regional-price-v2"} {
				if want == "" {
					if quotes[id] != nil {
						t.Fatal("deleted only price quoted")
					}
				} else if quotes[id] == nil || quotes[id].Amount != want {
					t.Fatal("shared fallback", id, quotes[id])
				}
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	checkQuote("9")
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(in)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("revoked removal replay", denied)
	}
	exec("UPDATE catalog_variant_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id=?", v)
	if _, e := save(in); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("unlinked removal replay", e)
	}
	exec("UPDATE catalog_variant_price_sets SET deleted_at=NULL WHERE variant_id=?", v)
	var base struct{ ID string }
	if e := db.Table("pricing_prices").Where("price_set_id=? AND deleted_at IS NULL", before.PriceSetID).Take(&base).Error; e != nil {
		t.Fatal(e)
	}
	baseContext, e := h.ReadPriceEdit(ctx, actor, p, v, base.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.RemovePrice(ctx, actor, p, v, base.ID, PriceRemoveInput{uuid.NewString(), baseContext.Revision}); e != nil {
		t.Fatal("remove final price", e)
	}
	checkQuote("")
	t.Log(`PRICE_REMOVE_NATIVE {"gate_closed":true,"stale_revision_rejected":true,"cross_action_uuid_denied":true,"late_receipt_rollback":true,"rules_rollback":true,"concurrent_attempts":6,"single_receipt":true,"soft_deleted_price_and_rules":true,"shared_sku_base_fallback":true,"final_price_no_quote":true,"revoked_replay_denied":true,"unlinked_replay_denied":true}`)
}
