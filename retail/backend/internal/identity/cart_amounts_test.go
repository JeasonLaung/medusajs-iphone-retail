package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCartAmounts(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("owned cart amount fixture failed")
		}
	}
	exec(`INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES ('eur','E','E',2,0,'{"value":"0","precision":20}','Euro',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("artificial amount scope")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("owned amount cart unavailable")
	}
	ref := created.Reference
	issued, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal("owned amount possession unavailable")
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: "http://127.0.0.1:18080", StoreID: "shop"})
	if err != nil {
		t.Fatal("owned amount HTTP unavailable")
	}
	fingerprint := func() string {
		t.Helper()
		all := []json.RawMessage{}
		for _, table := range []string{"carts", "cart_ownership", "cart_creation_operations", "cart_line_items", "cart_line_tax_lines", "cart_line_adjustments", "cart_shipping_methods", "cart_shipping_tax_lines", "cart_shipping_adjustments", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("owned amount fingerprint unavailable")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	call := func(path string, status int, cookie bool) []byte {
		t.Helper()
		before := fingerprint()
		r := httptest.NewRequest("GET", path, nil)
		if cookie {
			r.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: issued.Token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("cart amount status %d expected %d", w.Code, status)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("X-Request-ID") == "" || w.Header().Get("Set-Cookie") != "" || before != fingerprint() {
			t.Fatal("cart amount response cached, rotated or mutated")
		}
		for _, private := range []string{"snapshot-secret", "foreign-secret", "customer_id", "metadata", "generation", "raw_unit_price", "raw_amount", "provider_id", "email"} {
			if strings.Contains(w.Body.String(), private) {
				t.Fatal("cart amount private field escaped")
			}
		}
		return w.Body.Bytes()
	}
	path := "/api/store/carts/" + ref.CartID + "/amounts"
	check := func(want string) cart.AmountDetail {
		t.Helper()
		var result cart.AmountDetail
		b := call(path, 200, true)
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &result) != nil || json.Unmarshal(b, &fields) != nil || len(fields) != 8 || result.CartID != ref.CartID || result.Version != "1" || result.CurrencyCode != "eur" || result.Checkout || result.PricingRecalculated || result.Totals["total"] != want {
			t.Fatal("cart exact amount projection differs")
		}
		return result
	}
	if r := check("0"); len(r.Lines) != 0 || len(r.ShippingMethods) != 0 {
		t.Fatal("empty cart amounts differ")
	}
	exec(`INSERT INTO cart_line_items(id,cart_id,title,quantity,variant_id,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,is_custom_price,is_giftcard,metadata,created_at,updated_at) VALUES ('amount-cart-line',?,'Saved title',2,'retired-sku',TRUE,TRUE,FALSE,100,'{"value":"100","precision":20}',FALSE,FALSE,'{"private":"snapshot-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('amount-cart-deleted',?,'Deleted',1,NULL,TRUE,TRUE,FALSE,999,'{"value":"999","precision":20}',FALSE,FALSE,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID, ref.CartID)
	exec(`UPDATE cart_line_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='amount-cart-deleted'`)
	exec(`INSERT INTO cart_line_tax_lines(id,item_id,code,rate,metadata,created_at,updated_at) VALUES ('amount-cart-tax','amount-cart-line','VAT',25,'{"private":"snapshot-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('amount-cart-foreign-tax','amount-cart-deleted','VAT',999,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_line_adjustments(id,item_id,amount,raw_amount,is_tax_inclusive,metadata,created_at,updated_at) VALUES ('amount-cart-discount','amount-cart-line',20,'{"value":"20","precision":20}',FALSE,'{"private":"snapshot-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('amount-cart-deleted-discount','amount-cart-line',999,'{"value":"999","precision":20}',FALSE,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE cart_line_adjustments SET deleted_at=UTC_TIMESTAMP(6) WHERE id='amount-cart-deleted-discount'`)
	exec(`INSERT INTO cart_shipping_methods(id,cart_id,name,amount,raw_amount,is_tax_inclusive,metadata,created_at,updated_at) VALUES ('amount-cart-shipping',?,'Saved delivery',12.5,'{"value":"12.5","precision":20}',TRUE,'{"private":"snapshot-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	exec(`INSERT INTO cart_shipping_tax_lines(id,shipping_method_id,code,rate,metadata,created_at,updated_at) VALUES ('amount-cart-shipping-tax','amount-cart-shipping','VAT',25,'{"private":"snapshot-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_shipping_adjustments(id,shipping_method_id,amount,raw_amount,metadata,created_at,updated_at) VALUES ('amount-cart-shipping-discount','amount-cart-shipping',2,'{"value":"2","precision":20}','{"private":"snapshot-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	r := check("235")
	if len(r.Lines) != 1 || r.Lines[0].Quantity != "2" || r.Lines[0].UnitPrice != "100" || r.Lines[0].VariantID == nil || *r.Lines[0].VariantID != "retired-sku" || r.Lines[0].Totals["total"] != "225" || len(r.ShippingMethods) != 1 || r.ShippingMethods[0].Totals["total"] != "10" {
		t.Fatal("persisted line, tax or shipping semantics differ")
	}
	viewPath := "/api/store/carts/" + ref.CartID + "/view"
	exec(`UPDATE cart_line_items SET product_id='retired-product',product_title='Saved product',product_handle='saved-handle',thumbnail='https://example.com/saved.webp',variant_title='128 GB / S',variant_option_values='{"Storage":"128 GB","Grade":"S"}' WHERE id='amount-cart-line'`)
	var view cart.View
	if json.Unmarshal(call(viewPath, 200, true), &view) != nil || len(view.Items) != 1 || view.Items[0].ID != "amount-cart-line" || view.Items[0].Title != "Saved title" || view.Items[0].ProductID == nil || *view.Items[0].ProductID != "retired-product" || view.Items[0].ProductTitle == nil || *view.Items[0].ProductTitle != "Saved product" || view.Items[0].OptionValues["Grade"] != "S" || view.Amounts.Totals["total"] != "235" || view.Amounts.Version != "1" {
		t.Fatal("private saved cart presentation differs")
	}
	call(viewPath, 401, false)
	call(viewPath+"?fields=*", 400, true)
	call("/api/store/carts/legacy-cart/view", 404, true)
	exec(`UPDATE cart_line_items SET product_id=NULL WHERE id='amount-cart-line'`)
	if json.Unmarshal(call(viewPath, 200, true), &view) != nil || view.Items[0].ProductID != nil {
		t.Fatal("nullable saved cart product ID differs")
	}
	exec(`UPDATE cart_line_items SET product_id='../invalid' WHERE id='amount-cart-line'`)
	call(viewPath, 503, true)
	exec(`UPDATE cart_line_items SET product_id='retired-product' WHERE id='amount-cart-line'`)
	// A committed change between the amount read and display read must not
	// mix old amounts and new presentation in the same response.
	displayChanged := false
	if db.Callback().Row().After("gorm:row").Register("owned.cart-view-snapshot", func(query *gorm.DB) {
		if query.Statement.Table == "cart_line_items" && !displayChanged {
			displayChanged = true
			exec(`UPDATE cart_line_items SET title='Changed title' WHERE id='amount-cart-line'`)
		}
	}) != nil {
		t.Fatal("cart view snapshot observer unavailable")
	}
	inFlightView, viewErr := cart.ReadView(ctx, db, "shop", ref)
	if db.Callback().Row().Remove("owned.cart-view-snapshot") != nil {
		t.Fatal("cart view snapshot observer cleanup failed")
	}
	if viewErr != nil || !displayChanged || len(inFlightView.Items) != 1 || inFlightView.Items[0].Title != "Saved title" {
		t.Fatal("cart view mixed concurrent display snapshot")
	}
	exec(`UPDATE cart_line_items SET title='Saved title',variant_option_values='[]' WHERE id='amount-cart-line'`)
	call(viewPath, 503, true)
	exec(`UPDATE cart_line_items SET variant_option_values=NULL WHERE id='amount-cart-line'`)
	view = cart.View{}
	if json.Unmarshal(call(viewPath, 200, true), &view) != nil || view.Items[0].OptionValues == nil || len(view.Items[0].OptionValues) != 0 {
		t.Fatal("nullable saved options not preserved")
	}
	// Raw values outrun DECIMAL(38,18); do not silently quantize to SQL scale.
	exec(`UPDATE cart_line_items SET unit_price=9007199254740993,raw_unit_price='{"value":"9007199254740993.000000000000000000001","precision":38}' WHERE id='amount-cart-line'`)
	check("22517998136852467.5000000000000000000025")
	// Commit a tax change on another SQL connection after the line SELECT has
	// executed and before the relation SELECT. The in-flight projection must
	// retain its original repeatable-read snapshot, not mix new tax with old data.
	changed := false
	if db.Callback().Row().After("gorm:row").Register("owned.cart-amount-snapshot", func(query *gorm.DB) {
		if query.Statement.Table == "cart_line_items" && !changed {
			changed = true
			exec(`UPDATE cart_line_tax_lines SET rate=50 WHERE id='amount-cart-tax'`)
		}
	}) != nil {
		t.Fatal("owned snapshot observation unavailable")
	}
	inFlight, readErr := cart.ReadAmounts(ctx, db, "shop", ref)
	if db.Callback().Row().Remove("owned.cart-amount-snapshot") != nil {
		t.Fatal("owned snapshot observation cleanup failed")
	}
	if readErr != nil || !changed || inFlight.Totals["total"] != "22517998136852467.5000000000000000000025" {
		t.Fatal("cart amounts mixed concurrent tax snapshot")
	}
	after, readErr := cart.ReadAmounts(ctx, db, "shop", ref)
	if readErr != nil || after.Totals["total"] == inFlight.Totals["total"] {
		t.Fatal("independent committed tax change was not observed on next read")
	}
	exec(`UPDATE cart_line_tax_lines SET rate=25 WHERE id='amount-cart-tax'`)
	for _, fault := range []struct{ bad, restore string }{
		{`UPDATE cart_line_items SET raw_unit_price='{"value":"1","value":"2","precision":20}' WHERE id='amount-cart-line'`, `UPDATE cart_line_items SET raw_unit_price='{"value":"9007199254740993.000000000000000000001","precision":38}' WHERE id='amount-cart-line'`},
		{`UPDATE cart_line_items SET quantity=0 WHERE id='amount-cart-line'`, `UPDATE cart_line_items SET quantity=2 WHERE id='amount-cart-line'`},
		{`UPDATE cart_line_items SET raw_unit_price='{"value":"-1","precision":20}' WHERE id='amount-cart-line'`, `UPDATE cart_line_items SET raw_unit_price='{"value":"9007199254740993.000000000000000000001","precision":38}' WHERE id='amount-cart-line'`},
		{`UPDATE commerce_currencies SET deleted_at=UTC_TIMESTAMP(6) WHERE code='eur'`, `UPDATE commerce_currencies SET deleted_at=NULL WHERE code='eur'`},
		{`UPDATE cart_shipping_methods SET raw_amount='{}' WHERE id='amount-cart-shipping'`, `UPDATE cart_shipping_methods SET raw_amount='{"value":"12.5","precision":20}' WHERE id='amount-cart-shipping'`},
	} {
		exec(fault.bad)
		call(path, 503, true)
		exec(fault.restore)
	}
	// Never truncate the first 1,000 lines or first 100 relations and return an
	// apparently successful smaller total. Limits reject the whole projection.
	values, args := []string{}, []any{}
	for i := 0; i < 1000; i++ {
		values = append(values, `(?,?,'Limit',1,TRUE,TRUE,FALSE,1,'{"value":"1","precision":20}',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		args = append(args, fmt.Sprintf("amount-limit-%04d", i), ref.CartID)
	}
	exec(`INSERT INTO cart_line_items(id,cart_id,title,quantity,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,is_custom_price,is_giftcard,created_at,updated_at) VALUES `+strings.Join(values, ","), args...)
	call(path, 503, true)
	exec(`DELETE FROM cart_line_items WHERE cart_id=? AND id LIKE 'amount-limit-%'`, ref.CartID)
	for i := 0; i < 100; i++ {
		exec(`INSERT INTO cart_line_tax_lines(id,item_id,code,rate,created_at,updated_at) VALUES (?,'amount-cart-line','VAT',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, fmt.Sprintf("amount-limit-tax-%03d", i))
	}
	call(path, 503, true)
	exec(`DELETE FROM cart_line_tax_lines WHERE item_id='amount-cart-line' AND id LIKE 'amount-limit-tax-%'`)
	check("22517998136852467.5000000000000000000025")
	call(path, 401, false)
	call(path+"?fields=*", 400, true)
	call("/api/store/carts/legacy-cart/amounts", 404, true)
	exec(`UPDATE cart_ownership SET generation=REPEAT('d',64) WHERE cart_id=?`, ref.CartID)
	call(path, 404, true)
	call(viewPath, 404, true)
	if sessions.RevokeCart(ctx, issued.Token) != nil {
		t.Fatal("owned amount credential cleanup failed")
	}
	t.Log(`CART_AMOUNTS_REHEARSAL {"real_gin_mysql_redis":true,"empty_and_persisted_snapshots":true,"exact_raw_beyond_sql_scale":true,"tax_discount_shipping":true,"deleted_relations_filtered":true,"missing_catalog_variant_retained":true,"malformed_raw_and_currency_failure_closed":true,"line_and_relation_limits_reject_whole_result":true,"concurrent_tax_snapshot_consistent":true,"read_only_fingerprints":true,"current_authority":true,"private_projection":true,"saved_presentation_snapshot":true,"concurrent_presentation_snapshot":true,"pricing_recalculated":false,"checkout_integrated":false}`)
}
