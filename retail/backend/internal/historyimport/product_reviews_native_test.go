package historyimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalogimport"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/go-sql-driver/mysql"
)

func TestNativeProductReviewsRollbackReplayAndConflict(t *testing.T) {
	if os.Getenv("PRODUCT_REVIEWS_HISTORY_NATIVE") != "1" {
		t.Skip("explicit owned native history fixture required")
	}
	cfg, err := mysql.ParseDSN(os.Getenv("NATIVE_HISTORY_DSN"))
	if err != nil || cfg == nil {
		t.Fatal("native configuration")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || host != "127.0.0.1" || !strings.HasPrefix(cfg.DBName, "eurorefurb_review_history_") {
		t.Fatal("owned loopback target required")
	}
	f, err := os.Open(os.Getenv("NATIVE_HISTORY_SNAPSHOT"))
	if err != nil {
		t.Fatal("private source required")
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	p, err := Prepare(ctx, f, os.Getenv("NATIVE_HISTORY_SHA256"), 100000, 64<<20)
	if err != nil || len(p.productReviews) == 0 {
		t.Fatal("complete source review preparation")
	}
	ring, err := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
	if err != nil {
		t.Fatal("private fixture keyring required")
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("native target")
	}
	defer db.Close()
	tables := []string{"product_reviews", "product_review_commands", "product_review_runtime", "migration_catalog_rows", "customer_accounts", "customer_order_ownership"}
	before := nativeHistoryFingerprint(t, ctx, db, tables)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("native transaction")
	}
	defer tx.Rollback()
	if _, err = p.Stage(ctx, tx, ring); err != nil {
		t.Fatal("source parent staging")
	}
	protectedTables := []string{"inventory_levels", "inventory_reservations", "pricing_prices", "device_assignments", "payments", "payment_captures", "customer_accounts", "customer_order_ownership"}
	facts := nativeHistoryFingerprint(t, ctx, tx, protectedTables)
	first, err := p.StageProductReviews(ctx, tx)
	if err != nil || first.Rows != len(p.productReviews) || first.NewAccountBindings != 0 || first.RuntimeEnabled {
		t.Fatal("review history staging")
	}
	second, err := p.StageProductReviews(ctx, tx)
	if err != nil || first != second {
		t.Fatal("exact replay")
	}
	if !bytes.Equal(facts, nativeHistoryFingerprint(t, ctx, tx, protectedTables)) {
		t.Fatal("review import changed other facts")
	}
	id, err := importvalue.Text(p.productReviews[0]["id"].Raw, 191)
	if err != nil {
		t.Fatal("source review identity")
	}
	probe := func(name, query string, args ...any) {
		t.Helper()
		if _, e := tx.ExecContext(ctx, "SAVEPOINT review_probe"); e != nil {
			t.Fatal("savepoint")
		}
		if _, e := tx.ExecContext(ctx, query, args...); e != nil {
			t.Fatal("probe setup", name)
		}
		if _, e := p.StageProductReviews(ctx, tx); e == nil {
			t.Fatal("changed history accepted", name)
		}
		if _, e := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT review_probe"); e != nil {
			t.Fatal("probe rollback")
		}
	}
	probe("runtime reads", "UPDATE product_review_runtime SET reads_enabled=TRUE WHERE id=1")
	probe("moderation edit", "UPDATE product_reviews SET status=IF(status='approved','rejected','approved') WHERE id=?", id)
	probe("content edit", "UPDATE product_reviews SET content=CONCAT(content,' changed') WHERE id=?", id)
	if _, err = tx.ExecContext(ctx, "SAVEPOINT review_account"); err != nil {
		t.Fatal("account savepoint")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,created_at,updated_at) VALUES('history-probe','history-probe@example.invalid','artificial-unusable','New','Account',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`); err != nil {
		t.Fatal("artificial account")
	}
	probe("new account binding", "UPDATE product_reviews SET account_id='history-probe' WHERE id=?", id)
	if _, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT review_account"); err != nil {
		t.Fatal("account rollback")
	}
	for _, bad := range []struct {
		key  string
		cell rowimport.Cell
	}{
		{"rating", rowimport.Cell{Raw: json.RawMessage(`0`)}},
		{"product_id", rowimport.Cell{Raw: json.RawMessage(`"missing-product"`)}},
		{"customer_id", rowimport.Cell{Raw: json.RawMessage(`"missing-customer"`)}},
		{"order_id", rowimport.Cell{Raw: json.RawMessage(`"missing-order"`)}},
	} {
		row := rowimport.SourceRow{}
		for k, v := range p.productReviews[0] {
			row[k] = v
		}
		row[bad.key] = bad.cell
		if e := catalogimport.ImportProductReviewHistory(ctx, tx, p.report.SnapshotSHA, row); e == nil {
			t.Fatal("invalid source accepted", bad.key)
		}
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal("full rollback")
	}
	if !bytes.Equal(before, nativeHistoryFingerprint(t, ctx, db, tables)) {
		t.Fatal("history rollback changed target")
	}
	t.Logf(`PRODUCT_REVIEWS_HISTORY_NATIVE {"source_rows":%d,"exact_replay":true,"closed_runtime_required":true,"source_edits_rejected":true,"missing_parents_rejected":true,"new_account_binding_rejected":true,"other_facts_unchanged":true,"rollback_restored":true}`, len(p.productReviews))
}
