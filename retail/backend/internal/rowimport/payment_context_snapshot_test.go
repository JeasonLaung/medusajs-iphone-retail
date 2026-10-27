package rowimport

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func TestPinnedPaymentContextOffline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_PAYMENT_CONTEXT_BUNDLE")
	if path == "" {
		t.Skip("explicit ignored snapshot required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("source context snapshot missing")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != "aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132" {
		t.Fatal("source context snapshot hash differs")
	}
	var bundle struct {
		Tables map[string][]map[string]json.RawMessage `json:"tables"`
	}
	if json.Unmarshal(data, &bundle) != nil {
		t.Fatal("source context snapshot invalid")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary context key unavailable")
	}
	defer clear(key)
	ring, err := protected.New("offline-context", map[string][]byte{"offline-context": key})
	if err != nil {
		t.Fatal("temporary context ring invalid")
	}
	rows, exists := bundle.Tables["payment_session"]
	if !exists {
		t.Fatal("source session table missing")
	}
	removed := map[string]int{}
	modified, roundtrips := 0, 0
	for _, row := range rows {
		// These source cells are NON-NULL JSON objects. v1 null would be
		// ambiguous and is expressly held, never assumed to be SQL NULL.
		raw, exists := row["context"]
		if !exists || jsonNull(raw) {
			t.Fatal("source context NULL provenance unresolved")
		}
		reviewed, err := ReviewPaymentContext(raw)
		if err != nil {
			t.Fatal("real payment context needs further review")
		}
		if len(reviewed.Removed) != 0 {
			modified++
		}
		for name, count := range reviewed.Removed {
			removed[name] += count
		}
		var id string
		if json.Unmarshal(row["id"], &id) != nil || id == "" {
			t.Fatal("source context identity invalid")
		}
		stored, _, err := SealPaymentContext(ring, id, raw)
		if err != nil {
			t.Fatal("real context seal failed")
		}
		opened, err := OpenPaymentContext(ring, id, stored)
		if err != nil || !bytes.Equal(opened, reviewed.JSON) {
			t.Fatal("real context roundtrip failed")
		}
		clear(opened)
		roundtrips++
	}
	report, _ := json.Marshal(map[string]any{"source_contexts": len(rows), "contexts_with_excluded_profile": modified, "removed_fields": removed, "ephemeral_envelope_roundtrips": roundtrips})
	t.Log("PAYMENT_CONTEXT_AUDIT " + string(report))
}
