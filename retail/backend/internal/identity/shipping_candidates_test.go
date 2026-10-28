package identity_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"gorm.io/gorm"
)

func checkShippingCandidates(t *testing.T, ctx context.Context, db *gorm.DB) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("shipping candidate artificial fixture failed")
		}
	}
	exec(`INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('candidate-location','Candidate',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('candidate-location-2','Second',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, id := range []string{"candidate-location", "candidate-location-2"} {
		exec(`INSERT INTO commerce_channel_locations(sales_channel_id,stock_location_id,id,created_at,updated_at) VALUES('channel',?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, id+"-channel")
	}
	exec(`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES('candidate-set','Candidate','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, id := range []string{"candidate-location", "candidate-location-2"} {
		exec(`INSERT INTO fulfillment_location_sets(stock_location_id,fulfillment_set_id,id,created_at,updated_at) VALUES(?,'candidate-set',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, id+"-set")
	}
	exec(`INSERT INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('candidate-provider',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_option_types(id,label,code,created_at,updated_at) VALUES('candidate-type','Shipping','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, kind := range []string{"country", "province", "city", "zip", "object", "none", "return", "hidden", "nested"} {
		zone := "candidate-" + kind + "-zone"
		exec(`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES(?,'Candidate','candidate-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, zone)
		exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,provider_id,shipping_option_type_id,created_at,updated_at) VALUES(?,'Candidate','flat',?,'candidate-provider','candidate-type',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, "candidate-"+kind, zone)
		if kind == "none" {
			continue
		}
		geoType := kind
		if kind == "object" {
			geoType = "zip"
		}
		if kind == "return" || kind == "hidden" || kind == "nested" {
			geoType = "country"
		}
		var province, city, postal any
		if geoType != "country" {
			province = "01"
		}
		if geoType == "city" || geoType == "zip" {
			city = "Århus"
		}
		if geoType == "zip" {
			postal = `"00123"`
		}
		if kind == "object" {
			postal = `{"eq":["00123"]}`
		}
		exec(`INSERT INTO fulfillment_geo_zones(id,type,country_code,province_code,city,postal_expression,service_zone_id,created_at,updated_at) VALUES(?,?,'dk',?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, "candidate-"+kind+"-geo", geoType, province, city, postal, zone)
	}
	for _, r := range []struct{ id, attr, value string }{{"return", "is_return", `"true"`}, {"hidden", "enabled_in_store", `"false"`}, {"nested", "address.country", `"dk"`}} {
		exec(`INSERT INTO fulfillment_option_rules(id,attribute,operator,value,shipping_option_id,created_at,updated_at) VALUES(?,?,'eq',?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, "candidate-"+r.id+"-rule", r.attr, r.value, "candidate-"+r.id)
	}
	input := fulfillment.OptionSelection{Channel: "channel", Address: fulfillment.ShippingAddress{Country: "dk", Province: "01", City: "Århus", Postal: "00123"}, Context: json.RawMessage(`{"address":{"country":"dk"},"is_return":"true","enabled_in_store":"false"}`)}
	read := func(c fulfillment.OptionSelection, want ...string) {
		t.Helper()
		var out []fulfillment.Candidate
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; out, err = fulfillment.ReadCandidates(tx, c); return err })
		if err != nil {
			t.Fatal("candidate selection failed")
		}
		got := []string{}
		for _, row := range out {
			got = append(got, row.ID)
			if row.ProviderID == nil || *row.ProviderID != "candidate-provider" || row.ProviderEnabled || !reflect.DeepEqual(row.LocationIDs, []string{"candidate-location", "candidate-location-2"}) {
				t.Fatal("candidate location/provider evidence changed")
			}
		}
		if want == nil {
			want = []string{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("artificial candidate IDs %v want %v", got, want)
		}
	}
	read(input, "candidate-city", "candidate-country", "candidate-nested", "candidate-province", "candidate-zip")
	// Omitted ancestor address fields do not become SQL NULL requirements.
	input.Address = fulfillment.ShippingAddress{Country: "dk", Postal: "00123"}
	read(input, "candidate-country", "candidate-nested", "candidate-zip")
	input.Address.Postal = "123"
	read(input, "candidate-country", "candidate-nested")
	input.Address.Country = "DK"
	read(input)
	input.Address = fulfillment.ShippingAddress{Country: "dk"}
	input.Return = true
	read(input, "candidate-country", "candidate-nested", "candidate-return")
	no := false
	input.EnabledInStore = &no
	read(input, "candidate-country", "candidate-hidden", "candidate-nested", "candidate-return")
	input.Return = false
	input.EnabledInStore = nil
	input.Address = fulfillment.ShippingAddress{}
	read(input, "candidate-city", "candidate-country", "candidate-nested", "candidate-none", "candidate-object", "candidate-province", "candidate-zip")
	input.Address = fulfillment.ShippingAddress{Country: "dk"}
	exec(`UPDATE fulfillment_geo_zones SET deleted_at=UTC_TIMESTAMP(6) WHERE id='candidate-country-geo'`)
	read(input, "candidate-nested")
	exec(`UPDATE fulfillment_option_rules SET deleted_at=UTC_TIMESTAMP(6) WHERE id='candidate-return-rule'`)
	read(input, "candidate-nested", "candidate-return")
	exec(`UPDATE commerce_channel_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE stock_location_id='candidate-location-2'`)
	// A directory may retain disabled provider evidence; it cannot grant an offer.
	var out []fulfillment.Candidate
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; out, err = fulfillment.ReadCandidates(tx, input); return err }); err != nil || len(out) != 2 || !reflect.DeepEqual(out[0].LocationIDs, []string{"candidate-location"}) {
		t.Fatal("deleted channel/location edge retained")
	}
	exec(`UPDATE fulfillment_location_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE stock_location_id='candidate-location'`)
	read(input)
	exec(`UPDATE fulfillment_location_sets SET deleted_at=NULL`)
	input.Channel = "missing"
	read(input)
	if _, err := fulfillment.ReadCandidates(db, input); err != fulfillment.ErrOptions {
		t.Fatal("autocommit candidate selection accepted")
	}
	input.Channel = "channel"
	readError := func(want error) {
		t.Helper()
		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := fulfillment.ReadCandidates(tx, input); return err }); err != want {
			t.Fatalf("candidate boundary got %v want %v", err, want)
		}
	}
	exec(`UPDATE fulfillment_option_rules SET value='null' WHERE id='candidate-nested-rule'`)
	readError(fulfillment.ErrRules)
	exec(`UPDATE fulfillment_option_rules SET value='"dk"' WHERE id='candidate-nested-rule'`)
	exec(`UPDATE fulfillment_service_zones SET deleted_at=UTC_TIMESTAMP(6) WHERE id='candidate-nested-zone'`)
	// Deleted zone cannot be revived through an otherwise active rule.
	var surviving []fulfillment.Candidate
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		surviving, err = fulfillment.ReadCandidates(tx, input)
		return err
	}); err != nil || len(surviving) != 1 || surviving[0].ID != "candidate-return" {
		t.Fatal("deleted zone included")
	}
	exec(`UPDATE fulfillment_service_zones SET deleted_at=NULL WHERE id='candidate-nested-zone'`)
	exec(`UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='channel'`)
	read(input)
	exec(`UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='channel'`)
	exec(`UPDATE fulfillment_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE id='candidate-set'`)
	read(input)
	exec(`UPDATE fulfillment_sets SET deleted_at=NULL WHERE id='candidate-set'`)
	exec(`UPDATE geo_runtime_schema SET reads_enabled=FALSE`)
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := fulfillment.ReadCandidates(tx, input); return err }); err != fulfillment.ErrGeoSchema {
		t.Fatal("disabled geo schema selected")
	}
	exec(`UPDATE geo_runtime_schema SET reads_enabled=TRUE`)
	t.Log(`SHIPPING_CANDIDATES_REHEARSAL {"channel_location_set_zone":true,"multiple_locations_retained":true,"address_granularity_or":true,"absent_ancestors_omitted":true,"postal_json_string_equality":true,"object_not_expression":true,"case_and_leading_zero_preserved":true,"default_context_not_overridden":true,"return_and_enabled_context":true,"nested_rules":true,"soft_deleted_geo_rule_and_edges":true,"missing_channel_empty":true,"autocommit_rejected":true,"schema_disabled_rejected":true,"provider_called":false,"inventory_checked":false,"checkout":false}`)
}
