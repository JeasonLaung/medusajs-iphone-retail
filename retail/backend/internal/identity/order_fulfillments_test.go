package identity_test

import (
	"encoding/json"
	"testing"

	"github.com/eurorefurb/retail/internal/orders"
	"gorm.io/gorm"
)

func seedOrderFulfillmentFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO fulfillment_addresses(id,company,first_name,address_1,address_2,city,country_code,postal_code,phone,metadata,created_at,updated_at) VALUES ('read-address','','Historical recipient','Original street 17','Unit 2','Copenhagen','dk','00017','+4500012345','{"private":"address-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO fulfillments(id,location_id,requires_shipping,delivery_address_id,packed_at,shipped_at,data,metadata,marked_shipped_by,created_by,created_at,updated_at) VALUES ('read-shipment','retired-location',TRUE,'read-address',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"provider-must-not-escape"}','{"private":"shipment-must-not-escape"}','actor-must-not-escape','creator-must-not-escape',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-shipment','deleted-location',FALSE,NULL,NULL,NULL,NULL,NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-shipment','other-location',FALSE,NULL,NULL,NULL,NULL,NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE fulfillments SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-shipment'`,
		`INSERT INTO order_fulfillments(order_id,fulfillment_id,id,created_at,updated_at) VALUES ('read-order','read-shipment','read-shipment-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('read-order','missing-shipment','missing-shipment-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('read-order','deleted-shipment','deleted-shipment-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('read-order','other-shipment','deleted-shipment-link-two',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-order','other-shipment','other-shipment-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE order_fulfillments SET deleted_at=UTC_TIMESTAMP(6) WHERE order_id='read-order' AND fulfillment_id='other-shipment'`,
		`INSERT INTO fulfillment_items(id,title,sku,barcode,quantity,raw_quantity,line_item_id,inventory_item_id,fulfillment_id,created_at,updated_at) VALUES ('read-shipment-item','Packed historical phone','0000123456789012345','00000001234567890',9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}','read-line','missing-stock','read-shipment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-shipment-item','item-must-not-escape','','',1,'{"value":"1","precision":38}',NULL,NULL,'read-shipment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-shipment-item','Other packed phone','','',1,'{"value":"1","precision":38}',NULL,NULL,'other-shipment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE fulfillment_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-shipment-item'`,
		`INSERT INTO fulfillment_labels(id,tracking_number,tracking_url,label_url,fulfillment_id,created_at,updated_at) VALUES ('read-label','000012345678901234567890','https://example.invalid/track','https://example.invalid/label','read-shipment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('unsafe-label','Unsupported original URL','javascript:alert(1)','//example.invalid/label','read-shipment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-label','label-must-not-escape','','','read-shipment',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE fulfillment_labels SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-label'`,
	} {
		if db.Exec(q).Error != nil {
			t.Fatal("artificial fulfillment read seed failed")
		}
	}
}

func checkOrderFulfillments(t *testing.T, db *gorm.DB, call func(string, int) []byte) {
	t.Helper()
	var data orders.FulfillmentEvidence
	read := func() {
		t.Helper()
		if json.Unmarshal(call("/orders/read-order/fulfillments", 200), &data) != nil || data.OrderID != "read-order" || data.Version != 2 || data.OperationsEnabled || len(data.Fulfillments) != 3 {
			t.Fatal("fulfillment envelope differs")
		}
	}
	read()
	if data.Fulfillments[0].ID != "deleted-shipment" || data.Fulfillments[0].Availability != "deleted" || data.Fulfillments[0].Record != nil || data.Fulfillments[1].ID != "missing-shipment" || data.Fulfillments[1].Availability != "missing" || data.Fulfillments[1].Record != nil {
		t.Fatal("missing fulfillment evidence hidden or fabricated")
	}
	r := data.Fulfillments[2].Record
	if data.Fulfillments[2].Availability != "available" || r == nil || r.LocationID != "retired-location" || r.ProviderID != nil || r.ShippingOptionID != nil || !r.RequiresShipping || r.PackedAt == nil || r.ShippedAt == nil || r.DeliveredAt != nil || r.CanceledAt != nil || len(r.Items) != 1 || len(r.Labels) != 2 {
		t.Fatal("fulfillment facts differ")
	}
	if r.Address == nil || r.Address.FirstName == nil || *r.Address.FirstName != "Historical recipient" || r.Address.LastName != nil || r.Address.Company == nil || *r.Address.Company != "" || r.Address.Address1 == nil || *r.Address.Address1 != "Original street 17" || r.Address.Address2 == nil || *r.Address.Address2 != "Unit 2" || *r.Address.PostalCode != "00017" || *r.Address.Phone != "+4500012345" {
		t.Fatal("fulfillment address projection differs")
	}
	i := r.Items[0]
	if i.ID != "read-shipment-item" || i.Title != "Packed historical phone" || i.SKU != "0000123456789012345" || i.Barcode != "00000001234567890" || i.Quantity != "9007199254740993.000000000000000001" || i.LineItemID == nil || *i.LineItemID != "read-line" || i.InventoryItemID == nil || *i.InventoryItemID != "missing-stock" || r.Labels[0].TrackingNumber != "000012345678901234567890" || r.Labels[1].TrackingURL != "javascript:alert(1)" {
		t.Fatal("fulfillment exact quantity or original identifiers lost")
	}
	if json.Unmarshal(call("/orders/other-order/fulfillments", 200), &data) != nil || len(data.Fulfillments) != 1 || data.Fulfillments[0].ID != "other-shipment" || data.Fulfillments[0].Record.Address != nil || len(data.Fulfillments[0].Record.Items) != 1 || data.Fulfillments[0].Record.Items[0].ID != "other-shipment-item" {
		t.Fatal("fulfillment evidence crossed order boundary")
	}
	call("/orders/deleted-order/fulfillments", 404)
	call("/orders/missing/fulfillments", 404)
	call("/orders/read-order/fulfillments?fields=*", 400)
	for _, change := range [][2]string{
		{`UPDATE fulfillment_items SET raw_quantity='{"value":"invalid"}' WHERE id='read-shipment-item'`, `UPDATE fulfillment_items SET raw_quantity='{"value":"9007199254740993.000000000000000001","precision":38}' WHERE id='read-shipment-item'`},
		{"UPDATE fulfillment_runtime_schema SET reads_enabled=FALSE WHERE id=1", "UPDATE fulfillment_runtime_schema SET reads_enabled=TRUE WHERE id=1"},
	} {
		if db.Exec(change[0]).Error != nil {
			t.Fatal("fulfillment failure setup failed")
		}
		call("/orders/read-order/fulfillments", 503)
		if db.Exec(change[1]).Error != nil {
			t.Fatal("fulfillment failure restoration failed")
		}
		read()
	}
	if db.Exec("UPDATE fulfillment_addresses SET deleted_at=UTC_TIMESTAMP(6) WHERE id='read-address'").Error != nil {
		t.Fatal("fulfillment address deletion seed failed")
	}
	read()
	if data.Fulfillments[2].Record.Address != nil || data.Fulfillments[2].Record.DeliveryAddressID == nil || *data.Fulfillments[2].Record.DeliveryAddressID != "read-address" {
		t.Fatal("deleted address hidden without original reference")
	}
	if db.Exec("UPDATE fulfillment_addresses SET deleted_at=NULL WHERE id='read-address'").Error != nil {
		t.Fatal("fulfillment address restoration failed")
	}
	if db.Exec("UPDATE order_fulfillments SET deleted_at=UTC_TIMESTAMP(6) WHERE order_id='read-order' AND fulfillment_id!='other-shipment'").Error != nil {
		t.Fatal("fulfillment empty setup failed")
	}
	if json.Unmarshal(call("/orders/read-order/fulfillments", 200), &data) != nil || len(data.Fulfillments) != 0 {
		t.Fatal("deleted shipment links visible")
	}
	if db.Exec("UPDATE order_fulfillments SET deleted_at=NULL WHERE order_id='read-order' AND fulfillment_id!='other-shipment'").Error != nil {
		t.Fatal("fulfillment link restoration failed")
	}
	digits := "(SELECT 0 AS n UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9)"
	q := "INSERT INTO fulfillment_items(id,title,sku,barcode,quantity,raw_quantity,fulfillment_id,created_at,updated_at) SELECT CONCAT('limit-fulfillment-',a.n,b.n,c.n),i.title,i.sku,i.barcode,i.quantity,i.raw_quantity,i.fulfillment_id,i.created_at,i.updated_at FROM fulfillment_items i CROSS JOIN " + digits + " a CROSS JOIN " + digits + " b CROSS JOIN " + digits + " c WHERE i.id='read-shipment-item'"
	if db.Exec(q).Error != nil {
		t.Fatal("fulfillment child limit setup failed")
	}
	call("/orders/read-order/fulfillments", 503)
	if db.Exec("DELETE FROM fulfillment_items WHERE id LIKE 'limit-fulfillment-%'").Error != nil {
		t.Fatal("fulfillment child limit restoration failed")
	}
	read()
	t.Log(`ORDER_FULFILLMENT_READ_REHEARSAL {"exact_quantity_and_ids":true,"explicit_address_projection":true,"missing_deleted_links_preserved":true,"cross_order_isolation":true,"soft_deletes_filtered":true,"sql_null_preserved":true,"private_payload_excluded":true,"invalid_raw_rejected":true,"disabled_schema_rejected":true,"child_limit_rejected":true,"operations_enabled":false}`)
}
