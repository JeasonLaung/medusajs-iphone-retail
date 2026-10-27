package rowimport

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func accountHolderFixture() SourceRow {
	return SourceRow{"id": relationCell("holder-fixture"), "provider_id": relationCell("pp_system_default"), "external_id": relationCell("legacy-subject-fixture"),
		"email": relationCell("fixture@example.invalid"), "data": {Raw: json.RawMessage(` { } `)}, "metadata": {SQLNull: true},
		"created_at": relationCell("2026-10-27T00:00:00.123456Z"), "updated_at": relationCell("2026-10-27T00:00:00.123456Z"), "deleted_at": {SQLNull: true}}
}

func relationCell(s string) Cell { raw, _ := json.Marshal(s); return Cell{Raw: raw} }

func TestAccountHolderCompleteHistoryAndReadBoundary(t *testing.T) {
	ring, _ := protected.New("first", map[string][]byte{"first": bytes.Repeat([]byte{71}, 32)})
	h, _ := NewAccountHolderHistory(ring)
	for _, metadata := range []Cell{{SQLNull: true}, {Raw: json.RawMessage(` null `)}, {Raw: json.RawMessage(` { } `)}} {
		row := accountHolderFixture()
		row["metadata"] = metadata
		original, _ := json.Marshal(row)
		converted, err := h.Convert(row)
		if err != nil || len(converted.Row.Columns) != 9 || converted.Row.Table != "payment_account_holders" {
			t.Fatal("complete holder conversion failed")
		}
		if converted.Operations.Disposition != "hold" || converted.Operations.Reason != "legacy-account-holder-reconciliation" || converted.Operations.SourceID != "holder-fixture" {
			t.Fatal("mandatory holder disposition absent")
		}
		for j, column := range converted.Row.Columns {
			if column != "data" && column != "metadata" {
				continue
			}
			if row[column].SQLNull {
				if converted.Row.Values[j] != nil {
					t.Fatal("SQL NULL lost")
				}
				continue
			}
			stored, ok := converted.Row.Values[j].(string)
			if !ok {
				t.Fatal("holder envelope must be SQL-safe text")
			}
			plain, err := OpenAccountHolderJSON(ring, "holder-fixture", "pp_system_default", column, stored)
			if err != nil || !bytes.Equal(plain, row[column].Raw) {
				t.Fatal("holder JSON bytes changed")
			}
			clear(plain)
			for _, wrong := range [][3]string{{"other", "pp_system_default", column}, {"holder-fixture", "pp_other_other", column}, {"holder-fixture", "pp_system_default", "external_id"}} {
				if _, err := OpenAccountHolderJSON(ring, wrong[0], wrong[1], wrong[2], stored); err == nil {
					t.Fatal("holder wrong context accepted")
				}
			}
			otherColumn := "data"
			if column == "data" {
				otherColumn = "metadata"
			}
			if _, err := OpenAccountHolderJSON(ring, "holder-fixture", "pp_system_default", otherColumn, stored); err == nil {
				t.Fatal("holder column swap accepted")
			}
		}
		if value, err := Convert("account_holder", row); err != ErrHistoryPolicy || len(value.Values) != 0 {
			t.Fatal("ordinary holder bypass")
		}
		config, _ := NewConfigSecrets(ring, nil)
		if value, err := config.Convert("account_holder", row); err != ErrHistoryPolicy || len(value.Values) != 0 {
			t.Fatal("merchant holder bypass")
		}
		after, _ := json.Marshal(row)
		if !bytes.Equal(original, after) {
			t.Fatal("source row mutated")
		}
	}
}

func TestAccountHolderRejectsUnknownCapabilitiesAndBadRows(t *testing.T) {
	ring, _ := protected.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{73}, 32)})
	h, _ := NewAccountHolderHistory(ring)
	for _, bad := range []struct {
		column string
		cell   Cell
	}{
		{"provider_id", relationCell("pp_other_other")}, {"id", relationCell("")}, {"data", Cell{SQLNull: true}},
		{"data", Cell{Raw: json.RawMessage(`null`)}}, {"data", Cell{Raw: json.RawMessage(`[]`)}},
		{"data", Cell{Raw: json.RawMessage(`{"token":"private"}`)}}, {"data", Cell{Raw: json.RawMessage(`{"id":"old"}`)}},
		{"metadata", Cell{Raw: json.RawMessage(`{"customer_id":"old"}`)}}, {"metadata", Cell{Raw: json.RawMessage(`{"n":1,"n":2}`)}},
		{"updated_at", relationCell("invalid")}, {"email", Cell{SQLNull: true, Raw: json.RawMessage(`null`)}},
	} {
		row := accountHolderFixture()
		row[bad.column] = bad.cell
		result, err := h.Convert(row)
		if err == nil || !reflect.DeepEqual(result, HistoricalAccountHolder{}) {
			t.Fatal("unreviewed holder returned partial material", bad.column)
		}
	}
	row := accountHolderFixture()
	delete(row, "email")
	row["new_account_id"] = relationCell("new-account")
	if result, err := h.Convert(row); err == nil || !reflect.DeepEqual(result, HistoricalAccountHolder{}) {
		t.Fatal("ownership link accepted")
	}
	if _, err := NewAccountHolderHistory(nil); err == nil {
		t.Fatal("missing key ring accepted")
	}
	var nilHistory *AccountHolderHistory
	if _, err := nilHistory.Convert(accountHolderFixture()); err == nil {
		t.Fatal("nil converter accepted")
	}
}

func TestAccountHolderEnvelopeRotationTamperAndReview(t *testing.T) {
	old := bytes.Repeat([]byte{79}, 32)
	ring, _ := protected.New("first", map[string][]byte{"first": old})
	stored, err := sealAccountHolderJSON(ring, "fixture", "pp_system_default", "data", json.RawMessage(` { } `))
	if err != nil {
		t.Fatal("holder seal unavailable")
	}
	rotated, _ := protected.New("second", map[string][]byte{"first": old, "second": bytes.Repeat([]byte{81}, 32)})
	if plain, err := OpenAccountHolderJSON(rotated, "fixture", "pp_system_default", "data", stored); err != nil || string(plain) != ` { } ` {
		t.Fatal("old holder key cannot be read during rotation")
	}
	missing, _ := protected.New("second", map[string][]byte{"second": bytes.Repeat([]byte{81}, 32)})
	if _, err := OpenAccountHolderJSON(missing, "fixture", "pp_system_default", "data", stored); err == nil {
		t.Fatal("missing old holder key accepted")
	}
	for _, bad := range []string{`{}`, stored + "\n", stored + "=", "ers1.invalid", stored[:len(stored)-8] + "AAAAAAAA", strings.Repeat("x", 2*protected.MaxPlaintext+1)} {
		if _, err := OpenAccountHolderJSON(ring, "fixture", "pp_system_default", "data", bad); err == nil {
			t.Fatal("malformed or tampered holder envelope accepted")
		}
	}
	if _, err := accountHolderContext(string([]byte{255}), "pp_system_default", "data"); err == nil {
		t.Fatal("invalid UTF8 context accepted")
	}
	if _, err := accountHolderContext(strings.Repeat("界", 192), "pp_system_default", "data"); err == nil {
		t.Fatal("oversized context accepted")
	}
	// Even authenticated envelopes must satisfy the pinned payload review.
	aad, _ := accountHolderContext("fixture", "pp_system_default", "data")
	sealed, err := ring.Seal(aad, []byte(`{"token":"private"}`))
	if err != nil {
		t.Fatal("fixture authenticated secret unavailable")
	}
	// Build the exact SQL envelope without calling the guarded seal helper.
	forged := "ers1." + base64.StdEncoding.EncodeToString(sealed)
	if _, err := OpenAccountHolderJSON(ring, "fixture", "pp_system_default", "data", forged); err != ErrHistoryPolicy {
		t.Fatal("authenticated unknown holder payload accepted")
	}
}

func TestPinnedAccountHolderHistoryOffline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_ACCOUNT_HOLDER_SNAPSHOT")
	if path == "" {
		t.Skip("explicit private v2 snapshot required")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("private holder snapshot unavailable")
	}
	defer file.Close()
	var rows []SourceRow
	verified, err := VerifyStream(file, "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985", func(table string, row SourceRow) error {
		if table == "account_holder" {
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil || len(verified.Tables) != 159 || len(rows) != 1 {
		t.Fatal("complete holder source evidence invalid")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary holder key unavailable")
	}
	defer clear(key)
	ring, _ := protected.New("rehearsal", map[string][]byte{"rehearsal": key})
	h, _ := NewAccountHolderHistory(ring)
	converted, err := h.Convert(rows[0])
	if err != nil || converted.Operations.Disposition != "hold" {
		t.Fatal("real holder conversion failed")
	}
	readbacks, nulls, wrong := 0, 0, 0
	for j, column := range converted.Row.Columns {
		if column != "data" && column != "metadata" {
			continue
		}
		cell := rows[0][column]
		if cell.SQLNull {
			if converted.Row.Values[j] != nil {
				t.Fatal("real holder NULL changed")
			}
			nulls++
			continue
		}
		stored, ok := converted.Row.Values[j].(string)
		if !ok {
			t.Fatal("real holder envelope invalid")
		}
		plain, err := OpenAccountHolderJSON(ring, converted.Operations.SourceID, "pp_system_default", column, stored)
		if err != nil || !bytes.Equal(plain, cell.Raw) {
			t.Fatal("real holder protected bytes differ")
		}
		clear(plain)
		readbacks++
		if _, err := OpenAccountHolderJSON(ring, converted.Operations.SourceID+"-wrong", "pp_system_default", column, stored); err == nil {
			t.Fatal("real holder wrong context accepted")
		}
		wrong++
	}
	if readbacks != 1 || nulls != 1 || wrong != 1 {
		t.Fatal("real holder protection coverage differs")
	}
	t.Log(`ACCOUNT_HOLDER_AUDIT {"source_tables":159,"rows":1,"bound_columns":9,"protected_readbacks":1,"sql_nulls":1,"wrong_context_rejections":1,"required_operation_holds":1}`)
}
