package identity_test

import (
	"encoding/json"
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
	if _, e := orders.RefreshChangeSummaryTx(ctx, db, "allocation-order", 1); e == nil {
		t.Fatal("autocommit summary")
	}
	var before int64
	db.Table("order_line_items").Count(&before)
	e := db.Transaction(func(tx *gorm.DB) error {
		seed(tx)
		originalSummary := `{"transaction_total":"1000","raw_transaction_total":{"value":"1000.000000000000000000001","precision":100},"paid_total":"1000","refunded_total":"0","original_order_total":"1000","private_history":{"keep":"exact"}}`
		exec(tx, "INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES('exchange-summary','allocation-order',1,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", originalSummary)
		exec(tx, `INSERT INTO order_transactions(id,order_id,version,amount,raw_amount,currency_code,created_at,updated_at) VALUES('exchange-capture','allocation-order',1,1100,'{"value":"1100.000000000000000000001","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('exchange-refund','allocation-order',1,-100,'{"value":"-100","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
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
		refreshed, e := orders.RefreshChangeSummaryTx(ctx, tx, "allocation-order", version)
		if e != nil || refreshed.Summary == nil || refreshed.Summary.PendingDifference[0] != '-' {
			t.Fatal("exchange summary signed difference", e)
		}
		var updated, oldSummary string
		tx.Table("order_summaries").Select("totals").Where("order_id='allocation-order' AND version=?", version).Scan(&updated)
		tx.Table("order_summaries").Select("totals").Where("id='exchange-summary'").Scan(&oldSummary)
		var obj map[string]json.RawMessage
		if json.Unmarshal([]byte(updated), &obj) != nil || string(obj["paid_total"]) != `"1100.000000000000000000001"` || string(obj["refunded_total"]) != `"100"` || string(obj["private_history"]) != `{"keep":"exact"}` || oldSummary != originalSummary || string(obj["raw_transaction_total"]) != `{"value":"1000.000000000000000000001","precision":100}` {
			t.Fatal("exchange summary history changed", updated)
		}
		var current, pending string
		json.Unmarshal(obj["current_order_total"], &current)
		json.Unmarshal(obj["pending_difference"], &pending)
		if current != refreshed.Totals["total"] || pending != refreshed.Summary.PendingDifference || len(obj["raw_pending_difference"]) == 0 {
			t.Fatal("exchange summary amount mismatch")
		}
		exec(tx, "UPDATE order_transactions SET currency_code='usd' WHERE id='exchange-refund'")
		if _, e := orders.RefreshChangeSummaryTx(ctx, tx, "allocation-order", version); e == nil {
			t.Fatal("foreign ledger currency")
		}
		exec(tx, "UPDATE order_transactions SET currency_code='eur',version=99 WHERE id='exchange-refund'")
		if _, e := orders.RefreshChangeSummaryTx(ctx, tx, "allocation-order", version); e == nil {
			t.Fatal("future ledger version")
		}
		exec(tx, "UPDATE order_transactions SET version=1 WHERE id='exchange-refund'")
		if _, e := orders.RefreshChangeSummaryTx(ctx, tx, "allocation-order", 1); e == nil {
			t.Fatal("stale summary")
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
	nativeInput := orders.DeviceExchangeInput{OrderID: "allocation-order", OriginalItemID: "allocation-line", LocationID: "allocation-location", OriginalAssignmentID: "allocation-original", RequestID: "allocation-exchange", ActorID: "exchange-actor", ExpectedVersion: 1}
	nativeSeed := func(tx *gorm.DB) {
		seed(tx)
		exec(tx, `UPDATE order_items SET fulfilled_quantity=1,raw_fulfilled_quantity='{"value":"1","precision":100}',return_requested_quantity=0,raw_return_requested_quantity='{"value":"0","precision":100}',return_received_quantity=0,raw_return_received_quantity='{"value":"0","precision":100}',return_dismissed_quantity=0,raw_return_dismissed_quantity='{"value":"0","precision":100}',written_off_quantity=0,raw_written_off_quantity='{"value":"0","precision":100}' WHERE order_id='allocation-order' AND version=1`)
		exec(tx, `INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES('native-exchange-summary','allocation-order',1,'{"transaction_total":"0"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	}
	if _, e := orders.ApplyDeviceExchangeTx(ctx, db, nativeInput); e == nil {
		t.Fatal("native exchange autocommit")
	}
	e = db.Transaction(func(tx *gorm.DB) error {
		nativeSeed(tx)
		out, e := orders.ApplyDeviceExchangeTx(ctx, tx, nativeInput)
		if e != nil {
			t.Fatal("native exchange", e)
		}
		if out.ExchangeVersion != 2 || out.ReceivedVersion != 3 || out.ItemID == "allocation-line" || out.VariantID != "allocation-v" {
			t.Fatal("native versions/item", out)
		}
		var lines []struct {
			Version                                 int
			Requested, Dismissed, Written, Received string
		}
		if tx.Table("order_items").Select("version,CAST(return_requested_quantity AS CHAR) AS requested,CAST(return_dismissed_quantity AS CHAR) AS dismissed,CAST(written_off_quantity AS CHAR) AS written,CAST(return_received_quantity AS CHAR) AS received").Where("order_id='allocation-order' AND item_id='allocation-line'").Order("version").Scan(&lines).Error != nil || len(lines) != 3 {
			t.Fatal("native line versions")
		}
		if lines[0].Written != "0.000000000000000000" || lines[1].Requested != "1.000000000000000000" || lines[1].Written != "0.000000000000000000" || lines[2].Requested != "0.000000000000000000" || lines[2].Written != "1.000000000000000000" || lines[2].Dismissed != "1.000000000000000000" || lines[2].Received != "0.000000000000000000" {
			t.Fatal("native quarantine counters", lines)
		}
		var ret struct {
			Status       string
			OrderVersion int
			ExchangeID   string
		}
		tx.Table("returns").Select("status,order_version,exchange_id").Where("id=?", out.ReturnID).Take(&ret)
		var ri struct{ Received, Damaged string }
		tx.Table("return_items").Select("CAST(received_quantity AS CHAR) AS received,CAST(damaged_quantity AS CHAR) AS damaged").Where("return_id=?", out.ReturnID).Take(&ri)
		if ret.Status != "received" || ret.OrderVersion != 1 || ret.ExchangeID != out.ExchangeID || ri.Received != "1.000000000000000000" || ri.Damaged != "1.000000000000000000" {
			t.Fatal("native records", ret, ri)
		}
		var actions, changes, items int64
		tx.Table("order_change_actions").Where("order_id='allocation-order' AND applied=TRUE").Count(&actions)
		tx.Table("order_changes").Where("order_id='allocation-order' AND status='confirmed'").Count(&changes)
		tx.Table("order_exchange_items").Where("exchange_id=? AND item_id=?", out.ExchangeID, out.ItemID).Count(&items)
		var addAction struct{ Reference, Details string }
		tx.Table("order_change_actions").Select("reference,details").Where("order_id='allocation-order' AND action='ITEM_ADD'").Take(&addAction)
		var addDetail map[string]json.RawMessage
		if json.Unmarshal([]byte(addAction.Details), &addDetail) != nil || addAction.Reference != "order_exchange" || string(addDetail["unit_price"]) != `"123.123456789012345678901"` {
			t.Fatal("native item action reference/price", addAction)
		}
		if actions != 3 || changes != 2 || items != 1 {
			t.Fatal("native actions", actions, changes, items)
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal("native owner rollback", e)
	}
	exec(db, "CREATE TRIGGER exchange_receive_action_fault BEFORE INSERT ON order_change_actions FOR EACH ROW BEGIN IF NEW.action='RECEIVE_DAMAGED_RETURN_ITEM' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial exchange receive failure'; END IF; END")
	e = db.Transaction(func(tx *gorm.DB) error {
		nativeSeed(tx)
		_, e := orders.ApplyDeviceExchangeTx(ctx, tx, nativeInput)
		return e
	})
	exec(db, "DROP TRIGGER exchange_receive_action_fault")
	if e == nil {
		t.Fatal("native receive fault accepted")
	}
	var leaked int64
	txTables := []string{"order_exchanges", "order_changes", "order_change_actions"}
	for _, table := range txTables {
		db.Table(table).Where("order_id='allocation-order'").Count(&leaked)
		if leaked != 0 {
			t.Fatal("native rollback leaked", table)
		}
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
	exec(db, "CREATE TRIGGER exchange_summary_fault BEFORE UPDATE ON order_summaries FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial summary update failure'")
	e = db.Transaction(func(tx *gorm.DB) error {
		seed(tx)
		exec(tx, `INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES('exchange-fault-summary','allocation-order',1,'{"transaction_total":"0"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		v, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1)
		if e != nil {
			return e
		}
		if _, e = orders.CreateExchangeItemTx(ctx, tx, "allocation-order", "allocation-line", v); e != nil {
			return e
		}
		_, e = orders.RefreshChangeSummaryTx(ctx, tx, "allocation-order", v)
		return e
	})
	exec(db, "DROP TRIGGER exchange_summary_fault")
	if e == nil {
		t.Fatal("summary fault accepted")
	}
	db.Table("order_line_items").Count(&after)
	var versionAfter int
	db.Table("orders").Select("version").Where("id='allocation-order'").Scan(&versionAfter)
	if before != after || versionAfter != 1 {
		t.Fatal("summary failure leaked new line/version")
	}
	t.Log(`EXCHANGE_ITEM_REHEARSAL {"real_mysql":true,"same_order_sku":true,"original_exact_unit_price":true,"fresh_catalog_snapshot":true,"custom_price_without_quote":true,"inclusive_flag_not_copied":true,"current_quote_inclusive_and_sale_reference":true,"fresh_system_tax_original_untouched":true,"no_blind_discount_copy":true,"new_item_zero_fulfillment":true,"current_amount_reader":true,"stale_foreign_unpublished_provider_denied":true,"missing_region_and_disabled_channel_denied":true,"autocommit_denied":true,"owner_rollback":true,"late_tax_failure_atomic":true,"summary_preserves_history_and_old_version":true,"signed_exact_difference":true,"ledger_paid_and_refund_aggregates":true,"future_and_foreign_ledger_denied":true,"late_summary_failure_atomic":true,"native_exchange_and_quarantine_records":true,"two_confirmed_versions_three_actions":true,"late_receive_action_failure_atomic":true,"exchange_command_integrated":false}`)
}
