package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCartShippingRefresh(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("shipping refresh artificial fixture failed")
		}
	}
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	providers := []string{"candidate-provider"}
	run := func(rollback bool) (cart.ShippingRefreshResult, error) {
		var out cart.ShippingRefreshResult
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = cart.RefreshManualShippingMethods(ctx, tx, "shop", ref, "1", providers, at)
			if err != nil {
				return err
			}
			if rollback {
				return errors.New("artificial later refresh failure")
			}
			return nil
		})
		return out, err
	}
	// Single-method carts retain a priced orphan profile, matching the source
	// refresh's shipping_methods.length > 1 cleanup guard.
	exec(`INSERT INTO fulfillment_option_price_sets(shipping_option_id,price_set_id,id,created_at,updated_at) VALUES('unrelated-option','set','unrelated-refresh-price',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	before := fingerprint()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Exec(`UPDATE cart_shipping_methods SET deleted_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND id<>'old-shipping-unrelated' AND deleted_at IS NULL`, ref.CartID).Error != nil {
			t.Fatal("singleton fixture failed")
		}
		out, err := cart.RefreshManualShippingMethods(ctx, tx, "shop", ref, "1", providers, at)
		if err != nil || out.Updated != 1 || out.Removed != 0 {
			t.Fatalf("single method orphan semantics changed: %v", err)
		}
		return errors.New("singleton fixture rollback")
	})
	if err == nil || before != fingerprint() {
		t.Fatal("singleton refresh rollback changed data")
	}
	exec(`UPDATE carts SET region_id='region-other' WHERE id=?`, ref.CartID)
	before = fingerprint()
	if out, err := run(true); err == nil || out.Updated != 2 || out.Removed != 1 || before != fingerprint() {
		t.Fatal("later refresh failure retained repricing/removal")
	}
	exec(`CREATE TRIGGER shipping_refresh_failure BEFORE UPDATE ON cart_shipping_methods FOR EACH ROW BEGIN IF NEW.shipping_option_id='selected-other' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial refresh failure'; END IF; END`)
	if _, err := run(false); err != cart.ErrStorage || before != fingerprint() {
		t.Fatal("late refresh failure left partial changes")
	}
	exec(`DROP TRIGGER shipping_refresh_failure`)
	var beforeRows []struct{ ID, Data string }
	if db.Table("cart_shipping_methods").Select("id,data").Where("cart_id=? AND deleted_at IS NULL AND shipping_option_id IN ?", ref.CartID, []string{"candidate-nested", "selected-other"}).Order("id").Scan(&beforeRows).Error != nil {
		t.Fatal("saved method data unavailable")
	}
	if out, err := run(false); err != nil || out.Updated != 2 || out.Removed != 1 {
		t.Fatalf("shipping refresh failed: %v", err)
	}
	for _, old := range beforeRows {
		var saved struct {
			RawAmount, Data string
			Inclusive       bool
		}
		if db.Table("cart_shipping_methods").Select("raw_amount,data,is_tax_inclusive AS inclusive").Where("id=? AND deleted_at IS NULL", old.ID).Take(&saved).Error != nil {
			t.Fatal("refresh replaced saved ID")
		}
		var raw struct{ Value string }
		json.Unmarshal([]byte(saved.RawAmount), &raw)
		if raw.Value != "30" || saved.Data != old.Data || !saved.Inclusive {
			t.Fatal("refresh lost region price, data or tax preference")
		}
	}
	// A legitimate zero is retained, not mistaken for missing pricing.
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES('refresh-zero','set','eur',0,'{"value":"0","precision":38}',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE carts SET region_id='region' WHERE id=?`, ref.CartID)
	if out, err := run(false); err != nil || out.Updated != 2 || out.Removed != 0 {
		t.Fatal("zero-price methods removed")
	}
	var zero int64
	db.Table("cart_shipping_methods").Where("cart_id=? AND deleted_at IS NULL AND amount=0", ref.CartID).Count(&zero)
	if zero != 2 {
		t.Fatal("zero shipping amount not saved")
	}
	exec(`UPDATE fulfillment_option_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE id='selected-other-price'`)
	if out, err := run(false); err != nil || out.Updated != 1 || out.Removed != 1 {
		t.Fatal("unpriced method retained")
	}
	exec(`UPDATE cart_addresses SET country_code='se' WHERE id='quote-address'`)
	if out, err := run(false); err != nil || out.Updated != 0 || out.Removed != 1 {
		t.Fatal("ineligible address method retained")
	}
	if out, err := run(false); err != nil || out != (cart.ShippingRefreshResult{}) {
		t.Fatal("empty shipping refresh invented methods")
	}
	if _, err := cart.RefreshManualShippingMethods(ctx, db, "shop", ref, "1", providers, at); err != cart.ErrInput {
		t.Fatal("autocommit shipping refresh accepted")
	}
	t.Log(`CART_SHIPPING_REFRESH_REHEARSAL {"stored_region_reprices":true,"method_ids_and_data_preserved":true,"zero_price_retained":true,"unpriced_address_ineligible_removed":true,"multiple_method_orphan_removed":true,"single_method_orphan_guard_preserved":true,"later_and_late_failure_rollback":true,"empty_methods_noop":true,"autocommit_rejected":true,"inventory_reserved":false,"full_refresh":false,"checkout":false}`)
}
