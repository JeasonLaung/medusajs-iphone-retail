package rowimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestPinnedHistoricalMailOffline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_MAIL_HISTORY_BUNDLE")
	if path == "" {
		t.Skip("explicit ignored snapshot required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("offline mail snapshot missing")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != "aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132" {
		t.Fatal("offline mail snapshot hash differs")
	}
	var bundle struct {
		Tables map[string][]map[string]json.RawMessage `json:"tables"`
	}
	if json.Unmarshal(data, &bundle) != nil {
		t.Fatal("offline mail snapshot invalid")
	}
	plans, _ := compiledPlans()
	rows, exists := bundle.Tables["mail_message"]
	if !exists {
		t.Fatal("offline mail table missing")
	}
	counts := map[string]int{"rows": len(rows), "permanent_suppressions": 0, "held_business_mail": 0, "redacted_fields": 0, "source_verification_link_bodies": 0}
	for _, source := range rows {
		row := SourceRow{}
		for _, f := range plans["mail_message"].Fields {
			raw, present := source[f.Source]
			if !present {
				t.Fatal("offline mail column missing")
			}
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				if f.Transform == "exact_json_text" {
					t.Fatal("ambiguous v1 JSON null in mail")
				}
				row[f.Source] = Cell{SQLNull: true}
			} else {
				row[f.Source] = Cell{Raw: raw}
			}
		}
		result, err := ConvertHistoricalMail(row)
		if err != nil {
			t.Fatal("real historical mail conversion failed")
		}
		if result.Delivery.Disposition == "suppress-permanently" {
			counts["permanent_suppressions"]++
			for _, col := range []string{"text_body", "html_body"} {
				if bytes.Contains(row[col].Raw, []byte("/verify-account?token=")) {
					counts["source_verification_link_bodies"]++
				}
			}
			for i, col := range result.Row.Columns {
				switch col {
				case "text_body", "html_body", "error":
					if result.Row.Values[i] != "" {
						t.Fatal("auth text survived")
					}
					counts["redacted_fields"]++
				case "attachments":
					if result.Row.Values[i] != "[]" {
						t.Fatal("auth attachments survived")
					}
					counts["redacted_fields"]++
				}
			}
		} else {
			counts["held_business_mail"]++
		}
	}
	encoded, _ := json.Marshal(counts)
	t.Log("MAIL_HISTORY_AUDIT " + string(encoded))
}
