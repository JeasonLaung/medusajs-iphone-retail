package rowimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// Optional offline audit of scalar CHECK inputs only. v1 cannot distinguish
// SQL NULL from JSON null in JSON columns, so those are expressly prohibited.
// This is not Convert, a v2 snapshot acceptance check, or a database import.
func TestPinnedSourceChecksOffline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_SOURCE_CHECK_BUNDLE")
	if path == "" {
		t.Skip("explicit ignored offline snapshot required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("offline snapshot unavailable")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != "aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132" {
		t.Fatal("offline snapshot hash differs from pinned evidence")
	}
	var bundle struct {
		Tables map[string][]map[string]json.RawMessage `json:"tables"`
	}
	if json.Unmarshal(data, &bundle) != nil {
		t.Fatal("invalid offline snapshot")
	}
	plans, err := compiledPlans()
	if err != nil {
		t.Fatal(err)
	}
	tables, constraints, evaluations, nullInputs := 0, 0, 0, 0
	failures := map[string]int{}
	byConstraint := map[string]int{}
	populated := 0
	for _, plan := range plans {
		if len(plan.Checks) == 0 {
			continue
		}
		tables++
		rows, exists := bundle.Tables[plan.Source]
		if !exists {
			t.Fatal("offline snapshot missing mapped table")
		}
		fields := map[string]field{}
		for _, f := range plan.Fields {
			fields[f.Source] = f
		}
		for _, check := range plan.Checks {
			constraints++
			byConstraint[check.Name] = len(rows)
			if len(rows) > 0 {
				populated++
			}
			for _, source := range rows {
				row := SourceRow{}
				for _, column := range check.Columns {
					f, exists := fields[column]
					if !exists {
						t.Fatal("CHECK column missing from program")
					}
					switch f.Transform {
					case "utf8_text", "identity_string", "enum_text", "strict_boolean", "money_major_decimal", "checked_int64", "checked_int32":
					default:
						t.Fatal("v1 snapshot cannot prove CHECK column scalar NULL semantics")
					}
					raw, exists := source[column]
					if !exists {
						t.Fatal("offline source column missing")
					}
					if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
						nullInputs++
						row[column] = Cell{SQLNull: true}
					} else {
						row[column] = Cell{Raw: raw}
					}
				}
				evaluations++
				if checkSourceConstraints([]sourceCheck{check}, row) != nil {
					failures[check.Name]++
				}
			}
		}
	}
	if constraints != 47 {
		t.Fatal("CHECK coverage drift")
	}
	metadata := map[string]any{"constraints": constraints, "constraints_with_rows": populated, "tables": tables, "row_constraint_evaluations": evaluations, "null_column_inputs": nullInputs, "evaluations_by_constraint": byConstraint, "violations_by_constraint": failures}
	encoded, _ := json.Marshal(metadata)
	t.Log("SOURCE_CHECK_AUDIT " + string(encoded))
	if len(failures) != 0 {
		t.Fatal("offline source CHECK violations require reconciliation")
	}
}
