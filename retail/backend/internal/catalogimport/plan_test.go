package catalogimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Cross-file provenance is verified by ordinary Go checks too, so CI cannot
// silently use a stale generated layout after field/unique plans change.
func TestCatalogLayoutMatchesReviewedInputs(t *testing.T) {
	var document struct {
		Hashes map[string]string `json:"input_sha256"`
		Tables []table           `json:"tables"`
	}
	if json.Unmarshal(program, &document) != nil || len(document.Tables) != 36 || len(document.Hashes) != 3 {
		t.Fatal("catalog layout coverage differs")
	}
	for path, expected := range document.Hashes {
		data, err := os.ReadFile(filepath.Join("../../../..", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal("reviewed catalog input unavailable")
		}
		h := sha256.Sum256(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")))
		if hex.EncodeToString(h[:]) != expected {
			t.Fatal("catalog SQL layout input changed; regenerate and review")
		}
	}
	data, err := os.ReadFile("../rowimport/field_plans.json")
	if err != nil {
		t.Fatal("source field plan unavailable")
	}
	var fields struct {
		Tables []struct {
			Source  string   `json:"source"`
			Target  string   `json:"target"`
			Primary []string `json:"primary_key"`
			Fields  []struct {
				Source string `json:"source"`
				Target string `json:"target"`
				Type   string `json:"target_type"`
			} `json:"fields"`
		} `json:"tables"`
	}
	if json.Unmarshal(data, &fields) != nil {
		t.Fatal("source field plan invalid")
	}
	registry := 0
	for _, layout := range document.Tables {
		found := false
		for _, source := range fields.Tables {
			if source.Source != layout.Source {
				continue
			}
			found = true
			if source.Target != layout.Target || len(source.Fields) != len(layout.Columns) || len(source.Primary) != len(layout.Primary) {
				t.Fatal("catalog SQL shape differs")
			}
			for j, f := range source.Fields {
				c := layout.Columns[j]
				if f.Source != c.Source || f.Target != c.Target || strings.HasPrefix(f.Type, "DECIMAL(38,18)") != c.Decimal {
					t.Fatal("catalog SQL column/decimal comparison differs")
				}
			}
			for j, primary := range source.Primary {
				if layout.Primary[j].Source != primary {
					t.Fatal("catalog primary key order differs")
				}
			}
		}
		if !found {
			t.Fatal("unmapped catalog table")
		}
		if layout.Registry {
			registry++
		}
	}
	if registry != 7 {
		t.Fatal("catalog complete unique registry coverage differs")
	}
}
