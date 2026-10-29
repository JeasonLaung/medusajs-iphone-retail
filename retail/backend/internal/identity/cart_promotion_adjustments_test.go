package identity_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCartPromotionAdjustments(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	exec := func(tx *gorm.DB, query string, args ...any) {
		t.Helper()
		if tx.Exec(query, args...).Error != nil {
			t.Fatal("artificial adjustment setup failed")
		}
	}
	// All promotion configuration and adjustment writes are rolled back at the
	// end, leaving the parent fixture and existing manual adjustments untouched.
	rollback := errors.New("artificial later command failure")
	before := fingerprint()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exec(tx, `INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('apply-p','SAVE',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('old-p','OLD',FALSE,'standard','inactive',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO cart_line_adjustments(id,item_id,promotion_id,code,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES('old-coded','amount-cart-line',NULL,'',1,'{"value":"1","precision":20}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-parent-coded','amount-cart-deleted',NULL,'OLD',1,'{"value":"1","precision":20}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO cart_promotions(cart_id,promotion_id,id,created_at,updated_at,deleted_at) VALUES(?,'apply-p','stable-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),(?,'old-p','old-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`, ref.CartID, ref.CartID)
		actions := []promotion.ComputedAction{
			{Action: "addItemAdjustment", Code: "SAVE", PromotionID: "apply-p", ItemID: "amount-cart-line", Amount: "0.123456789012345678901", TaxInclusive: true},
			{Action: "campaignBudgetExceeded", Code: "SAVE", PromotionID: "apply-p"},
			{Action: "addShippingMethodAdjustment", Code: "SAVE", PromotionID: "apply-p", ShippingMethodID: "amount-cart-shipping", Amount: "1.000000000000000000001"},
		}
		result, err := cart.ReplacePromotionAdjustments(ctx, tx, "shop", ref, "1", actions)
		if err != nil || result.ItemAdjustments != 1 || result.ShippingAdjustments != 1 || result.Removed != 1 || result.Promotions != 1 || !reflect.DeepEqual(result.Skipped, []cart.PromotionSkip{{Code: "SAVE", Reason: "campaign_budget_exceeded"}}) {
			t.Fatalf("adjustment replacement: %+v %v", result, err)
		}
		var rows []struct {
			Amount, RawAmount string
			IsTaxInclusive    bool
		}
		if tx.Table("cart_line_adjustments").Select("CAST(amount AS CHAR) AS amount,raw_amount,is_tax_inclusive").Where("item_id='amount-cart-line' AND code='SAVE' AND deleted_at IS NULL").Scan(&rows).Error != nil || len(rows) != 1 || rows[0].Amount != "0.123456789012345678" || !strings.Contains(rows[0].RawAmount, "0.123456789012345678901") || !rows[0].IsTaxInclusive {
			t.Fatal("exact persisted discount amount or tax flag lost")
		}
		count := func(table, where string, args ...any) int64 {
			t.Helper()
			var n int64
			if tx.Table(table).Where(where, args...).Count(&n).Error != nil {
				t.Fatal("adjustment observation failed")
			}
			return n
		}
		if count("cart_line_adjustments", "id='amount-cart-discount' AND deleted_at IS NULL") != 1 || count("cart_shipping_adjustments", "id='amount-cart-shipping-discount' AND deleted_at IS NULL") != 1 || count("cart_line_adjustments", "id='deleted-parent-coded' AND deleted_at IS NULL") != 1 || count("cart_line_adjustments", "id='old-coded' AND deleted_at IS NOT NULL") != 1 {
			t.Fatal("manual/deleted-parent preservation or coded history failed")
		}
		if count("cart_promotions", "cart_id=? AND id='stable-link' AND deleted_at IS NULL", ref.CartID) != 1 || count("cart_promotions", "cart_id=? AND promotion_id='old-p' AND deleted_at IS NOT NULL", ref.CartID) != 1 {
			t.Fatal("promotion link replacement/history failed")
		}
		if count("promotions", "id='apply-p' AND used=0") != 1 {
			t.Fatal("cart refresh consumed checkout usage")
		}
		removed, err := cart.ReplacePromotionAdjustments(ctx, tx, "shop", ref, "1", nil)
		if err != nil || removed.Removed != 2 || removed.Promotions != 0 || count("cart_promotions", "cart_id=? AND deleted_at IS NULL", ref.CartID) != 0 {
			t.Fatal("empty desired promotions did not clear coded adjustments/links")
		}
		if _, err := cart.ReplacePromotionAdjustments(ctx, tx, "shop", ref, "1", actions); err != nil || count("cart_promotions", "cart_id=? AND id='stable-link' AND deleted_at IS NULL", ref.CartID) != 1 {
			t.Fatal("reapplied promotion did not restore stable relation")
		}
		// Invalid targets and code/ID bindings must fail before deleting the
		// just-created discounts; the complete caller command still rolls back.
		for _, bad := range []promotion.ComputedAction{
			{Action: "addItemAdjustment", Code: "SAVE", PromotionID: "apply-p", ItemID: "amount-cart-deleted", Amount: "1"},
			{Action: "addItemAdjustment", Code: "WRONG", PromotionID: "apply-p", ItemID: "amount-cart-line", Amount: "1"},
			{Action: "addItemAdjustment", Code: "SAVE", PromotionID: "apply-p", ItemID: "foreign-line", Amount: "1"},
			{Action: "addItemAdjustment", Code: "SAVE", PromotionID: "apply-p", ItemID: "amount-cart-line", Amount: "0"},
			{Action: "removeItemAdjustment", Code: "SAVE", PromotionID: "apply-p"},
		} {
			got, err := cart.ReplacePromotionAdjustments(ctx, tx, "shop", ref, "1", []promotion.ComputedAction{bad})
			if err == nil || !reflect.DeepEqual(got, cart.PromotionAdjustmentResult{}) || count("cart_line_adjustments", "item_id='amount-cart-line' AND code='SAVE' AND deleted_at IS NULL") != 1 {
				t.Fatal("invalid action mutated earlier adjustments")
			}
		}
		if _, err := cart.ReplacePromotionAdjustments(ctx, tx, "shop", ref, "2", nil); err != cart.ErrConflict {
			t.Fatal("stale version accepted for adjustments")
		}
		return rollback
	})
	if err != rollback || before != fingerprint() {
		t.Fatal("late command failure left promotion changes")
	}
	if got, err := cart.ReplacePromotionAdjustments(ctx, db, "shop", ref, "1", nil); err == nil || !reflect.DeepEqual(got, cart.PromotionAdjustmentResult{}) {
		t.Fatal("autocommit adjustment write accepted")
	}
	// Force the INSERT after old coded rows have been soft-deleted to fail.
	// The transaction owner rolls everything back, including history writes.
	injected := errors.New("artificial adjustment insert failure")
	if db.Callback().Raw().Before("gorm:raw").Register("owned.cart-promotion-fault", func(q *gorm.DB) {
		if strings.HasPrefix(q.Statement.SQL.String(), "INSERT INTO cart_line_adjustments(id,item_id,promotion_id,code,amount,raw_amount,is_tax_inclusive") {
			q.AddError(injected)
		}
	}) != nil {
		t.Fatal("adjustment fault injection unavailable")
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exec(tx, `INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('fault-p','FAULT',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO cart_line_adjustments(id,item_id,code,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES('fault-old','amount-cart-line','OLD',1,'{"value":"1","precision":20}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		_, err := cart.ReplacePromotionAdjustments(ctx, tx, "shop", ref, "1", []promotion.ComputedAction{{Action: "addItemAdjustment", Code: "FAULT", PromotionID: "fault-p", ItemID: "amount-cart-line", Amount: "1"}})
		return err
	})
	if db.Callback().Raw().Remove("owned.cart-promotion-fault") != nil {
		t.Fatal("adjustment fault cleanup failed")
	}
	if err == nil || before != fingerprint() {
		t.Fatal("failed adjustment insert escaped rollback")
	}
	t.Log(`CART_PROMOTION_ADJUSTMENTS_REHEARSAL {"actual_mysql":true,"exact_raw_and_inclusive_flag":true,"null_code_manual_preserved":true,"coded_adjustments_soft_deleted":true,"deleted_parent_untouched":true,"empty_and_reapply_restore_links":true,"feedback_with_positive_adjustments":true,"target_code_and_version_rejected_before_mutation":true,"late_and_write_failure_rollback":true,"budget_usage_not_registered":true,"public_command_or_checkout_integrated":false}`)
}
