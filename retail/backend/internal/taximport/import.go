// Package taximport stages historical tax evidence in a caller-owned transaction.
// No commit, HTTP, identity grants, provider calls or source database access.
package taximport

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/uniquekey"
)

var ErrPolicy = errors.New("tax import requires reviewed policy")
var ErrStorage = errors.New("tax import transaction failed")
var ErrConflict = errors.New("tax import evidence conflict")

//go:embed plans.json
var program []byte

type column struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Decimal  bool   `json:"decimal"`
	Deferred bool   `json:"deferred"`
}
type reference struct {
	Column       string `json:"column"`
	Parent       string `json:"parent"`
	ParentColumn string `json:"parent_column"`
}
type table struct {
	Source     string      `json:"source"`
	Target     string      `json:"target"`
	Columns    []column    `json:"columns"`
	Primary    []column    `json:"primary"`
	References []reference `json:"references"`
}
type plan struct {
	Version  int     `json:"version"`
	ModelSHA string  `json:"model_sha256"`
	Tables   []table `json:"tables"`
}

var plans = sync.OnceValues(func() (plan, error) {
	var p plan
	if json.Unmarshal(program, &p) != nil || p.Version != 1 || len(p.Tables) != 4 || len(p.ModelSHA) != 64 {
		return p, ErrPolicy
	}
	seen := map[string]bool{}
	for _, t := range p.Tables {
		if t.Source == "customer" || seen[t.Source] || len(t.Columns) == 0 || len(t.Primary) == 0 {
			return p, ErrPolicy
		}
		seen[t.Source] = true
	}
	return p, nil
})

type prepared struct {
	table        table
	source       rowimport.SourceRow
	bound        rowimport.BoundRow
	identity     []byte
	identityHash [32]byte
	sourceHash   []byte
	targetHash   [32]byte
	where        string
	primary      []any
	fresh        bool
}
type Result struct {
	Rows                      int
	ReplayedRows              int
	ObservedMissingReferences int
}

// SourceTables returns a fresh slice for a snapshot coordinator to initialize
// every ordinary table, including tables with zero rows. No caller may mutate
// the compiled import plan.
func SourceTables() ([]string, error) {
	p, err := plans()
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(p.Tables))
	for _, t := range p.Tables {
		result = append(result, t.Source)
	}
	return result, nil
}

func writePart(b *bytes.Buffer, value []byte) {
	b.Write(binary.BigEndian.AppendUint64(nil, uint64(len(value))))
	b.Write(value)
}
func sourceHash(row rowimport.SourceRow) []byte {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	b.WriteString("eurorefurb.tax-source.v1")
	for _, key := range keys {
		cell := row[key]
		writePart(&b, []byte(key))
		writePart(&b, cell.Raw)
		marker := byte(0)
		if cell.SQLNull {
			marker = 1
		}
		b.WriteByte(marker)
	}
	h := sha256.Sum256(b.Bytes())
	return h[:]
}

// Import stages one batch with all four ordinary table keys (including empty
// slices). Caller verifies the complete snapshot, schema, counts and source
// relationships, and MUST roll back on ANY error or subsequent domain failure.
// No success grants tax configuration writes, provider execution or checkout authority.
func Import(ctx context.Context, tx *sql.Tx, snapshotSHA string, input map[string][]rowimport.SourceRow) (Result, error) {
	var result Result
	p, err := plans()
	if err != nil || tx == nil || len(input) != len(p.Tables) {
		return Result{}, ErrPolicy
	}
	snapshot, err := hex.DecodeString(snapshotSHA)
	if err != nil || len(snapshot) != 32 || snapshotSHA != strings.ToLower(snapshotSHA) {
		return Result{}, ErrPolicy
	}
	var rows []*prepared
	for _, t := range p.Tables {
		sourceRows, ok := input[t.Source]
		if !ok || len(sourceRows) > 100000-len(rows) {
			return Result{}, ErrPolicy
		}
		var local []*prepared
		seen := map[string]bool{}
		for _, source := range sourceRows {
			bound, e := rowimport.Convert(t.Source, source)
			if e != nil || bound.Table != t.Target || len(bound.Columns) != len(t.Columns) {
				return Result{}, ErrPolicy
			}
			for i, c := range t.Columns {
				if bound.Columns[i] != c.Target {
					return Result{}, ErrPolicy
				}
			}
			var identity bytes.Buffer
			identity.WriteByte(1)
			var predicates []string
			var primary []any
			for _, c := range t.Primary {
				cell := source[c.Source]
				value, e := importvalue.Text(cell.Raw, 191)
				if cell.SQLNull || e != nil || value == "" {
					return Result{}, ErrPolicy
				}
				writePart(&identity, []byte(value))
				predicates = append(predicates, "`"+c.Target+"`=?")
				primary = append(primary, value)
			}
			if seen[identity.String()] {
				return Result{}, ErrPolicy
			}
			seen[identity.String()] = true
			encoded, e := json.Marshal(bound)
			if e != nil {
				return Result{}, ErrPolicy
			}
			local = append(local, &prepared{table: t, source: source, bound: bound, identity: identity.Bytes(),
				identityHash: sha256.Sum256(identity.Bytes()), sourceHash: sourceHash(source),
				targetHash: sha256.Sum256(encoded), where: strings.Join(predicates, " AND "), primary: primary})
		}
		sort.Slice(local, func(i, j int) bool { return bytes.Compare(local[i].identity, local[j].identity) < 0 })
		rows = append(rows, local...)
	}
	// One tax import at a time; an old RR snapshot must not hide receipts.
	// Runtime commands remain disabled and must not share this migration target.
	var version int
	var model string
	var reads, writes bool
	if tx.QueryRowContext(ctx, `SELECT schema_version,model_sha256,reads_enabled,writes_enabled FROM tax_runtime_schema WHERE id=1 FOR UPDATE`).Scan(&version, &model, &reads, &writes) != nil || version != 1 || model != p.ModelSHA || !reads || writes {
		return Result{}, ErrPolicy
	}
	var changes []uniquekey.Change
	for _, row := range rows {
		if row.table.Source != "tax_region" {
			continue
		}
		projection := uniquekey.TargetRow{}
		for _, column := range row.table.Columns {
			cell := row.source[column.Source]
			if cell.SQLNull {
				projection[column.Target] = json.RawMessage(`null`)
			} else {
				projection[column.Target] = cell.Raw
			}
		}
		claims, err := uniquekey.SourceChanges(row.table.Target, projection)
		if err != nil {
			return Result{}, ErrPolicy
		}
		changes = append(changes, claims...)
	}
	if len(changes) > 0 {
		if err := uniquekey.Apply(ctx, tx, changes); err != nil {
			if errors.Is(err, uniquekey.ErrConflict) {
				return Result{}, ErrConflict
			}
			return Result{}, ErrStorage
		}
	}
	for _, row := range rows {
		if err := register(ctx, tx, snapshot, row); err != nil {
			return Result{}, err
		}
		if row.fresh {
			var names, markers []string
			values := append([]any(nil), row.bound.Values...)
			for i, c := range row.table.Columns {
				names = append(names, "`"+c.Target+"`")
				markers = append(markers, "?")
				if c.Deferred {
					values[i] = nil
				}
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO `"+row.table.Target+"` ("+strings.Join(names, ",")+") VALUES ("+strings.Join(markers, ",")+")", values...); err != nil {
				return Result{}, ErrStorage
			}
		} else {
			result.ReplayedRows++
		}
	}
	// Restore only freshly inserted nullable cycle edges. Never update an existing
	// target on replay: it must still match every original field exactly.
	for _, row := range rows {
		if row.fresh {
			var sets []string
			var values []any
			for i, c := range row.table.Columns {
				if c.Deferred {
					sets = append(sets, "`"+c.Target+"`=?")
					values = append(values, row.bound.Values[i])
				}
			}
			if len(sets) > 0 {
				values = append(values, row.primary...)
				if _, err := tx.ExecContext(ctx, "UPDATE `"+row.table.Target+"` SET "+strings.Join(sets, ",")+" WHERE "+row.where, values...); err != nil {
					return Result{}, ErrStorage
				}
			}
		}
		if err := verify(ctx, tx, row); err != nil {
			return Result{}, err
		}
	}
	available := map[string]bool{}
	for _, row := range rows {
		for _, ref := range row.table.References {
			cell := row.source[ref.Column]
			if cell.SQLNull {
				continue
			}
			parent, e := importvalue.Text(cell.Raw, 191)
			if e != nil {
				return Result{}, ErrPolicy
			}
			exists, known := available[ref.Parent]
			if !known {
				var count int
				if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", ref.Parent).Scan(&count) != nil {
					return Result{}, ErrStorage
				}
				exists = count == 1
				available[ref.Parent] = exists
			}
			reason := "parent_domain_unavailable"
			if exists {
				var found int
				err := tx.QueryRowContext(ctx, "SELECT 1 FROM `"+ref.Parent+"` WHERE `"+ref.ParentColumn+"`=? LIMIT 1 FOR SHARE", parent).Scan(&found)
				if err == nil {
					continue
				}
				if err != sql.ErrNoRows {
					return Result{}, ErrStorage
				}
				reason = "missing_parent"
			}
			if err := preserveHold(ctx, tx, snapshot, row, ref, parent, reason); err != nil {
				return Result{}, err
			}
			result.ObservedMissingReferences++
		}
	}
	result.Rows = len(rows)
	return result, nil
}

func register(ctx context.Context, tx *sql.Tx, snapshot []byte, row *prepared) error {
	attempt := make([]byte, 16)
	if _, err := rand.Read(attempt); err != nil {
		return ErrStorage
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO migration_tax_rows(source_table,identity_sha256,full_identity,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version) VALUES(?,?,?,?,?,?,?,1) ON DUPLICATE KEY UPDATE source_table=migration_tax_rows.source_table`, row.table.Source, row.identityHash[:], row.identity, snapshot, row.sourceHash, row.targetHash[:], attempt)
	if err != nil {
		return ErrStorage
	}
	var identity, recordedSnapshot, source, target, initial []byte
	var version int
	if tx.QueryRowContext(ctx, `SELECT full_identity,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version FROM migration_tax_rows WHERE source_table=? AND identity_sha256=? FOR UPDATE`, row.table.Source, row.identityHash[:]).Scan(&identity, &recordedSnapshot, &source, &target, &initial, &version) != nil {
		return ErrStorage
	}
	if version != 1 || !bytes.Equal(identity, row.identity) || !bytes.Equal(recordedSnapshot, snapshot) || !bytes.Equal(source, row.sourceHash) || !bytes.Equal(target, row.targetHash[:]) {
		return ErrConflict
	}
	row.fresh = bytes.Equal(initial, attempt)
	return nil
}

func verify(ctx context.Context, tx *sql.Tx, row *prepared) error {
	var comparisons []string
	for _, c := range row.table.Columns {
		parameter := "?"
		if c.Decimal {
			parameter = "CAST(? AS DECIMAL(38,18))"
		}
		comparisons = append(comparisons, "(`"+c.Target+"` <=> "+parameter+")")
	}
	var equal bool
	args := append(append([]any(nil), row.bound.Values...), row.primary...)
	err := tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM `"+row.table.Target+"` WHERE "+row.where+" FOR UPDATE", args...).Scan(&equal)
	if err == sql.ErrNoRows || err == nil && !equal {
		return ErrConflict
	}
	if err != nil {
		return ErrStorage
	}
	return nil
}

func preserveHold(ctx context.Context, tx *sql.Tx, snapshot []byte, row *prepared, ref reference, parent, reason string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO tax_migration_reference_holds(source_table,identity_sha256,column_name,full_identity,parent_table,parent_column,parent_identity,reason,snapshot_sha256,source_row_sha256,released,policy_version) VALUES(?,?,?,?,?,?,?,?,?,?,FALSE,1) ON DUPLICATE KEY UPDATE source_table=tax_migration_reference_holds.source_table`, row.table.Source, row.identityHash[:], ref.Column, row.identity, ref.Parent, ref.ParentColumn, parent, reason, snapshot, row.sourceHash)
	if err != nil {
		return ErrStorage
	}
	var identity, recordedSnapshot, source []byte
	var parentTable, parentColumn, parentID, recordedReason string
	var released bool
	var version int
	if tx.QueryRowContext(ctx, `SELECT full_identity,parent_table,parent_column,parent_identity,reason,snapshot_sha256,source_row_sha256,released,policy_version FROM tax_migration_reference_holds WHERE source_table=? AND identity_sha256=? AND column_name=? FOR UPDATE`, row.table.Source, row.identityHash[:], ref.Column).Scan(&identity, &parentTable, &parentColumn, &parentID, &recordedReason, &recordedSnapshot, &source, &released, &version) != nil {
		return ErrStorage
	}
	if version != 1 || released || !bytes.Equal(identity, row.identity) || parentTable != ref.Parent || parentColumn != ref.ParentColumn || parentID != parent || recordedReason != reason || !bytes.Equal(recordedSnapshot, snapshot) || !bytes.Equal(source, row.sourceHash) {
		return ErrConflict
	}
	return nil
}
