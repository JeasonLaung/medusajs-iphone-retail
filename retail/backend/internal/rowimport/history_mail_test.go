package rowimport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func historicalMailFixture(t *testing.T) SourceRow {
	t.Helper()
	plans, _ := compiledPlans()
	row := SourceRow{}
	for _, f := range plans["mail_message"].Fields {
		if f.Nullable {
			row[f.Source] = Cell{SQLNull: true}
		} else {
			row[f.Source] = Cell{Raw: artificial(f)}
		}
	}
	satisfyFixtureChecks(plans["mail_message"], row)
	relationText(row, "id", "mail_verify_"+strings.Repeat("a", 64))
	relationText(row, "created_by", "registration-verification")
	relationText(row, "direction", "outbound")
	relationText(row, "subject", "Verify your email to join EuroRefurb")
	for _, key := range []string{"text_body", "html_body", "error"} {
		relationText(row, key, "https://example.invalid/verify-account?token=old-login-capability")
	}
	row["attachments"] = Cell{Raw: json.RawMessage(`[{"url":"https://example.invalid/verify-account?token=old-login-capability"}]`)}
	relationText(row, "status", "queued")
	return row
}

func TestAuthenticationMailIsRedactedAndPermanentlySuppressed(t *testing.T) {
	row := historicalMailFixture(t)
	original, _ := json.Marshal(row)
	result, err := ConvertHistoricalMail(row)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("old-login-capability")) {
		t.Fatal("legacy capability survived")
	}
	if result.Delivery.Disposition != "suppress-permanently" || result.Delivery.Reason != "excluded-legacy-registration-capability" || result.Delivery.SourceID != "mail_verify_"+strings.Repeat("a", 64) {
		t.Fatal("delivery exclusion missing")
	}
	for i, column := range result.Row.Columns {
		switch column {
		case "text_body", "html_body", "error":
			if result.Row.Values[i] != "" {
				t.Fatal("auth payload retained")
			}
		case "attachments":
			if result.Row.Values[i] != "[]" {
				t.Fatal("auth attachment retained")
			}
		case "status":
			if result.Row.Values[i] != "queued" {
				t.Fatal("delivery history rewritten")
			}
		}
	}
	after, _ := json.Marshal(row)
	if !bytes.Equal(original, after) {
		t.Fatal("source row mutated")
	}
	if _, err := Convert("mail_message", row); err != ErrHistoryPolicy {
		t.Fatal("ordinary conversion bypassed history")
	}
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{3}, 32)})
	config, _ := NewConfigSecrets(ring, nil)
	if _, err := config.Convert("mail_message", row); err != ErrHistoryPolicy {
		t.Fatal("config converter bypassed history")
	}
}

func TestHistoricalMailRejectsUnknownAuthenticationShape(t *testing.T) {
	for _, mutation := range []func(SourceRow){
		func(r SourceRow) { relationText(r, "subject", "unreviewed old token in subject") },
		func(r SourceRow) { relationText(r, "direction", "inbound") },
		func(r SourceRow) { relationText(r, "id", "arbitrary-auth-mail") },
		func(r SourceRow) { relationText(r, "created_by", "unknown-actor") },
	} {
		row := historicalMailFixture(t)
		mutation(row)
		result, err := ConvertHistoricalMail(row)
		if err != ErrHistoryPolicy || len(result.Row.Values) != 0 || result.Delivery.SourceID != "" {
			t.Fatal("unknown auth shape returned partial result")
		}
	}
	row := historicalMailFixture(t)
	row["text_body"] = Cell{Raw: json.RawMessage(`7`)}
	if _, err := ConvertHistoricalMail(row); err != ErrRow {
		t.Fatal("redaction concealed invalid source type")
	}
	row = historicalMailFixture(t)
	delete(row, "subject")
	if _, err := ConvertHistoricalMail(row); err != ErrRow {
		t.Fatal("missing source field hidden")
	}
	row = historicalMailFixture(t)
	row["unexpected"] = Cell{Raw: json.RawMessage(`true`)}
	if _, err := ConvertHistoricalMail(row); err != ErrRow {
		t.Fatal("extra source field hidden")
	}
}

func TestOrdinaryHistoricalMailIsHeldAndPreserved(t *testing.T) {
	row := historicalMailFixture(t)
	relationText(row, "id", "business-mail")
	relationText(row, "created_by", "historical-actor")
	relationText(row, "text_body", "business correspondence")
	result, err := ConvertHistoricalMail(row)
	if err != nil || result.Delivery.Disposition != "hold" {
		t.Fatal("ordinary mail not held")
	}
	for i, col := range result.Row.Columns {
		if col == "text_body" && result.Row.Values[i] != "business correspondence" {
			t.Fatal("unrelated business text changed")
		}
	}
}
