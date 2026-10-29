package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkPromotionAmounts(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if db.Exec(query, args...).Error != nil {
			t.Fatal("artificial promotion amount setup failed")
		}
	}
	read := func(reference sessionstore.CartReference, version string, wantErr bool) cart.AmountDetail {
		t.Helper()
		before := fingerprint()
		var result cart.AmountDetail
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.ReadPromotionAmounts(ctx, tx, "shop", reference, version)
			return err
		})
		if wantErr {
			if err == nil || !reflect.DeepEqual(result, cart.AmountDetail{}) {
				t.Fatal("invalid promotion amount facts leaked partial result")
			}
		} else if err != nil {
			t.Fatalf("promotion amount facts failed: %v", err)
		}
		if fingerprint() != before {
			t.Fatal("promotion amount read changed saved cart facts")
		}
		return result
	}
	if got, err := cart.ReadPromotionAmounts(ctx, db, "shop", ref, "1"); err == nil || !reflect.DeepEqual(got, cart.AmountDetail{}) {
		t.Fatal("autocommit promotion amount preparation accepted")
	}
	base := read(ref, "1", false)
	checkPromotionTargets(t, ctx, db, ref, fingerprint)
	checkPromotionContext(t, ctx, db, ref, fingerprint)
	checkPromotionRefresh(t, ctx, db, ref, fingerprint)
	if base.Totals["total"] != "262.5" || len(base.Lines) != 1 || base.Lines[0].Totals["subtotal"] != "200" || base.Lines[0].Totals["original_total"] != "250" || base.Lines[0].Totals["discount_total"] != "0" || len(base.ShippingMethods) != 1 || base.ShippingMethods[0].Totals["subtotal"] != "10" || base.ShippingMethods[0].Totals["original_total"] != "12.5" || base.PricingRecalculated || base.Checkout {
		t.Fatal("promotion preparation reused old discounts or lost tax bases")
	}
	read(ref, "2", true)
	wrong := ref
	wrong.Generation = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	read(wrong, "1", true)
	// Existing discounts are excluded entirely, even when their raw amount is
	// damaged. They remain stored and must be handled by a later refresh phase.
	exec(`UPDATE cart_line_adjustments SET raw_amount='{}' WHERE id='amount-cart-discount'`)
	if got := read(ref, "1", false); !reflect.DeepEqual(got, base) {
		t.Fatal("stale adjustment influenced undiscounted preparation")
	}
	exec(`UPDATE cart_line_adjustments SET raw_amount='{"value":"20","precision":20}' WHERE id='amount-cart-discount'`)
	exec(`UPDATE cart_line_items SET raw_unit_price='{"value":"9007199254740993.000000000000000000001","precision":38}' WHERE id='amount-cart-line'`)
	if got := read(ref, "1", false); got.Lines[0].Totals["subtotal"] != "18014398509481986.000000000000000000002" {
		t.Fatal("promotion monetary preparation rounded raw unit price")
	}
	exec(`UPDATE cart_line_items SET raw_unit_price='{"value":"100","precision":20}',is_tax_inclusive=TRUE WHERE id='amount-cart-line'`)
	if got := read(ref, "1", false); got.Lines[0].Totals["subtotal"] != "160" || got.Lines[0].Totals["original_total"] != "200" || got.Totals["total"] != "212.5" {
		t.Fatal("inclusive item promotion bases were not converted")
	}
	exec(`UPDATE cart_line_items SET is_tax_inclusive=FALSE WHERE id='amount-cart-line'`)
	// The helper must see earlier pricing/tax phase writes in the same owning
	// transaction, and must not commit those writes itself.
	rollback := errors.New("artificial later command failure")
	before := fingerprint()
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Exec("UPDATE cart_line_tax_lines SET rate=50 WHERE id='amount-cart-tax'").Error != nil {
			return errors.New("artificial tax update failed")
		}
		got, err := cart.ReadPromotionAmounts(ctx, tx, "shop", ref, "1")
		if err != nil || got.Totals["total"] != "312.5" {
			t.Fatal("promotion amount facts did not see caller refresh")
		}
		return rollback
	})
	if err != rollback || fingerprint() != before {
		t.Fatal("promotion amount preparation escaped caller rollback")
	}
	exec(`UPDATE cart_shipping_methods SET raw_amount='{}' WHERE id='amount-cart-shipping'`)
	read(ref, "1", true)
	exec(`UPDATE cart_shipping_methods SET raw_amount='{"value":"12.5","precision":20}' WHERE id='amount-cart-shipping'`)
	t.Log(`CART_PROMOTION_AMOUNTS_REHEARSAL {"actual_mysql":true,"undiscounted_item_and_shipping_tax_bases":true,"stale_adjustments_excluded_without_deletion":true,"current_authority_and_version":true,"caller_refresh_visible_and_rollback_atomic":true,"malformed_source_amount_rejected":true,"read_only_fingerprints":true,"target_rows_relations_and_existing_discounted_totals":true,"sql_targets_to_compute_and_adjustment_rollback":true,"payment_refresh_preparation_without_mutation":true,"order_address_region_and_payment_context":true,"anonymous_contact_does_not_restore_customer":true,"ambiguous_payment_collection_rejected":true,"public_http_or_checkout_integrated":false}`)
}

func checkPromotionContext(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	if plan, err := cart.PreparePaymentRefresh(ctx, db, "shop", ref, "1"); err == nil || !reflect.DeepEqual(plan, cart.PaymentRefreshPlan{}) {
		t.Fatal("autocommit payment preparation accepted")
	}
	data, err := os.ReadFile("../../../../docs/migration/fixtures/cart-promotion-context.json")
	var oracle struct {
		BaselineAmounts          map[string]string `json:"baseline_amounts"`
		Epsilon                  map[string]string
		QuantityRequested        bool `json:"quantity_requested"`
		ModelHasQuantity         bool `json:"model_has_quantity"`
		DecoratedCartHasQuantity bool `json:"decorated_cart_has_quantity"`
	}
	if err != nil || json.Unmarshal(data, &oracle) != nil || len(oracle.BaselineAmounts) != 7 || !oracle.QuantityRequested || oracle.ModelHasQuantity || oracle.DecoratedCartHasQuantity {
		t.Fatal("original order context evidence missing")
	}
	read := func(tx *gorm.DB, reference sessionstore.CartReference, version string, wantErr bool) (cart.PromotionContext, map[string]json.RawMessage) {
		t.Helper()
		got, err := cart.ReadPromotionContext(ctx, tx, "shop", reference, version)
		if wantErr {
			if err == nil || got.JSON != nil || got.CurrencyEpsilon != "" {
				t.Fatal("invalid order context returned partial private facts")
			}
			return got, nil
		}
		if err != nil {
			t.Fatalf("order context: %v", err)
		}
		var out map[string]json.RawMessage
		if json.Unmarshal(got.JSON, &out) != nil {
			t.Fatal("order context not JSON object")
		}
		return got, out
	}
	read(db, ref, "1", true)
	before := fingerprint()
	rollback := errors.New("artificial order context rollback")
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exec := func(query string, args ...any) {
			t.Helper()
			if tx.Exec(query, args...).Error != nil {
				t.Fatal("order context fixture failed")
			}
		}
		paymentPlan := func(wantErr bool) cart.PaymentRefreshPlan {
			t.Helper()
			plan, err := cart.PreparePaymentRefresh(ctx, tx, "shop", ref, "1")
			if wantErr {
				if err == nil || !reflect.DeepEqual(plan, cart.PaymentRefreshPlan{}) {
					t.Fatal("invalid payment preparation returned partial plan")
				}
			} else if err != nil {
				t.Fatalf("payment preparation: %v", err)
			}
			return plan
		}
		got, out := read(tx, ref, "1", false)
		if plan := paymentPlan(false); plan.ShouldExecute || plan.Changed || plan.UpdateCollection || plan.CollectionID != "" || plan.Amount != "235" || len(plan.DeleteSessionIDs) != 0 {
			t.Fatal("missing payment collection caused creation/update")
		}
		if got.CurrencyEpsilon != oracle.Epsilon["2"] || string(out["total"]) != "235" || string(out["subtotal"]) != "210" || string(out["original_item_total"]) != "250" || string(out["item_total"]) != "225" || string(out["item_subtotal"]) != "200" || string(out["shipping_subtotal"]) != "10" || string(out["customer"]) != "null" || string(out["shipping_address"]) != "null" || string(out["payment_collection"]) != "null" || string(out["promotions"]) != "[]" || string(out["email"]) != "null" {
			t.Fatal("order amount/null/empty facts changed")
		}
		for key, value := range oracle.BaselineAmounts {
			if string(out[key]) != value {
				t.Fatalf("original order amount changed: %s", key)
			}
		}
		for _, excluded := range []string{"quantity", "customer_id", "shipping_address_id", "billing_address", "version", "store_id", "generation", "tax_total", "raw_total"} {
			if _, exists := out[excluded]; exists {
				t.Fatalf("unselected order field leaked: %s", excluded)
			}
		}
		read(tx, ref, "2", true)
		wrong := ref
		wrong.StoreID = "other-shop"
		read(tx, wrong, "1", true)
		exec(`INSERT INTO cart_addresses(id,customer_id,first_name,country_code,city,metadata,created_at,updated_at) VALUES('context-address','historical-reference','Contact','dk','City','{"segment":"address-rule","numeric":9007199254740993}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`UPDATE carts SET shipping_address_id='context-address',email='Buyer@Example.test ',metadata='{"segment":"order-rule"}' WHERE id=?`, ref.CartID)
		exec(`UPDATE commerce_regions SET metadata='{"segment":"region-rule"}' WHERE id='region'`)
		exec(`INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at,deleted_at) VALUES('context-promotion','LINKED',FALSE,'standard','inactive',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('context-deleted-promotion','DELETED',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO cart_promotions(cart_id,promotion_id,id,created_at,updated_at) VALUES(?,'context-promotion','context-promo-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),(?,'context-deleted-promotion','context-deleted-promo-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID, ref.CartID)
		exec(`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,metadata,created_at,updated_at) VALUES('context-payment','usd',235,'{"value":"235.00000000000000000001","precision":100}','not_paid','{"private":"provider-secret"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('context-payment-two','eur',0,'{"value":"0","precision":20}','not_paid',NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) VALUES(?,'context-payment','context-payment-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
		exec(`INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,context,status,payment_collection_id,created_at,updated_at,deleted_at) VALUES('context-session','usd',235,'{"value":"235","precision":20}','artificial','{"token":"provider-secret"}','{"secret":"provider-secret"}','pending','context-payment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL),('context-deleted-session','usd',235,'{"value":"235","precision":20}','artificial','{}',NULL,'pending','context-payment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		got, out = read(tx, ref, "1", false)
		if plan := paymentPlan(false); !plan.ShouldExecute || !plan.Changed || !plan.UpdateCollection || plan.CollectionID != "context-payment" || plan.Amount != "235" || plan.Currency != "eur" || !reflect.DeepEqual(plan.DeleteSessionIDs, []string{"context-session"}) {
			t.Fatal("payment plan omitted exact amount/currency change or retained deleted session")
		}
		var unchangedPayment string
		if tx.Raw("SELECT currency_code FROM payment_collections WHERE id='context-payment'").Scan(&unchangedPayment).Error != nil || unchangedPayment != "usd" {
			t.Fatal("payment preparation changed collection")
		}
		exec(`UPDATE payment_collections SET currency_code='eur',raw_amount='{"value":"235.00000000000000000001","precision":100}' WHERE id='context-payment'`)
		if !paymentPlan(false).Changed {
			t.Fatal("sub-scalar raw difference ignored")
		}
		exec(`UPDATE payment_collections SET raw_amount='{"value":"2.35e2","precision":20}' WHERE id='context-payment'`)
		if plan := paymentPlan(false); plan.Changed || plan.UpdateCollection || len(plan.DeleteSessionIDs) != 0 {
			t.Fatal("equivalent exact payment amount invalidated sessions")
		}
		exec(`UPDATE payment_collections SET currency_code='usd',raw_amount='{"value":"235.00000000000000000001","precision":100}' WHERE id='context-payment'`)
		if string(out["customer"]) != "null" || string(out["email"]) != `"Buyer@Example.test "` || strings.Contains(string(got.JSON), "provider-secret") || strings.Contains(string(got.JSON), "context-deleted-session") || strings.Contains(string(got.JSON), "DELETED") {
			t.Fatal("contact identity/private payment/deletion semantics changed")
		}
		var collection map[string]json.RawMessage
		if json.Unmarshal(out["payment_collection"], &collection) != nil || len(collection) != 5 || string(collection["amount"]) != "235.00000000000000000001" || string(collection["currency_code"]) != `"usd"` || string(collection["payment_sessions"]) != `[{"id":"context-session"}]` {
			t.Fatal("selected payment graph lost raw precision or broadened private fields")
		}
		if !strings.Contains(string(out["shipping_address"]), `"numeric":9007199254740993`) {
			t.Fatal("address JSON numeric fact rounded during assembly")
		}
		for attribute, value := range map[string]string{"email": "Buyer@Example.test ", "total": "235", "shipping_address.country_code": "dk", "region.metadata.segment": "region-rule", "metadata.segment": "order-rule", "promotions.code": "LINKED", "payment_collection.payment_sessions.id": "context-session"} {
			raw, _ := json.Marshal(value)
			ok, err := promotion.MatchRules(got.JSON, []promotion.Rule{{Attribute: attribute, Operator: "in", Values: []promotion.RuleValue{{Value: raw}}}}, "")
			if err != nil || !ok {
				t.Fatalf("order rule path failed: %s", attribute)
			}
		}
		across := "across"
		computed, err := promotion.ComputeCandidates(promotion.ComputeInput{Candidates: []promotion.ActiveConfiguration{{ID: "context-rule-p", Code: "RULE", Type: "standard", Method: &promotion.ApplicationMethod{Type: "percentage", Target: "order", Value: "10", Allocation: &across}, Rules: []promotion.Rule{{Attribute: "shipping_address.country_code", Operator: "in", Values: []promotion.RuleValue{{Value: json.RawMessage(`"dk"`)}}}, {Attribute: "total", Operator: "gte", Values: []promotion.RuleValue{{Value: json.RawMessage(`"235"`)}}}}}}, Context: got.JSON, Epsilon: got.CurrencyEpsilon})
		if err != nil || len(computed.Actions) != 1 || computed.Actions[0].Amount != "20" || computed.Actions[0].ItemID != "amount-cart-line" {
			t.Fatalf("order SQL facts to original allocator: %v", err)
		}
		budget := promotion.ActiveConfiguration{ID: "context-rule-p", Code: "RULE", Type: "standard", Method: &promotion.ApplicationMethod{}, Budget: &promotion.CampaignBudget{ID: "context-budget", Type: "use_by_attribute", Attribute: func() *string { s := "customer_email"; return &s }()}}
		usages, err := promotion.ReadAttributeUsages(tx, []promotion.ActiveConfiguration{budget}, got.JSON)
		if err != nil || usages["context-budget"].Value != "Buyer@Example.test " || usages["context-budget"].Found {
			t.Fatal("cart contact attribute was changed or inferred from account")
		}
		idAttribute := "customer_id"
		budget.Budget.Attribute = &idAttribute
		if values, err := promotion.ReadAttributeUsages(tx, []promotion.ActiveConfiguration{budget}, got.JSON); err == nil || values != nil {
			t.Fatal("anonymous address historical actor granted customer identity")
		}
		exec(`UPDATE cart_addresses SET deleted_at=UTC_TIMESTAMP(6) WHERE id='context-address'`)
		_, out = read(tx, ref, "1", false)
		if string(out["shipping_address"]) != "null" {
			t.Fatal("deleted address survived")
		}
		exec(`UPDATE cart_promotions SET deleted_at=UTC_TIMESTAMP(6) WHERE id='context-promo-link'`)
		_, out = read(tx, ref, "1", false)
		if string(out["promotions"]) != "[]" {
			t.Fatal("deleted promotion link survived")
		}
		exec(`UPDATE payment_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE id='context-payment'`)
		if plan := paymentPlan(false); plan.ShouldExecute || plan.Changed || plan.CollectionID != "" {
			t.Fatal("deleted payment collection planned for refresh")
		}
		_, out = read(tx, ref, "1", false)
		if string(out["payment_collection"]) != "null" {
			t.Fatal("deleted payment collection survived")
		}
		exec(`UPDATE payment_collections SET deleted_at=NULL WHERE id='context-payment'`)
		exec(`INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) VALUES(?,'context-payment-two','context-payment-link-two',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
		read(tx, ref, "1", true)
		paymentPlan(true)
		exec(`UPDATE cart_payment_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE id='context-payment-link-two'`)
		exec(`UPDATE payment_collections SET raw_amount='{}' WHERE id='context-payment'`)
		paymentPlan(true)
		read(tx, ref, "1", true)
		exec(`UPDATE payment_collections SET raw_amount='{"value":"235","precision":20}' WHERE id='context-payment'`)
		exec(`UPDATE carts SET customer_id='historical-reference' WHERE id=?`, ref.CartID)
		read(tx, ref, "1", true)
		exec(`UPDATE carts SET customer_id=NULL WHERE id=?`, ref.CartID)
		for _, table := range []string{"payment_runtime_schema", "promotion_runtime_schema"} {
			exec("UPDATE " + table + " SET reads_enabled=FALSE")
			read(tx, ref, "1", true)
			if table == "payment_runtime_schema" {
				paymentPlan(true)
			}
			exec("UPDATE " + table + " SET reads_enabled=TRUE")
		}
		exec(`UPDATE commerce_currencies SET decimal_digits=0 WHERE code='eur'`)
		if result, _ := read(tx, ref, "1", false); result.CurrencyEpsilon != oracle.Epsilon["0"] {
			t.Fatal("zero-decimal epsilon changed")
		}
		exec(`UPDATE commerce_currencies SET decimal_digits=18 WHERE code='eur'`)
		if result, _ := read(tx, ref, "1", false); result.CurrencyEpsilon != oracle.Epsilon["18"] {
			t.Fatal("18-decimal epsilon changed")
		}
		return rollback
	})
	if err != rollback || before != fingerprint() {
		t.Fatalf("private order context escaped caller rollback: %v", err)
	}
}

func checkPromotionTargets(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, fingerprint func() string) {
	t.Helper()
	read := func(tx *gorm.DB, reference sessionstore.CartReference, version string, wantErr bool) cart.PromotionTargets {
		t.Helper()
		got, err := cart.ReadPromotionTargets(ctx, tx, "shop", reference, version)
		if wantErr {
			if err == nil || !reflect.DeepEqual(got, cart.PromotionTargets{}) {
				t.Fatal("invalid target read leaked partial facts")
			}
		} else if err != nil {
			t.Fatalf("promotion targets: %v", err)
		}
		return got
	}
	read(db, ref, "1", true)
	before := fingerprint()
	rollback := errors.New("artificial target chain rollback")
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exec := func(query string, args ...any) {
			t.Helper()
			if tx.Exec(query, args...).Error != nil {
				t.Fatal("target rule fixture failed")
			}
		}
		got := read(tx, ref, "1", false)
		if len(got.Items) != 1 || len(got.ShippingMethods) != 1 {
			t.Fatal("deleted target retained")
		}
		line, shipping := got.Items[0], got.ShippingMethods[0]
		if line["product"] != nil || line["variant"] != nil || shipping["shipping_option"] != nil || line["subtotal"] != json.Number("200") || line["original_total"] != json.Number("250") || line["total"] != json.Number("225") || shipping["subtotal"] != json.Number("10") || shipping["total"] != json.Number("10") || line["is_discountable"] != true || line["quantity"] != json.Number("2") || line["unit_price"] != json.Number("100") {
			t.Fatalf("saved target context or monetary bases changed: %v / %v", line["total"], shipping["total"])
		}
		if len(line["adjustments"].([]map[string]any)) != 1 || len(line["tax_lines"].([]map[string]any)) != 1 || len(shipping["adjustments"].([]map[string]any)) != 1 {
			t.Fatal("existing target relations omitted/deleted relation included")
		}
		if line["metadata"].(map[string]any)["private"] != "snapshot-secret" {
			t.Fatal("private rule metadata omitted")
		}
		if line["title"] != "Saved title" || line["subtitle"] != nil || line["deleted_at"] != nil || len(line["created_at"].(string)) != 24 || line["raw_unit_price"].(map[string]any)["value"] != "100" {
			t.Fatal("saved strings/nulls/timestamps/raw fields changed")
		}
		read(tx, ref, "2", true)
		wrong := ref
		wrong.Generation = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		read(tx, wrong, "1", true)
		exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('target-product','Current','target-product',FALSE,'draft',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO catalog_variants(id,title,product_id,allow_backorder,manage_inventory,weight,created_at,updated_at) VALUES('target-variant','Current','target-product',FALSE,FALSE,1.5,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO fulfillment_sets(id,name,type,created_at,updated_at) VALUES('target-set','Artificial','shipping',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO fulfillment_service_zones(id,name,fulfillment_set_id,created_at,updated_at) VALUES('unresolved-zone','Artificial','target-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO fulfillment_option_types(id,label,code,created_at,updated_at,deleted_at) VALUES('unresolved-type','Artificial','artificial',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO fulfillment_shipping_profiles(id,name,type,created_at,updated_at,deleted_at) VALUES('unresolved-profile','Artificial','default',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`INSERT INTO fulfillment_shipping_options(id,name,price_type,service_zone_id,shipping_option_type_id,shipping_profile_id,created_at,updated_at) VALUES('target-option','Current','flat','unresolved-zone','unresolved-type','unresolved-profile',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(`UPDATE cart_line_items SET product_id='target-product',variant_id='target-variant',raw_unit_price='{"value":"100.000000000000000000001","precision":100}',raw_compare_at_unit_price='{"value":"123.456789012345678901234","precision":100}' WHERE id='amount-cart-line'`)
		exec(`UPDATE cart_shipping_methods SET shipping_option_id='target-option' WHERE id='amount-cart-shipping'`)
		got = read(tx, ref, "1", false)
		encoded, err := json.Marshal(got)
		if err != nil {
			return err
		}
		for attribute, value := range map[string]string{"items.product.id": "target-product", "items.variant.weight": "1.5", "items.metadata.private": "snapshot-secret"} {
			row, _ := json.Marshal(got.Items[0])
			raw, _ := json.Marshal(value)
			ok, err := promotion.MatchRules(row, []promotion.Rule{{Attribute: attribute, Operator: "in", Values: []promotion.RuleValue{{Value: raw}}}}, "items")
			if err != nil || !ok {
				t.Fatalf("target item rule path failed: %s", attribute)
			}
		}
		option := got.ShippingMethods[0]["shipping_option"].(map[string]any)
		if option["shipping_option_type_id"] != "unresolved-type" || option["shipping_profile_id"] != "unresolved-profile" || len(option) != 2 {
			t.Fatal("option scalar references were filtered as linked entities")
		}
		if got.Items[0]["unit_price"] != json.Number("100.000000000000000000001") {
			t.Fatal("raw precision lost through target projection")
		}
		if got.Items[0]["compare_at_unit_price"] != json.Number("123.456789012345678901234") {
			t.Fatal("nullable scalar overrode effective compare price raw")
		}
		across := "across"
		rule := promotion.Rule{Attribute: "shipping_methods.shipping_option.shipping_option_type_id", Operator: "in", Values: []promotion.RuleValue{{Value: json.RawMessage(`"unresolved-type"`)}}}
		computed, err := promotion.ComputeCandidates(promotion.ComputeInput{Candidates: []promotion.ActiveConfiguration{{ID: "target-promotion", Code: "SHIP", Type: "standard", Method: &promotion.ApplicationMethod{ID: "target-method", Type: "percentage", Target: "shipping_methods", Value: "10", Allocation: &across, TargetRules: []promotion.Rule{rule}}}}, Context: encoded, Epsilon: "0.00001"})
		if err != nil || len(computed.Actions) != 1 || computed.Actions[0].Amount != "1" || computed.Actions[0].ShippingMethodID != "amount-cart-shipping" {
			t.Fatalf("SQL targets to original shipping allocator: %v", err)
		}
		exec(`INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('target-promotion','SHIP',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		if _, err := cart.ReplacePromotionAdjustments(ctx, tx, "shop", ref, "1", computed.Actions); err != nil {
			return err
		}
		got = read(tx, ref, "1", false)
		if len(got.ShippingMethods[0]["adjustments"].([]map[string]any)) != 2 || got.ShippingMethods[0]["total"] != json.Number("8.75") {
			t.Fatal("target graph did not see same-transaction adjustment persistence")
		}
		exec(`UPDATE fulfillment_shipping_options SET deleted_at=UTC_TIMESTAMP(6) WHERE id='target-option'`)
		if read(tx, ref, "1", false).ShippingMethods[0]["shipping_option"] != nil {
			t.Fatal("deleted shipping option survived")
		}
		exec(`UPDATE cart_line_adjustments SET raw_amount='{}' WHERE id='amount-cart-discount'`)
		read(tx, ref, "1", true)
		exec(`UPDATE cart_line_adjustments SET raw_amount='{"value":"20","precision":20}' WHERE id='amount-cart-discount'`)
		exec(`UPDATE cart_line_items SET raw_compare_at_unit_price='{"value":"1","value":"2","precision":20}' WHERE id='amount-cart-line'`)
		read(tx, ref, "1", true)
		exec(`UPDATE cart_line_items SET raw_compare_at_unit_price=NULL WHERE id='amount-cart-line'`)
		exec(`UPDATE cart_line_items SET metadata=JSON_OBJECT('large',REPEAT('x',1048577)) WHERE id='amount-cart-line'`)
		read(tx, ref, "1", true)
		return rollback
	})
	if err != rollback || before != fingerprint() {
		t.Fatalf("SQL target compute/persist chain escaped rollback: %v", err)
	}
}
