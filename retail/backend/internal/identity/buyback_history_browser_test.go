package identity_test

import (
	"database/sql"
	"os"
	"testing"

	driver "github.com/go-sql-driver/mysql"
)

// Artificial historical fixture, created only after original durable checks.
func TestPrepareBuybackHistoryBrowser(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned fixture required")
	}
	c := driver.NewConfig()
	c.User = "root"
	c.Passwd = os.Getenv("MYSQL_PWD")
	c.Net = "unix"
	c.Addr = "/var/run/mysqld/mysqld.sock"
	c.DBName = "browser_identity"
	db, e := sql.Open("mysql", c.FormatDSN())
	if e != nil {
		t.Fatal("fixture")
	}
	defer db.Close()
	tx, e := db.BeginTx(t.Context(), nil)
	if e != nil {
		t.Fatal("fixture transaction")
	}
	defer tx.Rollback()
	queries := []string{
		`INSERT INTO buyback_requests(id,customer_id,submission_key,contact_name,email,phone,return_address,items,status,reference_amount,raw_reference_amount,final_amount,raw_final_amount,currency_code,shipping_instructions,tracking_number,return_tracking,payment_reference,review_note,history,closed_at,created_at,updated_at)
SELECT 'historical-browser-request','legacy-no-login','historical-browser-key','Historical Customer','historical@example.invalid',phone,return_address,items,'paid',reference_amount,raw_reference_amount,final_amount,raw_final_amount,currency_code,shipping_instructions,tracking_number,return_tracking,'HISTORICAL-PAY',review_note,history,closed_at,created_at,updated_at FROM buyback_requests WHERE status IN ('paid','returned') LIMIT 1`,
		`INSERT INTO buyback_history_stores VALUES('historical-browser-request','fixture-store',UNHEX(REPEAT('a',64)))`,
		`INSERT INTO buyback_photos(id,request_id,item_id,photo_type,uploaded_at,storage_key,mime_type,review_status,review_note,created_at,updated_at) SELECT 'historical-browser-photo','historical-browser-request',p.item_id,p.photo_type,p.uploaded_at,'historical-private-key',p.mime_type,p.review_status,p.review_note,p.created_at,p.updated_at FROM buyback_photos p JOIN buyback_photo_bytes b ON b.photo_id=p.id LIMIT 1`,
		`INSERT INTO buyback_photo_bytes(photo_id,mime_type,sha256,content,created_at) SELECT 'historical-browser-photo',b.mime_type,b.sha256,b.content,b.created_at FROM buyback_photo_bytes b LIMIT 1`,
		`INSERT INTO buyback_photo_imports(photo_id,source_key,snapshot_sha256,content_sha256,mime_type,created_at) SELECT photo_id,'historical-private-key',UNHEX(REPEAT('a',64)),sha256,mime_type,created_at FROM buyback_photo_bytes WHERE photo_id='historical-browser-photo'`,
	}
	for _, q := range queries {
		if _, e := tx.ExecContext(t.Context(), q); e != nil {
			t.Fatal("historical fixture staging")
		}
	}
	if tx.Commit() != nil {
		t.Fatal("historical fixture commit")
	}
	t.Log("OWNED_BUYBACK_HISTORY_BROWSER_READY")
}
