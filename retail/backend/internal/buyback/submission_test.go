package buyback

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOriginalSubmissionValidator(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/buyback-submission.json")
	var fixture struct {
		Sources []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"source_hashes"`
		Cases []struct {
			Name     string          `json:"name"`
			Input    json.RawMessage `json:"input"`
			Accepted bool            `json:"accepted"`
			Expected json.RawMessage `json:"expected"`
		} `json:"cases"`
	}
	if err != nil || json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) < 100 || len(fixture.Sources) != 2 {
		t.Fatal("original submission fixture unavailable")
	}
	for _, source := range fixture.Sources {
		data, err := os.ReadFile(filepath.Join("../../../../", source.Path))
		h := sha256.Sum256(data)
		if err != nil || hex.EncodeToString(h[:]) != source.SHA {
			t.Fatal("original submission source changed")
		}
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			in, err := ParseSubmission(c.Input)
			if (err == nil) != c.Accepted {
				t.Fatal("submission acceptance differs from original validator")
			}
			if err == nil {
				actual, _ := json.Marshal(in)
				var a, b any
				if json.Unmarshal(actual, &a) != nil || json.Unmarshal(c.Expected, &b) != nil || !reflect.DeepEqual(a, b) {
					t.Fatal("submission normalization differs from original validator")
				}
			}
		})
	}
}

func TestSubmissionRejectsAmbiguousJSON(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/buyback-submission.json")
	var fixture struct {
		Cases []struct {
			Input json.RawMessage `json:"input"`
		} `json:"cases"`
	}
	if err != nil || json.Unmarshal(data, &fixture) != nil {
		t.Fatal("submission fixture unavailable")
	}
	good := string(fixture.Cases[0].Input)
	for _, raw := range []string{
		strings.Replace(good, `"phone":`, `"phone":"ignored","phone":`, 1),
		strings.Replace(good, `"name":`, `"name":"ignored","name":`, 1),
		strings.Replace(good, `"model":`, `"model":"ignored","model":`, 1),
		strings.Replace(good, `"battery":`, `"battery":"80","battery":`, 1),
		strings.Replace(good, `"contact_name":`, `"CONTACT_NAME":`, 1),
		strings.Replace(good, `"country_code":`, `"Country_Code":`, 1),
		strings.Replace(good, `"battery":`, `"Battery":`, 1),
		good + `{}`, strings.Repeat(" ", 16385),
	} {
		if _, err := ParseSubmission([]byte(raw)); err == nil {
			t.Fatal("ambiguous submission accepted")
		}
	}
	invalid := bytes.Replace([]byte(good), []byte("Customer"), []byte{0xff}, 1)
	if _, err := ParseSubmission(invalid); err == nil {
		t.Fatal("invalid UTF8 submission accepted")
	}
}
