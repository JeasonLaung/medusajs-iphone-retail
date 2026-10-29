package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCartShippingSelection(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("cart shipping selection artificial fixture failed")
		}
	}
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	read := func(ids []string, version string) (cart.ManualShippingSelection, error) {
		before := fingerprint()
		var out cart.ManualShippingSelection
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = cart.ReadSelectedManualShipping(ctx, tx, "shop", ref, version, ids, []string{"candidate-provider"}, at)
			return err
		})
		if before != fingerprint() {
			t.Fatal("shipping selection changed cart or reservations")
		}
		return out, err
	}
	if _, err := read([]string{"candidate-nested"}, "1"); err != cart.ErrItemUnavailable {
		t.Fatal("uncovered profile accepted")
	}
	exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,provider_id,shipping_profile_id,shipping_option_type_id,created_at,updated_at) VALUES('selected-other','Other','flat','candidate-nested-zone','candidate-provider','other-profile','candidate-type',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_option_price_sets(shipping_option_id,price_set_id,id,created_at,updated_at) VALUES('selected-other','set','selected-other-price',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	ids := []string{"candidate-nested", "selected-other"}
	if out, err := read(ids, "1"); err != nil || len(out.Groups) != 2 || !out.Stock.CanFulfill {
		t.Fatalf("complete unmanaged selection failed: %v", err)
	}
	for _, bad := range [][]string{nil, {"candidate-nested"}, {"selected-other"}, {"candidate-nested", "selected-other", "unknown"}} {
		if _, err := read(bad, "1"); err != cart.ErrItemUnavailable {
			t.Fatal("incomplete/extra option selection accepted")
		}
	}
	if _, err := read([]string{"candidate-nested", "candidate-nested"}, "1"); err != cart.ErrInput {
		t.Fatal("duplicate option accepted")
	}
	exec(`INSERT INTO inventory_items(id,title,requires_shipping,created_at,updated_at) VALUES('shipping-shared','Shared',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, variant := range []string{"v", "no-price"} {
		exec(`UPDATE catalog_variants SET manage_inventory=TRUE WHERE id=?`, variant)
		exec(`INSERT INTO catalog_variant_inventory_items(variant_id,inventory_item_id,required_quantity,id,created_at,updated_at) VALUES(?,'shipping-shared',1,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, variant, "selected-"+variant)
	}
	exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES('selected-level','shipping-shared','candidate-location',10,0,999,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	if _, err := read(ids, "1"); err != cart.ErrItemStock {
		t.Fatal("separate profile checks oversold shared warehouse")
	}
	exec(`UPDATE inventory_levels SET stocked_quantity=11 WHERE id='selected-level'`)
	out, err := read(ids, "1")
	if err != nil || len(out.Stock.Assignments) != 4 {
		t.Fatalf("shared selection assignment failed: %v", err)
	}
	for _, assignment := range out.Stock.Assignments {
		if assignment.LocationID != "candidate-location" {
			t.Fatal("shipping warehouse escaped provider association")
		}
	}
	// A tracked non-shipping line still consumes shared stock.
	exec(`UPDATE cart_line_items SET requires_shipping=FALSE WHERE id='c'`)
	exec(`UPDATE inventory_levels SET stocked_quantity=10 WHERE id='selected-level'`)
	if _, err := read(ids, "1"); err != cart.ErrItemStock {
		t.Fatal("non-shipping tracked inventory omitted")
	}
	exec(`UPDATE inventory_levels SET stocked_quantity=11 WHERE id='selected-level'`)
	if _, err := read(ids, "1"); err != nil {
		t.Fatal("non-shipping tracked line cannot share channel inventory")
	}
	exec(`UPDATE cart_line_items SET requires_shipping=TRUE WHERE id='c'`)
	exec(`UPDATE fulfillment_location_providers SET deleted_at=UTC_TIMESTAMP(6) WHERE id='manual-provider-link'`)
	if _, err := read(ids, "1"); err != cart.ErrItemUnavailable {
		t.Fatal("stale provider warehouse selection accepted")
	}
	exec(`UPDATE fulfillment_location_providers SET deleted_at=NULL WHERE id='manual-provider-link'`)
	if _, err := read(ids, "2"); err != cart.ErrConflict {
		t.Fatal("stale selected cart version accepted")
	}
	if _, err := cart.ReadSelectedManualShipping(ctx, db, "shop", ref, "1", ids, []string{"candidate-provider"}, at); err != cart.ErrInput {
		t.Fatal("autocommit selected shipping accepted")
	}
	checkCartShippingMethods(t, ctx, db, ref)
	t.Log(`CART_SHIPPING_SELECTION_REHEARSAL {"current_quotes_and_full_profile_coverage":true,"duplicate_extra_missing_rejected":true,"provider_warehouse_revalidated":true,"shared_stock_across_profiles":true,"nonshipping_tracked_stock_counted":true,"unmanaged_shipping_requires_warehouse":true,"readonly_fingerprint":true,"stale_version_and_autocommit_rejected":true,"inventory_reserved":false,"shipping_saved":false,"checkout":false}`)
}
