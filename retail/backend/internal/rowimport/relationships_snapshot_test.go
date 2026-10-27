package rowimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// v1 is used only for keys, reference text and deleted_at. JSON payloads are
// never treated as proven SQL NULL; this does not certify or import a v2 stream.
func TestPinnedRelationshipsOffline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_RELATION_BUNDLE")
	if path == "" {
		t.Skip("explicit ignored offline snapshot required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("offline snapshot unavailable")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != "aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132" {
		t.Fatal("offline snapshot hash mismatch")
	}
	var bundle struct {
		Tables map[string][]map[string]json.RawMessage `json:"tables"`
	}
	if json.Unmarshal(data, &bundle) != nil {
		t.Fatal("invalid offline snapshot")
	}
	c, err := newRelationCollector(RelationLimits{MaxRows: 100000, KeyBytes: 64 << 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, plan := range c.plans {
		rows, exists := bundle.Tables[plan.Source]
		if !exists {
			t.Fatal("offline table missing")
		}
		used := map[string]bool{}
		for _, col := range plan.PrimaryKey {
			used[col] = true
		}
		for _, col := range c.needed[plan.Source] {
			used[col] = true
		}
		for _, rule := range plan.References {
			used[rule.Column] = true
		}
		if _, exists := c.fields[plan.Source]["deleted_at"]; exists {
			used["deleted_at"] = true
		}
		for col := range used {
			f := c.fields[plan.Source][col]
			switch f.Transform {
			case "identity_string", "code_string", "utf8_text", "strict_boolean", "utc_datetime6":
			default:
				t.Fatal("v1 cannot prove relationship column scalar NULL semantics")
			}
		}
		for _, source := range rows {
			row := SourceRow{}
			for _, f := range plan.Fields {
				raw, exists := source[f.Source]
				if !exists {
					t.Fatal("offline source column missing")
				}
				if used[f.Source] && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
					row[f.Source] = Cell{SQLNull: true}
				} else {
					row[f.Source] = Cell{Raw: raw}
				}
			}
			if err := c.add(plan.Source, row); err != nil {
				t.Fatal("offline relationship collection failed")
			}
		}
	}
	evidence, err := c.finish()
	if err != nil && err != ErrReferences {
		t.Fatal("offline audit incomplete")
	}
	report := struct {
		Relations RelationEvidence `json:"relations"`
		Accepted  bool             `json:"relations_accepted"`
	}{evidence, err == nil}
	encoded, _ := json.Marshal(report)
	t.Log("RELATION_AUDIT " + string(encoded))
}
