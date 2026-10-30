package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/catalogimport"
	"github.com/eurorefurb/retail/internal/rowimport"
	driver "github.com/go-sql-driver/mysql"
)

func TestVerifyBuybackHistoryImport(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned fixture required")
	}
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal("fixture SQL")
	}
	defer db.Close()
	ctx := context.Background()
	row := func(values map[string]any) rowimport.SourceRow {
		r := rowimport.SourceRow{}
		for k, v := range values {
			if v == nil {
				r[k] = rowimport.Cell{SQLNull: true}
			} else {
				b, _ := json.Marshal(v)
				r[k] = rowimport.Cell{Raw: b}
			}
		}
		return r
	}
	stamp := "2026-10-20T12:00:00.123456Z"
	request := row(map[string]any{"id": "history-request", "customer_id": "legacy-customer-no-login", "submission_key": "legacy-submission", "contact_name": "Historical 客户", "email": "history@example.invalid", "phone": "123", "return_address": map[string]any{"city": "Historical"}, "items": []any{map[string]any{"id": "history-item"}}, "status": "paid", "reference_amount": nil, "final_amount": nil, "currency_code": "eur", "shipping_instructions": "keep original", "tracking_number": "OLD-SHIP", "return_tracking": "", "payment_reference": "OLD-PAY", "review_note": "original note", "history": []any{map[string]any{"action": "mark_paid"}}, "closed_at": stamp, "created_at": stamp, "updated_at": stamp, "deleted_at": nil, "raw_reference_amount": nil, "raw_final_amount": nil})
	photo := row(map[string]any{"id": "history-photo", "request_id": "history-request", "item_id": "history-item", "photo_type": "front", "uploaded_at": stamp, "storage_key": "legacy/private/photo.png", "mime_type": "image/png", "review_status": "approved", "review_note": "original approval", "created_at": stamp, "updated_at": stamp, "deleted_at": nil})
	link := row(map[string]any{"id": "history-link", "request_id": "history-request", "item_id": "history-item", "device_id": "inspection-browser-device", "created_at": stamp, "updated_at": stamp, "deleted_at": nil})
	sha := strings.Repeat("a", 64)
	for _, mode := range []string{"replay", "source-drift", "snapshot-drift", "target-drift", "foreign-item", "duplicate-item", "duplicate-device", "soft-deleted", "decimal-drift"} {
		t.Run(mode, func(t *testing.T) {
			tx, e := db.BeginTx(ctx, nil)
			if e != nil {
				t.Fatal("transaction")
			}
			defer tx.Rollback()
			apply := func(table string, r rowimport.SourceRow) error {
				return catalogimport.ImportBuybackHistory(ctx, tx, sha, table, r)
			}
			r := rowimport.SourceRow{}
			for k, v := range request {
				r[k] = v
			}
			if mode == "decimal-drift" {
				r["reference_amount"] = rowimport.Cell{Raw: json.RawMessage(`1.000000000000000001`)}
				r["raw_reference_amount"] = rowimport.Cell{Raw: json.RawMessage(`{"value":"1.000000000000000001","precision":100}`)}
			}
			for _, entry := range []struct {
				table string
				r     rowimport.SourceRow
			}{{"buyback_request", r}, {"buyback_photo", photo}, {"buyback_device_link", link}} {
				if e := apply(entry.table, entry.r); e != nil {
					t.Fatal("initial history staging", entry.table, e)
				}
				if e := apply(entry.table, entry.r); e != nil {
					t.Fatal("exact replay", e)
				}
			}
			var expected error
			switch mode {
			case "source-drift":
				r["review_note"] = rowimport.Cell{Raw: json.RawMessage(`"changed"`)}
				expected = apply("buyback_request", r)
			case "snapshot-drift":
				expected = catalogimport.ImportBuybackHistory(ctx, tx, strings.Repeat("b", 64), "buyback_request", r)
			case "target-drift":
				if _, e := tx.ExecContext(ctx, "UPDATE buyback_requests SET review_note='changed' WHERE id='history-request'"); e != nil {
					t.Fatal(e)
				}
				expected = apply("buyback_request", r)
			case "decimal-drift":
				if _, e := tx.ExecContext(ctx, "UPDATE buyback_requests SET reference_amount=1.000000000000000002 WHERE id='history-request'"); e != nil {
					t.Fatal(e)
				}
				expected = apply("buyback_request", r)
			case "foreign-item":
				p := rowimport.SourceRow{}
				for k, v := range photo {
					p[k] = v
				}
				p["id"] = rowimport.Cell{Raw: json.RawMessage(`"foreign-photo"`)}
				p["item_id"] = rowimport.Cell{Raw: json.RawMessage(`"other-item"`)}
				expected = apply("buyback_photo", p)
			case "duplicate-item", "duplicate-device", "soft-deleted":
				l := rowimport.SourceRow{}
				for k, v := range link {
					l[k] = v
				}
				l["id"] = rowimport.Cell{Raw: json.RawMessage(`"second-link"`)}
				if mode == "duplicate-item" {
					if _, e := tx.ExecContext(ctx, `INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) SELECT 'history-second-device','HISTORY-SECOND','000000000000009',variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at FROM devices WHERE id='inspection-browser-device'`); e != nil {
						t.Fatal("second device fixture")
					}
					l["device_id"] = rowimport.Cell{Raw: json.RawMessage(`"history-second-device"`)}
				}
				if mode == "duplicate-device" {
					if _, e := tx.ExecContext(ctx, `UPDATE buyback_requests SET items='[{"id":"history-item"},{"id":"second-item"}]' WHERE id='history-request'`); e != nil {
						t.Fatal(e)
					}
					l["item_id"] = rowimport.Cell{Raw: json.RawMessage(`"second-item"`)}
				}
				if mode == "soft-deleted" {
					l["deleted_at"] = rowimport.Cell{Raw: json.RawMessage(`"2026-10-20T12:00:00.123456Z"`)}
				}
				expected = apply("buyback_device_link", l)
			}
			if mode != "replay" && mode != "soft-deleted" && expected == nil {
				t.Fatal("conflict accepted")
			}
			if mode == "soft-deleted" && expected != nil {
				t.Fatal("soft deleted history lost", expected)
			}
			if mode == "source-drift" || mode == "snapshot-drift" || mode == "target-drift" || mode == "decimal-drift" {
				if !errors.Is(expected, catalogimport.ErrConflict) {
					t.Fatal("expected evidence conflict", expected)
				}
			}
			var bindings, accounts, bytes int
			if tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM customer_buyback_submissions WHERE request_id='history-request'),(SELECT COUNT(*) FROM customer_accounts WHERE id='legacy-customer-no-login'),(SELECT COUNT(*) FROM buyback_photo_bytes WHERE photo_id='history-photo')`).Scan(&bindings, &accounts, &bytes) != nil || bindings != 0 || accounts != 0 || bytes != 0 {
				t.Fatal("history acquired ownership or invented file bytes")
			}
		})
	}
	var count int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM buyback_requests WHERE id='history-request'").Scan(&count) != nil || count != 0 {
		t.Fatal("caller rollback failed")
	}
	t.Log(`BUYBACK_HISTORY_IMPORT {"cases":9,"exact_replay":true,"conflict_rejected":true,"ownership_granted":false,"external_files_touched":false,"caller_rollback":true}`)
}
