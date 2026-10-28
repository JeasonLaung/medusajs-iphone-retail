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

func checkFlatShippingPrices(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("flat shipping artificial fixture failed")
		}
	}
	exec(`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES('flat-set','Shipping','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES('flat-zone','Zone','flat-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_option_types(id,label,code,created_at,updated_at) VALUES('flat-type','Standard','standard',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, id := range []string{"flat-a", "flat-b", "calculated", "unpriced", "deleted"} {
		kind := "flat"
		if id == "calculated" {
			kind = "calculated"
		}
		exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,shipping_option_type_id,created_at,updated_at) VALUES(?,'Shipping',?,'flat-zone','flat-type',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, kind)
		if id != "unpriced" {
			exec(`INSERT INTO fulfillment_option_price_sets(shipping_option_id,price_set_id,id,created_at,updated_at) VALUES(?,'set',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, id+"-price")
		}
	}
	exec(`UPDATE fulfillment_shipping_options SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted'`)
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	c := pricing.Context{Region: "region", Currency: "eur", Channel: "channel", Quantity: "1"}
	fingerprint := func() string {
		t.Helper()
		var all []json.RawMessage
		for _, table := range []string{"fulfillment_shipping_options", "fulfillment_option_price_sets", "pricing_price_sets", "pricing_prices", "pricing_price_rules", "pricing_price_lists", "pricing_price_list_rules", "pricing_preferences"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("flat shipping fingerprint unavailable")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	read := func(ids []string, context pricing.Context) (map[string]*pricing.Quote, error) {
		var out map[string]*pricing.Quote
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = fulfillment.ReadFlatPrices(tx, ids, context, at)
			return err
		})
		return out, err
	}
	before := fingerprint()
	if _, err := fulfillment.ReadFlatPrices(db, []string{"flat-a"}, c, at); err != pricing.ErrInput {
		t.Fatal("autocommit flat pricing accepted")
	}
	ids := []string{"flat-a", "flat-b", "calculated", "unpriced", "deleted", "missing"}
	quotes, err := read(ids, c)
	if err != nil || len(quotes) != 2 || quotes["flat-a"] == nil || quotes["flat-b"] == nil || quotes["flat-a"].Amount != "100.000000000000000000001" || !quotes["flat-a"].CalculatedTaxInclusive || fingerprint() != before {
		t.Fatal("shared flat price exactness or scope differs")
	}
	c.Quantity = "2"
	// Missing quantity differs from quantity=1: the original ignores maximum.
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,max_quantity,raw_max_quantity,created_at,updated_at) VALUES('flat-small-max','set','eur',4,'{"value":"4","precision":38}',0,0.5,'{"value":"0.5","precision":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	omitted := c
	omitted.Quantity = ""
	quotes, err = read(ids, omitted)
	if err != nil || quotes["flat-a"] == nil || quotes["flat-a"].Amount != "4" {
		t.Fatal("omitted flat quantity treated as one")
	}
	exec(`UPDATE pricing_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='flat-small-max'`)
	quotes, err = read(ids, c)
	if err != nil || quotes["flat-a"].Amount != "80.000000000000000000001" {
		t.Fatal("trusted flat quantity context ignored")
	}
	c.Region = "region-other"
	quotes, err = read(ids, c)
	if err != nil || quotes["flat-a"].Amount != "30" {
		t.Fatal("trusted flat region context ignored")
	}
	c.Currency = "usd"
	quotes, err = read(ids, c)
	if err != nil || quotes["flat-a"] != nil || quotes["flat-b"] != nil {
		t.Fatal("missing flat currency fabricated")
	}
	if _, err := read([]string{"flat-a", "flat-a"}, c); err != pricing.ErrInput {
		t.Fatal("duplicate option accepted")
	}
	exec(`UPDATE pricing_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE id='set'`)
	quotes, err = read(ids, c)
	if err != nil || len(quotes) != 0 {
		t.Fatal("deleted shared set quoted")
	}
	exec(`UPDATE pricing_price_sets SET deleted_at=NULL WHERE id='set'`)
	exec(`INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('ambiguous-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_option_price_sets(shipping_option_id,price_set_id,id,created_at,updated_at) VALUES('flat-a','ambiguous-set','ambiguous-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	before = fingerprint()
	if _, err := read(ids, c); err != pricing.ErrInput || fingerprint() != before {
		t.Fatal("ambiguous shipping price link accepted")
	}
	exec(`UPDATE fulfillment_option_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE id='ambiguous-link'`)
	c.Currency = "eur"
	quotes, err = read(ids, c)
	if err != nil || quotes["flat-a"] == nil || quotes["flat-a"].Amount != "30" {
		t.Fatal("deleted link still ambiguous")
	}
	exec(`UPDATE fulfillment_runtime_schema SET reads_enabled=FALSE WHERE id=1`)
	if _, err := read(ids, c); err != fulfillment.ErrSchema {
		t.Fatal("disabled fulfillment schema priced")
	}
	exec(`UPDATE fulfillment_runtime_schema SET reads_enabled=TRUE WHERE id=1`)
	t.Log(`FLAT_SHIPPING_PRICE_REHEARSAL {"shared_set":true,"omitted_quantity_ignores_maximum":true,"exact_raw":true,"trusted_region_quantity_currency":true,"tax_inclusive_preference":true,"missing_not_zero":true,"calculated_excluded":true,"soft_deleted_option_link_set_excluded":true,"ambiguous_links_rejected":true,"readonly_fingerprint":true,"autocommit_rejected":true,"schema_disabled_rejected":true,"eligibility_checked":false,"provider_called":false,"checkout":false}`)
}
