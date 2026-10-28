package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func seedOrderReadFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO orders(id,version,status,is_draft_order,currency_code,display_id,custom_display_id,email,created_at,updated_at,metadata) VALUES
 ('read-order',2,'pending',FALSE,'eur',17,'9007199254740993123','buyer50%_!@example.invalid','2026-10-28 00:00:00.123456','2026-10-28 00:00:00.123456','{"private":"order-secret-must-not-escape"}'),
 ('other-order',1,'canceled',FALSE,'dkk',18,NULL,NULL,'2026-10-27 00:00:00','2026-10-27 00:00:00',NULL),
 ('deleted-order',1,'pending',FALSE,'eur',19,NULL,NULL,'2026-10-26 00:00:00','2026-10-26 00:00:00',NULL)`,
		`UPDATE orders SET deleted_at=UTC_TIMESTAMP(6) WHERE id='deleted-order'`,
		`INSERT INTO order_line_items(id,title,variant_id,variant_sku,requires_shipping,is_discountable,is_tax_inclusive,unit_price,raw_unit_price,is_custom_price,is_giftcard,metadata,created_at,updated_at) VALUES ('read-line','Original historical phone','deleted-source-variant','0000123456789012345',TRUE,TRUE,FALSE,10,'{"value":"10","precision":38}',FALSE,FALSE,'{"private":"line-secret-must-not-escape"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`,
	}
	for _, s := range statements {
		if db.Exec(s).Error != nil {
			t.Fatal("artificial order read seed failed")
		}
	}
	for _, v := range []struct {
		id      string
		version int
		price   string
	}{{"old-order-item", 1, "1"}, {"current-order-item", 2, "9007199254740993.000000000000000001"}} {
		if db.Exec(`INSERT INTO order_items(id,order_id,version,item_id,quantity,raw_quantity,fulfilled_quantity,raw_fulfilled_quantity,shipped_quantity,raw_shipped_quantity,return_requested_quantity,raw_return_requested_quantity,return_received_quantity,raw_return_received_quantity,return_dismissed_quantity,raw_return_dismissed_quantity,written_off_quantity,raw_written_off_quantity,delivered_quantity,raw_delivered_quantity,unit_price,raw_unit_price,created_at,updated_at)
 VALUES (?,'read-order',?,'read-line',2,'{"value":"2","precision":38}',1,'{"value":"1","precision":38}',0,'{"value":"0","precision":38}',1,'{"value":"1","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',0,'{"value":"0","precision":38}',?, ?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, v.id, v.version, v.price, `{"value":"`+v.price+`","precision":38}`).Error != nil {
			t.Fatal("artificial order version seed failed")
		}
	}
	if db.Exec(`UPDATE order_line_items SET totals_id='current-order-item' WHERE id='read-line'`).Error != nil {
		t.Fatal("artificial order cycle seed failed")
	}
	seedOrderAmountFixture(t, db)
	seedOrderPaymentFixture(t, db)
	seedOrderFulfillmentFixture(t, db)
}

func TestOwnedOrderRead(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned SQL fixture required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("order read root unavailable")
	}
	defer root.Close()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE order_read_rehearsal"); err != nil {
		t.Fatal("order read database failed")
	}
	defer root.ExecContext(context.Background(), "DROP DATABASE order_read_rehearsal")
	cfg.DBName = "order_read_rehearsal"
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("order read adapter unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("order read plan unavailable")
	}
	r, err := schema.Apply(ctx, pool, plan)
	if err != nil || r.Applied != 25 {
		t.Fatal("order read schema failed")
	}
	seedOrderReadFixture(t, db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := orders.Handler{DB: db}
	router.GET("/orders", h.List)
	router.GET("/orders/:id", h.Detail)
	router.GET("/orders/:id/amounts", h.Amounts)
	router.GET("/orders/:id/payments", h.Payments)
	router.GET("/orders/:id/fulfillments", h.Fulfillments)
	call := func(path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatal("order response status differs: " + path)
		}
		if strings.Contains(w.Body.String(), "must-not-escape") {
			t.Fatal("order private payload leaked")
		}
		return w.Body.Bytes()
	}
	fingerprint := func() [32]byte {
		var evidence []json.RawMessage
		for _, table := range []string{"orders", "order_line_items", "order_items", "order_line_tax_lines", "order_line_adjustments", "order_shipping_methods", "order_shipping_versions", "order_shipping_tax_lines", "order_shipping_adjustments", "order_credit_lines", "order_summaries", "order_payment_collections", "payment_collections", "payment_sessions", "payments", "payment_captures", "payment_refunds", "migration_payment_dispositions", "order_fulfillments", "fulfillments", "fulfillment_addresses", "fulfillment_items", "fulfillment_labels"} {
			var rows []map[string]any
			sort := "id ASC"
			if table == "order_fulfillments" {
				sort = "order_id,fulfillment_id"
			}
			if table == "order_payment_collections" {
				sort = "order_id,payment_collection_id"
			}
			if table == "migration_payment_dispositions" {
				sort = "source_table,source_id"
			}
			if db.Table(table).Order(sort).Find(&rows).Error != nil {
				t.Fatal("order fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			evidence = append(evidence, b)
		}
		b, _ := json.Marshal(evidence)
		return sha256.Sum256(b)
	}
	before := fingerprint()
	var list struct {
		Orders               []orders.Summary
		Count, Limit, Offset int
	}
	if json.Unmarshal(call("/orders?limit=1", 200), &list) != nil || list.Count != 2 || len(list.Orders) != 1 || list.Orders[0].ID != "read-order" || list.Orders[0].LineCount != 1 || list.Orders[0].CustomDisplayID == nil || *list.Orders[0].CustomDisplayID != "9007199254740993123" || *list.Orders[0].DisplayID != "17" {
		t.Fatal("order list projection differs")
	}
	if json.Unmarshal(call("/orders?q=50%25_%21&status=pending", 200), &list) != nil || list.Count != 1 {
		t.Fatal("order literal wildcard search differs")
	}
	if json.Unmarshal(call("/orders?sort=created_at_asc", 200), &list) != nil || list.Orders[0].ID != "other-order" {
		t.Fatal("order stable ascending order differs")
	}
	if json.Unmarshal(call("/orders?offset=100", 200), &list) != nil || list.Count != 2 || len(list.Orders) != 0 {
		t.Fatal("order empty page differs")
	}
	for _, q := range []string{"limit=0", "limit=101", "offset=-1", "q=%20", "status=paid", "sort=id_desc", "fields=*", "limit=1&limit=2", "q=%00", "q=%zz"} {
		call("/orders?"+q, 400)
	}
	var detail struct {
		Order orders.Summary
		Lines []orders.Line
	}
	if json.Unmarshal(call("/orders/read-order", 200), &detail) != nil || len(detail.Lines) != 1 || detail.Lines[0].ID != "current-order-item" || detail.Lines[0].Title != "Original historical phone" || detail.Lines[0].UnitPrice != "9007199254740993.000000000000000001" || *detail.Lines[0].VariantSKU != "0000123456789012345" || detail.Lines[0].Quantity != "2.000000000000000000" || detail.Lines[0].ReturnRequested != "1.000000000000000000" {
		t.Fatal("order current version or exact snapshot differs")
	}
	call("/orders/deleted-order", 404)
	call("/orders/missing", 404)
	call("/orders/read-order?fields=*", 400)
	checkOrderAmounts(t, call)
	checkOrderPayments(t, db, call)
	checkOrderFulfillments(t, db, call)
	if before != fingerprint() {
		t.Fatal("order queries mutated history")
	}
	checkOrderAmountFailures(t, db, call)
	if db.Exec("UPDATE order_items SET unit_price=NULL WHERE id='current-order-item'").Error != nil {
		t.Fatal("order fallback fixture failed")
	}
	if json.Unmarshal(call("/orders/read-order", 200), &detail) != nil || detail.Lines[0].UnitPrice != "10.000000000000000000" {
		t.Fatal("order snapshot price fallback incorrect")
	}
	var financial orders.AmountDetail
	if json.Unmarshal(call("/orders/read-order/amounts", 200), &financial) != nil || financial.Lines[0].UnitPrice != "9007199254740993.000000000000000001" {
		t.Fatal("independent raw price precedence lost")
	}
	if db.Exec("UPDATE order_items SET raw_unit_price=NULL WHERE id='current-order-item'").Error != nil {
		t.Fatal("raw fallback fixture failed")
	}
	if json.Unmarshal(call("/orders/read-order/amounts", 200), &financial) != nil || financial.Lines[0].UnitPrice != "10" || financial.Totals["total"] != "19.5" {
		t.Fatal("raw snapshot fallback differs")
	}
	if db.Exec("UPDATE order_line_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='read-line'").Error != nil {
		t.Fatal("order incomplete fixture failed")
	}
	call("/orders/read-order", 503)
	if db.Exec("UPDATE order_runtime_schema SET model_sha256=REPEAT('0',64)").Error != nil {
		t.Fatal("order drift fixture failed")
	}
	call("/orders", 503)
	t.Log(`ORDER_READ_REHEARSAL {"migrations":25,"read_only_fingerprint":true,"exact_decimal_and_ids":true,"current_version_only":true,"snapshot_price_override_and_fallback":true,"literal_wildcard_search":true,"stable_pagination":true,"private_payload_excluded":true,"incomplete_snapshot_rejected":true,"schema_drift_rejected":true,"order_amounts_exact":true,"amount_version_filters":true,"independent_raw_price_precedence":true,"amount_dependency_failures_rejected":true}`)
}
