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

func TestOriginalInspectionCommandValidator(t *testing.T) {
	data, e := os.ReadFile("../../../../docs/migration/fixtures/buyback-inspection-command.json")
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
			got, e := ParseInspectionCommand(c.Input)
			if (e == nil) != c.Accepted {
				t.Fatalf("acceptance mismatch accepted=%v error=%v", c.Accepted, e)
			}
			if e == nil {
				actual, _ := json.Marshal(got)
				var a, b any
				json.Unmarshal(actual, &a)
				json.Unmarshal(c.Expected, &b)
				if !reflect.DeepEqual(a, b) {
					t.Fatal("normalization mismatch")
				}
			}
		})
	}
	for _, raw := range []string{
		`{"action":"link_device","item_id":"i","device_id":"d","device_id":"other"}`,
		`{"action":"link_device","Item_id":"i","device_id":"d"}`,
		`{"action":"final_offer","items":[{"item_id":"i","amount":1,"amount":2,"notes":"inspected"}]}`,
		`{"action":"final_offer","items":[{"item_id":"i","amount":"1","notes":"inspected"}]}`,
	} {
		if _, e := ParseInspectionCommand([]byte(raw)); e == nil {
			t.Fatal("invalid inspection input accepted")
		}
	}
	// Inspection actions retain their separate strict parser; review-only input
	// must not silently accept additional union arms.
	for _, raw := range []string{`{"action":"link_device","item_id":"i","device_id":"d"}`, `{"action":"final_offer","items":[{"item_id":"i","amount":0,"notes":"inspected"}]}`} {
		if _, e := ParseAdminCommand([]byte(raw)); e == nil {
			t.Fatal("review parser accepted inspection action")
		}
	}
}
