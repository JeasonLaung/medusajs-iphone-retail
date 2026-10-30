package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/catalogimport"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Complete pinned source plus actual private bytes, owned target only. Failures
// deliberately contain no identifiers, values, paths or driver errors.
func TestActualBuybackHistoryImport(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" || os.Getenv("ACTUAL_BUYBACK_HISTORY") != "1" {
		t.Skip("explicit owned real-data rehearsal required")
	}
	const snapshot = "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
	f, e := os.Open("/tmp/buyback-history-source.ndjson")
	if e != nil {
		t.Fatal("pinned source unavailable")
	}
	defer f.Close()
	selected := map[string][]rowimport.SourceRow{"buyback_request": {}, "buyback_photo": {}, "buyback_device_link": {}}
	customers := map[string]bool{}
	_, e = rowimport.VerifyStream(f, snapshot, func(table string, r rowimport.SourceRow) error {
		if _, ok := selected[table]; ok {
			selected[table] = append(selected[table], r)
		}
		if table == "customer" {
			id, e := importvalue.Text(r["id"].Raw, 191)
			if e != nil {
				return e
			}
			customers[id] = true
		}
		return nil
	})
	if e != nil || len(selected["buyback_request"]) != 1 || len(selected["buyback_photo"]) != 6 || len(selected["buyback_device_link"]) != 0 {
		t.Fatal("complete pinned source counts differ")
	}
	root, e := os.OpenRoot("/tmp/buyback-private")
	if e != nil {
		t.Fatal("private source unavailable")
	}
	defer root.Close()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal("owned SQL unavailable")
	}
	defer db.Close()
	ctx := context.Background()
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal("owned transaction unavailable")
	}
	defer tx.Rollback()
	gdb, e := gorm.Open(mysql.New(mysql.Config{Conn: tx, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("owned adapter unavailable")
	}
	var actor identity.User
	if gdb.Where("id='new-browser-fixture'").Take(&actor).Error != nil {
		t.Fatal("new employee fixture unavailable")
	}
	text := func(r rowimport.SourceRow, k string) string {
		v, e := importvalue.Text(r[k].Raw, 0)
		if e != nil {
			t.Fatal("source text invalid")
		}
		return v
	}
	for _, r := range selected["buyback_request"] {
		if !customers[text(r, "customer_id")] {
			t.Fatal("historical customer evidence absent")
		}
		if catalogimport.ImportBuybackHistory(ctx, tx, snapshot, "buyback_request", r) != nil {
			t.Fatal("actual request import failed")
		}
		if buyback.BindHistoricalStoreTx(ctx, gdb, text(r, "id"), "fixture-store", snapshot) != nil {
			t.Fatal("explicit rehearsal store binding failed")
		}
	}
	for _, r := range selected["buyback_photo"] {
		if catalogimport.ImportBuybackHistory(ctx, tx, snapshot, "buyback_photo", r) != nil {
			t.Fatal("actual photo metadata import failed")
		}
		key := text(r, "storage_key")
		data, e := buyback.ReadLegacyPhoto(root, key)
		if e != nil {
			t.Fatal("actual private read failed")
		}
		sum := sha256.Sum256(data)
		for i := 0; i < 2; i++ {
			if buyback.ImportLegacyPhotoTx(ctx, gdb, snapshot, text(r, "request_id"), text(r, "id"), key, hex.EncodeToString(sum[:]), data) != nil {
				t.Fatal("actual private copy or replay failed")
			}
		}
		if catalogimport.ImportBuybackHistory(ctx, tx, snapshot, "buyback_photo", r) != nil {
			t.Fatal("private copy changed source metadata")
		}
		v, e := buyback.AdminPhotoTx(ctx, gdb, actor, "fixture-store", text(r, "request_id"), text(r, "id"))
		if e != nil {
			t.Fatal("actual private platform read failed")
		}
		decoded, e := base64.StdEncoding.DecodeString(v.Content)
		if e != nil || sha256.Sum256(decoded) != sum || v.MimeType != text(r, "mime_type") {
			t.Fatal("actual private roundtrip differs")
		}
	}
	for _, r := range selected["buyback_request"] {
		id := text(r, "id")
		out, e := buyback.AdminRequestTx(ctx, gdb, actor, "fixture-store", id, nil)
		if e != nil || !out.Historical || out.Enabled || out.Request.ID != id || out.Request.Status != text(r, "status") || out.Request.ContactName != text(r, "contact_name") || out.Request.Email != text(r, "email") || out.Request.Phone != text(r, "phone") || len(out.Request.Photos) != 6 {
			t.Fatal("actual historical detail differs")
		}
		encoded, e := json.Marshal(out.Request)
		if e != nil || os.WriteFile("/tmp/actual-buyback-history-detail.json", encoded, 0600) != nil {
			t.Fatal("private DTO verification input unavailable")
		}
		list, e := buyback.AdminListTx(ctx, gdb, actor, "fixture-store", text(r, "status"), id, 0)
		if e != nil || list.Count != 1 || len(list.Requests) != 1 {
			t.Fatal("actual historical list differs")
		}
		if _, e := buyback.ReadRequest(ctx, gdb, "fixture-store", "cust_buyback-browser", id); e == nil {
			t.Fatal("actual history granted customer access")
		}
		if catalogimport.ImportBuybackHistory(ctx, tx, snapshot, "buyback_request", r) != nil {
			t.Fatal("actual request replay failed")
		}
		var ownership int
		if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM customer_buyback_submissions WHERE request_id=?", id).Scan(&ownership) != nil || ownership != 0 {
			t.Fatal("actual history granted ownership")
		}
	}
	if tx.Rollback() != nil {
		t.Fatal("actual history rollback failed")
	}
	for _, r := range selected["buyback_request"] {
		var count int
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM buyback_requests WHERE id=?", text(r, "id")).Scan(&count) != nil || count != 0 {
			t.Fatal("actual history rollback leaked")
		}
	}
	report, _ := json.Marshal(map[string]any{"source_requests": 1, "source_photos": 6, "source_device_links": 0, "complete_snapshot_verified": true, "exact_metadata_replay": true, "private_bytes_roundtrip": 6, "platform_read": true, "customer_access_granted": false, "caller_rollback": true, "real_store_assignment_verified": false})
	t.Log("ACTUAL_BUYBACK_HISTORY " + string(report))
}
