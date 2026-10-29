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

func checkCartShippingMethods(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("shipping method artificial fixture failed")
		}
	}
	fingerprint := func() string {
		t.Helper()
		all := []json.RawMessage{}
		for _, table := range []string{"carts", "cart_ownership", "cart_shipping_methods", "cart_shipping_adjustments", "cart_shipping_tax_lines", "inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("shipping method fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	// A user may pick one profile at a time. The first choice is not a
	// complete shipping selection; adding the second must retain the first.
	initial := fingerprint()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
		providers := []string{"candidate-provider"}
		first, err := cart.ReplaceManualShippingMethods(ctx, tx, "shop", ref, "1", []cart.ManualShippingMethodInput{{OptionID: "candidate-nested", Data: json.RawMessage(`{"first":true}`)}}, providers, at)
		if err != nil || len(first) != 1 {
			t.Fatalf("first partial shipping choice failed: %v", err)
		}
		if _, err := cart.ReadSelectedManualShipping(ctx, tx, "shop", ref, "1", []string{"candidate-nested"}, providers, at); err != cart.ErrItemUnavailable {
			t.Fatal("partial shipping claimed full coverage")
		}
		second, err := cart.ReplaceManualShippingMethods(ctx, tx, "shop", ref, "1", []cart.ManualShippingMethodInput{{OptionID: "selected-other"}}, providers, at)
		if err != nil || len(second) != 1 {
			t.Fatalf("second partial shipping choice failed: %v", err)
		}
		var savedData string
		if tx.Table("cart_shipping_methods").Select("data").Where("id=? AND deleted_at IS NULL", first[0].ID).Scan(&savedData).Error != nil || savedData != `{"first":true}` {
			t.Fatal("second choice replaced unrelated first method")
		}
		full, err := cart.ReadSelectedManualShipping(ctx, tx, "shop", ref, "1", []string{"candidate-nested", "selected-other"}, providers, at)
		if err != nil || !full.Complete || !full.Stock.CanFulfill {
			t.Fatal("merged shipping choices not complete")
		}
		return errors.New("artificial partial selection rollback")
	})
	if err == nil || initial != fingerprint() {
		t.Fatal("partial shipping rollback left state")
	}
	exec(`INSERT INTO fulfillment_shipping_profiles(id,name,type,created_at,updated_at) VALUES('unrelated-profile','Unrelated','default',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,provider_id,shipping_profile_id,shipping_option_type_id,created_at,updated_at) VALUES('unrelated-option','Unrelated','flat','candidate-nested-zone','candidate-provider','unrelated-profile','candidate-type',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, row := range []struct {
		id     string
		option any
	}{{"old-shipping-one", "candidate-nested"}, {"old-shipping-two", "selected-other"}, {"old-shipping-unknown", nil}, {"old-shipping-unrelated", "unrelated-option"}} {
		exec(`INSERT INTO cart_shipping_methods(id,cart_id,name,amount,raw_amount,is_tax_inclusive,shipping_option_id,data,created_at,updated_at) VALUES(?,?,'Historical method',7,'{"value":"7.000000000000000000003","precision":38}',FALSE,?,'{"historical":true}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, row.id, ref.CartID, row.option)
	}
	exec(`INSERT INTO cart_shipping_adjustments(id,amount,raw_amount,shipping_method_id,created_at,updated_at) VALUES('historical-shipping-adjustment',1,'{"value":"1","precision":38}','old-shipping-one',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	methodData := json.RawMessage(`{"reference":"0000123","large":123456789012345678901234567890}`)
	exec(`UPDATE fulfillment_shipping_options SET data='{"private":"option-configuration"}' WHERE id IN ('candidate-nested','selected-other')`)
	input := []cart.ManualShippingMethodInput{{OptionID: "selected-other", Data: methodData}, {OptionID: "candidate-nested"}}
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	run := func(input []cart.ManualShippingMethodInput, version string, rollback bool) ([]cart.SavedShippingMethod, error) {
		var out []cart.SavedShippingMethod
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = cart.ReplaceManualShippingMethods(ctx, tx, "shop", ref, version, input, []string{"candidate-provider"}, at)
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
	before := fingerprint()
	if out, err := run(input, "1", true); err == nil || len(out) != 2 || before != fingerprint() {
		t.Fatal("later refresh failure retained shipping changes")
	}
	exec(`CREATE TRIGGER shipping_insert_failure BEFORE INSERT ON cart_shipping_methods FOR EACH ROW BEGIN IF NEW.shipping_option_id='selected-other' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial shipping insert failure'; END IF; END`)
	if _, err := run(input, "1", false); err != cart.ErrStorage || before != fingerprint() {
		t.Fatal("late insert failure left partial shipping replacement")
	}
	exec(`DROP TRIGGER shipping_insert_failure`)
	for _, raw := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`[]`), json.RawMessage(`{"broken":`)} {
		bad := append([]cart.ManualShippingMethodInput(nil), input...)
		bad[0].Data = raw
		if _, err := run(bad, "1", false); err != cart.ErrInput || before != fingerprint() {
			t.Fatal("invalid manual data changed shipping methods")
		}
	}
	if _, err := run(input, "2", false); err != cart.ErrConflict || before != fingerprint() {
		t.Fatal("stale shipping replacement changed state")
	}
	out, err := run(input, "1", false)
	if err != nil || len(out) != 2 {
		t.Fatalf("shipping method persistence failed: %v", err)
	}
	for _, method := range out {
		var saved struct {
			OptionID, Name, Amount, RawAmount, Data string
			Inclusive                               bool
		}
		if db.Table("cart_shipping_methods").Select("shipping_option_id AS option_id,name,CAST(amount AS CHAR) AS amount,raw_amount,data,is_tax_inclusive AS inclusive").Where("id=? AND cart_id=? AND deleted_at IS NULL", method.ID, ref.CartID).Take(&saved).Error != nil {
			t.Fatal("saved shipping projection unavailable")
		}
		var raw struct{ Value string }
		if json.Unmarshal([]byte(saved.RawAmount), &raw) != nil || raw.Value != "100.000000000000000000001" || saved.Amount != "100.000000000000000000" || !saved.Inclusive || saved.OptionID != method.OptionID {
			t.Fatal("saved shipping amount lost exact value or tax preference")
		}
		want := "{}"
		wantName := "Candidate"
		if method.OptionID == "selected-other" {
			want = string(methodData)
			wantName = "Other"
		}
		if saved.Data != want || saved.Name != wantName {
			t.Fatal("manual data changed or private option data copied")
		}
	}
	var deleted, retained, adjustments int64
	db.Table("cart_shipping_methods").Where("id IN ? AND deleted_at IS NOT NULL", []string{"old-shipping-one", "old-shipping-two", "old-shipping-unknown"}).Count(&deleted)
	db.Table("cart_shipping_methods").Where("id='old-shipping-unrelated' AND deleted_at IS NULL").Count(&retained)
	db.Table("cart_shipping_adjustments").Where("id='historical-shipping-adjustment' AND deleted_at IS NULL").Count(&adjustments)
	if deleted != 3 || retained != 1 || adjustments != 1 {
		t.Fatal("collision replacement lost unrelated or historical evidence")
	}
	if state, err := cart.Inspect(ctx, db, "shop", ref); err != nil || state.Version != "1" {
		t.Fatal("persistence phase changed cart version")
	}
	if _, err := cart.ReplaceManualShippingMethods(ctx, db, "shop", ref, "1", input, []string{"candidate-provider"}, at); err != cart.ErrInput {
		t.Fatal("autocommit shipping persistence accepted")
	}
	// An unchanged profile is still revalidated. It cannot keep a stale
	// calculated/otherwise unregistered quote while another profile changes.
	exec(`UPDATE fulfillment_shipping_options SET price_type='calculated' WHERE id='selected-other'`)
	before = fingerprint()
	if _, err := run([]cart.ManualShippingMethodInput{{OptionID: "candidate-nested"}}, "1", false); err != cart.ErrItemUnavailable || before != fingerprint() {
		t.Fatal("retained stale profile was not revalidated")
	}
	exec(`UPDATE fulfillment_shipping_options SET price_type='flat' WHERE id='selected-other'`)
	t.Log(`CART_SHIPPING_METHODS_REHEARSAL {"partial_first_and_second_selection":true,"unchanged_method_id_and_data_retained":true,"partial_not_full_coverage":true,"retained_quote_revalidated":true,"exact_raw_amount_and_tax_preference":true,"server_quote_name_and_amount":true,"manual_data_preserved_without_number_coercion":true,"same_and_unknown_profiles_soft_deleted":true,"unrelated_methods_and_historical_children_retained":true,"later_refresh_and_late_insert_rollback":true,"invalid_data_and_stale_version_unchanged":true,"version_not_incremented_by_phase":true,"autocommit_rejected":true,"inventory_reserved":false,"full_refresh":false,"http_integrated":false,"checkout":false}`)
}
