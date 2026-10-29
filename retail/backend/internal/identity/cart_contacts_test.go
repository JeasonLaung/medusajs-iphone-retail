package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Reuses only the owned CartRuntime's artificial current catalog after item
// tests. No historical customer, external sender or payment provider is used.
func checkCartContacts(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.WithContext(ctx).Exec(q, args...).Error; err != nil {
			t.Fatalf("owned artificial contact fixture mutation failed: %v", err)
		}
	}
	exec("UPDATE cart_item_runtime SET enabled=TRUE WHERE id=1")
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("owned checkout contacts")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("contact cart creation failed")
	}
	ref := created.Reference
	blank, err := cart.ReadContacts(ctx, db, "shop", ref)
	if err != nil || blank.Email != "" || blank.Shipping != nil || blank.Billing != nil || blank.Version != "1" {
		t.Fatal("empty contact read differs")
	}
	added, err := cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"})
	if err != nil {
		t.Fatal("contact item creation failed")
	}
	exec(`INSERT INTO commerce_countries(iso_2,iso_3,num_code,name,display_name,region_id,created_at,updated_at) VALUES('dk','dnk','208','Denmark','Denmark','region',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE region_id='region'`)
	address := cart.ContactAddress{FirstName: "New", LastName: "Customer", Company: "", Address1: "Private street 1", City: "Copenhagen", PostalCode: "0012", Country: "dk", Phone: "+45 111"}
	in := cart.ContactCommand{OperationID: uuid.NewString(), Version: added.Version, Email: "contact@example.invalid", Shipping: address, Billing: address}
	if _, err := cart.SaveContacts(ctx, db, "shop", ref, in, nil); err != cart.ErrSchema {
		t.Fatal("disabled contact command accepted")
	}
	exec("UPDATE cart_contact_runtime SET enabled=TRUE WHERE id=1")
	// Tax fixture proves the persisted country is used by the enclosing refresh.
	exec(`INSERT INTO tax_providers(id,is_enabled,created_at,updated_at) VALUES('tp_system',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE is_enabled=TRUE`)
	exec(`INSERT INTO tax_regions(id,country_code,provider_id,created_at,updated_at) VALUES('contact-tax','dk','tp_system',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_rates(id,rate,name,code,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES('contact-rate',25,'VAT','VAT',TRUE,FALSE,'contact-tax',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	// An existing quote changes before contact save; the entire cart must reprice.
	exec(`UPDATE pricing_prices SET amount=120,raw_amount='{"value":"120","precision":100}' WHERE id='add-single'`)
	var wg sync.WaitGroup
	results, errs := make([]cart.ContactResult, 8), make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = cart.SaveContacts(ctx, db, "shop", ref, in, nil) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.CartID != ref.CartID || r.Version != "3" {
			t.Fatalf("contact concurrent command failed: %v", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("contact operation applied more than once")
	}
	var saved struct{ Email, ShippingAddressID, BillingAddressID string }
	if db.Table("carts").Select("email,shipping_address_id,billing_address_id").Where("id=?", ref.CartID).Take(&saved).Error != nil || saved.Email != in.Email || saved.ShippingAddressID == saved.BillingAddressID {
		t.Fatal("contact copies not saved")
	}
	var postal string
	if db.Table("cart_addresses").Select("postal_code").Where("id=?", saved.ShippingAddressID).Scan(&postal).Error != nil || postal != "0012" {
		t.Fatal("leading postal zero lost")
	}
	detail, err := cart.ReadContacts(ctx, db, "shop", ref)
	if err != nil || detail.Email != in.Email || detail.Shipping == nil || *detail.Shipping != in.Shipping || detail.Billing == nil || *detail.Billing != in.Billing || detail.Version != "3" {
		t.Fatal("owned contact detail differs")
	}
	amounts, err := cart.ReadAmounts(ctx, db, "shop", ref)
	if err != nil || amounts.Totals["total"] != "150" {
		t.Fatalf("contact refresh did not reprice and tax: %v total=%s", err, amounts.Totals["total"])
	}
	fingerprint := func() string {
		t.Helper()
		var all []json.RawMessage
		for _, table := range []string{"carts", "cart_addresses", "cart_ownership", "cart_contact_operations", "cart_line_items", "cart_line_tax_lines", "cart_line_adjustments", "cart_promotions", "cart_shipping_methods", "cart_shipping_tax_lines", "cart_shipping_adjustments"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("contact fingerprint failed")
			}
			raw, _ := json.Marshal(rows)
			all = append(all, raw)
		}
		out, _ := json.Marshal(all)
		return string(out)
	}
	reject := func(command cart.ContactCommand, want error) {
		t.Helper()
		before := fingerprint()
		if _, err := cart.SaveContacts(ctx, db, "shop", ref, command, nil); err != want {
			t.Fatalf("contact rejection differs: %v", err)
		}
		if before != fingerprint() {
			t.Fatal("rejected contact changed data")
		}
	}
	changed := in
	changed.Email = "other@example.invalid"
	reject(changed, cart.ErrConflict)
	stale := in
	stale.OperationID = uuid.NewString()
	reject(stale, cart.ErrConflict)
	wrong := in
	wrong.OperationID = uuid.NewString()
	wrong.Version = "3"
	wrong.Shipping.Country = "us"
	reject(wrong, cart.ErrRegion)
	wrong.Shipping.Country = "dk"
	exec("UPDATE commerce_countries SET deleted_at=UTC_TIMESTAMP(6) WHERE iso_2='dk'")
	reject(wrong, cart.ErrRegion)
	exec("UPDATE commerce_countries SET deleted_at=NULL WHERE iso_2='dk'")
	// Fail the final receipt after address copies/pricing/taxes: all must roll back.
	next := in
	next.OperationID = uuid.NewString()
	next.Version = "3"
	next.Shipping.Address1 = "Updated private street"
	// Missing current prices fail after address insertion and must roll it back.
	exec("UPDATE pricing_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE id='add-single'")
	reject(next, cart.ErrItemUnavailable)
	exec("UPDATE pricing_prices SET deleted_at=NULL WHERE id='add-single'")
	// An unimplemented delivery provider must not be silently removed.
	exec(`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES('contact-set','Contact','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_providers(id,is_enabled,created_at,updated_at) VALUES('contact-provider',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_option_types(id,label,code,created_at,updated_at) VALUES('contact-type','Shipping','contact',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES('contact-zone','Contact','contact-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,provider_id,shipping_option_type_id,created_at,updated_at) VALUES('contact-unsupported','Unsupported','flat','contact-zone','contact-provider','contact-type',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_shipping_methods(id,cart_id,name,amount,raw_amount,is_tax_inclusive,shipping_option_id,created_at,updated_at) VALUES('contact-method',?,'Unsupported delivery',7,'{"value":"7","precision":100}',FALSE,'contact-unsupported',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	reject(next, cart.ErrConfiguration)
	exec("DELETE FROM cart_shipping_methods WHERE id='contact-method'")
	exec("DELETE FROM fulfillment_shipping_options WHERE id='contact-unsupported'")
	exec("DELETE FROM fulfillment_service_zones WHERE id='contact-zone'")
	exec("DELETE FROM fulfillment_option_types WHERE id='contact-type'")
	exec("DELETE FROM fulfillment_providers WHERE id='contact-provider'")
	exec("DELETE FROM fulfillment_sets WHERE id='contact-set'")
	exec(`CREATE TRIGGER fail_contact_receipt BEFORE INSERT ON cart_contact_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned receipt failure'`)
	reject(next, cart.ErrStorage)
	exec("DROP TRIGGER fail_contact_receipt")
	after, err := cart.SaveContacts(ctx, db, "shop", ref, next, nil)
	if err != nil || after.Version != "4" || after.Replayed {
		t.Fatal("contact retry after rollback failed")
	}
	// A payment link is a hard boundary until provider refresh is attached.
	exec(`INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) VALUES(?,'contact-payment','contact-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	paid := next
	paid.OperationID = uuid.NewString()
	paid.Version = "4"
	reject(paid, cart.ErrConflict)
	exec("DELETE FROM cart_payment_collections WHERE id='contact-link'")
	var receipt map[string]any
	if db.Table("cart_contact_operations").Where("cart_id=?", ref.CartID).Take(&receipt).Error != nil {
		t.Fatal("contact receipt missing")
	}
	encoded, _ := json.Marshal(receipt)
	if strings.Contains(string(encoded), in.Email) || strings.Contains(string(encoded), address.Address1) {
		t.Fatal("private contact escaped receipt")
	}
	bad := ref
	bad.Generation = strings.Repeat("0", 64)
	if _, err := cart.SaveContacts(ctx, db, "shop", bad, in, nil); err != cart.ErrDenied {
		t.Fatal("wrong generation replay granted")
	}
	var count int64
	if db.Table("inventory_reservations").Where("line_item_id=?", added.LineID).Count(&count).Error != nil || count != 0 {
		t.Fatal("contact save reserved stock")
	}
	checkCartContactsHTTP(t, ctx, db, sessions, ref, next, fingerprint)
	checkDeliveryCommands(t, ctx, db, sessions, ref)
	exec("UPDATE cart_contact_runtime SET enabled=FALSE WHERE id=1")
	if _, err := cart.SaveContacts(ctx, db, "shop", ref, in, nil); err != cart.ErrSchema {
		t.Fatal("disabled contact replay accepted")
	}
	exec("UPDATE cart_item_runtime SET enabled=FALSE WHERE id=1")
	t.Log(`CART_CONTACT_REHEARSAL {"real_mysql":true,"concurrent_attempts":8,"single_fresh_receipt":true,"exact_replay":true,"price_and_tax_refresh":true,"missing_price_rollback":true,"unsupported_shipping_unchanged":true,"deleted_country_denied":true,"contact_copies":true,"private_current_contact_read":true,"leading_postal_zero":true,"late_receipt_failure_atomic":true,"retry_after_rollback":true,"stale_version_denied":true,"foreign_country_denied":true,"wrong_generation_denied":true,"payment_link_closed":true,"pii_absent_from_receipt":true,"disabled_replay_denied":true,"http_integrated":true,"checkout_page":false}`)
}
