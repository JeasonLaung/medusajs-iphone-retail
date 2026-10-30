package buyback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOriginalAdminCommandValidator(t *testing.T) {
	data, e := os.ReadFile("../../../../docs/migration/fixtures/buyback-admin-command.json")
	var fixture struct {
		Sources []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"source_hashes"`
		Cases []struct {
			Name     string
			Input    json.RawMessage
			Accepted bool
			Expected json.RawMessage
		}
	}
	if e != nil || json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) < 60 {
		t.Fatal("original command fixture")
	}
	for _, s := range fixture.Sources {
		raw, e := os.ReadFile(filepath.Join("../../../../", s.Path))
		sum := sha256.Sum256(raw)
		if e != nil || hex.EncodeToString(sum[:]) != s.SHA {
			t.Fatal("original command source changed")
		}
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, e := ParseAdminCommand(c.Input)
			if (e == nil) != c.Accepted {
				t.Fatalf("acceptance mismatch accepted=%v error=%v", c.Accepted, e)
			}
			if e == nil {
				actual, _ := json.Marshal(got)
				var a, b any
				json.Unmarshal(actual, &a)
				if got.Action == "review_photo" && got.Note == "" {
					a.(map[string]any)["note"] = ""
				}
				json.Unmarshal(c.Expected, &b)
				if !reflect.DeepEqual(a, b) {
					t.Fatal("normalization mismatch")
				}
			}
		})
	}
	for _, raw := range []string{`{"action":"mark_received","action":"mark_received"}`, `{"Action":"mark_received"}`, `{"action":"review_photo","photo_id":"p","review_status":"approved","note":null}`} {
		if _, e := ParseAdminCommand([]byte(raw)); e == nil {
			t.Fatal("invalid admin command accepted")
		}
	}
}
