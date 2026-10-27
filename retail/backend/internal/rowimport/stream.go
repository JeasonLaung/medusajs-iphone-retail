package rowimport

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
)

type TableEvidence struct {
	Rows   int64
	SHA256 string
}
type SnapshotEvidence struct {
	SHA256 string
	Tables map[string]TableEvidence
}

// VerifyStream checks ALL 159 tables in pinned order and the external complete
// file hash. visit may stage rows, but MUST NOT commit business writes before
// this function succeeds. Any callback/input/hash/footer error invalidates the
// entire stream. This validates framing and cell provenance, not domain values.
func VerifyStream(input io.Reader, expectedSHA256 string, visit func(string, SourceRow) error) (SnapshotEvidence, error) {
	decoded, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(decoded) != 32 || strings.ToLower(expectedSHA256) != expectedSHA256 {
		return SnapshotEvidence{}, ErrRow
	}
	var program struct {
		Tables []tablePlan `json:"tables"`
	}
	if json.Unmarshal(fieldPlans, &program) != nil {
		return SnapshotEvidence{}, ErrRow
	}
	planHash := sha256.Sum256(bytes.ReplaceAll(fieldPlans, []byte("\r\n"), []byte("\n")))
	fileHash := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(input, fileHash))
	scanner.Buffer(make([]byte, 64<<10), MaxRecordBytes+1024)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, data[:i+1], nil
		}
		if atEOF && len(data) > 0 {
			return 0, nil, ErrRow
		}
		return 0, nil, nil
	})
	next := func() ([]byte, error) {
		if !scanner.Scan() {
			return nil, ErrRow
		}
		line := scanner.Bytes()
		if len(line) == 0 || len(line) > MaxRecordBytes || bytes.ContainsRune(line, '\r') {
			return nil, ErrRow
		}
		return line[:len(line)-1], nil
	}
	control := func(expectedKind string) (map[string]json.RawMessage, error) {
		line, e := next()
		if e != nil {
			return nil, e
		}
		values, e := object(line)
		if e != nil {
			return nil, e
		}
		var kind string
		if json.Unmarshal(values["kind"], &kind) != nil || kind != expectedKind {
			return nil, ErrRow
		}
		return values, nil
	}
	header, err := control("snapshot")
	if err != nil || len(header) != 3 {
		return SnapshotEvidence{}, ErrRow
	}
	var version int
	var fingerprint string
	if json.Unmarshal(header["format_version"], &version) != nil || version != 2 || json.Unmarshal(header["plan_sha256"], &fingerprint) != nil || fingerprint != hex.EncodeToString(planHash[:]) {
		return SnapshotEvidence{}, ErrRow
	}
	evidence := SnapshotEvidence{Tables: map[string]TableEvidence{}}
	for _, plan := range program.Tables {
		begin, e := control("table_begin")
		if e != nil || len(begin) != 2 {
			return SnapshotEvidence{}, ErrRow
		}
		var table string
		if json.Unmarshal(begin["table"], &table) != nil || table != plan.Source {
			return SnapshotEvidence{}, ErrRow
		}
		var count int64
		tableHash := sha256.New()
		for {
			line, e := next()
			if e != nil {
				return SnapshotEvidence{}, ErrRow
			}
			values, e := object(line)
			if e != nil {
				return SnapshotEvidence{}, ErrRow
			}
			if rawKind, exists := values["kind"]; exists {
				var kind, endTable string
				var rows int64
				if len(values) != 3 || json.Unmarshal(rawKind, &kind) != nil || kind != "table_end" || json.Unmarshal(values["table"], &endTable) != nil || endTable != table || json.Unmarshal(values["rows"], &rows) != nil || rows != count {
					return SnapshotEvidence{}, ErrRow
				}
				break
			}
			rowTable, row, e := DecodeRecord(line)
			if e != nil || rowTable != table {
				return SnapshotEvidence{}, ErrRow
			}
			_, _ = tableHash.Write(line)
			_, _ = tableHash.Write([]byte{'\n'})
			count++
			if visit != nil {
				if visit(table, row) != nil {
					return SnapshotEvidence{}, ErrRow
				}
			}
		}
		evidence.Tables[table] = TableEvidence{count, hex.EncodeToString(tableHash.Sum(nil))}
	}
	footer, err := control("snapshot_complete")
	if err != nil || len(footer) != 2 {
		return SnapshotEvidence{}, ErrRow
	}
	var tables int
	if json.Unmarshal(footer["tables"], &tables) != nil || tables != len(program.Tables) {
		return SnapshotEvidence{}, ErrRow
	}
	if scanner.Scan() || scanner.Err() != nil {
		return SnapshotEvidence{}, ErrRow
	}
	evidence.SHA256 = hex.EncodeToString(fileHash.Sum(nil))
	if evidence.SHA256 != expectedSHA256 {
		return SnapshotEvidence{}, ErrRow
	}
	return evidence, nil
}
