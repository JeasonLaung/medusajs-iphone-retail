package identity_test

import (
	"errors"
	"testing"

	"github.com/eurorefurb/retail/internal/orders"
	"gorm.io/gorm"
)

func checkExchangeItem(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := t.Context()
	rollback := errors.New("exchange item owner rollback")
	exec := func(tx *gorm.DB, q string, args ...any) {
		t.Helper()
		if e := tx.Exec(q, args...).Error; e != nil {
			t.Fatal("exchange item fixture", e)
		}
	}
	seed := func(tx *gorm.DB) {
		exec(tx, `INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES('exchange-item-channel','allocation-product','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO catalog_product_options(id,product_id,product_option_id,created_at,updated_at) VALUES('exchange-grade-member','allocation-product','service-grade',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('exchange-battery-member','allocation-product','service-battery',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES('exchange-grade-value','exchange-grade-member','service-grade-b',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('exchange-battery-value','exchange-battery-member','service-battery-85',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `UPDATE orders SET region_id='price-edit-region' WHERE id='allocation-order'`)
		exec(tx, `UPDATE catalog_products SET title='Current exchange product' WHERE id='allocation-product'`)
		exec(tx, `UPDATE order_items SET unit_price=123.123456789012345678,raw_unit_price='{"value":"123.123456789012345678901","precision":100}' WHERE order_id='allocation-order' AND version=1`)
		exec(tx, `UPDATE order_line_items SET is_tax_inclusive=TRUE WHERE id='allocation-line'`)
		exec(tx, `INSERT INTO tax_providers VALUES('tp_system',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`)
		exec(tx, `INSERT INTO tax_regions(id,provider_id,country_code,created_at,updated_at) VALUES('exchange-item-country','tp_system','dk',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO tax_rates(id,rate,code,name,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES('exchange-item-vat',20,'VAT','Current VAT',TRUE,FALSE,'exchange-item-country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO order_line_tax_lines(id,item_id,code,rate,raw_rate,created_at,updated_at) VALUES('exchange-original-tax','allocation-line','OLD',5,'{"value":"5","precision":100}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	}
	if _, e := orders.CreateExchangeItemTx(ctx, db, "allocation-order", "allocation-line", 1); e == nil {
		t.Fatal("autocommit exchange item")
	}
	var before int64
	db.Table("order_line_items").Count(&before)
	e := db.Transaction(func(tx *gorm.DB) error {
		seed(tx)
		version, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1)
		if e != nil {
			t.Fatal("exchange version fixture", e)
		}
		item, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", version)
		if e != nil {
			t.Fatal("create exchange item", e)
		}
		if item.UnitPrice != "123.123456789012345678901" || item.VariantID != "allocation-v" || item.TaxInclusive || item.TaxLines != 1 || item.ItemID == "allocation-line" {
			t.Fatal("new exchange item snapshot", item)
		}
		var saved struct {
			Title, RawUnitPrice string
			IsCustomPrice       bool
		}
		tx.Table("order_line_items").Select("title,raw_unit_price,is_custom_price").Where("id=?", item.ItemID).Take(&saved)
		if saved.Title != "Current exchange product" || !saved.IsCustomPrice || saved.RawUnitPrice != `{"value":"123.123456789012345678901","precision":100}` {
			t.Fatal("catalog/price snapshot lost")
		}
		var tax, old string
		tx.Table("order_line_tax_lines").Select("CAST(rate AS CHAR)").Where("item_id=?", item.ItemID).Scan(&tax)
		tx.Table("order_line_tax_lines").Select("CAST(rate AS CHAR)").Where("id='exchange-original-tax'").Scan(&old)
		if tax != "20.000000000000000000" || old != "5.000000000000000000" {
			t.Fatal("fresh tax or historical tax", tax, old)
		}
		var adj int64
		tx.Table("order_line_adjustments").Where("item_id=?", item.ItemID).Count(&adj)
		if adj != 0 {
			t.Fatal("old discounts copied without carry-over calculation")
		}
		amounts, e := orders.ReadAmountSnapshot(tx, "allocation-order")
		if e != nil || len(amounts.Lines) != 2 {
			t.Fatal("exchange item unreadable", e)
		}
		if _, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", 1); e == nil {
			t.Fatal("stale item version")
		}
		if _, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "foreign-item", version); e == nil {
			t.Fatal("foreign item")
		}
		exec(tx, `INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('exchange-item-price-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO catalog_variant_price_sets(id,variant_id,price_set_id,created_at,updated_at) VALUES('exchange-item-price-link','allocation-v','exchange-item-price-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO pricing_price_lists(id,title,description,type,status,rules_count,created_at,updated_at) VALUES('exchange-item-sale','Current sale','Fixture','sale','active',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,price_list_id,created_at,updated_at) VALUES('exchange-item-base','exchange-item-price-set','eur',300,'{"value":"300","precision":100}',0,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('exchange-item-sale-price','exchange-item-price-set','eur',250,'{"value":"250","precision":100}',0,'exchange-item-sale',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO pricing_preferences(id,attribute,value,is_tax_inclusive,created_at,updated_at) VALUES('exchange-item-inclusive','currency_code','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		sale, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", version)
		if e != nil || !sale.TaxInclusive || sale.UnitPrice != "123.123456789012345678901" {
			t.Fatal("current sale must not replace custom price", e)
		}
		var compare string
		tx.Table("order_line_items").Select("CAST(compare_at_unit_price AS CHAR)").Where("id=?", sale.ItemID).Scan(&compare)
		if compare != "300.000000000000000000" {
			t.Fatal("current sale reference price", compare)
		}
		exec(tx, "UPDATE orders SET region_id=NULL WHERE id='allocation-order'")
		if _, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", version); e == nil {
			t.Fatal("missing region must not silently suppress taxes")
		}
		exec(tx, "UPDATE orders SET region_id='price-edit-region' WHERE id='allocation-order'")
		exec(tx, "UPDATE commerce_sales_channels SET is_disabled=TRUE WHERE id='price-edit-channel'")
		if _, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", version); e == nil {
			t.Fatal("disabled channel")
		}
		exec(tx, "UPDATE commerce_sales_channels SET is_disabled=FALSE WHERE id='price-edit-channel'")
		var count int64
		tx.Table("order_items").Where("id=? AND quantity=1 AND fulfilled_quantity=0 AND shipped_quantity=0 AND return_requested_quantity=0 AND return_received_quantity=0", item.DetailID).Count(&count)
		if count != 1 {
			t.Fatal("new item operational quantities")
		}
		exec(tx, "UPDATE tax_providers SET is_enabled=FALSE WHERE id='tp_system'")
		if _, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", version); e == nil {
			t.Fatal("disabled provider")
		}
		exec(tx, "UPDATE tax_providers SET is_enabled=TRUE WHERE id='tp_system'")
		exec(tx, "UPDATE catalog_products SET status='draft' WHERE id='allocation-product'")
		if _, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", version); e == nil {
			t.Fatal("unpublished replacement")
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal("owner rollback", e)
	}
	var after int64
	db.Table("order_line_items").Count(&after)
	if before != after {
		t.Fatal("exchange item rollback leaked")
	}
	exec(db, "CREATE TRIGGER exchange_item_tax_fault BEFORE INSERT ON order_line_tax_lines FOR EACH ROW BEGIN IF NEW.item_id <> 'allocation-line' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial exchange tax failure'; END IF; END")
	e = db.Transaction(func(tx *gorm.DB) error {
		seed(tx)
		_, e := orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", 1)
		return e
	})
	exec(db, "DROP TRIGGER exchange_item_tax_fault")
	if e == nil {
		t.Fatal("late tax failure accepted")
	}
	db.Table("order_line_items").Count(&after)
	if before != after {
		t.Fatal("late tax failure leaked")
	}
	t.Log(`EXCHANGE_ITEM_REHEARSAL {"real_mysql":true,"same_order_sku":true,"original_exact_unit_price":true,"fresh_catalog_snapshot":true,"custom_price_without_quote":true,"inclusive_flag_not_copied":true,"current_quote_inclusive_and_sale_reference":true,"fresh_system_tax_original_untouched":true,"no_blind_discount_copy":true,"new_item_zero_fulfillment":true,"current_amount_reader":true,"stale_foreign_unpublished_provider_denied":true,"missing_region_and_disabled_channel_denied":true,"autocommit_denied":true,"owner_rollback":true,"late_tax_failure_atomic":true,"exchange_command_integrated":false}`)
}
