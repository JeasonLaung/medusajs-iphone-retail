package holderimport

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/go-sql-driver/mysql"
)

// Opt-in, fixed tmpfs input and unix-only disposable target. Private failures
// are fixed strings. Source values, driver errors and in-memory hashes are not logged.
func TestActualHolderMySQL(t *testing.T) {
	if os.Getenv("HOLDER_MYSQL_REHEARSAL") != "1" {
		t.Skip("explicit owned isolated rehearsal required")
	}
	const hash = "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
	file, err := os.Open("/tmp/holder-source.ndjson")
	if err != nil {
		t.Fatal("private holder snapshot unavailable")
	}
	defer file.Close()
	var holders, providers []rowimport.SourceRow
	verified, err := rowimport.VerifyStream(file, hash, func(table string, row rowimport.SourceRow) error {
		if table == "account_holder" {
			holders = append(holders, row)
		}
		if table == "payment_provider" {
			providers = append(providers, row)
		}
		return nil
	})
	if err != nil || len(verified.Tables) != 159 || len(holders) != 1 {
		t.Fatal("complete holder stream invalid")
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal("private holder relationship input unavailable")
	}
	relations, err := rowimport.VerifyRelationships(file, hash, rowimport.RelationLimits{MaxRows: 100000, KeyBytes: 64 << 20})
	missing := int64(0)
	for _, n := range relations.Relations.Missing {
		missing += n
	}
	if err != rowimport.ErrReferences || relations.Snapshot.SHA256 != hash || missing != 375 || len(relations.Relations.PhysicalMissing) != 0 {
		t.Fatal("holder relationship evidence differs")
	}
	// Retain the actual related provider row as history; never run its methods.
	providerID, err := importvalue.Text(holders[0]["provider_id"].Raw, 191)
	if err != nil || providerID != "pp_system_default" {
		t.Fatal("holder provider baseline differs")
	}
	var provider rowimport.SourceRow
	for _, row := range providers {
		id, _ := importvalue.Text(row["id"].Raw, 191)
		if id == providerID {
			if provider != nil {
				t.Fatal("holder provider duplicated")
			}
			provider = row
		}
	}
	if provider == nil {
		t.Fatal("holder provider reference missing")
	}
	parent, err := rowimport.Convert("payment_provider", provider)
	if err != nil {
		t.Fatal("holder provider conversion failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "holder_rehearsal"
	cfg.Params = map[string]string{"time_zone": "'+00:00'", "sql_mode": "'STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("isolated holder target invalid")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary holder key unavailable")
	}
	defer clear(key)
	ring, err := protected.New("rehearsal", map[string][]byte{"rehearsal": key})
	if err != nil {
		t.Fatal("temporary holder key invalid")
	}
	importer, _ := New(ring)
	counts := map[string]int64{"source_verified_tables": 159, "source_verified_rows": relations.Relations.Rows, "unresolved_global_references": missing}
	tables := []string{"payment_providers", "payment_account_holders", "migration_holder_dispositions", "unique_key_owners", "unique_key_buckets", "unique_key_claims", "unique_key_secret_scopes"}
	count := func(table string) int64 {
		var n int64
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n) != nil {
			t.Fatal("holder target count unavailable")
		}
		return n
	}
	for _, table := range tables {
		if count(table) != 0 {
			t.Fatal("holder target must be empty")
		}
	}
	stage := func(insertParent, commit bool) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("real holder transaction unavailable")
		}
		defer tx.Rollback()
		var columns, markers, comparisons []string
		for _, column := range parent.Columns {
			quoted := "`" + column + "`"
			columns = append(columns, quoted)
			markers = append(markers, "?")
			comparisons = append(comparisons, "("+quoted+" <=> ?)")
		}
		if insertParent {
			if _, err := tx.ExecContext(ctx, "INSERT INTO payment_providers ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", parent.Values...); err != nil {
				t.Fatal("real holder provider insert failed")
			}
		}
		var equal bool
		args := append(append([]any{}, parent.Values...), providerID)
		if tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM payment_providers WHERE id=? FOR UPDATE", args...).Scan(&equal) != nil || !equal {
			t.Fatal("real holder provider readback differs")
		}
		if err := importer.Import(ctx, tx, hash, holders[0]); err != nil {
			t.Fatal("real holder atomic import failed")
		}
		id, _ := importvalue.Text(holders[0]["id"].Raw, 191)
		if d, err := Disposition(ctx, tx, id); err != nil || d != "hold" {
			t.Fatal("real holder mandatory hold absent")
		}
		counts["operation_holds_checked"]++
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal("real holder commit or rollback failed")
		}
	}
	stage(true, false)
	for _, table := range tables {
		if count(table) != 0 {
			t.Fatal("real holder rollback left evidence")
		}
	}
	counts["rollback_holder_rows"] = 1
	counts["rollback_provider_rows"] = 1
	counts["rollback_hold_rows"] = 1
	counts["rollback_identity_claims"] = 1
	stage(true, true)
	for _, table := range tables {
		want := int64(1)
		if table == "unique_key_secret_scopes" {
			want = 0
		}
		if count(table) != want {
			t.Fatal("real holder committed counts differ")
		}
	}
	counts["committed_holder_rows"] = 1
	counts["committed_provider_rows"] = 1
	counts["committed_hold_rows"] = 1
	counts["committed_identity_claims"] = 1
	fingerprint := func() []byte {
		h := sha256.New()
		for _, table := range tables {
			// Each fixed table has at most one row in this target. Hash both
			// framing and NULL markers; no values or digests leave process memory.
			h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(table))))
			h.Write([]byte(table))
			rows, err := db.QueryContext(ctx, "SELECT * FROM `"+table+"`")
			if err != nil {
				t.Fatal("private holder fingerprint unavailable")
			}
			columns, err := rows.Columns()
			if err != nil {
				t.Fatal("private holder fingerprint columns invalid")
			}
			for rows.Next() {
				values := make([][]byte, len(columns))
				dest := make([]any, len(columns))
				for j := range values {
					dest[j] = &values[j]
				}
				if rows.Scan(dest...) != nil {
					t.Fatal("private holder fingerprint invalid")
				}
				for _, v := range values {
					marker := byte(0)
					if v != nil {
						marker = 1
					}
					h.Write([]byte{marker})
					h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(v))))
					h.Write(v)
				}
			}
			if rows.Err() != nil || rows.Close() != nil {
				t.Fatal("private holder fingerprint incomplete")
			}
		}
		return h.Sum(nil)
	}
	before := fingerprint()
	stage(false, true)
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("real holder replay changed evidence")
	}
	counts["exact_replay_holder_rows"] = 1
	counts["exact_replay_provider_rows"] = 1
	id, _ := importvalue.Text(holders[0]["id"].Raw, 191)
	var data string
	var metadata sql.NullString
	if db.QueryRowContext(ctx, "SELECT data,metadata FROM payment_account_holders WHERE id=?", id).Scan(&data, &metadata) != nil {
		t.Fatal("real holder ciphertext unavailable")
	}
	plain, err := rowimport.OpenAccountHolderJSON(ring, id, providerID, "data", data)
	if err != nil || !bytes.Equal(plain, holders[0]["data"].Raw) {
		clear(plain)
		t.Fatal("real holder persisted bytes differ")
	}
	clear(plain)
	counts["protected_readbacks"] = 1
	if !holders[0]["metadata"].SQLNull || metadata.Valid {
		t.Fatal("real holder SQL NULL changed")
	}
	counts["protected_sql_nulls"] = 1
	if _, err := rowimport.OpenAccountHolderJSON(ring, id+"-wrong", providerID, "data", data); err == nil {
		t.Fatal("real holder wrong context accepted")
	}
	counts["wrong_context_rejections"] = 1
	var claims int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM unique_key_claims WHERE scope=? AND owner=?", "source_uk_81dae4f5552d02c1dcaedcd4", id).Scan(&claims) != nil || claims != 1 {
		t.Fatal("real holder identity claim absent")
	}
	counts["identity_claims_checked"] = 1
	// No customer/auth/link rows are imported, even when external_id names an
	// old customer. The complete historical scalar row is compared by Import.
	counts["customer_links_restored"] = 0
	encoded, _ := json.Marshal(counts)
	t.Log("HOLDER_MYSQL_REHEARSAL " + string(encoded))
}
