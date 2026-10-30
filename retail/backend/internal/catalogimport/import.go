// Package catalogimport stages reviewed catalog/configuration rows exactly.
// No source database, environment, provider, files, authorization or commit.
package catalogimport

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

var (
	ErrPolicy   = errors.New("catalog row requires reviewed import policy")
	ErrStorage  = errors.New("catalog row transaction failed")
	ErrConflict = errors.New("catalog row evidence or identity conflict")
)

//go:embed plans.json
var program []byte

type column struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Decimal bool   `json:"decimal"`
}
type table struct {
	Source   string   `json:"source"`
	Target   string   `json:"target"`
	Primary  []column `json:"primary"`
	Columns  []column `json:"columns"`
	Registry bool     `json:"registry"`
}

var plans = sync.OnceValues(func() (map[string]table, error) {
	var p struct {
		Version int     `json:"version"`
		Tables  []table `json:"tables"`
	}
	if json.Unmarshal(program, &p) != nil || p.Version != 1 || len(p.Tables) != 37 {
		return nil, ErrPolicy
	}
	result := map[string]table{}
	for _, t := range p.Tables {
		if t.Source == "" || t.Target == "" || len(t.Primary) == 0 || len(t.Columns) == 0 {
			return nil, ErrPolicy
		}
		if _, ok := result[t.Source]; ok {
			return nil, ErrPolicy
		}
		result[t.Source] = t
	}
	return result, nil
})

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
	b.WriteString("eurorefurb.catalog-source.v1")
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

// Import executes only after a complete strict row conversion. Caller MUST roll
// back on ANY error; success is staging evidence, never a full snapshot/domain
// grant to commit. Full stream integrity, referenced parents, source schema,
// file and domain checks remain required. No overwrite or implicit upsert.
func Import(ctx context.Context, tx *sql.Tx, snapshotSHA, sourceTable string, source rowimport.SourceRow) error {
	if tx == nil {
		return ErrPolicy
	}
	p, err := plans()
	if err != nil {
		return err
	}
	t, ok := p[sourceTable]
	if !ok {
		return ErrPolicy
	}
	return importTable(ctx, tx, snapshotSHA, sourceTable, source, t)
}

// importTable accepts only package-owned layouts. Public entry points must
// select their reviewed scope before reaching this common evidence writer.
func importTable(ctx context.Context, tx *sql.Tx, snapshotSHA, sourceTable string, source rowimport.SourceRow, t table) error {
	snapshot, err := hex.DecodeString(snapshotSHA)
	if err != nil || len(snapshot) != 32 || snapshotSHA != strings.ToLower(snapshotSHA) {
		return ErrPolicy
	}
	bound, err := rowimport.Convert(sourceTable, source)
	if err != nil || bound.Table != t.Target || len(bound.Columns) != len(t.Columns) {
		return ErrPolicy
	}
	projection := uniquekey.TargetRow{}
	for j, c := range t.Columns {
		if bound.Columns[j] != c.Target {
			return ErrPolicy
		}
		cell := source[c.Source]
		if cell.SQLNull {
			projection[c.Target] = json.RawMessage(`null`)
		} else {
			projection[c.Target] = cell.Raw
		}
	}
	var identity bytes.Buffer
	identity.WriteByte(1)
	var where []string
	var primaryValues []any
	for _, c := range t.Primary {
		cell := source[c.Source]
		if cell.SQLNull {
			return ErrPolicy
		}
		text, err := importvalue.Text(cell.Raw, 191)
		if err != nil || text == "" {
			return ErrPolicy
		}
		writePart(&identity, []byte(text))
		where = append(where, "`"+c.Target+"` = ?")
		primaryValues = append(primaryValues, text)
	}
	identityHash := sha256.Sum256(identity.Bytes())
	rowHash := sourceHash(source)
	encoded, err := json.Marshal(bound)
	if err != nil {
		return ErrPolicy
	}
	targetHash := sha256.Sum256(encoded)
	attempt := make([]byte, 16)
	if _, err := rand.Read(attempt); err != nil {
		return ErrStorage
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO migration_catalog_rows
	 (source_table,identity_sha256,full_identity,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version)
	 VALUES(?,?,?,?,?,?,?,1) ON DUPLICATE KEY UPDATE source_table=migration_catalog_rows.source_table`, sourceTable, identityHash[:], identity.Bytes(), snapshot, rowHash, targetHash[:], attempt)
	if err != nil {
		return ErrStorage
	}
	var recordedIdentity, recordedSnapshot, recordedRow, recordedTarget, recordedAttempt []byte
	var version int
	err = tx.QueryRowContext(ctx, `SELECT full_identity,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version
	 FROM migration_catalog_rows WHERE source_table=? AND identity_sha256=? FOR UPDATE`, sourceTable, identityHash[:]).Scan(&recordedIdentity, &recordedSnapshot, &recordedRow, &recordedTarget, &recordedAttempt, &version)
	if err != nil {
		return ErrStorage
	}
	if version != 1 || !bytes.Equal(recordedIdentity, identity.Bytes()) || !bytes.Equal(recordedSnapshot, snapshot) || !bytes.Equal(recordedRow, rowHash) || !bytes.Equal(recordedTarget, targetHash[:]) {
		return ErrConflict
	}
	if t.Registry {
		if err := uniquekey.ApplySourceRow(ctx, tx, t.Target, projection); err != nil {
			if errors.Is(err, uniquekey.ErrConflict) {
				return ErrConflict
			}
			return ErrStorage
		}
	}
	var names, markers, comparisons []string
	for _, c := range t.Columns {
		quoted := "`" + c.Target + "`"
		names = append(names, quoted)
		markers = append(markers, "?")
		parameter := "?"
		// MySQL string-vs-DECIMAL comparison can otherwise coerce to DOUBLE,
		// hiding a one-unit change at the 18th fractional place.
		if c.Decimal {
			parameter = "CAST(? AS DECIMAL(38,18))"
		}
		comparisons = append(comparisons, "("+quoted+" <=> "+parameter+")")
	}
	if bytes.Equal(recordedAttempt, attempt) {
		if _, err := tx.ExecContext(ctx, "INSERT INTO `"+t.Target+"` ("+strings.Join(names, ",")+") VALUES("+strings.Join(markers, ",")+")", bound.Values...); err != nil {
			return ErrStorage
		}
	}
	var equal bool
	args := append(append([]any{}, bound.Values...), primaryValues...)
	err = tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM `"+t.Target+"` WHERE "+strings.Join(where, " AND ")+" FOR UPDATE", args...).Scan(&equal)
	if err == sql.ErrNoRows || (err == nil && !equal) {
		return ErrConflict
	}
	if err != nil {
		return ErrStorage
	}
	return nil
}
