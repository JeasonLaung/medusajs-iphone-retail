package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The existing CartRuntime fixture supplies an actual current product, stock,
// manual delivery provider and system tax. All new promotions/cart IDs are owned
// here; there is no external provider, worker, account or historical data.
func checkContextCheckout(t *testing.T, ctx context.Context, db *gorm.DB, fingerprint func() string) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatal("context checkout fixture", err)
		}
	}
	for _, table := range []string{"cart_contact_runtime", "cart_shipping_command_runtime", "cart_promotion_command_runtime"} {
		var enabled bool
		if db.Table(table).Select("enabled").Where("id=1").Scan(&enabled).Error != nil {
			t.Fatal("context checkout gate")
		}
		exec("UPDATE " + table + " SET enabled=TRUE WHERE id=1")
		defer exec("UPDATE "+table+" SET enabled=? WHERE id=1", enabled)
	}
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
		t.Fatal("context checkout price")
	}
	exec(`UPDATE pricing_prices SET amount=100,raw_amount='{"value":"100","precision":100}' WHERE id='add-single'`)
	defer exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
	var shippingPrice struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='delivery-price'").Take(&shippingPrice).Error != nil {
		t.Fatal("context checkout shipping price")
	}
	defer exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='delivery-price'", shippingPrice.Amount, shippingPrice.Raw)
	exec(`UPDATE pricing_prices SET amount=7,raw_amount='{"value":"7","precision":100}' WHERE id='delivery-price'`)
	for _, p := range []struct {
		id, code, currency string
		automatic          bool
		percent            int
	}{
		{"context-eur-promo", "CONTEXT_EUR10", "eur", false, 10},
		{"context-usd-auto", "CONTEXT_USD20", "usd", true, 20},
	} {
		exec(`INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES(?,?,?,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, p.id, p.code, p.automatic)
		exec(`INSERT INTO promotion_application_methods(id,promotion_id,type,target_type,allocation,value,raw_value,created_at,updated_at) VALUES(?,?,'percentage','items','across',?,JSON_OBJECT('value',CAST(? AS CHAR),'precision',100),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, p.id+"-method", p.id, p.percent, p.percent)
		exec(`INSERT INTO promotion_rules(id,attribute,operator,created_at,updated_at) VALUES(?,'currency_code','eq',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, p.id+"-rule")
		exec(`INSERT INTO promotion_rule_values(id,promotion_rule_id,value,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, p.id+"-value", p.id+"-rule", p.currency)
		exec(`INSERT INTO promotion_order_rules(promotion_id,promotion_rule_id) VALUES(?,?)`, p.id, p.id+"-rule")
		defer exec("UPDATE promotions SET status='inactive' WHERE id=?", p.id)
	}
	protected := func() string {
		t.Helper()
		var out []string
		for _, table := range []string{"inventory_levels", "inventory_reservations", "payment_collections", "payment_sessions", "orders", "promotion_usage_operations"} {
			var rows []map[string]any
			if db.Table(table).Find(&rows).Error != nil {
				t.Fatal("context checkout protected rows")
			}
			var encoded []string
			for _, row := range rows {
				b, err := json.Marshal(row)
				if err != nil {
					t.Fatal(err)
				}
				encoded = append(encoded, string(b))
			}
			sort.Strings(encoded)
			out = append(out, table+strings.Join(encoded, "\n"))
		}
		return strings.Join(out, "\n")
	}
	beforeProtected := protected()
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("owned context checkout")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("context checkout create", err)
	}
	ref := created.Reference
	providers := []string{"delivery-provider"}
	added, err := cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"})
	if err != nil {
		t.Fatal("context checkout add", err)
	}
	address := cart.ContactAddress{FirstName: "Context", LastName: "Buyer", Address1: "Artificial street", City: "Copenhagen", PostalCode: "0012", Country: "dk"}
	contact, err := cart.SaveContacts(ctx, db, "shop", ref, cart.ContactCommand{OperationID: uuid.NewString(), Version: added.Version, Email: "context-checkout@example.invalid", Shipping: address, Billing: address}, providers)
	if err != nil {
		t.Fatal("context checkout contacts", err)
	}
	shipping, err := cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: contact.Version, OptionID: "delivery-standard"}, providers)
	if err != nil {
		t.Fatal("context checkout shipping", err)
	}
	promo, err := cart.SavePromotions(ctx, db, "shop", ref, cart.PromotionCommand{OperationID: uuid.NewString(), Version: shipping.Version, Codes: []string{"CONTEXT_EUR10"}}, providers)
	if err != nil {
		t.Fatal("context checkout promotion", err)
	}
	amount := func(want string) {
		t.Helper()
		a, e := cart.ReadAmounts(ctx, db, "shop", ref)
		if e != nil || a.Totals["total"] != want {
			t.Fatalf("context checkout total got %s want %s: %v", a.Totals["total"], want, e)
		}
	}
	count := func(table, where string, want int64, args ...any) {
		t.Helper()
		var n int64
		if db.Table(table).Where(where, args...).Count(&n).Error != nil || n != want {
			t.Fatalf("context checkout rows %s want %d got %d", table, want, n)
		}
	}
	amount("121.25") // (100 - 10% + 7 delivery) * 1.25 DK system tax.
	var methodID, shippingID, billingID string
	if db.Table("cart_shipping_methods").Select("id").Where("cart_id=? AND deleted_at IS NULL", ref.CartID).Scan(&methodID).Error != nil || methodID == "" {
		t.Fatal("context checkout method identity")
	}
	if db.Table("carts").Select("shipping_address_id").Where("id=?", ref.CartID).Scan(&shippingID).Error != nil || shippingID == "" {
		t.Fatal("context checkout address")
	}
	if db.Table("carts").Select("billing_address_id").Where("id=?", ref.CartID).Scan(&billingID).Error != nil || billingID == "" {
		t.Fatal("context checkout billing")
	}
	exec(`UPDATE pricing_prices SET amount=8,raw_amount='{"value":"8","precision":100}' WHERE id='delivery-price'`)
	in := cart.ContextCommand{OperationID: uuid.NewString(), Version: promo.Version, RegionID: "region", Locale: "zh-CN"}
	r, err := cart.SaveContext(ctx, db, "shop", ref, in, providers)
	if err != nil || r.Version != "6" {
		t.Fatal("context checkout locale refresh", err)
	}
	amount("122.5") // Same ID/address retained; fresh delivery quote and tax.
	count("cart_shipping_methods", "id=? AND deleted_at IS NULL AND amount=8", 1, methodID)
	count("carts", "id=? AND shipping_address_id=? AND billing_address_id=?", 1, ref.CartID, shippingID, billingID)
	in = cart.ContextCommand{OperationID: uuid.NewString(), Version: r.Version, RegionID: "context-us", Locale: "en"}
	exec(`CREATE TRIGGER fail_context_checkout_receipt BEFORE INSERT ON cart_context_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned combined refresh rollback'`)
	before := fingerprint()
	if _, e := cart.SaveContext(ctx, db, "shop", ref, in, providers); e != cart.ErrStorage || before != fingerprint() {
		t.Fatalf("context checkout full rollback %v", e)
	}
	exec("DROP TRIGGER fail_context_checkout_receipt")
	r, err = cart.SaveContext(ctx, db, "shop", ref, in, providers)
	if err != nil || r.Version != "7" {
		t.Fatal("context checkout currency refresh", err)
	}
	amount("36.9600000000000000000011") // Source allocator yields exactly 8.4 discount (original division precision);
	// the remaining raw quote fraction is retained for tax and total.
	count("cart_line_adjustments", "item_id=? AND code='CONTEXT_USD20' AND deleted_at IS NULL AND JSON_UNQUOTE(JSON_EXTRACT(raw_amount,'$.value'))='8.4'", 1, added.LineID)
	count("cart_shipping_methods", "id=? AND deleted_at IS NOT NULL", 1, methodID)
	count("cart_shipping_methods", "cart_id=? AND deleted_at IS NULL", 0, ref.CartID)
	count("cart_shipping_tax_lines", "shipping_method_id=? AND deleted_at IS NULL", 0, methodID)
	count("cart_promotions", "cart_id=? AND promotion_id='context-eur-promo' AND deleted_at IS NULL", 0, ref.CartID)
	count("cart_promotions", "cart_id=? AND promotion_id='context-usd-auto' AND deleted_at IS NULL", 1, ref.CartID)
	count("carts", "id=? AND billing_address_id=?", 1, ref.CartID, billingID)
	if again, e := cart.SaveContext(ctx, db, "shop", ref, in, providers); e != nil || !again.Replayed || again.Version != "7" {
		t.Fatal("context checkout replay", e)
	}
	amount("36.9600000000000000000011")
	draft, err := cart.ReadContacts(ctx, db, "shop", ref)
	if err != nil || draft.Shipping == nil || draft.Shipping.Country != "us" || draft.Shipping.FirstName != "" || draft.Shipping.Address1 != "" || draft.Billing == nil || *draft.Billing != address {
		t.Fatal("context checkout draft address cannot be read for completion", err)
	}
	before = fingerprint()
	if _, e := cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: r.Version, OptionID: "delivery-standard"}, providers); e != cart.ErrInput || before != fingerprint() {
		t.Fatalf("incomplete draft address reached shipping: %v", e)
	}
	in = cart.ContextCommand{OperationID: uuid.NewString(), Version: r.Version, RegionID: "context-multi", Locale: "en"}
	r, err = cart.SaveContext(ctx, db, "shop", ref, in, providers)
	if err != nil || r.Version != "8" {
		t.Fatal("context checkout clear address", err)
	}
	amount("100")
	count("carts", "id=? AND shipping_address_id IS NULL AND billing_address_id=?", 1, ref.CartID, billingID)
	count("cart_line_tax_lines", "item_id=? AND deleted_at IS NULL", 0, added.LineID)
	count("cart_promotions", "cart_id=? AND deleted_at IS NULL", 0, ref.CartID)
	count("promotions", "id IN ('context-eur-promo','context-usd-auto') AND used=0", 2)
	if beforeProtected != protected() {
		t.Fatal("context checkout touched stock/payment/order/usage facts")
	}
	t.Log(`CART_CONTEXT_CHECKOUT_REHEARSAL {"actual_mysql":true,"locale_reprices_existing_delivery_same_id_address":true,"currency_removes_ineligible_delivery_and_old_tax":true,"manual_and_automatic_currency_promotions":true,"exact_usd_discount_tax":true,"multi_country_clears_tax_and_promotions":true,"billing_preserved":true,"late_receipt_all_table_rollback":true,"replay_once":true,"protected_stock_payment_order_usage_rows_unchanged":true}`)
}
