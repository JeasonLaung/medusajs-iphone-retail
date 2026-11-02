package historyimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"net"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/catalogimport"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/inventoryimport"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/go-sql-driver/mysql"
)

// All source and target facts remain private. Both transactions roll back,
// including the deliberately conflicting edit. No schema or provider actions.
func TestNativeHistoryRollbackReplayAndConflict(t *testing.T) {
	if os.Getenv("HISTORY_NATIVE_REHEARSAL") != "1" {
		t.Skip("explicit owned native target required")
	}
	cfg, err := mysql.ParseDSN(os.Getenv("NATIVE_HISTORY_DSN"))
	if err != nil || cfg == nil {
		t.Fatal("native target configuration")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || (host != "127.0.0.1" && host != "localhost" && host != "::1") || !strings.HasPrefix(cfg.DBName, "eurorefurb_") {
		t.Fatal("dedicated native loopback target required")
	}
	ring, err := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
	if err != nil {
		t.Fatal("persistent private history keyring required")
	}
	var paymentRing *protected.Ring
	if os.Getenv("NATIVE_HISTORY_INCLUDE_PAYMENTS") == "1" {
		paymentRing, err = protected.ParseKeyring(os.Getenv("PAYMENT_KEY_ACTIVE"), os.Getenv("PAYMENT_KEYRING_JSON"))
		if err != nil {
			t.Fatal("persistent independent payment keyring required")
		}
	}
	f, err := os.Open(os.Getenv("NATIVE_HISTORY_SNAPSHOT"))
	if err != nil {
		t.Fatal("native private source required")
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	p, err := Prepare(ctx, f, os.Getenv("NATIVE_HISTORY_SHA256"), 100000, 64<<20)
	if err != nil {
		t.Fatal("native complete preparation failed")
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("native target unavailable")
	}
	defer db.Close()
	stage := func(tx *sql.Tx) (Report, error) {
		var r Report
		var e error
		if paymentRing != nil {
			r, e = p.StageWithPayments(ctx, tx, ring, paymentRing)
		} else {
			r, e = p.Stage(ctx, tx, ring)
		}
		if e != nil {
			return Report{}, e
		}
		if os.Getenv("NATIVE_HISTORY_INCLUDE_LOGISTICS") == "1" {
			l, e := p.StageLogistics(ctx, tx)
			if e != nil {
				return Report{}, e
			}
			r.Logistics = &l
		}
		if os.Getenv("NATIVE_HISTORY_INCLUDE_CARTS") == "1" {
			c, e := p.StageCarts(ctx, tx)
			if e != nil {
				return Report{}, e
			}
			r.Carts = &c
		}
		if os.Getenv("NATIVE_HISTORY_INCLUDE_DEVICE_PREREQUISITES") == "1" {
			d, e := p.StageDevicePrerequisites(ctx, tx)
			if e != nil {
				return Report{}, e
			}
			r.DevicePrerequisites = &d
		}
		return r, nil
	}
	// Sort independent row digests so auxiliary tables with composite identities
	// are fingerprinted deterministically without exposing their values.
	fingerprint := func() []byte {
		tables := []string{"historical_customer_subjects", "migration_customer_dispositions", "migration_catalog_rows", "migration_order_rows", "order_migration_reference_holds", "migration_inventory_rows", "inventory_migration_holds", "unique_key_owners", "unique_key_buckets", "unique_key_claims", "unique_key_secret_scopes", "admin_users", "admin_sessions", "customer_accounts", "customer_order_ownership"}
		for name := range p.orders {
			tables = append(tables, p.targets[name])
		}
		for name := range p.inventory {
			tables = append(tables, p.targets[name])
		}
		if paymentRing != nil {
			for name := range p.payments {
				tables = append(tables, p.targets[name])
			}
			tables = append(tables, "migration_payment_rows", "migration_payment_dispositions", "migration_holder_dispositions", "payment_migration_reference_holds")
		}
		if os.Getenv("NATIVE_HISTORY_INCLUDE_LOGISTICS") == "1" {
			for name := range p.fulfillment {
				tables = append(tables, p.targets[name])
			}
			for name := range p.tax {
				tables = append(tables, p.targets[name])
			}
			tables = append(tables, "migration_fulfillment_rows", "fulfillment_migration_reference_holds", "migration_tax_rows", "tax_migration_reference_holds")
		}
		if os.Getenv("NATIVE_HISTORY_INCLUDE_CARTS") == "1" {
			for name := range p.carts {
				tables = append(tables, p.targets[name])
			}
			tables = append(tables, "cart_ownership", "cart_creation_scopes", "cart_creation_operations", "cart_checkout_attempts", "cart_checkout_observations", "cart_completion_receipts", "cart_command_capabilities", "cart_item_runtime", "cart_contact_runtime", "cart_shipping_command_runtime", "cart_promotion_command_runtime")
		}
		if os.Getenv("NATIVE_HISTORY_INCLUDE_DEVICE_PREREQUISITES") == "1" {
			for name := range p.devicePrerequisites {
				tables = append(tables, p.targets[name])
			}
			tables = append(tables, "devices", "device_assignments", "device_events", "device_identity_claims", "device_runtime_schema")
		}
		return nativeHistoryFingerprint(t, ctx, db, tables)
	}
	before := fingerprint()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("native transaction")
	}
	defer tx.Rollback()
	first, err := stage(tx)
	if err != nil {
		t.Fatal("native history staging rejected")
	}
	second, err := stage(tx)
	if err != nil || second.OrderReferenceHolds > first.OrderReferenceHolds || first.InventoryHolds != second.InventoryHolds {
		t.Fatal("native exact history replay rejected")
	}
	if first.Carts != nil {
		if first.Carts.RuntimeAuthorityRestored || first.Carts.Rows != int(first.CartSourceRows) {
			t.Fatal("historical cart authority/scope changed")
		}
		if _, err = tx.ExecContext(ctx, "SAVEPOINT cart_precision_probe"); err != nil {
			t.Fatal("cart precision savepoint")
		}
		result, e := tx.ExecContext(ctx, "UPDATE cart_line_items SET unit_price=unit_price+0.000000000000000001 ORDER BY id LIMIT 1")
		if e != nil {
			t.Fatal("cart precision setup")
		}
		n, e := result.RowsAffected()
		if e != nil || n != 1 {
			t.Fatal("existing historical cart line required")
		}
		if _, e = p.StageCarts(ctx, tx); e != catalogimport.ErrConflict {
			t.Fatal("historical cart decimal edit accepted")
		}
		if _, e = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT cart_precision_probe"); e != nil {
			t.Fatal("cart precision rollback")
		}
		if _, err = tx.ExecContext(ctx, "SAVEPOINT cart_ownership_probe"); err != nil {
			t.Fatal("cart authority savepoint")
		}
		if len(p.carts["cart"]) == 0 {
			t.Fatal("historical cart required")
		}
		id, e := importvalue.Text(p.carts["cart"][0]["id"].Raw, 191)
		if e != nil {
			t.Fatal("historical cart identity")
		}
		_, e = tx.ExecContext(ctx, "INSERT INTO cart_ownership(cart_id,store_id,generation,version,active,created_at,updated_at) SELECT ?,id,?,1,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM commerce_stores ORDER BY id LIMIT 1", id, strings.Repeat("a", 64))
		if e != nil {
			t.Fatal("artificial uncommitted cart authority setup")
		}
		if _, e = p.StageCarts(ctx, tx); e != catalogimport.ErrConflict {
			t.Fatal("owned historical cart accepted")
		}
		if _, e = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT cart_ownership_probe"); e != nil {
			t.Fatal("cart authority rollback")
		}
	}
	if first.DevicePrerequisites != nil {
		probeDevicePrerequisites(t, ctx, tx, p)
	}
	// The source rows have passed strict conversion and exact SQL readback. A
	// one-unit change in the final decimal place must not be rounded away.
	result, err := tx.ExecContext(ctx, "UPDATE inventory_levels SET incoming_quantity=incoming_quantity+0.000000000000000001 ORDER BY id LIMIT 1")
	if err != nil {
		t.Fatal("native precision conflict setup")
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		t.Fatal("existing inventory level required")
	}
	if _, err = stage(tx); err != inventoryimport.ErrConflict {
		t.Fatal("native precision conflict accepted")
	}
	if tx.Rollback() != nil {
		t.Fatal("native history rollback failed")
	}
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("history rehearsal changed stored facts, ledgers or account ownership")
	}
	t.Logf("native history full rollback, exact replay and decimal conflict rejection passed; order rows=%d, customer subjects=%d, inventory rows=%d, order reference holds=%d, quantity holds=%d; account/ownership fingerprints unchanged", first.OrderRows, first.CustomerRows, first.InventoryRows, first.OrderReferenceHolds, first.InventoryHolds)
	if paymentRing != nil {
		t.Logf("historical payment rows=%d, staged=%t, reference holds=%d; no provider or worker invoked", first.PaymentSourceRows, first.PaymentHistoryStaged, first.PaymentReferenceHolds)
	}
	if first.Logistics != nil {
		t.Logf("historical fulfillment rows=%d, reference holds=%d, tax rows=%d, reference holds=%d; no carrier/tax provider invoked", first.Logistics.FulfillmentRows, first.Logistics.FulfillmentReferenceHolds, first.Logistics.TaxRows, first.Logistics.TaxReferenceHolds)
	}
	if first.DevicePrerequisites != nil {
		t.Logf("device prerequisites source=%d tracking=%d supply=%d; no device identities or quantities changed; source-field drift, missing parent and enabled gate rejected", first.DevicePrerequisites.SourceRows, first.DevicePrerequisites.TrackingRows, first.DevicePrerequisites.SupplyRows)
	}
	if first.Carts != nil {
		t.Logf("historical cart rows=%d; possession/creation/checkout authority fingerprints unchanged; last-place decimal edit and owned source cart rejected; artificial probes rolled back", first.Carts.Rows)
	}
}

func nativeHistoryFingerprint(t *testing.T, ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, tables []string) []byte {
	t.Helper()
	sort.Strings(tables)
	sum := sha256.New()
	for _, table := range tables {
		rows, e := db.QueryContext(ctx, "SELECT * FROM `"+table+"`")
		if e != nil {
			t.Fatal("native fingerprint query")
		}
		cols, e := rows.Columns()
		if e != nil {
			rows.Close()
			t.Fatal("native fingerprint columns")
		}
		var hashes []string
		for rows.Next() {
			cells := make([]sql.RawBytes, len(cols))
			args := make([]any, len(cols))
			for i := range cells {
				args[i] = &cells[i]
			}
			if rows.Scan(args...) != nil {
				rows.Close()
				t.Fatal("native fingerprint scan")
			}
			row := sha256.New()
			for _, cell := range cells {
				marker := byte(1)
				if cell == nil {
					marker = 0
				}
				row.Write([]byte{marker})
				row.Write(binary.BigEndian.AppendUint64(nil, uint64(len(cell))))
				row.Write(cell)
			}
			hashes = append(hashes, hex.EncodeToString(row.Sum(nil)))
		}
		if rows.Err() != nil {
			rows.Close()
			t.Fatal("native fingerprint rows")
		}
		rows.Close()
		sort.Strings(hashes)
		sum.Write([]byte(table))
		for _, hash := range hashes {
			sum.Write([]byte(hash))
		}
	}
	return sum.Sum(nil)
}
