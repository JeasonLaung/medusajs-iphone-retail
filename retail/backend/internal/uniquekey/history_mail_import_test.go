package uniquekey

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

func mailSQLFixture(suffix string) rowimport.SourceRow {
	text := func(s string) rowimport.Cell { raw, _ := json.Marshal(s); return rowimport.Cell{Raw: raw} }
	r := rowimport.SourceRow{}
	for _, col := range []string{"previous_folder", "from_address", "subject", "text_body", "html_body", "status", "error", "created_by"} {
		r[col] = text("")
	}
	r["id"] = text("mail_verify_" + strings.Repeat(suffix, 64))
	r["account_id"] = text("fixture-account")
	r["folder"] = text("outbox")
	r["direction"] = text("outbound")
	r["subject"] = text("Verify your email to join EuroRefurb")
	r["created_by"] = text("registration-verification")
	r["text_body"] = text("old-auth-capability")
	r["html_body"] = text("old-auth-capability")
	r["status"] = text("queued")
	for _, col := range []string{"to", "cc", "bcc", "attachments", "reply_to"} {
		r[col] = rowimport.Cell{Raw: json.RawMessage(`[]`)}
	}
	for _, col := range []string{"read", "starred"} {
		r[col] = rowimport.Cell{Raw: json.RawMessage(`false`)}
	}
	r["attempts"] = rowimport.Cell{Raw: json.RawMessage(`2`)}
	for _, col := range []string{"provider_id", "message_id", "in_reply_to", "sent_at", "received_at", "deleted_at", "first_attempt_at"} {
		r[col] = rowimport.Cell{SQLNull: true}
	}
	for _, col := range []string{"created_at", "updated_at"} {
		r[col] = text("2026-10-27T00:00:00.123456Z")
	}
	return r
}

func testHistoricalMailSQL(t *testing.T, ctx context.Context) {
	t.Helper()
	cfg := mysql.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "relational_fixture"
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("mail fixture configuration failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	ddl, err := migrations.Files.ReadFile("0004_migration_mail_dispositions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(ddl)); err != nil {
		t.Fatal("mail ledger migration failed", err)
	}
	snapshot := strings.Repeat("a", 64)
	run := func(row rowimport.SourceRow, hash string, commit bool) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := rowimport.ImportHistoricalMail(ctx, tx, hash, row); err != nil {
			return err
		}
		if commit {
			return tx.Commit()
		}
		return tx.Rollback()
	}
	count := func(table, id string) int {
		var n int
		column := "id"
		if table == "migration_mail_dispositions" {
			column = "source_id"
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE "+column+"=?", id).Scan(&n); err != nil {
			t.Fatal("mail fixture count failed")
		}
		return n
	}
	id := "mail_verify_" + strings.Repeat("a", 64)
	if err := run(mailSQLFixture("a"), snapshot, false); err != nil {
		t.Fatal("mail rollback staging failed", err)
	}
	if count("mail_messages", id) != 0 || count("migration_mail_dispositions", id) != 0 {
		t.Fatal("mail or ledger survived rollback")
	}
	if err := run(mailSQLFixture("a"), snapshot, true); err != nil {
		t.Fatal("mail atomic commit failed", err)
	}
	if err := run(mailSQLFixture("a"), snapshot, true); err != nil {
		t.Fatal("exact replay refused", err)
	}
	var body, html, attachments, status string
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT text_body,html_body,attachments,status,attempts FROM mail_messages WHERE id=?", id).Scan(&body, &html, &attachments, &status, &attempts); err != nil || body != "" || html != "" || attachments != "[]" || status != "queued" || attempts != 2 {
		t.Fatal("persisted mail redaction/history differs")
	}
	tx, _ := db.BeginTx(ctx, nil)
	disposition, err := rowimport.HistoricalMailDeliveryDisposition(ctx, tx, id)
	_ = tx.Rollback()
	if err != nil || disposition != "suppress-permanently" {
		t.Fatal("durable suppression unavailable")
	}
	tx, _ = db.BeginTx(ctx, nil)
	_, err = rowimport.HistoricalMailDeliveryDisposition(ctx, tx, "missing-mail")
	_ = tx.Rollback()
	if err != rowimport.ErrHistoryStorage {
		t.Fatal("missing ledger allowed delivery")
	}
	if err := run(mailSQLFixture("a"), strings.Repeat("b", 64), true); err != rowimport.ErrHistoryConflict {
		t.Fatal("changed snapshot accepted")
	}
	changed := mailSQLFixture("a")
	changed["text_body"] = rowimport.Cell{Raw: json.RawMessage(`"changed source auth body"`)}
	if err := run(changed, snapshot, true); err != rowimport.ErrHistoryConflict {
		t.Fatal("redaction hid source drift")
	}
	if _, err := db.ExecContext(ctx, "UPDATE mail_messages SET status='sending' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := run(mailSQLFixture("a"), snapshot, true); err != rowimport.ErrHistoryConflict {
		t.Fatal("target drift accepted")
	}
	if _, err := db.ExecContext(ctx, "UPDATE mail_messages SET status='queued' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	// Eight exact imports must serialize on the immutable initial ledger token.
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- run(mailSQLFixture("b"), snapshot, true) }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("concurrent exact mail import failed", err)
		}
	}
	bID := "mail_verify_" + strings.Repeat("b", 64)
	if count("mail_messages", bID) != 1 || count("migration_mail_dispositions", bID) != 1 {
		t.Fatal("concurrent import duplicated facts")
	}
	// Force a DB failure AFTER the ledger insertion, then prove caller rollback.
	if _, err := db.ExecContext(ctx, "ALTER TABLE mail_messages ADD CONSTRAINT fixture_reject_mail CHECK (account_id <> 'reject-fixture')"); err != nil {
		t.Fatal("mail failure fixture unavailable")
	}
	rejected := mailSQLFixture("c")
	rejected["account_id"] = rowimport.Cell{Raw: json.RawMessage(`"reject-fixture"`)}
	if err := run(rejected, snapshot, true); err != rowimport.ErrHistoryStorage {
		t.Fatal("expected business insert failure missing")
	}
	cID := "mail_verify_" + strings.Repeat("c", 64)
	if count("mail_messages", cID) != 0 || count("migration_mail_dispositions", cID) != 0 {
		t.Fatal("partial failed import persisted")
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM mail_messages WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := run(mailSQLFixture("a"), snapshot, true); err != rowimport.ErrHistoryConflict || count("mail_messages", id) != 0 {
		t.Fatal("missing target silently recreated")
	}
	if _, err := db.ExecContext(ctx, "UPDATE migration_mail_dispositions SET disposition='allow' WHERE source_id=?", bID); err == nil {
		t.Fatal("ledger granted unsupported permission")
	}
	business := mailSQLFixture("d")
	business["id"] = rowimport.Cell{Raw: json.RawMessage(`"business-history-mail"`)}
	business["created_by"] = rowimport.Cell{Raw: json.RawMessage(`"legacy-staff"`)}
	business["text_body"] = rowimport.Cell{Raw: json.RawMessage(`" business text retained "`)}
	if err := run(business, snapshot, true); err != nil {
		t.Fatal("business history hold failed", err)
	}
	tx, _ = db.BeginTx(ctx, nil)
	disposition, err = rowimport.HistoricalMailDeliveryDisposition(ctx, tx, "business-history-mail")
	_ = tx.Rollback()
	if err != nil || disposition != "hold" {
		t.Fatal("ordinary imported business mail not held")
	}
	if err := db.QueryRowContext(ctx, "SELECT text_body FROM mail_messages WHERE id='business-history-mail'").Scan(&body); err != nil || body != " business text retained " {
		t.Fatal("ordinary mail body changed")
	}
}
