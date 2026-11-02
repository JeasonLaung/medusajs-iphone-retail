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

func verifyPriceListRemoval(t *testing.T, db *gorm.DB, actor identity.User, prior PriceListInput) {
	t.Helper()
	h := Handler{DB: db}
	ctx := context.Background()
	id := "native-price-list"
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO pricing_price_rules(id,price_id,attribute,operator,value,priority,created_at,updated_at) VALUES('native-list-price-rule','native-list-price','region_id','eq','tier-region',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE pricing_prices SET rules_count=1 WHERE id='native-list-price'")
	read := func() PriceListDetail {
		t.Helper()
		d, e := h.ReadPriceList(ctx, actor, id)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	in := PriceRemoveInput{uuid.NewString(), read().Revision}
	save := func(in PriceRemoveInput) (EditResult, error) { return h.RemovePriceList(ctx, actor, id, in) }
	fingerprint := func() string {
		t.Helper()
		var s string
		e := db.Raw("SELECT SHA2(CONCAT(l.id,l.title,l.description,l.type,l.status,COALESCE(l.metadata,''),r.id,r.attribute,r.value,p.id,p.amount,p.raw_amount,p.currency_code,p.price_set_id,p.rules_count,pr.id,pr.attribute,pr.operator,pr.value,COALESCE(l.deleted_at,''),COALESCE(r.deleted_at,''),COALESCE(p.deleted_at,''),COALESCE(pr.deleted_at,'')),256) FROM pricing_price_lists l JOIN pricing_price_list_rules r ON r.price_list_id=l.id JOIN pricing_prices p ON p.price_list_id=l.id JOIN pricing_price_rules pr ON pr.price_id=p.id WHERE l.id=?", id).Scan(&s).Error
		if e != nil || s == "" {
			t.Fatal(e)
		}
		return s
	}
	original := fingerprint()
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	_, closed := save(in)
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	if !errors.Is(closed, ErrSchema) {
		t.Fatal("closed removal gate", closed)
	}
	collision := in
	collision.OperationID = prior.OperationID
	if _, e := save(collision); !errors.Is(e, ErrEditConflict) {
		t.Fatal("edit receipt authorized deletion", e)
	}
	exec("CREATE TRIGGER fail_native_list_remove BEFORE INSERT ON catalog_price_list_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned list removal receipt failure'")
	if _, e := save(in); e == nil {
		t.Fatal("late removal failure absent")
	}
	if read().Revision != in.ExpectedRevision || fingerprint() != original {
		t.Fatal("removal did not fully roll back")
	}
	exec("DROP TRIGGER fail_native_list_remove")
	stale := in
	stale.OperationID = uuid.NewString()
	stale.ExpectedRevision = prior.ExpectedRevision
	if _, e := save(stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale list deletion", e)
	}
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
		t.Fatal("concurrent list removal", fresh, revisions, errs)
	}
	var count int64
	if e := db.Raw("SELECT COUNT(*) FROM pricing_price_lists l JOIN pricing_price_list_rules r ON r.price_list_id=l.id JOIN pricing_prices p ON p.price_list_id=l.id JOIN pricing_price_rules pr ON pr.price_id=p.id WHERE l.id=? AND l.deleted_at IS NOT NULL AND r.deleted_at=l.deleted_at AND p.deleted_at=l.deleted_at AND pr.deleted_at=l.deleted_at AND p.raw_amount=? AND l.metadata=?", id, `{"value":"200.123456789012345678","precision":20}`, `{"exact":9007199254740993}`).Scan(&count).Error; e != nil || count != 1 {
		t.Fatal("retained exact cascade", count, e)
	}
	if _, e := h.ReadPriceList(ctx, actor, id); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("removed detail readable", e)
	}
	if e := db.Transaction(func(tx *gorm.DB) error {
		q, e := pricing.Read(tx, []string{"tier-price-v", "tier-price-v2"}, pricing.Context{Currency: "cad", Region: "tier-region", Quantity: "1"}, time.Now().UTC())
		if e != nil {
			return e
		}
		for _, v := range []string{"tier-price-v", "tier-price-v2"} {
			if q[v] == nil || q[v].Amount != "75" {
				t.Fatalf("shared fallback: %+v", q)
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	changed := in
	changed.ExpectedRevision = prior.ExpectedRevision
	if _, e := save(changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed deleted retry", e)
	}
	unknown := in
	unknown.OperationID = uuid.NewString()
	if _, e := save(unknown); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("fresh operation reused deleted list", e)
	}
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='price.write'", actor.ID)
	_, denied := save(in)
	exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'price.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if !errors.Is(denied, authorization.ErrDenied) {
		t.Fatal("revoked removal replay", denied)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	_, closed = save(in)
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	if !errors.Is(closed, ErrSchema) {
		t.Fatal("closed replay gate", closed)
	}
	t.Log(`PRICE_LIST_REMOVE_NATIVE {"default_closed":true,"cross_action_uuid_denied":true,"late_failure_full_rollback":true,"stale_denied":true,"concurrent_attempts":6,"single_receipt":true,"soft_cascade_exact_ids_raw_metadata":true,"shared_sku_fallback":true,"deleted_detail_404":true,"changed_unknown_retry_denied":true,"current_permission_gate_rechecked":true}`)
}
