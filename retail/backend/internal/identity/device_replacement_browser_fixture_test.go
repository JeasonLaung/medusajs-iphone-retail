package identity_test

import (
	"testing"

	"gorm.io/gorm"
)

// Artificial browser fixture only; the browser performs the actual allocation,
// packing, return, exchange, inspection and shipment through production HTTP.
func seedDeviceReplacementBrowser(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,auth_version,verified_at,created_at,updated_at) VALUES('cust_replacement_notice','replacement-notice@example.invalid','artificial-no-login','Artificial','Recipient',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) VALUES('allocation-order','cust_replacement_notice','replacement-notice-cart','price-edit-store',UTC_TIMESTAMP(6))`,
		`UPDATE after_sales_requests SET type='exchange' WHERE id='asr-browser-return'`,
		`UPDATE orders SET region_id='price-edit-region' WHERE id='allocation-order'`,
		`INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES('exchange-browser-channel','allocation-product','price-edit-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO tax_providers VALUES('tp_system',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`,
		`INSERT INTO tax_regions(id,provider_id,country_code,created_at,updated_at) VALUES('exchange-browser-country','tp_system','dk',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO tax_rates(id,rate,code,name,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES('exchange-browser-vat',0,'VAT','Fixture zero VAT',TRUE,FALSE,'exchange-browser-country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES('exchange-browser-summary','allocation-order',1,'{"transaction_total":"200","paid_total":"200","refunded_total":"0","original_order_total":"200","current_order_total":"200","pending_difference":"0"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_transactions(id,order_id,version,amount,raw_amount,currency_code,created_at,updated_at) VALUES('exchange-browser-capture','allocation-order',1,200,'{"value":"200","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	} {
		if e := db.Exec(q).Error; e != nil {
			t.Fatal("replacement browser fixture", e)
		}
	}
}
