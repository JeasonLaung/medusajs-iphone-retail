package rowimport

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/legacysecret"
	"github.com/eurorefurb/retail/internal/protected"
)

// The explicit offline runner pipes private values to stdin, never a command
// argument, environment variable, temporary file or report. The fixture is not
// a new snapshot format and cannot establish JSON SQL NULL provenance.
func TestActualMerchantConfigOfflinePipe(t *testing.T) {
	if os.Getenv("ROWIMPORT_CONFIG_PIPE") != "1" {
		t.Skip("requires explicit private offline pipe")
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, (8<<20)+1))
	if err != nil || len(data) > 8<<20 {
		t.Fatal("private audit input unavailable or too large")
	}
	defer clear(data)
	var input struct {
		Keys map[string]string `json:"keys"`
		Rows []struct {
			Table    string                     `json:"table"`
			Cells    map[string]json.RawMessage `json:"cells"`
			Expected map[string]string          `json:"expected"`
		} `json:"rows"`
	}
	if json.Unmarshal(data, &input) != nil || len(input.Rows) == 0 {
		t.Fatal("invalid private audit input")
	}
	keys, err := legacysecret.New(input.Keys)
	if err != nil {
		t.Fatal("selected legacy configuration invalid")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary target key unavailable")
	}
	defer clear(key)
	ring, err := protected.New("offline-ephemeral", map[string][]byte{"offline-ephemeral": key})
	if err != nil {
		t.Fatal("temporary target ring unavailable")
	}
	converter, _ := NewConfigSecrets(ring, keys)
	plans, _ := compiledPlans()
	counts := map[string]int{}
	fields, nonempty, empty := 0, 0, 0
	for _, source := range input.Rows {
		switch source.Table {
		case "mail_account", "seo_provider", "seo_setting", "webhook_endpoint", "mollie_config":
		default:
			t.Fatal("private audit table not explicitly selected")
		}
		plan := plans[source.Table]
		row := SourceRow{}
		if len(source.Cells) != len(plan.Fields) {
			t.Fatal("private audit source columns differ")
		}
		for _, f := range plan.Fields {
			raw, present := source.Cells[f.Source]
			if !present {
				t.Fatal("private audit source column missing")
			}
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				if f.Transform == "exact_json_text" || f.Transform == "protected_exact_json" {
					t.Fatal("v1 cannot prove JSON SQL NULL provenance")
				}
				row[f.Source] = Cell{SQLNull: true}
			} else {
				row[f.Source] = Cell{Raw: raw}
			}
		}
		bound, err := converter.Convert(source.Table, row)
		if err != nil {
			t.Fatal("actual source row conversion failed")
		}
		id, err := importvalue.Text(row["id"].Raw, 191)
		if err != nil {
			t.Fatal("private source ID invalid")
		}
		expectedFields := 0
		for i, f := range plan.Fields {
			if f.Transform != "protected_string" {
				continue
			}
			expectedFields++
			expected, present := source.Expected[f.Source]
			if !present {
				t.Fatal("independent source expectation missing")
			}
			sealed, ok := bound.Values[i].([]byte)
			if !ok {
				t.Fatal("actual secret did not produce envelope")
			}
			_, purpose, err := configDomain(source.Table, f.Source, row)
			if err != nil {
				t.Fatal("actual secret context invalid")
			}
			context := protected.Context{Purpose: purpose, Table: plan.Target, Column: f.Target, Row: id}
			plain, err := ring.Open(context, sealed)
			if err != nil || string(plain) != expected {
				clear(plain)
				t.Fatal("actual source-to-target bytes differ")
			}
			clear(plain)
			context.Row = "different-offline-row"
			if _, err := ring.Open(context, sealed); err == nil {
				t.Fatal("actual envelope row binding failed")
			}
			fields++
			if expected == "" {
				empty++
			} else {
				nonempty++
			}
		}
		if expectedFields != len(source.Expected) {
			t.Fatal("private expectation columns differ")
		}
		counts[source.Table]++
	}
	report, _ := json.Marshal(map[string]any{"rows_by_table": counts, "sealed_fields": fields, "nonempty": nonempty, "empty": empty, "wrong_row_rejected": fields})
	t.Log("CONFIG_PIPE_AUDIT " + string(report))
}
