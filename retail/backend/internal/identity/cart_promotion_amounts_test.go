package identity_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkPromotionAmounts(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("artificial promotion amount setup failed")
		}
	}
	read := func(reference sessionstore.CartReference, version string, wantErr bool) cart.AmountDetail {
		t.Helper()
		before := fingerprint()
		var result cart.AmountDetail
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.ReadPromotionAmounts(ctx, tx, "shop", reference, version)
			return err
		})
		if wantErr {
			if err == nil || !reflect.DeepEqual(result, cart.AmountDetail{}) {
				t.Fatal("invalid promotion amount facts leaked partial result")
			}
		} else if err != nil {
			t.Fatalf("promotion amount facts failed: %v", err)
		}
		if fingerprint() != before {
			t.Fatal("promotion amount read changed saved cart facts")
		}
		return result
	}
	if got, err := cart.ReadPromotionAmounts(ctx, db, "shop", ref, "1"); err == nil || !reflect.DeepEqual(got, cart.AmountDetail{}) {
		t.Fatal("autocommit promotion amount preparation accepted")
	}
	base := read(ref, "1", false)
	if base.Totals["total"] != "262.5" || len(base.Lines) != 1 || base.Lines[0].Totals["subtotal"] != "200" || base.Lines[0].Totals["original_total"] != "250" || base.Lines[0].Totals["discount_total"] != "0" || len(base.ShippingMethods) != 1 || base.ShippingMethods[0].Totals["subtotal"] != "10" || base.ShippingMethods[0].Totals["original_total"] != "12.5" || base.PricingRecalculated || base.Checkout {
		t.Fatal("promotion preparation reused old discounts or lost tax bases")
	}
	read(ref, "2", true)
	wrong := ref
	wrong.Generation = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	read(wrong, "1", true)
	// Existing discounts are excluded entirely, even when their raw amount is
	// damaged. They remain stored and must be handled by a later refresh phase.
	exec(`UPDATE cart_line_adjustments SET raw_amount='{}' WHERE id='amount-cart-discount'`)
	if got := read(ref, "1", false); !reflect.DeepEqual(got, base) {
		t.Fatal("stale adjustment influenced undiscounted preparation")
	}
	exec(`UPDATE cart_line_adjustments SET raw_amount='{"value":"20","precision":20}' WHERE id='amount-cart-discount'`)
	exec(`UPDATE cart_line_items SET raw_unit_price='{"value":"9007199254740993.000000000000000000001","precision":38}' WHERE id='amount-cart-line'`)
	if got := read(ref, "1", false); got.Lines[0].Totals["subtotal"] != "18014398509481986.000000000000000000002" {
		t.Fatal("promotion monetary preparation rounded raw unit price")
	}
	exec(`UPDATE cart_line_items SET raw_unit_price='{"value":"100","precision":20}',is_tax_inclusive=TRUE WHERE id='amount-cart-line'`)
	if got := read(ref, "1", false); got.Lines[0].Totals["subtotal"] != "160" || got.Lines[0].Totals["original_total"] != "200" || got.Totals["total"] != "212.5" {
		t.Fatal("inclusive item promotion bases were not converted")
	}
	exec(`UPDATE cart_line_items SET is_tax_inclusive=FALSE WHERE id='amount-cart-line'`)
	// The helper must see earlier pricing/tax phase writes in the same owning
	// transaction, and must not commit those writes itself.
	rollback := errors.New("artificial later command failure")
	before := fingerprint()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Exec("UPDATE cart_line_tax_lines SET rate=50 WHERE id='amount-cart-tax'").Error != nil {
			return errors.New("artificial tax update failed")
		}
		got, err := cart.ReadPromotionAmounts(ctx, tx, "shop", ref, "1")
		if err != nil || got.Totals["total"] != "312.5" {
			t.Fatal("promotion amount facts did not see caller refresh")
		}
		return rollback
	})
	if err != rollback || fingerprint() != before {
		t.Fatal("promotion amount preparation escaped caller rollback")
	}
	exec(`UPDATE cart_shipping_methods SET raw_amount='{}' WHERE id='amount-cart-shipping'`)
	read(ref, "1", true)
	exec(`UPDATE cart_shipping_methods SET raw_amount='{"value":"12.5","precision":20}' WHERE id='amount-cart-shipping'`)
	t.Log(`CART_PROMOTION_AMOUNTS_REHEARSAL {"actual_mysql":true,"undiscounted_item_and_shipping_tax_bases":true,"stale_adjustments_excluded_without_deletion":true,"current_authority_and_version":true,"caller_refresh_visible_and_rollback_atomic":true,"malformed_source_amount_rejected":true,"read_only_fingerprints":true,"public_http_or_checkout_integrated":false}`)
}
