package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Runs inside the owned native-FK CartRuntime fixture after its catalog tests.
// All context commands use a new cart, with no provider or customer credentials.
func checkCartContext(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("context artificial SQL", e)
		}
	}
	fingerprint := func() string {
		t.Helper()
		var tables []string
		if db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name").Scan(&tables).Error != nil {
			t.Fatal("context table inventory")
		}
		var all []string
		for _, table := range tables {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("context fingerprint", table)
			}
			var encoded []string
			for _, row := range rows {
				b, e := json.Marshal(row)
				if e != nil {
					t.Fatal(e)
				}
				encoded = append(encoded, string(b))
			}
			sort.Strings(encoded)
			b, _ := json.Marshal(encoded)
			all = append(all, table+string(b))
		}
		return strings.Join(all, "\n")
	}
	created, e := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("owned context cart")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if e != nil {
		t.Fatal("context create", e)
	}
	ref := created.Reference
	in := cart.ContextCommand{OperationID: uuid.NewString(), Version: "1", RegionID: "context-us", Locale: "zh-CN"}
	reject := func(command cart.ContextCommand, want error) {
		t.Helper()
		before := fingerprint()
		if _, e := cart.SaveContext(ctx, db, "shop", ref, command, nil); e != want {
			t.Fatalf("context rejection want %v got %v", want, e)
		}
		if before != fingerprint() {
			t.Fatal("rejected context changed a table")
		}
	}
	reject(in, cart.ErrSchema)
	exec("UPDATE cart_context_runtime SET enabled=TRUE WHERE id=1")
	reject(in, cart.ErrRegion)
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('context-us','Context US','usd',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('context-multi','Context multi','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	reject(in, cart.ErrRegion) // target currency must belong to this store
	exec(`INSERT INTO commerce_store_currencies(id,store_id,currency_code,is_default,created_at,updated_at) VALUES('context-usd','shop','usd',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_countries(iso_2,iso_3,num_code,name,display_name,region_id,created_at,updated_at) VALUES('us','usa','840','US','US','context-us',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('fr','fra','250','France','France','context-multi',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('de','deu','276','Germany','Germany','context-multi',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE region_id=VALUES(region_id),deleted_at=NULL`)
	exec("UPDATE cart_item_runtime SET enabled=TRUE WHERE id=1")
	added, e := cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"})
	if e != nil {
		t.Fatal("context item", e)
	}
	in.Version = added.Version
	// No USD quote: changing region, address and item flags must all roll back.
	reject(in, cart.ErrItemUnavailable)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES('context-dollar','add-set','usd',42,'{"value":"42.000000000000000000001","precision":38}',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_regions(id,country_code,provider_id,created_at,updated_at) VALUES('context-tax','us','tp_system',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_rates(id,rate,name,code,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES('context-rate',10,'Context tax','CTX',TRUE,FALSE,'context-tax',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	// A second custom-priced item is deleted according to the original workflow.
	exec(`INSERT INTO cart_line_items(id,cart_id,title,quantity,unit_price,raw_unit_price,is_custom_price,is_tax_inclusive,is_discountable,requires_shipping,is_giftcard,metadata,created_at,updated_at) VALUES('context-custom',?,'Custom',1,7,'{"value":"7","precision":100}',TRUE,FALSE,TRUE,TRUE,FALSE,'{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	// Missing currency precision must fail without committing a partial context.
	reject(in, cart.ErrSchema)
	exec(`INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('usd','$','$',2,0,'{"value":"0","precision":20}','US dollar',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`CREATE TRIGGER fail_context_receipt BEFORE INSERT ON cart_context_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned receipt failure'`)
	reject(in, cart.ErrStorage)
	exec("DROP TRIGGER fail_context_receipt")
	results, errs := make([]cart.ContextResult, 8), make([]error, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = cart.SaveContext(ctx, db, "shop", ref, in, nil) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.CartID != ref.CartID || r.Version != "3" {
			t.Fatalf("context concurrent result %+v %v", r, errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("context applied more than once")
	}
	snap, e := cart.Inspect(ctx, db, "shop", ref)
	if e != nil || snap.RegionID != "context-us" || snap.CurrencyCode != "usd" || snap.Locale != "zh-CN" || snap.Version != "3" {
		t.Fatal("context not persisted", snap, e)
	}
	var address struct {
		ShippingAddressID string
		BillingAddressID  *string
	}
	if db.Table("carts").Select("shipping_address_id,billing_address_id").Where("id=?", ref.CartID).Take(&address).Error != nil || address.ShippingAddressID == "" || address.BillingAddressID != nil {
		t.Fatal("context address policy")
	}
	var country string
	if db.Table("cart_addresses").Select("country_code").Where("id=?", address.ShippingAddressID).Scan(&country).Error != nil || country != "us" {
		t.Fatal("single-country address missing")
	}
	var removed int64
	if db.Table("cart_line_items").Where("id='context-custom' AND deleted_at IS NOT NULL").Count(&removed).Error != nil || removed != 1 {
		t.Fatal("custom-price line retained")
	}
	amounts, e := cart.ReadAmounts(ctx, db, "shop", ref)
	if e != nil || amounts.Totals["total"] != "46.2000000000000000000011" {
		t.Fatal("context exact USD and tax", amounts.Totals, e)
	}
	changed := in
	changed.Locale = "en"
	reject(changed, cart.ErrConflict)
	changed.OperationID = uuid.NewString()
	reject(changed, cart.ErrConflict)
	changed.Version = "3"
	changed.Locale = "fr-FR"
	reject(changed, cart.ErrRegion)
	changed.Locale = "en"
	r, e := cart.SaveContext(ctx, db, "shop", ref, changed, nil)
	if e != nil || r.Version != "4" {
		t.Fatal("same region locale", e)
	}
	var retained string
	if db.Table("carts").Select("shipping_address_id").Where("id=?", ref.CartID).Scan(&retained).Error != nil || retained != address.ShippingAddressID {
		t.Fatal("locale change replaced address")
	}
	changed.OperationID = uuid.NewString()
	changed.Version = "4"
	changed.RegionID = "context-multi"
	r, e = cart.SaveContext(ctx, db, "shop", ref, changed, nil)
	if e != nil || r.Version != "5" {
		t.Fatal("multi-country context", e)
	}
	var cleared sql.NullString
	if db.Table("carts").Select("shipping_address_id").Where("id=?", ref.CartID).Scan(&cleared).Error != nil || cleared.Valid {
		t.Fatal("multi-country old address retained")
	}
	var activeTaxes int64
	if db.Table("cart_line_tax_lines").Where("item_id=? AND deleted_at IS NULL", added.LineID).Count(&activeTaxes).Error != nil || activeTaxes != 0 {
		t.Fatal("multi-country unaddressed context retained tax")
	}
	// Frozen provider state must refuse before any local mutation.
	exec(`INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) VALUES(?,'context-payment','context-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	changed.OperationID = uuid.NewString()
	changed.Version = "5"
	changed.RegionID = "context-us"
	reject(changed, cart.ErrConflict)
	exec("DELETE FROM cart_payment_collections WHERE id='context-link'")
	checkCartContextHTTP(t, ctx, db, sessions, ref, fingerprint)
	bad := ref
	bad.Generation = strings.Repeat("0", 64)
	before := fingerprint()
	if _, e = cart.SaveContext(ctx, db, "shop", bad, in, nil); e != cart.ErrDenied || before != fingerprint() {
		t.Fatal("foreign generation replay")
	}
	checkContextCheckout(t, ctx, db, fingerprint)
	exec("UPDATE cart_context_runtime SET enabled=FALSE WHERE id=1")
	reject(in, cart.ErrSchema)
	exec("UPDATE cart_item_runtime SET enabled=FALSE WHERE id=1")
	t.Log(`CART_CONTEXT_REHEARSAL {"real_mysql":true,"concurrent_attempts":8,"fresh_receipt":1,"source_custom_line_deletion":true,"source_single_and_multi_country_address":true,"exact_currency_price_tax":true,"same_region_locale_preserves_address":true,"all_table_failure_fingerprints":true,"late_receipt_failure_atomic":true,"provider_attached_rejected":true,"http_integrated":true,"browser_integrated":false}`)
}
