package rowimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func streamFixture() []byte {
	var program struct {
		Tables []tablePlan `json:"tables"`
	}
	_ = json.Unmarshal(fieldPlans, &program)
	var data bytes.Buffer
	emit := func(value any) { encoded, _ := json.Marshal(value); data.Write(encoded); data.WriteByte('\n') }
	hash := sha256.Sum256(bytes.ReplaceAll(fieldPlans, []byte("\r\n"), []byte("\n")))
	emit(map[string]any{"kind": "snapshot", "format_version": 2, "plan_sha256": hex.EncodeToString(hash[:])})
	for _, plan := range program.Tables {
		emit(map[string]any{"kind": "table_begin", "table": plan.Source})
		count := 0
		if plan.Source == "product_tag" {
			data.Write(recordFixture())
			data.WriteByte('\n')
			count = 1
		}
		emit(map[string]any{"kind": "table_end", "table": plan.Source, "rows": count})
	}
	emit(map[string]any{"kind": "snapshot_complete", "tables": 159})
	return data.Bytes()
}

func sum(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }

func TestFullStreamIntegrityAndFraming(t *testing.T) {
	data := streamFixture()
	visits := 0
	evidence, err := VerifyStream(bytes.NewReader(data), sum(data), func(table string, row SourceRow) error { visits++; _, err := Convert(table, row); return err })
	if err != nil || visits != 1 || len(evidence.Tables) != 159 || evidence.Tables["product_tag"].Rows != 1 {
		t.Fatal("complete stream rejected", err)
	}
	mutations := [][]byte{data[:len(data)-1], append(bytes.Clone(data), '\n'), bytes.Replace(data, []byte(`"tables":159`), []byte(`"tables":158`), 1), bytes.Replace(data, []byte(`"rows":1`), []byte(`"rows":2`), 1), bytes.Replace(data, []byte(`"format_version":2`), []byte(`"format_version":1`), 1), bytes.Replace(data, []byte(`"table":"currency"`), []byte(`"table":"user"`), 1)}
	for _, bad := range mutations {
		if _, err := VerifyStream(bytes.NewReader(bad), sum(bad), nil); err != ErrRow {
			t.Fatal("invalid framing accepted")
		}
	}
	if _, err := VerifyStream(bytes.NewReader(data), strings.Repeat("0", 64), nil); err != ErrRow {
		t.Fatal("wrong file hash accepted")
	}
	if _, err := VerifyStream(bytes.NewReader(data), sum(data), func(string, SourceRow) error { return ErrProtection }); err != ErrRow {
		t.Fatal("callback failure ignored")
	}
}

func TestActualPostgresSnapshotStream(t *testing.T) {
	path := os.Getenv("ROWIMPORT_POSTGRES_STREAM")
	if path == "" {
		t.Skip("requires isolated artificial source stream")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("artificial stream missing")
	}
	defer file.Close()
	checksum, err := os.ReadFile(path + ".sha256")
	if err != nil {
		t.Fatal("artificial checksum missing")
	}
	visits := 0
	evidence, err := VerifyStream(file, strings.TrimSpace(string(checksum)), func(table string, row SourceRow) error { visits++; _, err := Convert(table, row); return err })
	if err != nil || len(evidence.Tables) != 159 || visits != 3 || evidence.Tables["product_tag"].Rows != 3 {
		t.Fatal("actual export rejected", err)
	}
}
