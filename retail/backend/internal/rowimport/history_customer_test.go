package rowimport

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func customerHistoryFixture() SourceRow {
	return SourceRow{"id": relationCell("historical-customer"), "company_name": {SQLNull: true},
		"first_name": relationCell(" Original 名称 "), "last_name": relationCell(""),
		"email": relationCell("same-as-new@example.invalid"), "phone": {SQLNull: true},
		"has_account": {Raw: json.RawMessage(`true`)}, "metadata": {SQLNull: true},
		"created_at": relationCell("2026-10-28T00:00:00.123456Z"), "updated_at": relationCell("2026-10-28T00:00:00.123456Z"),
		"deleted_at": {SQLNull: true}, "created_by": relationCell("old-operator")}
}

func TestCustomerSubjectBaselineAndNoLoginGrant(t *testing.T) {
	ring, _ := protected.New("first", map[string][]byte{"first": bytes.Repeat([]byte{61}, 32)})
	h, _ := NewCustomerHistory(ring)
	for _, cell := range []Cell{{SQLNull: true}, {Raw: json.RawMessage(` null `)}, {Raw: json.RawMessage(` { } `)}} {
		row := customerHistoryFixture()
		row["metadata"] = cell
		original, _ := json.Marshal(row)
		if _, err := Convert("customer", row); err != ErrHistoryPolicy {
			t.Fatal("ordinary conversion bypassed historical identity policy")
		}
		result, err := h.Convert(row)
		if err != nil || result.Row.Table != "historical_customer_subjects" || len(result.Row.Columns) != 12 || result.Binding.Disposition != "excluded" || result.Binding.Effect != "customer-account-binding" {
			t.Fatal("historical customer conversion invalid")
		}
		for i, name := range result.Row.Columns {
			switch name {
			case "legacy_had_account":
				if result.Row.Values[i] != true {
					t.Fatal("historical account observation changed")
				}
			case "first_name":
				if result.Row.Values[i] != " Original 名称 " {
					t.Fatal("historical name changed")
				}
			case "last_name":
				if result.Row.Values[i] != "" {
					t.Fatal("historical empty text changed")
				}
			case "metadata":
				if cell.SQLNull {
					if result.Row.Values[i] != nil {
						t.Fatal("SQL NULL became ciphertext")
					}
					continue
				}
				sealed, ok := result.Row.Values[i].([]byte)
				if !ok || bytes.Equal(sealed, cell.Raw) {
					t.Fatal("customer JSON not sealed as binary")
				}
				plain, err := OpenCustomerMetadata(ring, "historical-customer", sealed)
				if err != nil || !bytes.Equal(plain, cell.Raw) {
					t.Fatal("metadata whitespace or JSON null lost")
				}
				clear(plain)
				if _, err := OpenCustomerMetadata(ring, "different-subject", sealed); err == nil {
					t.Fatal("another historical subject opened metadata")
				}
			default:
				if strings.Contains(name, "password") || strings.Contains(name, "token") || strings.Contains(name, "session") || name == "new_account_id" {
					t.Fatal("historical row contains login authority")
				}
			}
		}
		after, _ := json.Marshal(row)
		if !bytes.Equal(original, after) {
			t.Fatal("source row mutated")
		}
	}
}

func TestCustomerSubjectRejectsUnknownCapabilitiesAndCorruption(t *testing.T) {
	ring, _ := protected.New("first", map[string][]byte{"first": bytes.Repeat([]byte{62}, 32)})
	h, _ := NewCustomerHistory(ring)
	for _, raw := range []string{`{"token":"old"}`, `{"new_account_id":"new"}`, `{"roles":["admin"]}`, `{"business_note":"unreviewed"}`, `{"nested":{"password":"old"}}`, `{"x":1,"x":2}`, `[]`, `""`, `false`, `1`, `{"unterminated"`, strings.Repeat(" ", protected.MaxPlaintext+1)} {
		row := customerHistoryFixture()
		row["metadata"] = Cell{Raw: json.RawMessage(raw)}
		if result, err := h.Convert(row); err == nil || !reflect.DeepEqual(result, HistoricalCustomer{}) {
			t.Fatal("unknown metadata returned partial subject material")
		}
	}
	for _, mutate := range []func(SourceRow){
		func(row SourceRow) { delete(row, "email") },
		func(row SourceRow) { row["password"] = relationCell("old") },
		func(row SourceRow) { row["new_account_id"] = relationCell("new") },
		func(row SourceRow) { row["id"] = relationCell("") },
		func(row SourceRow) { row["id"] = relationCell(strings.Repeat("x", 192)) },
		func(row SourceRow) { row["has_account"] = Cell{Raw: json.RawMessage(`"true"`)} },
		func(row SourceRow) { row["metadata"] = Cell{SQLNull: true, Raw: json.RawMessage(`null`)} },
		func(row SourceRow) { row["updated_at"] = relationCell("invalid") },
	} {
		row := customerHistoryFixture()
		mutate(row)
		if result, err := h.Convert(row); err == nil || !reflect.DeepEqual(result, HistoricalCustomer{}) {
			t.Fatal("invalid customer returned partial material")
		}
	}
	if _, err := NewCustomerHistory(nil); err == nil {
		t.Fatal("missing key accepted")
	}
	var nilHistory *CustomerHistory
	if _, err := nilHistory.Convert(customerHistoryFixture()); err == nil {
		t.Fatal("nil converter accepted")
	}
}

func TestCustomerSubjectEnvelopeRotationAndTampering(t *testing.T) {
	key := bytes.Repeat([]byte{63}, 32)
	ring, _ := protected.New("first", map[string][]byte{"first": key})
	h, _ := NewCustomerHistory(ring)
	row := customerHistoryFixture()
	row["metadata"] = Cell{Raw: json.RawMessage(` { } `)}
	result, err := h.Convert(row)
	if err != nil {
		t.Fatal("customer envelope unavailable")
	}
	var sealed []byte
	for i, name := range result.Row.Columns {
		if name == "metadata" {
			sealed = result.Row.Values[i].([]byte)
		}
	}
	rotated, _ := protected.New("second", map[string][]byte{"first": key, "second": bytes.Repeat([]byte{64}, 32)})
	plain, err := OpenCustomerMetadata(rotated, "historical-customer", sealed)
	if err != nil || string(plain) != ` { } ` {
		t.Fatal("old customer key unreadable during rotation")
	}
	clear(plain)
	missing, _ := protected.New("second", map[string][]byte{"second": bytes.Repeat([]byte{64}, 32)})
	if _, err := OpenCustomerMetadata(missing, "historical-customer", sealed); err == nil {
		t.Fatal("missing old key accepted")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := OpenCustomerMetadata(ring, "historical-customer", sealed); err == nil {
		t.Fatal("tampered customer metadata accepted")
	}
	aad, _ := customerHistoryContext("historical-customer")
	unknown, _ := ring.Seal(aad, []byte(`{"token":"old"}`))
	if _, err := OpenCustomerMetadata(ring, "historical-customer", unknown); err == nil {
		t.Fatal("valid ciphertext bypassed metadata policy")
	}
	for _, value := range [][]byte{nil, {}, []byte("{}"), []byte("legacy-plaintext")} {
		if _, err := OpenCustomerMetadata(ring, "historical-customer", value); err == nil {
			t.Fatal("plaintext or missing ciphertext accepted")
		}
	}
}

func TestPinnedCustomerSubjectOffline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_CUSTOMER_SNAPSHOT")
	if path == "" {
		t.Skip("explicit pinned private source required")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("private customer snapshot unavailable")
	}
	defer file.Close()
	var rows []SourceRow
	verified, err := VerifyStream(file, "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985", func(table string, row SourceRow) error {
		if table == "customer" {
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil || len(verified.Tables) != 159 || len(rows) != 5 {
		t.Fatal("complete customer snapshot evidence invalid")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("ephemeral customer key unavailable")
	}
	defer clear(key)
	ring, _ := protected.New("rehearsal", map[string][]byte{"rehearsal": key})
	h, _ := NewCustomerHistory(ring)
	fields, nulls, blocked := 0, 0, 0
	for _, row := range rows {
		if _, err := Convert("customer", row); err != ErrHistoryPolicy {
			t.Fatal("actual subject bypassed dedicated history policy")
		}
		blocked++
		result, err := h.Convert(row)
		if err != nil || result.Binding.Disposition != "excluded" || result.Row.Table != "historical_customer_subjects" || len(result.Row.Columns) != 12 {
			t.Fatal("actual customer conversion failed")
		}
		for i, column := range result.Row.Columns {
			source := column
			if column == "legacy_had_account" {
				source = "has_account"
			}
			cell := row[source]
			if cell.SQLNull {
				if result.Row.Values[i] != nil {
					t.Fatal("actual customer NULL changed")
				}
				if column == "metadata" {
					nulls++
				}
			} else if column == "metadata" {
				t.Fatal("unreviewed actual nonempty metadata baseline")
			} else {
				plans, _ := compiledPlans()
				var found bool
				for _, f := range plans["customer"].Fields {
					if f.Target == column {
						expected, err := convertValue(f, cell.Raw)
						if err != nil || !reflect.DeepEqual(expected, result.Row.Values[i]) {
							t.Fatal("actual customer business field changed")
						}
						found = true
					}
				}
				if !found {
					t.Fatal("unmapped actual customer field")
				}
			}
			fields++
		}
	}
	if fields != 60 || nulls != 5 || blocked != 5 {
		t.Fatal("actual customer conversion coverage differs")
	}
	t.Log(`CUSTOMER_SUBJECT_AUDIT {"source_tables":159,"source_rows":27140,"customer_rows":5,"bound_fields":60,"metadata_sql_nulls":5,"ordinary_import_rejections":5,"excluded_account_bindings":5,"sql_written":false,"login_accounts_created":0}`)
}
