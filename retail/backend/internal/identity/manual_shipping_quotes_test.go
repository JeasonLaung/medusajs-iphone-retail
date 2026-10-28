package identity_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/pricing"
	"gorm.io/gorm"
)

func checkManualShippingQuotes(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("manual shipping fixture failed")
		}
	}
	exec(`INSERT INTO fulfillment_shipping_profiles(id,name,type,created_at,updated_at) VALUES('manual-profile','Shipping','default',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-profile','Other','default',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE fulfillment_shipping_options SET shipping_profile_id='manual-profile' WHERE id='candidate-nested'`)
	exec(`INSERT INTO fulfillment_option_price_sets(shipping_option_id,price_set_id,id,created_at,updated_at) VALUES('candidate-nested','set','manual-quote-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	selection := fulfillment.OptionSelection{Channel: "channel", Address: fulfillment.ShippingAddress{Country: "dk"}, Context: json.RawMessage(`{"address":{"country":"dk"}}`)}
	c := pricing.Context{Region: "region", Currency: "eur", Channel: "channel"}
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	read := func(profiles, providers []string, context pricing.Context) ([]fulfillment.ManualFlatQuote, error) {
		var out []fulfillment.ManualFlatQuote
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = fulfillment.ReadManualFlatQuotes(tx, selection, profiles, providers, context, at)
			return err
		})
		return out, err
	}
	profiles, providers := []string{"manual-profile"}, []string{"candidate-provider"}
	empty := func() {
		t.Helper()
		if out, err := read(profiles, providers, c); err != nil || len(out) != 0 {
			t.Fatal("unavailable manual shipping quoted")
		}
	}
	empty() // Disabled provider, even with a real price.
	exec(`UPDATE fulfillment_providers SET is_enabled=TRUE WHERE id='candidate-provider'`)
	empty() // No provider/location association.
	exec(`INSERT INTO fulfillment_location_providers(stock_location_id,fulfillment_provider_id,id,created_at,updated_at) VALUES('candidate-location','candidate-provider','manual-provider-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	quote := func(want string) {
		t.Helper()
		out, err := read(profiles, providers, c)
		if err != nil || len(out) != 1 || out[0].Option.ID != "candidate-nested" || out[0].Price.Amount != want || !out[0].Price.CalculatedTaxInclusive || len(out[0].Option.LocationIDs) != 1 || out[0].Option.LocationIDs[0] != "candidate-location" {
			t.Fatal("manual exact quote/profile/location differs")
		}
	}
	quote("100.000000000000000000001")
	c.Region = "region-other"
	quote("30")
	c.Currency = "usd"
	empty()
	c.Currency = "eur"
	if out, err := read([]string{"other-profile"}, providers, c); err != nil || len(out) != 0 {
		t.Fatal("foreign profile quoted")
	}
	if out, err := read(profiles, []string{"unregistered"}, c); err != nil || len(out) != 0 {
		t.Fatal("unregistered provider executed")
	}
	if out, err := read(nil, providers, c); err != nil || len(out) != 0 {
		t.Fatal("empty shipping profile invented")
	}
	if _, err := read([]string{"missing-profile"}, providers, c); err != fulfillment.ErrOptions {
		t.Fatal("missing profile accepted")
	}
	exec(`UPDATE fulfillment_location_providers SET deleted_at=UTC_TIMESTAMP(6) WHERE id='manual-provider-link'`)
	empty()
	exec(`UPDATE fulfillment_location_providers SET deleted_at=NULL WHERE id='manual-provider-link'`)
	exec(`UPDATE fulfillment_shipping_options SET price_type='calculated' WHERE id='candidate-nested'`)
	empty()
	exec(`UPDATE fulfillment_shipping_options SET price_type='flat' WHERE id='candidate-nested'`)
	exec(`UPDATE fulfillment_shipping_profiles SET deleted_at=UTC_TIMESTAMP(6) WHERE id='manual-profile'`)
	if _, err := read(profiles, providers, c); err != fulfillment.ErrOptions {
		t.Fatal("deleted profile quoted")
	}
	exec(`UPDATE fulfillment_shipping_profiles SET deleted_at=NULL WHERE id='manual-profile'`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES('manual-free','set','eur',0,'{"value":"0","precision":38}',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	c.Region = "region"
	quote("0")
	exec(`UPDATE pricing_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='manual-free'`)
	quote("100.000000000000000000001")
	wrong := c
	wrong.Channel = "other"
	if _, err := read(profiles, providers, wrong); err != fulfillment.ErrOptions {
		t.Fatal("pricing/selection channel mismatch accepted")
	}
	wrong = c
	wrong.Quantity = "1"
	if _, err := read(profiles, providers, wrong); err != fulfillment.ErrOptions {
		t.Fatal("cart item quantity injected into shipping")
	}
	if _, err := read(append(profiles, profiles[0]), providers, c); err != fulfillment.ErrOptions {
		t.Fatal("duplicate profile accepted")
	}
	if _, err := fulfillment.ReadManualFlatQuotes(db, selection, profiles, providers, c, at); err != fulfillment.ErrOptions {
		t.Fatal("autocommit manual quote accepted")
	}
	t.Log(`MANUAL_SHIPPING_QUOTES_REHEARSAL {"active_provider_and_registration":true,"provider_location_link":true,"active_cart_profile":true,"exact_shared_price_and_region_currency":true,"zero_price_valid":true,"calculated_not_fabricated":true,"soft_deleted_profile_and_provider_link":true,"context_channel_bound":true,"omitted_quantity":true,"autocommit_rejected":true,"provider_called":false,"inventory_reserved":false,"cart_saved":false,"checkout":false}`)
}
