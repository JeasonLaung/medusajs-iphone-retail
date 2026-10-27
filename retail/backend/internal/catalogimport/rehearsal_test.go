package catalogimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/go-sql-driver/mysql"
)

type rehearsalLayout struct {
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	Primary []string `json:"primary_key"`
	Fields  []struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Type   string `json:"target_type"`
	} `json:"fields"`
	References []struct {
		Column       string `json:"column"`
		Parent       string `json:"parent_table"`
		ParentColumn string `json:"parent_column"`
	} `json:"references"`
}

// Fixed source, private tmpfs, no app DSN or source connection. Driver errors,
// row values, keys and fingerprints must never appear in test output.
func TestActualCatalogMySQL(t *testing.T) {
	if os.Getenv("CATALOG_MYSQL_REHEARSAL") != "1" {
		t.Skip("owned isolated real-data rehearsal required")
	}
	const snapshotHash = "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985"
	catalog, err := plans()
	if err != nil {
		t.Fatal("fixed catalog program unavailable")
	}
	data, err := os.ReadFile("/tmp/field-plans.json")
	if err != nil {
		t.Fatal("rehearsal field layout unavailable")
	}
	var manifest struct {
		Hashes map[string]string `json:"input_sha256"`
	}
	if json.Unmarshal(program, &manifest) != nil {
		t.Fatal("fixed catalog input hashes invalid")
	}
	digest := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
	if hex.EncodeToString(digest[:]) != manifest.Hashes["retail/backend/internal/rowimport/field_plans.json"] {
		t.Fatal("rehearsal layout differs from reviewed program")
	}
	var layouts struct {
		Tables []rehearsalLayout `json:"tables"`
	}
	if json.Unmarshal(data, &layouts) != nil || len(layouts.Tables) != 159 {
		t.Fatal("rehearsal layout invalid")
	}
	all := map[string]rehearsalLayout{}
	for _, p := range layouts.Tables {
		all[p.Source] = p
	}
	scope := map[string]bool{}
	for table := range catalog {
		scope[table] = true
	}
	for changed := true; changed; {
		changed = false
		for table := range scope {
			for _, r := range all[table].References {
				if !scope[r.Parent] {
					scope[r.Parent] = true
					changed = true
				}
			}
		}
	}
	if len(scope) != 45 {
		t.Fatal("catalog parent closure differs")
	}
	selected := map[string][]rowimport.SourceRow{}
	file, err := os.Open("/tmp/catalog-source.ndjson")
	if err != nil {
		t.Fatal("private catalog snapshot unavailable")
	}
	defer file.Close()
	verified, err := rowimport.VerifyStream(file, snapshotHash, func(table string, row rowimport.SourceRow) error {
		if scope[table] {
			selected[table] = append(selected[table], row)
		}
		return nil
	})
	if err != nil || len(verified.Tables) != 159 {
		t.Fatal("complete catalog source stream invalid")
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal("catalog relationship source unavailable")
	}
	relations, err := rowimport.VerifyRelationships(file, snapshotHash, rowimport.RelationLimits{MaxRows: 100000, KeyBytes: 64 << 20})
	missing := int64(0)
	for name, n := range relations.Relations.Missing {
		missing += n
		for table := range scope {
			if strings.HasPrefix(name, table+".") {
				t.Fatal("catalog dependency closure contains unresolved references")
			}
		}
	}
	if err != rowimport.ErrReferences || relations.Snapshot.SHA256 != snapshotHash || missing != 375 || len(relations.Relations.PhysicalMissing) != 0 {
		t.Fatal("global catalog reference evidence differs")
	}
	// Topologically order tables without disabling actual physical constraints.
	var order []string
	pending := map[string]bool{}
	for table := range scope {
		pending[table] = true
	}
	for len(pending) > 0 {
		var ready []string
		for table := range pending {
			blocked := false
			for _, r := range all[table].References {
				if r.Parent != table && pending[r.Parent] {
					blocked = true
				}
			}
			if !blocked {
				ready = append(ready, table)
			}
		}
		if len(ready) == 0 {
			t.Fatal("catalog table dependency cycle requires review")
		}
		sort.Strings(ready)
		for _, table := range ready {
			order = append(order, table)
			delete(pending, table)
		}
	}
	// Categories have self references: order actual parents, not source file order.
	for _, table := range order {
		for _, ref := range all[table].References {
			if ref.Parent != table {
				continue
			}
			index := map[string]rowimport.SourceRow{}
			for _, row := range selected[table] {
				key, err := importvalue.Text(row[ref.ParentColumn].Raw, 191)
				if err != nil {
					t.Fatal("self-reference parent invalid")
				}
				index[key] = row
			}
			state := map[string]byte{}
			var sorted []rowimport.SourceRow
			var visit func(string)
			visit = func(key string) {
				if state[key] == 2 {
					return
				}
				if state[key] == 1 {
					t.Fatal("catalog self-reference cycle")
				}
				row, ok := index[key]
				if !ok {
					t.Fatal("catalog self-reference parent absent")
				}
				state[key] = 1
				if !row[ref.Column].SQLNull {
					parent, err := importvalue.Text(row[ref.Column].Raw, 191)
					if err != nil {
						t.Fatal("catalog self-reference invalid")
					}
					visit(parent)
				}
				state[key] = 2
				sorted = append(sorted, row)
			}
			keys := make([]string, 0, len(index))
			for key := range index {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				visit(key)
			}
			selected[table] = sorted
		}
	}
	counts := map[string]int64{"source_verified_tables": 159, "source_verified_rows": relations.Relations.Rows, "unresolved_global_references": missing, "catalog_tables": 36, "dependency_tables": 9, "closure_reference_missing": 0}
	for _, table := range order {
		if _, ok := catalog[table]; ok {
			counts["catalog_rows"] += int64(len(selected[table]))
		} else {
			counts["dependency_rows"] += int64(len(selected[table]))
		}
	}
	if counts["catalog_rows"] != 21837 || counts["dependency_rows"] != 1722 {
		t.Fatal("catalog scope row counts differ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock", "catalog_rehearsal"
	cfg.Params = map[string]string{"time_zone": "'+00:00'", "sql_mode": "'STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("isolated catalog connection invalid")
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	var enabled int
	if db.QueryRowContext(ctx, "SELECT @@foreign_key_checks").Scan(&enabled) != nil || enabled != 1 {
		t.Fatal("catalog physical constraints must remain enabled")
	}
	count := func(table string) int64 {
		var n int64
		if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n) != nil {
			t.Fatal("catalog target count unavailable")
		}
		return n
	}
	aux := []string{"migration_catalog_rows", "unique_key_owners", "unique_key_buckets", "unique_key_claims", "unique_key_secret_scopes"}
	for _, table := range order {
		if count(all[table].Target) != 0 {
			t.Fatal("catalog target must be empty")
		}
	}
	for _, table := range aux {
		if count(table) != 0 {
			t.Fatal("catalog evidence target must be empty")
		}
	}
	parentSQL := func(tx *sql.Tx, layout rehearsalLayout, row rowimport.SourceRow, insert bool) error {
		bound, err := rowimport.Convert(layout.Source, row)
		if err != nil {
			return ErrPolicy
		}
		var names, markers, comparisons, where []string
		var pk []any
		for j, c := range layout.Fields {
			if bound.Columns[j] != c.Target {
				return ErrPolicy
			}
			quoted := "`" + c.Target + "`"
			names = append(names, quoted)
			markers = append(markers, "?")
			param := "?"
			if strings.HasPrefix(c.Type, "DECIMAL(38,18)") {
				param = "CAST(? AS DECIMAL(38,18))"
			}
			comparisons = append(comparisons, "("+quoted+" <=> "+param+")")
		}
		for _, name := range layout.Primary {
			for j, c := range layout.Fields {
				if c.Source == name {
					where = append(where, "`"+c.Target+"` = ?")
					pk = append(pk, bound.Values[j])
				}
			}
		}
		if insert {
			if _, err := tx.ExecContext(ctx, "INSERT INTO `"+layout.Target+"` ("+strings.Join(names, ",")+") VALUES("+strings.Join(markers, ",")+")", bound.Values...); err != nil {
				return ErrStorage
			}
		}
		var equal bool
		args := append(append([]any{}, bound.Values...), pk...)
		if tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM `"+layout.Target+"` WHERE "+strings.Join(where, " AND ")+" FOR UPDATE", args...).Scan(&equal) != nil || !equal {
			return ErrConflict
		}
		return nil
	}
	stage := func(insertParents, commit bool) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("catalog transaction unavailable")
		}
		defer tx.Rollback()
		for _, table := range order {
			for _, row := range selected[table] {
				if _, ok := catalog[table]; ok {
					err = Import(ctx, tx, snapshotHash, table, row)
				} else {
					err = parentSQL(tx, all[table], row, insertParents)
				}
				if err != nil {
					t.Fatal("actual catalog atomic import or readback failed: " + table)
				}
			}
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal("catalog commit or rollback failed")
		}
	}
	stage(true, false)
	for _, table := range order {
		if count(all[table].Target) != 0 {
			t.Fatal("actual catalog rollback left business rows")
		}
	}
	for _, table := range aux {
		if count(table) != 0 {
			t.Fatal("actual catalog rollback left evidence")
		}
	}
	counts["rollback_business_rows"] = 23559
	counts["rollback_provenance_rows"] = 21837
	stage(true, true)
	for _, table := range order {
		n := count(all[table].Target)
		if n != int64(len(selected[table])) {
			t.Fatal("catalog committed counts differ")
		}
		counts["committed_"+table] = n
	}
	if count("migration_catalog_rows") != 21837 {
		t.Fatal("catalog provenance count differs")
	}
	counts["committed_provenance_rows"] = 21837
	counts["committed_unique_claims"] = count("unique_key_claims")
	fingerprint := func() []byte {
		h := sha256.New()
		targets := map[string]string{}
		for _, table := range order {
			layout := all[table]
			var columns []string
			for _, primary := range layout.Primary {
				for _, f := range layout.Fields {
					if f.Source == primary {
						columns = append(columns, "`"+f.Target+"`")
					}
				}
			}
			targets[layout.Target] = strings.Join(columns, ",")
		}
		for table, sort := range map[string]string{"migration_catalog_rows": "source_table,identity_sha256", "unique_key_owners": "scope,owner", "unique_key_buckets": "scope,digest", "unique_key_claims": "scope,owner", "unique_key_secret_scopes": "scope"} {
			targets[table] = sort
		}
		var names []string
		for name := range targets {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, table := range names {
			write := func(v []byte) { h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(v)))); h.Write(v) }
			write([]byte(table))
			rows, err := db.QueryContext(ctx, "SELECT * FROM `"+table+"` ORDER BY "+targets[table])
			if err != nil {
				t.Fatal("private catalog fingerprint unavailable")
			}
			cols, err := rows.Columns()
			if err != nil {
				t.Fatal("private catalog columns unavailable")
			}
			for rows.Next() {
				values := make([][]byte, len(cols))
				dest := make([]any, len(cols))
				for j := range values {
					dest[j] = &values[j]
				}
				if rows.Scan(dest...) != nil {
					t.Fatal("private catalog fingerprint invalid")
				}
				h.Write([]byte{2})
				for _, v := range values {
					marker := byte(0)
					if v != nil {
						marker = 1
					}
					h.Write([]byte{marker})
					write(v)
				}
			}
			if rows.Err() != nil || rows.Close() != nil {
				t.Fatal("private catalog fingerprint incomplete")
			}
			h.Write([]byte{3})
		}
		return h.Sum(nil)
	}
	before := fingerprint()
	stage(false, true)
	if !bytes.Equal(before, fingerprint()) {
		t.Fatal("actual catalog replay changed evidence")
	}
	counts["exact_replay_catalog_rows"] = 21837
	counts["exact_replay_dependency_rows"] = 1722
	counts["physical_foreign_key_checks_enabled"] = 1
	report, _ := json.Marshal(counts)
	t.Log("CATALOG_MYSQL_REHEARSAL " + string(report))
}
