package merchantimport

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/legacysecret"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/go-sql-driver/mysql"
)

// Opt-in only. The owning runner supplies a fixed private snapshot in tmpfs
// and independently decrypted Node expectations through anonymous stdin.
// Never include private rows, ciphertext, driver errors or key IDs in output.
func TestActualMerchantMySQL(t *testing.T) {
	if os.Getenv("MERCHANT_MYSQL_REHEARSAL") != "1" {
		t.Skip("owned disconnected rehearsal required")
	}
	const snapshotHash = "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
	private, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	if err != nil || len(private) > 1<<20 {
		t.Fatal("private input unavailable")
	}
	defer clear(private)
	var input struct {
		Keys     map[string]string `json:"keys"`
		Expected []struct {
			Table  string             `json:"table"`
			ID     string             `json:"id"`
			Fields map[string]*string `json:"fields"`
		} `json:"expected"`
	}
	if json.Unmarshal(private, &input) != nil || len(input.Expected) != 5 {
		t.Fatal("private expectations invalid")
	}
	legacy, err := legacysecret.New(input.Keys)
	if err != nil {
		t.Fatal("explicit source keys invalid")
	}
	input.Keys = nil
	file, err := os.Open("/tmp/merchant-source.ndjson")
	if err != nil {
		t.Fatal("private snapshot unavailable")
	}
	defer file.Close()
	selected := map[string][]rowimport.SourceRow{}
	snapshot, err := rowimport.VerifyStream(file, snapshotHash, func(table string, r rowimport.SourceRow) error {
		if _, ok := plans[table]; ok {
			selected[table] = append(selected[table], r)
		}
		return nil
	})
	if err != nil || len(snapshot.Tables) != 159 {
		t.Fatal("complete source stream invalid")
	}
	expectedCounts := map[string]int{"mail_account": 1, "seo_provider": 0, "seo_setting": 0, "webhook_endpoint": 1, "mollie_config": 1, "paypal_connection": 1, "paypal_settings": 1}
	for table, n := range expectedCounts {
		if len(selected[table]) != n {
			t.Fatal("selected source row counts differ")
		}
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal("source relationship input unavailable")
	}
	relations, err := rowimport.VerifyRelationships(file, snapshotHash, rowimport.RelationLimits{MaxRows: 100000, KeyBytes: 64 << 20})
	missing := int64(0)
	for _, n := range relations.Relations.Missing {
		missing += n
	}
	if err != rowimport.ErrReferences || relations.Snapshot.SHA256 != snapshotHash || missing != 375 || len(relations.Relations.PhysicalMissing) != 0 {
		t.Fatal("source relationship evidence differs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "merchant_rehearsal"
	cfg.Params = map[string]string{"time_zone": "'+00:00'", "sql_mode": "'STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("isolated target invalid")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary target key unavailable")
	}
	defer clear(key)
	ring, err := protected.New("rehearsal", map[string][]byte{"rehearsal": key})
	if err != nil {
		t.Fatal("temporary target key invalid")
	}
	importer, err := New(ring, legacy, nil)
	if err != nil {
		t.Fatal("merchant converter unavailable")
	}
	counts := map[string]int64{"source_verified_tables": 159, "source_verified_rows": relations.Relations.Rows, "unresolved_global_references": missing}
	count := func(table string) int64 {
		var n int64
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n) != nil {
			t.Fatal("target cardinality unavailable")
		}
		return n
	}
	tables := []string{"migration_merchant_dispositions", "unique_key_owners", "unique_key_buckets", "unique_key_claims", "unique_key_secret_scopes"}
	for _, p := range plans {
		tables = append(tables, p.target)
	}
	for _, table := range tables {
		if count(table) != 0 {
			t.Fatal("isolated target must be empty")
		}
	}
	stage := func(commit bool) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("merchant transaction unavailable")
		}
		defer tx.Rollback()
		for _, table := range []string{"mail_account", "seo_provider", "seo_setting", "webhook_endpoint", "mollie_config", "paypal_connection", "paypal_settings"} {
			for _, r := range selected[table] {
				if err := importer.Import(ctx, tx, snapshotHash, table, r); err != nil {
					t.Fatal("real merchant import or replay failed")
				}
				id, err := importvalue.Text(r["id"].Raw, 191)
				if err != nil {
					t.Fatal("source identity invalid")
				}
				if d, err := Disposition(ctx, tx, table, id); err != nil || d != "hold" {
					t.Fatal("real merchant mandatory hold missing")
				}
				counts["operation_holds_checked"]++
			}
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal("merchant commit or rollback failed")
		}
	}
	stage(false)
	for _, table := range tables {
		if count(table) != 0 {
			t.Fatal("real merchant rollback left evidence")
		}
	}
	counts["rollback_business_rows"] = 5
	counts["rollback_hold_rows"] = 5
	counts["rollback_identity_claims"] = 1
	stage(true)
	for table, n := range expectedCounts {
		if count(plans[table].target) != int64(n) {
			t.Fatal("committed merchant row counts differ")
		}
		counts["committed_"+table] = int64(n)
	}
	if count("migration_merchant_dispositions") != 5 || count("unique_key_claims") != 1 {
		t.Fatal("committed holds or claims differ")
	}
	counts["committed_hold_rows"] = 5
	counts["committed_identity_claims"] = 1
	// Full rows, holds and registry contents are fingerprinted in memory only.
	// Ordering uses fixed source identities and fixed claim scopes, never report data.
	fingerprint := func() []byte {
		h := sha256.New()
		for _, table := range tables {
			order := "id"
			switch table {
			case "migration_merchant_dispositions":
				order = "source_table,source_id"
			case "unique_key_owners", "unique_key_claims":
				order = "scope,owner"
			case "unique_key_buckets":
				order = "scope,digest"
			case "unique_key_secret_scopes":
				order = "scope"
			}
			rows, err := db.QueryContext(ctx, "SELECT * FROM `"+table+"` ORDER BY "+order)
			if err != nil {
				t.Fatal("private row fingerprint unavailable")
			}
			columns, err := rows.Columns()
			if err != nil {
				t.Fatal("private fingerprint columns unavailable")
			}
			for rows.Next() {
				values := make([][]byte, len(columns))
				dest := make([]any, len(columns))
				for j := range values {
					dest[j] = &values[j]
				}
				if rows.Scan(dest...) != nil {
					t.Fatal("private row fingerprint invalid")
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
				t.Fatal("private row fingerprint incomplete")
			}
		}
		return h.Sum(nil)
	}
	before := fingerprint()
	stage(true)
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("exact merchant replay changed original evidence")
	}
	counts["exact_replay_business_rows"] = 5
	// Independent Node decryption expectations, not a second Go Convert result.
	seen := map[string]bool{}
	for _, expected := range input.Expected {
		p, ok := plans[expected.Table]
		if !ok || seen[expected.Table] || len(expected.Fields) != len(p.secrets) {
			t.Fatal("private expectation coverage differs")
		}
		seen[expected.Table] = true
		var source rowimport.SourceRow
		for _, r := range selected[expected.Table] {
			id, _ := importvalue.Text(r["id"].Raw, 191)
			if id == expected.ID {
				source = r
			}
		}
		if source == nil {
			t.Fatal("private expectation source differs")
		}
		for _, column := range p.secrets {
			want, present := expected.Fields[column]
			if !present {
				t.Fatal("private field expectation missing")
			}
			var stored []byte
			if db.QueryRowContext(ctx, "SELECT `"+column+"` FROM `"+p.target+"` WHERE id=?", expected.ID).Scan(&stored) != nil {
				t.Fatal("protected merchant readback unavailable")
			}
			if source[column].SQLNull {
				if want != nil || stored != nil {
					t.Fatal("merchant SQL NULL provenance changed")
				}
				counts["protected_sql_nulls"]++
				continue
			}
			if want == nil || stored == nil {
				t.Fatal("nonNULL merchant field missing")
			}
			aad := protected.Context{Purpose: "merchant-config", Table: p.target, Column: column, Row: expected.ID}
			plain, err := ring.Open(aad, stored)
			if err != nil || string(plain) != *want {
				clear(plain)
				t.Fatal("persisted merchant plaintext differs from Node")
			}
			clear(plain)
			aad.Row += "-wrong"
			if _, err := ring.Open(aad, stored); err == nil {
				t.Fatal("merchant wrong context accepted")
			}
			counts["independent_protected_readbacks"]++
			counts["wrong_context_rejections"]++
			if *want == "" {
				counts["empty_protected_strings"]++
			}
		}
	}
	if len(seen) != 5 {
		t.Fatal("private row expectations incomplete")
	}
	// Verify source email identity is held by its original row without logging it.
	var id string
	id, _ = importvalue.Text(selected["mail_account"][0]["id"].Raw, 191)
	var claimCount int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM unique_key_claims WHERE scope=? AND owner=?", "source_uk_0dd39c15e819059c91c55ba4", id).Scan(&claimCount) != nil || claimCount != 1 {
		t.Fatal("real mailbox identity claim absent")
	}
	counts["independent_identity_claims_checked"] = 1
	if strings.TrimSpace(snapshot.SHA256) != snapshotHash {
		t.Fatal("source evidence changed")
	}
	report, _ := json.Marshal(counts)
	t.Log("MERCHANT_MYSQL_REHEARSAL " + string(report))
}
