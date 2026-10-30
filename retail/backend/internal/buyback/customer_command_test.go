package buyback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOriginalCustomerCommandValidator(t *testing.T) {
	data, e := os.ReadFile("../../../../docs/migration/fixtures/buyback-customer-command.json")
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
			got, e := ParseCustomerCommand(c.Input)
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
	for _, raw := range []string{`{"action":"cancel","action":"cancel"}`, `{"Action":"cancel"}`, `{"action":"accept_offer","final_amount":"1.00"}`, `{"action":"cancel"}{}`, `{"action":"submit_photos","items":[{"item_id":"one","serial_number":"SN","imei":"000000000000001","imei":"000000000000002"}]}`} {
		if _, e := ParseCustomerCommand([]byte(raw)); e == nil {
			t.Fatal("ambiguous command accepted")
		}
	}
	if _, e := ParseCustomerCommand([]byte(`{"action":"mark_shipped","tracking_number":"` + strings.Repeat("x", 16384) + `"}`)); e == nil {
		t.Fatal("oversized command accepted")
	}
}
