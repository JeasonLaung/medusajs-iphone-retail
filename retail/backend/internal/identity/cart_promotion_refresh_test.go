package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkPromotionRefresh(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	refresh := func(tx *gorm.DB, reference sessionstore.CartReference, version string, in cart.PromotionRefresh, wantErr bool) cart.PromotionAdjustmentResult {
		t.Helper()
		got, err := cart.RefreshPromotions(ctx, tx, "shop", reference, version, in)
		if wantErr {
			if err == nil || !reflect.DeepEqual(got, cart.PromotionAdjustmentResult{}) {
				t.Fatal("invalid refresh returned partial success")
			}
		} else if err != nil {
			t.Fatalf("promotion refresh failed: %v", err)
		}
		return got
	}
	refresh(db, ref, "1", cart.PromotionRefresh{At: at}, true)
	before := fingerprint()
	rollback := errors.New("artificial later refresh command failure")
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exec := func(query string, args ...any) {
			t.Helper()
			if tx.Exec(query, args...).Error != nil {
				t.Fatal("promotion refresh fixture failed")
			}
		}
		count := func(query string, want int) {
			t.Helper()
			var n int
			if tx.Raw(query).Scan(&n).Error != nil || n != want {
				t.Fatalf("promotion refresh invariant count want %d got %d", want, n)
			}
		}
		amount := func(want string) {
			t.Helper()
			facts, err := cart.ReadPromotionTargets(ctx, tx, "shop", ref, "1")
			if err != nil || len(facts.Items) != 1 {
				t.Fatal("refreshed target amounts unavailable")
			}
			// ReadAmounts owns its own transaction; use its internal amount bases
			// through the trusted target projection instead of another connection.
			if facts.Items[0]["total"] != json.Number(want) {
				t.Fatalf("refreshed item total differs, want %s", want)
			}
		}
		exec("INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,`limit`,created_at,updated_at) VALUES('refresh-main','MAIN',FALSE,'standard','active',FALSE,0,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('refresh-auto','AUTO',TRUE,'standard','active',FALSE,0,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('refresh-limited','LIMITED',FALSE,'standard','active',FALSE,1,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		exec(`INSERT INTO promotion_application_methods(id,promotion_id,type,target_type,allocation,value,raw_value,created_at,updated_at) VALUES('refresh-main-method','refresh-main','percentage','items','across',10,'{"value":"10","precision":20}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('refresh-auto-method','refresh-auto','fixed','items','across',5,'{"value":"5","precision":20}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('refresh-limited-method','refresh-limited','percentage','items','across',10,'{"value":"10","precision":20}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		refresh(tx, ref, "1", cart.PromotionRefresh{}, true)
		refresh(tx, ref, "2", cart.PromotionRefresh{At: at}, true)
		wrong := ref
		wrong.Generation = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		refresh(tx, wrong, "1", cart.PromotionRefresh{At: at}, true)
		first := refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"MAIN", "LIMITED"}}, false)
		if first.ItemAdjustments != 2 || first.ShippingAdjustments != 0 || first.Promotions != 2 || !reflect.DeepEqual(first.Skipped, []cart.PromotionSkip{{Code: "LIMITED", Reason: "promotion_limit_exceeded"}}) {
			t.Fatal("manual/automatic/limit feedback did not compose")
		}
		amount("193.75")
		count("SELECT COUNT(*) FROM cart_line_adjustments WHERE item_id='amount-cart-line' AND deleted_at IS NULL AND code IS NULL", 1)
		count("SELECT COUNT(*) FROM cart_line_adjustments WHERE item_id='amount-cart-line' AND deleted_at IS NULL AND code IN ('MAIN','AUTO')", 2)
		var linkID string
		if tx.Raw("SELECT id FROM cart_promotions WHERE promotion_id='refresh-main' AND deleted_at IS NULL").Scan(&linkID).Error != nil || linkID == "" {
			t.Fatal("main promotion link missing")
		}
		again := refresh(tx, ref, "1", cart.PromotionRefresh{At: at}, false)
		if again.ItemAdjustments != 2 || again.Removed != 2 || again.Promotions != 2 {
			t.Fatal("recalculation accumulated old coded discounts")
		}
		amount("193.75")
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"MAIN"}, Action: "remove"}, false)
		amount("218.75")
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"AUTO"}, Action: "remove"}, false)
		amount("218.75")
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"MAIN"}, Action: "replace"}, false)
		amount("193.75")
		var restored string
		if tx.Raw("SELECT id FROM cart_promotions WHERE promotion_id='refresh-main' AND deleted_at IS NULL").Scan(&restored).Error != nil || restored != linkID {
			t.Fatal("re-added promotion replaced original link identity")
		}
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"missing"}}, true)
		amount("193.75")
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Action: "replace"}, false)
		amount("218.75")
		exec(`UPDATE promotions SET status='inactive' WHERE id IN ('refresh-main','refresh-auto')`)
		cleared := refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"MAIN"}, Action: "replace"}, false)
		if cleared.ItemAdjustments != 0 || cleared.Promotions != 0 || cleared.Removed != 1 {
			t.Fatal("inactive configured code retained adjustments")
		}
		amount("225")
		exec(`INSERT INTO promotion_campaigns(id,name,campaign_identifier,starts_at,ends_at,created_at,updated_at) VALUES('refresh-campaign','Artificial','artificial',NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec("INSERT INTO promotion_campaign_budgets(id,campaign_id,type,used,raw_used,`limit`,raw_limit,attribute,created_at,updated_at) VALUES('refresh-budget','refresh-campaign','use_by_attribute',0,'{\"value\":\"0\",\"precision\":20}',1,'{\"value\":\"1\",\"precision\":20}','customer_email',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		exec(`UPDATE promotions SET status='active',campaign_id='refresh-campaign' WHERE id='refresh-main'`)
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"MAIN"}}, true)
		amount("225")
		exec(`UPDATE carts SET email='refresh@example.test' WHERE id=?`, ref.CartID)
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"MAIN"}}, false)
		amount("200")
		exec(`UPDATE promotion_campaigns SET ends_at='2026-10-29 00:00:00' WHERE id='refresh-campaign'`)
		expired := refresh(tx, ref, "1", cart.PromotionRefresh{At: at}, false)
		if expired.Promotions != 0 || expired.ItemAdjustments != 0 {
			t.Fatal("campaign exclusive end retained discounts")
		}
		amount("225")
		exec(`UPDATE promotion_campaigns SET ends_at=NULL WHERE id='refresh-campaign'`)
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at, Codes: []string{"MAIN"}}, false)
		amount("200")
		exec(`INSERT INTO promotion_budget_usages(id,budget_id,attribute_value,used,raw_used,created_at,updated_at) VALUES('refresh-usage','refresh-budget','refresh@example.test',1,'{"value":"1","precision":20}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		skipped := refresh(tx, ref, "1", cart.PromotionRefresh{At: at}, false)
		if skipped.Promotions != 0 || !reflect.DeepEqual(skipped.Skipped, []cart.PromotionSkip{{Code: "MAIN", Reason: "campaign_budget_exceeded"}}) {
			t.Fatal("actual attribute budget preview did not gate refresh")
		}
		amount("225")
		count("SELECT COUNT(*) FROM promotion_campaign_budgets WHERE id='refresh-budget' AND used=0", 1)
		count("SELECT COUNT(*) FROM promotions WHERE id='refresh-main' AND used=0", 1)
		count("SELECT COUNT(*) FROM promotion_budget_usages WHERE id='refresh-usage' AND used=1", 1)
		count("SELECT COUNT(*) FROM promotion_usage_operations", 0)
		count("SELECT COUNT(*) FROM cart_line_adjustments WHERE id='amount-cart-discount' AND deleted_at IS NULL", 1)
		count("SELECT COUNT(*) FROM cart_shipping_adjustments WHERE id='amount-cart-shipping-discount' AND deleted_at IS NULL", 1)
		exec(`UPDATE payment_runtime_schema SET reads_enabled=FALSE`)
		refresh(tx, ref, "1", cart.PromotionRefresh{At: at}, true)
		exec(`UPDATE payment_runtime_schema SET reads_enabled=TRUE`)
		amount("225")
		return rollback
	})
	if err != rollback || before != fingerprint() {
		t.Fatalf("promotion refresh escaped caller rollback: %v", err)
	}
	t.Log(`CART_PROMOTION_REFRESH_REHEARSAL {"actual_mysql":true,"server_owned_selection_context_config_usage_compute_persist":true,"manual_and_automatic_recalculation":true,"remove_replace_and_original_link_restore":true,"limit_and_attribute_budget_feedback":true,"manual_adjustments_preserved":true,"no_usage_registration":true,"later_failure_full_rollback":true,"public_http_or_checkout_integrated":false}`)
}
