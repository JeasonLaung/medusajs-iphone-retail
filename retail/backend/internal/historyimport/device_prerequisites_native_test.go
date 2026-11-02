package historyimport

import (
	"bytes"
	"context"
	"database/sql"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalogimport"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/go-sql-driver/mysql"
)

// This focused test does not stage orders or depend on their runtime version.
// It uses only already imported catalog parents, and always rolls back.
func TestNativeDevicePrerequisitesRollback(t *testing.T) {
	if os.Getenv("DEVICE_PREREQUISITES_NATIVE_REHEARSAL") != "1" {
		t.Skip("explicit owned native target required")
	}
	cfg, err := mysql.ParseDSN(os.Getenv("NATIVE_HISTORY_DSN"))
	if err != nil || cfg == nil {
		t.Fatal("native configuration")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || (host != "127.0.0.1" && host != "localhost" && host != "::1") || !strings.HasPrefix(cfg.DBName, "eurorefurb_") {
		t.Fatal("dedicated native loopback target required")
	}
	f, err := os.Open(os.Getenv("NATIVE_HISTORY_SNAPSHOT"))
	if err != nil {
		t.Fatal("private snapshot required")
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	p, err := Prepare(ctx, f, os.Getenv("NATIVE_HISTORY_SHA256"), 100000, 64<<20)
	if err != nil {
		t.Fatal("full stream verification")
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("native connection")
	}
	defer db.Close()
	tables := []string{"device_sources", "device_variant_tracking", "device_supply_commitments", "migration_catalog_rows", "devices", "device_assignments", "device_events", "device_identity_claims", "device_runtime_schema", "inventory_levels", "inventory_reservations"}
	if os.Getenv("NATIVE_HISTORY_INCLUDE_CARTS") == "1" {
		for name := range p.carts {
			tables = append(tables, p.targets[name])
		}
		tables = append(tables, "cart_ownership", "cart_creation_scopes", "cart_creation_operations", "cart_checkout_attempts", "cart_checkout_observations", "cart_completion_receipts", "cart_command_capabilities")
	}
	before := nativeHistoryFingerprint(t, ctx, db, tables)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("transaction")
	}
	defer tx.Rollback()
	protectedTables := []string{"devices", "device_assignments", "device_events", "device_identity_claims", "device_runtime_schema", "inventory_levels", "inventory_reservations"}
	protectedBefore := nativeHistoryFingerprint(t, ctx, tx, protectedTables)
	first, err := p.StageDevicePrerequisites(ctx, tx)
	if err != nil {
		t.Fatal("device prerequisite staging")
	}
	second, err := p.StageDevicePrerequisites(ctx, tx)
	if err != nil || first != second || first.DeviceIdentitiesImported || first.CommandsEnabled || int64(first.SourceRows+first.TrackingRows+first.SupplyRows) != p.report.DevicePrerequisiteRows {
		t.Fatal("device prerequisite replay/scope")
	}
	if !bytes.Equal(protectedBefore, nativeHistoryFingerprint(t, ctx, tx, protectedTables)) {
		t.Fatal("staging changed identities, gates or quantities")
	}
	if os.Getenv("NATIVE_HISTORY_INCLUDE_CARTS") == "1" {
		a, e := p.StageCarts(ctx, tx)
		if e != nil {
			t.Fatal("committed historical cart staging")
		}
		b, e := p.StageCarts(ctx, tx)
		if e != nil || a != b || a.RuntimeAuthorityRestored || int64(a.Rows) != p.report.CartSourceRows {
			t.Fatal("historical cart replay after shared writer change")
		}
		if _, e = tx.ExecContext(ctx, "SAVEPOINT shared_cart_writer_probe"); e != nil {
			t.Fatal("cart savepoint")
		}
		r, e := tx.ExecContext(ctx, "UPDATE cart_line_items SET unit_price=unit_price+0.000000000000000001 ORDER BY id LIMIT 1")
		if e != nil {
			t.Fatal("cart precision setup")
		}
		n, e := r.RowsAffected()
		if e != nil || n != 1 {
			t.Fatal("existing cart line required")
		}
		if _, e = p.StageCarts(ctx, tx); e != catalogimport.ErrConflict {
			t.Fatal("cart drift accepted after writer change")
		}
		if _, e = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT shared_cart_writer_probe"); e != nil {
			t.Fatal("cart probe rollback")
		}
		t.Logf("shared history writer cart replay rows=%d; decimal conflict rejected; no runtime authority restored", a.Rows)
	}
	probeDevicePrerequisites(t, ctx, tx, p)
	if tx.Rollback() != nil || !bytes.Equal(before, nativeHistoryFingerprint(t, ctx, db, tables)) {
		t.Fatal("rollback changed stored device, quantity or evidence facts")
	}
	t.Logf("native device prerequisite rollback and exact replay passed; source=%d tracking=%d supply=%d; source-field drift, missing parent and enabled gate rejected; identities/claims/quantities unchanged", first.SourceRows, first.TrackingRows, first.SupplyRows)
}

func probeDevicePrerequisites(t *testing.T, ctx context.Context, tx *sql.Tx, p *Snapshot) {
	t.Helper()
	if len(p.devicePrerequisites["device_variant"]) == 0 {
		t.Fatal("tracking source required")
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT device_prerequisite_probe"); err != nil {
		t.Fatal("device savepoint")
	}
	r, err := tx.ExecContext(ctx, "UPDATE device_variant_tracking SET updated_at=DATE_ADD(updated_at, INTERVAL 1 MICROSECOND) ORDER BY id LIMIT 1")
	if err != nil {
		t.Fatal("tracking drift setup")
	}
	n, err := r.RowsAffected()
	if err != nil || n != 1 {
		t.Fatal("tracking fact required")
	}
	if _, err = p.StageDevicePrerequisites(ctx, tx); err != catalogimport.ErrConflict {
		t.Fatal("tracking drift accepted")
	}
	if _, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT device_prerequisite_probe"); err != nil {
		t.Fatal("tracking drift rollback")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE device_runtime_schema SET identity_enabled=TRUE WHERE id=1"); err != nil {
		t.Fatal("gate setup")
	}
	if _, err = p.StageDevicePrerequisites(ctx, tx); err != ErrPolicy {
		t.Fatal("enabled identity gate accepted")
	}
	if _, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT device_prerequisite_probe"); err != nil {
		t.Fatal("gate rollback")
	}
	if _, err = tx.ExecContext(ctx, "UPDATE device_runtime_schema SET commands_enabled=TRUE WHERE id=1"); err != nil {
		t.Fatal("command gate setup")
	}
	if _, err = p.StageDevicePrerequisites(ctx, tx); err != ErrPolicy {
		t.Fatal("enabled command gate accepted")
	}
	if _, err = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT device_prerequisite_probe"); err != nil {
		t.Fatal("command gate rollback")
	}
	// Valid strict source row, but an absent parent; the row importer must reject
	// before the immutable row writer even on an existing exact source identity.
	source := p.devicePrerequisites["device_variant"][0]
	altered := rowimport.SourceRow{}
	for k, v := range source {
		altered[k] = v
	}
	altered["variant_id"] = rowimport.Cell{Raw: []byte(`"native-prerequisite-absent-parent"`)}
	if err = catalogimport.ImportDevicePrerequisite(ctx, tx, p.report.SnapshotSHA, "device_variant", altered); err != catalogimport.ErrConflict {
		t.Fatal("absent tracking parent accepted")
	}
}
