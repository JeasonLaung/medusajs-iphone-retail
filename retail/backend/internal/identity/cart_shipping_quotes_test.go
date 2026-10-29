package identity_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCartShippingQuotes(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("cart shipping quote artificial fixture failed")
		}
	}
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	fingerprint := func() string {
		t.Helper()
		all := []json.RawMessage{}
		for _, table := range []string{"carts", "cart_ownership", "cart_addresses", "cart_line_items", "cart_shipping_methods", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("cart quote fingerprint unavailable")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	read := func(providers []string, version string) ([]cart.ShippingQuotesGroup, error) {
		before := fingerprint()
		var out []cart.ShippingQuotesGroup
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = cart.ReadManualShippingQuotes(ctx, tx, "shop", ref, version, providers, at)
			return err
		})
		if fingerprint() != before {
			t.Fatal("cart quote changed saved data or reservations")
		}
		return out, err
	}
	providers := []string{"candidate-provider"}
	exec(`INSERT INTO cart_addresses(id,country_code,province,city,postal_code,created_at,updated_at) VALUES('quote-address','dk','01','Århus','00123',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE carts SET shipping_address_id='quote-address' WHERE id=?`, ref.CartID)
	// Existing custom address.country rule needs a server hook. Persisted
	// address is a geo filter, not an implicitly fabricated hook context.
	if out, err := read(providers, "1"); err != nil || len(out) != 2 || len(out[0].Quotes) != 0 || len(out[1].Quotes) != 0 {
		t.Fatal("custom shipping hook context fabricated")
	}
	exec(`UPDATE fulfillment_option_rules SET attribute='enabled_in_store',value='"true"' WHERE id='candidate-nested-rule'`)
	wantAmount := "30" // Earlier price phase saved region-other on this cart.
	quotes := func(want int) {
		t.Helper()
		out, err := read(providers, "1")
		if err != nil || len(out) != 2 || out[0].Group.ProfileID != "manual-profile" || len(out[0].Group.Lines) != 3 || len(out[0].Quotes) != want || out[1].Group.ProfileID != "other-profile" || len(out[1].Quotes) != 0 {
			t.Fatalf("cart quote grouping differs: %v", err)
		}
		if want == 1 && (out[0].Quotes[0].Option.ID != "candidate-nested" || out[0].Quotes[0].Price.Amount != wantAmount || !out[0].Quotes[0].Price.CalculatedTaxInclusive) {
			t.Fatal("cart persisted price context lost")
		}
	}
	quotes(1)
	exec(`UPDATE carts SET region_id='region' WHERE id=?`, ref.CartID)
	wantAmount = "100.000000000000000000001"
	quotes(1)
	exec(`UPDATE cart_addresses SET country_code='se' WHERE id='quote-address'`)
	quotes(0)
	exec(`UPDATE cart_addresses SET country_code='dk' WHERE id='quote-address'`)
	exec(`UPDATE carts SET shipping_address_id=NULL WHERE id=?`, ref.CartID)
	quotes(1) // Original no-address query has no geo restriction.
	exec(`UPDATE carts SET shipping_address_id='quote-address' WHERE id=?`, ref.CartID)
	if out, err := read(nil, "1"); err != nil || len(out) != 2 || len(out[0].Quotes) != 0 {
		t.Fatal("unregistered cart provider quoted")
	}
	if _, err := read(providers, "2"); err != cart.ErrConflict {
		t.Fatal("stale cart quoted")
	}
	exec(`UPDATE cart_addresses SET deleted_at=UTC_TIMESTAMP(6) WHERE id='quote-address'`)
	if _, err := read(providers, "1"); err != cart.ErrSchema {
		t.Fatal("deleted saved shipping address ignored")
	}
	exec(`UPDATE cart_addresses SET deleted_at=NULL,postal_code=REPEAT('x',4097) WHERE id='quote-address'`)
	if _, err := read(providers, "1"); err == nil {
		t.Fatal("oversized saved address silently truncated")
	}
	exec(`UPDATE cart_addresses SET postal_code='00123' WHERE id='quote-address'`)
	if _, err := cart.ReadManualShippingQuotes(ctx, db, "shop", ref, "1", providers, at); err != cart.ErrInput {
		t.Fatal("autocommit cart quote accepted")
	}
	if _, err := cart.ReadManualShippingQuotes(ctx, db, "shop", ref, "1", providers, time.Time{}); err != cart.ErrInput {
		t.Fatal("zero time cart quote accepted")
	}
	checkCartShippingSelection(t, ctx, db, ref, fingerprint)
	t.Log(`CART_SHIPPING_QUOTES_REHEARSAL {"persisted_address_channel_region_currency":true,"stored_region_changes_price":true,"exact_manual_price":true,"uncovered_profiles_retained":true,"custom_context_not_fabricated":true,"missing_address_original_geo_semantics":true,"deleted_and_oversized_address_rejected":true,"registered_providers_only":true,"stale_version_rejected":true,"saved_data_and_reservations_unchanged":true,"autocommit_and_zero_time_rejected":true,"inventory_checked":false,"shipping_saved":false,"checkout":false}`)
}
