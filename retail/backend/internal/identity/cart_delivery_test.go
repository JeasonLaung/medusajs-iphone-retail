package identity_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkDeliveryCommands(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, ref sessionstore.CartReference) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("owned delivery fixture failed: %v", err)
		}
	}
	exec(`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES('delivery-set','Delivery','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('delivery-pickup-set','Pickup','pickup',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('delivery-provider',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_shipping_profiles(id,name,type,created_at,updated_at) VALUES('delivery-profile','Delivery','default',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_shipping_profiles(product_id,shipping_profile_id,id,created_at,updated_at) VALUES('add-product','delivery-profile','delivery-product-profile',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_option_types(id,label,code,description,created_at,updated_at) VALUES('delivery-type','Shipping','shipping','Tracked delivery',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, set := range []string{"delivery-set", "delivery-pickup-set"} {
		exec(`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES(?,'Europe',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, set+"-zone", set)
		exec(`INSERT INTO fulfillment_geo_zones(id,type,country_code,service_zone_id,created_at,updated_at) VALUES(?,'country','dk',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, set+"-geo", set+"-zone")
		exec(`INSERT INTO fulfillment_location_sets(stock_location_id,fulfillment_set_id,id,created_at,updated_at) VALUES('add-location',?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, set, set+"-location")
	}
	exec(`INSERT INTO fulfillment_location_providers(stock_location_id,fulfillment_provider_id,id,created_at,updated_at) VALUES('add-location','delivery-provider','delivery-provider-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('delivery-price-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES('delivery-price','delivery-price-set','eur',7,'{"value":"7","precision":100}',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for i, option := range []string{"delivery-standard", "delivery-pickup"} {
		set := []string{"delivery-set", "delivery-pickup-set"}[i]
		exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,provider_id,shipping_profile_id,shipping_option_type_id,data,created_at,updated_at) VALUES(?,?,'flat',?,'delivery-provider','delivery-profile','delivery-type','{"private":"not-for-browser"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, option, option, set+"-zone")
		exec(`INSERT INTO fulfillment_option_price_sets(shipping_option_id,price_set_id,id,created_at,updated_at) VALUES(?,'delivery-price-set',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, option, option+"-price")
	}
	exec("UPDATE cart_line_items SET requires_shipping=TRUE WHERE cart_id=?", ref.CartID)
	providers := []string{"delivery-provider"}
	fingerprint := func() string {
		t.Helper()
		all := []json.RawMessage{}
		for _, table := range []string{"carts", "cart_addresses", "cart_ownership", "cart_shipping_command_operations", "cart_line_items", "cart_line_tax_lines", "cart_line_adjustments", "cart_promotions", "cart_shipping_methods", "cart_shipping_adjustments", "cart_shipping_tax_lines", "inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("delivery fingerprint failed")
			}
			raw, _ := json.Marshal(rows)
			all = append(all, raw)
		}
		raw, _ := json.Marshal(all)
		return string(raw)
	}
	read := func() cart.DeliveryOptions {
		t.Helper()
		before := fingerprint()
		out, err := cart.ReadDeliveryOptions(ctx, db, "shop", ref, providers)
		if err != nil || before != fingerprint() {
			t.Fatalf("delivery read failed or mutated: %v", err)
		}
		return out
	}
	options := read()
	if len(options.Options) != 2 || len(options.Selected) != 0 || options.Complete || options.Version != "5" {
		t.Fatal("initial delivery options differ")
	}
	kinds := map[string]bool{}
	for _, option := range options.Options {
		if option.Amount != "7" || option.InsufficientInventory || option.Description != "Tracked delivery" || option.ProfileID != "delivery-profile" {
			t.Fatal("delivery option projection differs")
		}
		kinds[option.Kind] = true
	}
	if !kinds["pickup"] || !kinds["shipping"] {
		t.Fatal("delivery grouping lost")
	}
	in := cart.ShippingCommand{OperationID: uuid.NewString(), Version: "5", OptionID: "delivery-standard"}
	if _, err := cart.SaveShipping(ctx, db, "shop", ref, in, providers); err != cart.ErrSchema {
		t.Fatal("disabled shipping command accepted")
	}
	exec("UPDATE cart_shipping_command_runtime SET enabled=TRUE WHERE id=1")
	var wg sync.WaitGroup
	results, errs := make([]cart.ShippingResult, 8), make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = cart.SaveShipping(ctx, db, "shop", ref, in, providers)
		}(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.Version != "6" || r.OptionID != in.OptionID {
			t.Fatalf("shipping concurrent receipt differs: %v", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("shipping applied more than once")
	}
	options = read()
	if !options.Complete || len(options.Selected) != 1 || options.Selected[0] != in.OptionID {
		t.Fatal("saved shipping not reflected")
	}
	amount, err := cart.ReadAmounts(ctx, db, "shop", ref)
	if err != nil || amount.Totals["total"] != "158.75" || amount.Totals["shipping_subtotal"] != "7" {
		t.Fatalf("shipping totals/tax differ: %v total=%s", err, amount.Totals["total"])
	}
	reject := func(input cart.ShippingCommand, want error) {
		t.Helper()
		before := fingerprint()
		if _, err := cart.SaveShipping(ctx, db, "shop", ref, input, providers); err != want {
			t.Fatalf("shipping rejection differs: %v want %v", err, want)
		}
		if before != fingerprint() {
			t.Fatal("shipping rejection mutated data")
		}
	}
	changed := in
	changed.OptionID = "delivery-pickup"
	reject(changed, cart.ErrConflict)
	stale := in
	stale.OperationID = uuid.NewString()
	reject(stale, cart.ErrConflict)
	next := cart.ShippingCommand{OperationID: uuid.NewString(), Version: "6", OptionID: "delivery-pickup"}
	missing := next
	missing.OptionID = "foreign-option"
	reject(missing, cart.ErrItemUnavailable)
	exec("UPDATE fulfillment_shipping_options SET price_type='calculated' WHERE id='delivery-standard'")
	reject(next, cart.ErrConfiguration)
	exec("UPDATE fulfillment_shipping_options SET price_type='flat' WHERE id='delivery-standard'")
	exec("UPDATE inventory_levels SET stocked_quantity=0 WHERE id='add-level'")
	unavailable := read()
	for _, option := range unavailable.Options {
		if !option.InsufficientInventory {
			t.Fatal("out of stock option enabled")
		}
	}
	reject(next, cart.ErrItemStock)
	exec("UPDATE inventory_levels SET stocked_quantity=10 WHERE id='add-level'")
	exec(`CREATE TRIGGER fail_delivery_receipt BEFORE INSERT ON cart_shipping_command_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned delivery receipt failure'`)
	reject(next, cart.ErrStorage)
	exec("DROP TRIGGER fail_delivery_receipt")
	after, err := cart.SaveShipping(ctx, db, "shop", ref, next, providers)
	if err != nil || after.Version != "7" || after.Replayed {
		t.Fatal("shipping retry failed")
	}
	var live int64
	if db.Table("cart_shipping_methods").Where("cart_id=? AND deleted_at IS NULL", ref.CartID).Count(&live).Error != nil || live != 1 {
		t.Fatal("profile replacement left duplicate methods")
	}
	checkDeliveryHTTP(t, ctx, db, sessions, ref, fingerprint)
	exec(`INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) VALUES(?,'delivery-payment','delivery-payment-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	paid := next
	paid.Version = "8"
	paid.OperationID = uuid.NewString()
	reject(paid, cart.ErrConflict)
	exec("DELETE FROM cart_payment_collections WHERE id='delivery-payment-link'")
	checkPromotionCommands(t, ctx, db, sessions)
	checkInitialPayments(t, ctx, db, sessions, ref)
	exec("UPDATE cart_shipping_command_runtime SET enabled=FALSE WHERE id=1")
	reject(in, cart.ErrSchema)
	t.Log(`CART_DELIVERY_REHEARSAL {"real_mysql_redis":true,"options_read_only":true,"exact_price_tax_refresh":true,"pickup_and_shipping":true,"current_stock_observation":true,"concurrent_attempts":8,"one_fresh_receipt":true,"replay_immutable":true,"profile_replacement":true,"stale_and_foreign_denied":true,"stock_failure_atomic":true,"late_receipt_failure_atomic":true,"payment_link_closed":true,"disabled_replay_denied":true,"http":true,"no_payment_no_order":true}`)
}
