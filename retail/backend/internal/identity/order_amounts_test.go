package identity_test

import (
	"encoding/json"
	"testing"

	"github.com/eurorefurb/retail/internal/orders"
	"gorm.io/gorm"
)

func seedOrderAmountFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	queries := []string{
		`INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES ('eur','€','€',2,0,'{"value":"0","precision":20}','Euro',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('dkk','kr','kr',2,0,'{"value":"0","precision":20}','Krone',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_line_tax_lines(id,code,rate,raw_rate,item_id,metadata,created_at,updated_at) VALUES ('amount-tax','VAT',25,'{"value":"25","precision":20}','read-line','{"private":"tax-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_line_adjustments(id,amount,raw_amount,item_id,is_tax_inclusive,version,created_at,updated_at) VALUES ('amount-discount',10,'{"value":"10","precision":20}','read-line',FALSE,2,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('old-discount',999,'{"value":"999","precision":20}','read-line',FALSE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('future-discount',999,'{"value":"999","precision":20}','read-line',FALSE,3,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-discount',999,'{"value":"999","precision":20}','read-line',FALSE,2,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE order_line_adjustments SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-discount'`,
		`INSERT INTO order_shipping_methods(id,name,amount,raw_amount,is_tax_inclusive,is_custom_amount,data,created_at,updated_at) VALUES ('amount-method','Historical delivery',12.5,'{"value":"12.5","precision":20}',TRUE,FALSE,'{"private":"shipping-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_shipping_versions(id,order_id,version,shipping_method_id,created_at,updated_at) VALUES ('amount-version','read-order',2,'amount-method',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('old-shipping-version','read-order',1,'amount-method',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_shipping_tax_lines(id,code,rate,raw_rate,shipping_method_id,created_at,updated_at) VALUES ('amount-shipping-tax','VAT',25,'{"value":"25","precision":20}','amount-method',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_shipping_adjustments(id,amount,raw_amount,shipping_method_id,version,created_at,updated_at) VALUES ('amount-shipping-discount',2,'{"value":"2","precision":20}','amount-method',2,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('old-shipping-discount',999,'{"value":"999","precision":20}','amount-method',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_credit_lines(id,order_id,amount,raw_amount,version,metadata,created_at,updated_at) VALUES ('amount-credit','read-order',3,'{"value":"3","precision":20}',2,'{"private":"credit-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('old-credit','read-order',999,'{"value":"999","precision":20}',1,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES ('amount-summary','read-order',2,'{"transaction_total":25,"raw_transaction_total":{"value":"25","precision":20},"private":"summary-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	}
	for _, q := range queries {
		if db.Exec(q).Error != nil {
			t.Fatal("artificial amount seed failed")
		}
	}
}
func checkOrderAmounts(t *testing.T, call func(string, int) []byte) {
	t.Helper()
	var detail orders.AmountDetail
	if json.Unmarshal(call("/orders/read-order/amounts", 200), &detail) != nil || detail.Version != 2 || detail.CurrencyCode != "eur" || detail.Totals["total"] != "22517998136852477.0000000000000000025" || len(detail.Lines) != 1 || detail.Lines[0].UnitPrice != "9007199254740993.000000000000000001" || len(detail.ShippingMethods) != 1 || detail.ShippingMethods[0].Name != "Historical delivery" || detail.ShippingMethods[0].Totals["total"] != "10" || detail.Summary == nil || detail.Summary.TransactionTotal != "25" || detail.Summary.PendingDifference != "11258999068426217.00000000000000000125" {
		t.Fatal("order exact amounts or selected version differs")
	}
	if json.Unmarshal(call("/orders/other-order/amounts", 200), &detail) != nil || detail.Totals["total"] != "0" || len(detail.Lines) != 0 || len(detail.ShippingMethods) != 0 || detail.Summary != nil {
		t.Fatal("empty order amounts differ")
	}
	call("/orders/deleted-order/amounts", 404)
	call("/orders/missing/amounts", 404)
	call("/orders/read-order/amounts?fields=*", 400)
}
func checkOrderAmountFailures(t *testing.T, db *gorm.DB, call func(string, int) []byte) {
	t.Helper()
	for _, row := range []struct{ change, restore string }{
		{`UPDATE order_items SET raw_unit_price='{"value":"1","value":"2","precision":20}' WHERE id='current-order-item'`, `UPDATE order_items SET raw_unit_price='{"value":"9007199254740993.000000000000000001","precision":38}' WHERE id='current-order-item'`},
		{`UPDATE order_shipping_methods SET deleted_at=UTC_TIMESTAMP(6) WHERE id='amount-method'`, `UPDATE order_shipping_methods SET deleted_at=NULL WHERE id='amount-method'`},
		{`UPDATE commerce_currencies SET deleted_at=UTC_TIMESTAMP(6) WHERE code='eur'`, `UPDATE commerce_currencies SET deleted_at=NULL WHERE code='eur'`},
		{`UPDATE order_summaries SET totals='{"transaction_total":1,"transaction_total":2}' WHERE id='amount-summary'`, `UPDATE order_summaries SET totals='{"transaction_total":25,"raw_transaction_total":{"value":"25","precision":20},"private":"summary-must-not-escape"}' WHERE id='amount-summary'`},
		{`INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES ('duplicate-summary','read-order',2,'{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, `DELETE FROM order_summaries WHERE id='duplicate-summary'`},
		{`UPDATE order_line_tax_lines SET raw_rate='{"value":"NaN","precision":20}' WHERE id='amount-tax'`, `UPDATE order_line_tax_lines SET raw_rate='{"value":"25","precision":20}' WHERE id='amount-tax'`},
	} {
		if db.Exec(row.change).Error != nil {
			t.Fatal("artificial amount failure setup failed")
		}
		call("/orders/read-order/amounts", 503)
		if db.Exec(row.restore).Error != nil {
			t.Fatal("artificial amount failure restore failed")
		}
		call("/orders/read-order/amounts", 200)
	}
}
