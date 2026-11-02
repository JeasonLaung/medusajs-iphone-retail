package catalogimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Explicit opt-in real local target verification. No default DSN, source
// connection or schema mutation. The conflict is NEVER committed.
func TestNativeCatalogReplayAndConflictRollback(t *testing.T) {
	if os.Getenv("CATALOG_NATIVE_REHEARSAL") != "1" {
		t.Skip("explicit owned native target and snapshot required")
	}
	cfg, err := mysql.ParseDSN(os.Getenv("NATIVE_CATALOG_DSN"))
	if err != nil || cfg == nil {
		t.Fatal("native target configuration required")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if cfg.Net != "tcp" || err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") || !strings.HasPrefix(cfg.DBName, "eurorefurb_") {
		t.Fatal("dedicated loopback target required")
	}
	f, err := os.Open(os.Getenv("NATIVE_CATALOG_SNAPSHOT"))
	if err != nil {
		t.Fatal("private complete snapshot required")
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	p, err := PrepareCatalog(ctx, f, os.Getenv("NATIVE_CATALOG_SHA256"), 100000, 64<<20)
	if err != nil {
		t.Fatal("native source preparation failed")
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("native target unavailable")
	}
	defer db.Close()
	// Capture every field of every scoped table plus migration/unique evidence
	// without exposing business values or digests to test output.
	fingerprint := func() []byte {
		h := sha256.New()
		for _, name := range append(append([]string{}, p.order...), "migration_catalog_rows", "unique_key_owners", "unique_key_buckets", "unique_key_claims", "unique_key_secret_scopes") {
			target := name
			primary := "1"
			if l, ok := p.layouts[name]; ok {
				target = l.Target
				var keys []string
				for _, pk := range l.Primary {
					for _, c := range l.Columns {
						if c.Source == pk {
							keys = append(keys, "`"+c.Target+"`")
						}
					}
				}
				primary = strings.Join(keys, ",")
			}
			rows, e := db.QueryContext(ctx, "SELECT * FROM `"+target+"` ORDER BY "+primary)
			if e != nil {
				t.Fatal("native fingerprint unavailable")
			}
			columns, e := rows.Columns()
			if e != nil {
				rows.Close()
				t.Fatal("native fingerprint columns unavailable")
			}
			h.Write([]byte(name))
			for rows.Next() {
				values := make([]sql.RawBytes, len(columns))
				args := make([]any, len(columns))
				for i := range values {
					args[i] = &values[i]
				}
				if rows.Scan(args...) != nil {
					rows.Close()
					t.Fatal("native fingerprint scan failed")
				}
				for _, v := range values {
					marker := byte(1)
					if v == nil {
						marker = 0
					}
					h.Write([]byte{marker})
					h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(v))))
					h.Write(v)
				}
			}
			if rows.Err() != nil {
				rows.Close()
				t.Fatal("native fingerprint read failed")
			}
			rows.Close()
		}
		return h.Sum(nil)
	}
	before := fingerprint()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("native replay transaction")
	}
	if err = p.Stage(ctx, tx); err != nil {
		tx.Rollback()
		t.Fatal("native exact replay rejected")
	}
	if tx.Commit() != nil {
		t.Fatal("native replay commit failed")
	}
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("exact replay changed stored business/evidence fields")
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("native conflict transaction")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE catalog_products SET title='artificial-native-conflict' ORDER BY id LIMIT 1")
	if err != nil {
		t.Fatal("native conflict setup failed")
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		t.Fatal("existing native product required")
	}
	if err = p.Stage(ctx, tx); err != ErrConflict {
		t.Fatal("source replay must reject an existing business edit")
	}
	if tx.Rollback() != nil {
		t.Fatal("native conflict rollback failed")
	}
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("failed replay changed existing business/evidence fields")
	}
	t.Log("native catalog exact replay and conflicting edit rollback passed; all scoped business/evidence fields unchanged")
}
