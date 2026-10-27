package rowimport

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func TestPinnedPaymentHistoryV2Offline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_PAYMENT_HISTORY_V2")
	if path == "" {
		t.Skip("explicit ignored v2 snapshot required")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("v2 payment snapshot unavailable")
	}
	defer file.Close()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary payment key unavailable")
	}
	defer clear(key)
	ring, err := protected.New("offline-payment-row", map[string][]byte{"offline-payment-row": key})
	if err != nil {
		t.Fatal("temporary payment key invalid")
	}
	converter, _ := NewPaymentHistory(ring)
	counts := map[string]int64{}
	snapshot, err := VerifyStream(file, "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985", func(table string, row SourceRow) error {
		if table != "payment_session" && table != "payment" {
			return nil
		}
		result, err := converter.Convert(table, row)
		if err != nil || result.Operations.Disposition != "hold" || len(result.Row.Values) != 14 {
			return ErrRow
		}
		identity, _ := paymentRowIdentity(table, row)
		for i, column := range result.Row.Columns {
			value := result.Row.Values[i]
			if column != "data" && column != "context" && column != "metadata" {
				continue
			}
			source := row[column]
			if source.SQLNull {
				if value != nil {
					return ErrRow
				}
				counts[table+"."+column+".sql_null"]++
				continue
			}
			stored, ok := value.(string)
			if !ok {
				return ErrRow
			}
			var opened, expected json.RawMessage
			switch column {
			case "data":
				opened, err = OpenPaymentData(ring, identity, stored)
				expected = source.Raw
			case "context":
				opened, err = OpenPaymentContext(ring, identity.SourceID, stored)
				reviewed, reviewErr := ReviewPaymentContext(source.Raw)
				if reviewErr != nil {
					return ErrRow
				}
				expected = reviewed.JSON
			case "metadata":
				opened, err = OpenPaymentMetadata(ring, identity, stored)
				expected = source.Raw
			}
			if err != nil || !bytes.Equal(opened, expected) {
				return ErrRow
			}
			clear(opened)
			counts[table+"."+column+".sealed_roundtrips"]++
		}
		for _, amount := range result.ContextRemoved {
			counts["context_excluded_fields"] += int64(amount)
		}
		counts[table+".complete_rows"]++
		counts["operation_holds"]++
		return nil
	})
	if err != nil || len(snapshot.Tables) != 159 || counts["operation_holds"] != 42 {
		t.Fatal("full stream or payment row conversion incomplete")
	}
	counts["verified_tables"] = int64(len(snapshot.Tables))
	for _, table := range snapshot.Tables {
		counts["verified_stream_rows"] += table.Rows
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal("v2 relationship evidence unavailable")
	}
	relations, err := VerifyRelationships(file, snapshot.SHA256, RelationLimits{MaxRows: 100000, KeyBytes: 64 << 20})
	if err != ErrReferences || relations.Snapshot.SHA256 != snapshot.SHA256 || len(relations.Relations.PhysicalMissing) != 0 {
		t.Fatal("real snapshot relationship evidence changed")
	}
	for _, missing := range relations.Relations.Missing {
		counts["unresolved_references"] += missing
	}
	if counts["unresolved_references"] != 375 {
		t.Fatal("real snapshot unresolved references changed")
	}
	report, _ := json.Marshal(map[string]any{"counts": counts, "relations": relations.Relations})
	t.Log("PAYMENT_HISTORY_V2_AUDIT " + string(report))
}
