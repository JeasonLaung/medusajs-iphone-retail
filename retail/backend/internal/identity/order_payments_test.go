package identity_test

import (
	"encoding/json"
	"github.com/eurorefurb/retail/internal/orders"
	"gorm.io/gorm"
	"testing"
)

func seedOrderPaymentFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	queries := []string{
		`INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,captured_amount,raw_captured_amount,metadata,created_at,updated_at) VALUES ('read-collection','eur',9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}','completed',10,'{"value":"10","precision":38}','{"private":"collection-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-collection','dkk',7,'{"value":"7","precision":38}','authorized',NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO order_payment_collections(order_id,payment_collection_id,id,created_at,updated_at) VALUES ('read-order','read-collection','read-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('other-order','other-collection','other-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,context,status,payment_collection_id,metadata,created_at,updated_at) VALUES ('read-session','eur',9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}','retired-provider','sealed-must-not-escape','context-must-not-escape','captured','read-collection','metadata-must-not-escape',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO payments(id,amount,raw_amount,currency_code,provider_id,payment_collection_id,payment_session_id,data,metadata,captured_at,created_at,updated_at) VALUES ('read-payment',9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}','eur','retired-provider','read-collection','read-session','payment-must-not-escape','metadata-must-not-escape',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-payment',99,'{"value":"99","precision":38}','eur','retired-provider','read-collection','deleted-session',NULL,NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE payments SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-payment'`,
		`INSERT INTO payment_captures(id,amount,raw_amount,payment_id,metadata,created_at,updated_at) VALUES ('read-capture',9007199254740993.000000000000000001,'{"value":"9007199254740993.000000000000000001","precision":38}','read-payment','{"private":"capture-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('deleted-capture',99,'{"value":"99","precision":38}','read-payment',NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`UPDATE payment_captures SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-capture'`,
		`INSERT INTO payment_refunds(id,amount,raw_amount,payment_id,note,created_by,created_at,updated_at) VALUES ('read-refund',0.000000000000000001,'{"value":"0.000000000000000001","precision":38}','read-payment','note-must-not-escape','actor-must-not-escape',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
		`INSERT INTO migration_payment_dispositions(source_table,source_id,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason) VALUES ('payment','read-payment',UNHEX(REPEAT('0',64)),UNHEX(REPEAT('0',64)),UNHEX(REPEAT('0',64)),UNHEX(REPEAT('0',32)),1,'hold','legacy-payment-reconciliation')`,
	}
	for _, q := range queries {
		if db.Exec(q).Error != nil {
			t.Fatal("artificial order payment seed failed")
		}
	}
}
func checkOrderPayments(t *testing.T, db *gorm.DB, call func(string, int) []byte) {
	t.Helper()
	var data orders.PaymentEvidence
	if json.Unmarshal(call("/orders/read-order/payments", 200), &data) != nil || data.OrderID != "read-order" || data.Version != 2 || data.OperationsEnabled || len(data.Collections) != 1 {
		t.Fatal("order payment envelope differs")
	}
	c := data.Collections[0]
	if c.ID != "read-collection" || c.Status != "completed" || c.Amount != "9007199254740993.000000000000000001" || c.AuthorizedAmount != nil || c.CapturedAmount == nil || *c.CapturedAmount != "10" || c.RefundedAmount != nil || len(c.Sessions) != 1 || len(c.Payments) != 1 {
		t.Fatal("payment collection facts differ")
	}
	p := c.Payments[0]
	if p.ID != "read-payment" || p.OperationHold != "held" || p.Amount != c.Amount || p.CapturedAt == nil || p.CanceledAt != nil || len(p.Captures) != 1 || len(p.Refunds) != 1 || p.Captures[0].Amount != c.Amount || p.Refunds[0].Amount != "0.000000000000000001" {
		t.Fatal("payment movement precision or visibility differs")
	}
	if c.Sessions[0].ID != "read-session" || c.Sessions[0].OperationHold != "unverified" || c.Sessions[0].Status != "captured" {
		t.Fatal("missing payment hold treated as authorization")
	}
	if json.Unmarshal(call("/orders/other-order/payments", 200), &data) != nil || len(data.Collections) != 1 || data.Collections[0].ID != "other-collection" || len(data.Collections[0].Payments) != 0 || len(data.Collections[0].Sessions) != 0 {
		t.Fatal("payment collections leaked across orders")
	}
	call("/orders/deleted-order/payments", 404)
	call("/orders/missing/payments", 404)
	call("/orders/read-order/payments?fields=*", 400)
	changes := [][2]string{
		{"UPDATE payment_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE id='read-collection'", "UPDATE payment_collections SET deleted_at=NULL WHERE id='read-collection'"},
		{`UPDATE payment_captures SET raw_amount='{"value":"invalid"}' WHERE id='read-capture'`, `UPDATE payment_captures SET raw_amount='{"value":"9007199254740993.000000000000000001","precision":38}' WHERE id='read-capture'`},
		{"UPDATE payment_runtime_schema SET reads_enabled=FALSE WHERE id=1", "UPDATE payment_runtime_schema SET reads_enabled=TRUE WHERE id=1"},
	}
	for _, change := range changes {
		if db.Exec(change[0]).Error != nil {
			t.Fatal("order payment failure seed failed")
		}
		call("/orders/read-order/payments", 503)
		if db.Exec(change[1]).Error != nil {
			t.Fatal("order payment failure restoration failed")
		}
		call("/orders/read-order/payments", 200)
	}
	if db.Exec("UPDATE order_payment_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE order_id='read-order'").Error != nil {
		t.Fatal("payment empty seed failed")
	}
	if json.Unmarshal(call("/orders/read-order/payments", 200), &data) != nil || len(data.Collections) != 0 {
		t.Fatal("deleted payment link still visible")
	}
	if db.Exec("UPDATE order_payment_collections SET deleted_at=NULL WHERE order_id='read-order'").Error != nil {
		t.Fatal("payment link restore failed")
	}

	digits := "(SELECT 0 AS n UNION ALL SELECT 1 UNION ALL SELECT 2 UNION ALL SELECT 3 UNION ALL SELECT 4 UNION ALL SELECT 5 UNION ALL SELECT 6 UNION ALL SELECT 7 UNION ALL SELECT 8 UNION ALL SELECT 9)"
	limitQuery := "INSERT INTO payment_sessions(id,currency_code,amount,raw_amount,provider_id,data,status,payment_collection_id,created_at,updated_at) SELECT CONCAT('limit-session-',a.n,b.n,c.n),s.currency_code,s.amount,s.raw_amount,s.provider_id,s.data,s.status,s.payment_collection_id,s.created_at,s.updated_at FROM payment_sessions s CROSS JOIN " + digits + " a CROSS JOIN " + digits + " b CROSS JOIN " + digits + " c WHERE s.id='read-session'"
	if db.Exec(limitQuery).Error != nil {
		t.Fatal("payment limit seed failed")
	}
	call("/orders/read-order/payments", 503)
	if db.Exec("DELETE FROM payment_sessions WHERE id LIKE 'limit-session-%'").Error != nil {
		t.Fatal("payment limit restore failed")
	}
	call("/orders/read-order/payments", 200)
	t.Log(`ORDER_PAYMENT_READ_REHEARSAL {"exact_amounts":true,"explicit_projection":true,"no_private_payload":true,"cross_order_isolation":true,"soft_deletes_filtered":true,"hold_missing_unverified":true,"missing_collection_rejected":true,"invalid_raw_rejected":true,"disabled_schema_rejected":true,"child_limit_rejected":true,"operations_enabled":false}`)
}
