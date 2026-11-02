package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Uses the real, activated encrypted Mollie fixture created by the initial
// payment suite. The preview itself must never invoke that adapter or persist.
func checkContextPaymentPreview(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, options cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context payment preview fixture", err)
		}
	}
	fingerprint := func() [32]byte {
		t.Helper()
		var tables []string
		if db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name").Scan(&tables).Error != nil {
			t.Fatal("preview table inventory")
		}
		var all []string
		for _, table := range tables {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("preview full table fingerprint", table)
			}
			var encoded []string
			for _, row := range rows {
				raw, err := json.Marshal(row)
				if err != nil {
					t.Fatal(err)
				}
				encoded = append(encoded, string(raw))
			}
			sort.Strings(encoded)
			raw, _ := json.Marshal(encoded)
			all = append(all, table+string(raw))
		}
		return sha256.Sum256([]byte(strings.Join(all, "\n")))
	}
	command := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "region", Locale: "en"}
	preview := func(in cart.ContextCommand, authority sessionstore.CartReference, config cart.InitialPaymentOptions, want error) cart.ContextPaymentPreview {
		t.Helper()
		before := fingerprint()
		got, err := cart.PreviewContextPayment(ctx, db, "shop", authority, in, config)
		if err != want {
			t.Fatalf("context preview want %v got %v", want, err)
		}
		if want != nil && !reflect.DeepEqual(got, cart.ContextPaymentPreview{}) {
			t.Fatal("rejected preview exposed a target")
		}
		if fingerprint() != before {
			t.Fatal("context preview persisted a base-table change")
		}
		return got
	}
	preview(command, ref, options, cart.ErrSchema)
	exec("UPDATE cart_context_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_context_runtime SET enabled=FALSE WHERE id=1")
	got := preview(command, ref, options, nil)
	if got.CartID != ref.CartID || got.Version != "5" || got.RegionID != "region" || got.Currency != "eur" || got.Locale != "en" || got.CheckoutOperationID == "" || got.Payment.Amount != "158.75" || got.Payment.Changed || got.Payment.UpdateCollection || len(got.Payment.DeleteSessionIDs) != 0 || got.SourceSHA256 == ([32]byte{}) {
		t.Fatal("unchanged amount/currency did not retain existing session", got)
	}
	if again := preview(command, ref, options, nil); !reflect.DeepEqual(again, got) {
		t.Fatal("rolled-back preview was not repeatable")
	}
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
		t.Fatal("preview price fixture")
	}
	exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
	changed := preview(command, ref, options, nil)
	if !changed.Payment.UpdateCollection || changed.Payment.Amount != "160" || changed.Payment.Currency != "eur" || len(changed.Payment.DeleteSessionIDs) != 1 || changed.SourceSHA256 != got.SourceSHA256 {
		t.Fatal("same-currency new price did not plan source session deletion", changed.Payment)
	}
	exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
	exec(`INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('cad','$','$',2,0,'{"value":"0","precision":20}','Preview CAD',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_store_currencies(id,store_id,currency_code,is_default,created_at,updated_at) VALUES('preview-cad','shop','cad',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('preview-canada','Preview Canada','cad',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_countries(iso_2,iso_3,num_code,name,display_name,region_id,created_at,updated_at) VALUES('ca','can','124','Canada','Canada','preview-canada',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES('preview-cad-price','add-set','cad',158.75,'{"value":"158.75","precision":100}',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	foreign := command
	foreign.RegionID = "preview-canada"
	changed = preview(foreign, ref, options, nil)
	if !changed.Payment.UpdateCollection || changed.Payment.Amount != "158.75" || changed.Payment.Currency != "cad" || len(changed.Payment.DeleteSessionIDs) != 1 || changed.SourceSHA256 != got.SourceSHA256 {
		t.Fatal("currency-only change did not plan source session deletion", changed.Payment)
	}
	exec(`UPDATE pricing_prices SET amount=42,raw_amount='{"value":"42.000000000000000000001","precision":38}' WHERE id='preview-cad-price'`)
	changed = preview(foreign, ref, options, nil)
	if changed.Payment.Amount != "42.000000000000000000001" || changed.Currency != "cad" {
		t.Fatal("target raw precision lost", changed.Payment)
	}
	// A late failure after locale/item/delivery changes rolls back all of them.
	exec(`CREATE TRIGGER fail_context_preview_tax BEFORE INSERT ON cart_line_tax_lines FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned preview rollback'`)
	preview(command, ref, options, cart.ErrStorage)
	exec("DROP TRIGGER fail_context_preview_tax")
	bad := command
	bad.Version = "4"
	preview(bad, ref, options, cart.ErrConflict)
	bad = command
	bad.RegionID = "missing-region"
	preview(bad, ref, options, cart.ErrRegion)
	badRef := ref
	badRef.Generation = strings.Repeat("a", 64)
	preview(command, badRef, options, cart.ErrDenied)
	badOptions := options
	badOptions.Ring = nil
	preview(command, ref, badOptions, cart.ErrConfiguration)
	exec("UPDATE payment_sessions SET status='authorized' WHERE id=(SELECT session_id FROM cart_checkout_attempts WHERE cart_id=? AND operation_id=?)", ref.CartID, got.CheckoutOperationID)
	preview(command, ref, options, cart.ErrConflict)
	exec("UPDATE payment_sessions SET status='pending' WHERE id=(SELECT session_id FROM cart_checkout_attempts WHERE cart_id=? AND operation_id=?)", ref.CartID, got.CheckoutOperationID)
	before := fingerprint()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := cart.PreviewContextPayment(ctx, tx, "shop", ref, command, options); err != cart.ErrInput {
			t.Fatal("preview accepted a transaction it cannot own/rollback", err)
		}
		return nil
	}); err != nil || fingerprint() != before {
		t.Fatal("nested preview affected caller transaction", err)
	}
	// Public commands still cannot use the private preview capability.
	if _, err := cart.SaveContext(ctx, db, "shop", ref, command, options.ManualProviders); err != cart.ErrConflict || fingerprint() != before {
		t.Fatal("preview released the public checkout freeze", err)
	}
	checkContextPaymentStage(t, ctx, db, ref, options, fingerprint)
	t.Log(`CART_CONTEXT_PAYMENT_PREVIEW {"real_mysql":true,"activated_encrypted_checkout":true,"unchanged_session_retained":true,"changed_amount_and_currency_planned":true,"exact_raw_target":true,"all_base_tables_unchanged":true,"late_failure_rollback":true,"public_freeze_retained":true,"provider_calls":0,"context_save_connected":false}`)
}
