package identity_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCartShippingProfiles(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("shipping profile artificial fixture failed")
		}
	}
	read := func(version string) ([]cart.ShippingGroup, error) {
		var out []cart.ShippingGroup
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = cart.ReadShippingGroups(ctx, tx, "shop", ref, version)
			return err
		})
		return out, err
	}
	// Existing artificial cart includes a manual shipping line: saved
	// product text does not grant it a current variant/profile relationship.
	if _, err := read("1"); err != cart.ErrItemUnavailable {
		t.Fatal("manual shipping line silently discarded")
	}
	exec(`UPDATE cart_line_items SET requires_shipping=FALSE WHERE id='manual'`)
	if _, err := read("1"); err != cart.ErrItemUnavailable {
		t.Fatal("unprofiled shipping product accepted")
	}
	exec(`INSERT INTO catalog_product_shipping_profiles(product_id,shipping_profile_id,id,created_at,updated_at) VALUES('product','manual-profile','cart-profile',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	groups, err := read("1")
	if err != nil || len(groups) != 1 || groups[0].ProfileID != "manual-profile" || len(groups[0].Lines) != 4 {
		t.Fatalf("cart current shipping profile grouping differs: error=%v groups=%+v", err, groups)
	}
	if groups[0].Lines[0].ID != "a" || groups[0].Lines[0].Quantity != "1" || groups[0].Lines[3].ID != "no-price-custom" {
		t.Fatal("shipping group order/quantities changed")
	}
	// A saved product snapshot must never be used to select its profile.
	exec(`UPDATE cart_line_items SET product_id='stale-saved-product' WHERE id='a'`)
	if out, err := read("1"); err != nil || len(out) != 1 || len(out[0].Lines) != 4 {
		t.Fatal("saved product ID overrode current variant")
	}
	exec(`UPDATE fulfillment_shipping_profiles SET deleted_at=UTC_TIMESTAMP(6) WHERE id='manual-profile'`)
	if _, err := read("1"); err != cart.ErrItemUnavailable {
		t.Fatal("deleted profile accepted")
	}
	exec(`UPDATE fulfillment_shipping_profiles SET deleted_at=NULL WHERE id='manual-profile'`)
	exec(`INSERT INTO catalog_product_shipping_profiles(product_id,shipping_profile_id,id,created_at,updated_at) VALUES('product','other-profile','cart-profile-other',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	if _, err := read("1"); err != cart.ErrItemUnavailable {
		t.Fatal("ambiguous product profiles selected arbitrarily")
	}
	exec(`UPDATE catalog_product_shipping_profiles SET deleted_at=UTC_TIMESTAMP(6) WHERE id='cart-profile-other'`)
	exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('shipping-other-product','Other','shipping-other',FALSE,'published',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_channels(product_id,sales_channel_id,id,created_at,updated_at) VALUES('shipping-other-product','channel','shipping-other-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_shipping_profiles(product_id,shipping_profile_id,id,created_at,updated_at) VALUES('shipping-other-product','other-profile','shipping-other-profile',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE catalog_variants SET product_id='shipping-other-product' WHERE id='no-price'`)
	groups, err = read("1")
	if err != nil || len(groups) != 2 || groups[0].ProfileID != "manual-profile" || len(groups[0].Lines) != 3 || groups[1].ProfileID != "other-profile" || len(groups[1].Lines) != 1 {
		t.Fatal("multiple current product profiles collapsed")
	}
	if _, err := read("2"); err != cart.ErrConflict {
		t.Fatal("stale cart version grouped")
	}
	wrong := ref
	wrong.Generation = "0" + ref.Generation[1:]
	if wrong.Generation == ref.Generation {
		wrong.Generation = "1" + ref.Generation[1:]
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := cart.ReadShippingGroups(ctx, tx, "shop", wrong, "1"); return err }); err != cart.ErrDenied {
		t.Fatal("stale cart authority grouped")
	}
	before := func() string {
		var rows []map[string]any
		if db.Table("cart_line_items").Where("cart_id=?", ref.CartID).Order("id").Find(&rows).Error != nil {
			t.Fatal("shipping fingerprint unavailable")
		}
		b, _ := json.Marshal(rows)
		return string(b)
	}
	fingerprint := before()
	if _, err := read("1"); err != nil || fingerprint != before() {
		t.Fatal("group read changed saved lines")
	}
	checkCartShippingQuotes(t, ctx, db, ref)
	exec(`UPDATE cart_line_items SET requires_shipping=FALSE WHERE cart_id=?`, ref.CartID)
	if out, err := read("1"); err != nil || len(out) != 0 {
		t.Fatal("nonshipping cart invented a profile")
	}
	if _, err := cart.ReadShippingGroups(ctx, db, "shop", ref, "1"); err != cart.ErrInput {
		t.Fatal("autocommit grouping accepted")
	}
	t.Log(`CART_SHIPPING_PROFILES_REHEARSAL {"current_variant_product_relationship":true,"manual_shipping_unavailable":true,"unprofiled_deleted_ambiguous_rejected":true,"multiple_profiles_grouped":true,"saved_product_not_authority":true,"quantity_and_order_preserved":true,"removed_nonshipping_excluded":true,"version_and_authority_checked":true,"saved_lines_unchanged":true,"autocommit_rejected":true,"checkout":false}`)
}
