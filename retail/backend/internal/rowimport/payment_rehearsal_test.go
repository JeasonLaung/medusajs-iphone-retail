package rowimport

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

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
)

// This opt-in test is launched only by the owned, disconnected, disposable
// MySQL runner. It never connects to the source or to a configured app DSN.
// All private failures are fixed strings; never include driver errors/rows.
func TestActualPaymentHistoryMySQL(t *testing.T) {
	if os.Getenv("ROWIMPORT_PAYMENT_REHEARSAL") != "1" {
		t.Skip("explicit isolated real-data rehearsal required")
	}
	const snapshotHash = "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
	file, err := os.Open("/tmp/payment-source.ndjson")
	if err != nil {
		t.Fatal("private rehearsal snapshot unavailable")
	}
	defer file.Close()
	selected := map[string][]SourceRow{}
	snapshot, err := VerifyStream(file, snapshotHash, func(table string, row SourceRow) error {
		if table == "payment_collection" || table == "payment_session" || table == "payment" {
			selected[table] = append(selected[table], row)
		}
		return nil
	})
	if err != nil || len(snapshot.Tables) != 159 || len(selected["payment_collection"]) != 23 || len(selected["payment_session"]) != 21 || len(selected["payment"]) != 21 {
		t.Fatal("private rehearsal complete stream invalid")
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal("private relationship evidence unavailable")
	}
	relations, err := VerifyRelationships(file, snapshotHash, RelationLimits{MaxRows: 100000, KeyBytes: 64 << 20})
	missing := int64(0)
	for _, count := range relations.Relations.Missing {
		missing += count
	}
	if err != ErrReferences || relations.Snapshot.SHA256 != snapshotHash || missing != 375 || len(relations.Relations.PhysicalMissing) != 0 {
		t.Fatal("private rehearsal relationship evidence differs")
	}
	// The known unresolved references remain a full-import acceptance failure.
	// This dedicated database only rehearses the verified payment subgraph.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "payment_rehearsal"
	cfg.Params = map[string]string{"time_zone": "'+00:00'", "sql_mode": "'STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("isolated rehearsal connection invalid")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("formal payment plan invalid")
	}
	applied, err := schema.Apply(ctx, db, plan)
	if err != nil || applied.Applied != 23 || schema.CheckApplied(ctx, db, plan) != nil {
		t.Fatal("formal payment schema application failed")
	}
	replay, err := schema.Apply(ctx, db, plan)
	if err != nil || replay.Skipped != 23 {
		t.Fatal("formal payment schema replay failed")
	}

	counts := map[string]int64{"explicit_migrations": 23, "payment_schema_tables": 8, "payment_source_columns": 83, "source_verified_tables": 159, "unresolved_global_references": missing, "source_verified_rows": relations.Relations.Rows}
	targetCount := func(table string) int64 {
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); err != nil {
			t.Fatal("isolated rehearsal count unavailable")
		}
		return count
	}
	for _, table := range []string{"payment_collections", "payment_sessions", "payments", "migration_payment_dispositions"} {
		if targetCount(table) != 0 {
			t.Fatal("rehearsal target must be empty")
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary rehearsal key unavailable")
	}
	defer clear(key)
	ring, err := protected.New("rehearsal", map[string][]byte{"rehearsal": key})
	if err != nil {
		t.Fatal("temporary rehearsal key invalid")
	}
	converter, _ := NewPaymentHistory(ring)
	var parents []BoundRow
	for _, row := range selected["payment_collection"] {
		// No unreviewed extension material is copied by this rehearsal.
		if !row["metadata"].SQLNull {
			t.Fatal("real payment collection metadata requires review")
		}
		bound, err := Convert("payment_collection", row)
		if err != nil {
			t.Fatal("real payment collection conversion failed")
		}
		parents = append(parents, bound)
	}
	parentSQL := func(tx *sql.Tx, row BoundRow, insert bool) error {
		var columns, markers, comparisons []string
		for _, column := range row.Columns {
			quoted := "`" + column + "`"
			columns = append(columns, quoted)
			markers = append(markers, "?")
			parameter := "?"
			if column == "amount" || column == "authorized_amount" || column == "captured_amount" || column == "refunded_amount" {
				parameter = "CAST(? AS DECIMAL(38,18))"
			}
			comparisons = append(comparisons, "("+quoted+" <=> "+parameter+")")
		}
		if insert {
			_, err := tx.ExecContext(ctx, "INSERT INTO `"+row.Table+"` ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", row.Values...)
			if err != nil {
				return ErrPaymentStorage
			}
		}
		var equal bool
		args := append(append([]any{}, row.Values...), row.Values[0]) // pinned first column is source id
		if err := tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM `"+row.Table+"` WHERE id=? FOR UPDATE", args...).Scan(&equal); err != nil || !equal {
			return ErrPaymentConflict
		}
		return nil
	}
	stage := func(insertParents, commit bool) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("real payment transaction unavailable")
		}
		defer tx.Rollback()
		for _, parent := range parents {
			if err := parentSQL(tx, parent, insertParents); err != nil {
				t.Fatal("real parent insert or readback failed")
			}
		}
		for _, table := range []string{"payment_session", "payment"} {
			for _, row := range selected[table] {
				if err := converter.Import(ctx, tx, snapshotHash, table, row); err != nil {
					t.Fatal("real payment atomic import or replay failed")
				}
				identity, _ := paymentRowIdentity(table, row)
				if disposition, err := HistoricalPaymentDisposition(ctx, tx, table, identity.SourceID); err != nil || disposition != "hold" {
					t.Fatal("real imported payment lacks mandatory hold")
				}
			}
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal("real payment commit or rollback failed")
		}
	}
	stage(true, false)
	for _, table := range []string{"payment_collections", "payment_sessions", "payments", "migration_payment_dispositions"} {
		if targetCount(table) != 0 {
			t.Fatal("real payment rollback left rows")
		}
	}
	counts["rollback_business_rows"] = 65
	counts["rollback_hold_rows"] = 42
	stage(true, true)
	for table, expected := range map[string]int64{"payment_collections": 23, "payment_sessions": 21, "payments": 21, "migration_payment_dispositions": 42} {
		if targetCount(table) != expected {
			t.Fatal("real payment committed counts differ")
		}
		counts["committed_"+table] = expected
	}
	// Retain private digests in memory only to prove no nonce/hash overwrite.
	fingerprint := func() []byte {
		hash := sha256.New()
		for _, query := range []string{
			"SELECT data,context,metadata FROM payment_sessions ORDER BY id",
			"SELECT data,NULL,metadata FROM payments ORDER BY id",
			"SELECT source_row_sha256,target_row_sha256,initial_attempt FROM migration_payment_dispositions ORDER BY source_table,source_id",
		} {
			rows, err := db.QueryContext(ctx, query)
			if err != nil {
				t.Fatal("private payment fingerprint unavailable")
			}
			for rows.Next() {
				var values [3]sql.NullString
				if rows.Scan(&values[0], &values[1], &values[2]) != nil {
					t.Fatal("private payment fingerprint invalid")
				}
				for _, value := range values {
					marker := byte(0)
					if value.Valid {
						marker = 1
					}
					hash.Write([]byte{marker})
					hash.Write(binary.BigEndian.AppendUint64(nil, uint64(len(value.String))))
					hash.Write([]byte(value.String))
				}
			}
			if rows.Err() != nil || rows.Close() != nil {
				t.Fatal("private payment fingerprint incomplete")
			}
		}
		return hash.Sum(nil)
	}
	before := fingerprint()
	stage(false, true)
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("real exact replay changed original payment evidence")
	}
	counts["exact_replay_payment_rows"] = 42
	counts["exact_replay_parent_rows"] = 23
	counts["operation_holds_checked"] = 126 // rollback + commit + replay
	// Verify persisted projection independently of its replay logic, never log
	// decrypted contents. All other columns are compared by Import/parentSQL.
	for _, table := range []string{"payment_session", "payment"} {
		for _, row := range selected[table] {
			identity, _ := paymentRowIdentity(table, row)
			target := "payments"
			query := "SELECT data,NULL,metadata FROM payments WHERE id=?"
			if table == "payment_session" {
				target = "payment_sessions"
				query = "SELECT data,context,metadata FROM payment_sessions WHERE id=?"
			}
			var stored [3]sql.NullString
			if db.QueryRowContext(ctx, query, identity.SourceID).Scan(&stored[0], &stored[1], &stored[2]) != nil {
				t.Fatal("real encrypted readback unavailable")
			}
			for index, column := range []string{"data", "context", "metadata"} {
				cell, exists := row[column]
				if !exists {
					continue
				}
				if cell.SQLNull {
					if stored[index].Valid {
						t.Fatal("real SQL NULL became a protected JSON document")
					}
					counts[target+"."+column+".sql_null"]++
					continue
				}
				if !stored[index].Valid {
					t.Fatal("real protected JSON became SQL NULL")
				}
				var plain, expected json.RawMessage
				switch column {
				case "data":
					plain, err = OpenPaymentData(ring, identity, stored[index].String)
					expected = cell.Raw
				case "context":
					plain, err = OpenPaymentContext(ring, identity.SourceID, stored[index].String)
					reviewed, reviewErr := ReviewPaymentContext(cell.Raw)
					if reviewErr != nil {
						t.Fatal("real context review differs")
					}
					expected = reviewed.JSON
					for _, removed := range reviewed.Removed {
						counts["excluded_profile_fields"] += int64(removed)
					}
				case "metadata":
					plain, err = OpenPaymentMetadata(ring, identity, stored[index].String)
					expected = cell.Raw
				}
				if err != nil || !bytes.Equal(plain, expected) {
					t.Fatal("real persisted encrypted evidence differs")
				}
				clear(plain)
				counts[target+"."+column+".protected_readbacks"]++
			}
		}
	}
	report, _ := json.Marshal(counts)
	t.Log("PAYMENT_MYSQL_REHEARSAL " + string(report))
}
