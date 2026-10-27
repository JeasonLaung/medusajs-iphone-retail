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

func TestPinnedPaymentDataOffline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_PAYMENT_DATA_BUNDLE")
	if path == "" {
		t.Skip("explicit ignored snapshot required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("source payment snapshot missing")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != "aa18ee30d5a7dcb11efe20a5c8785abfdbe561c362db4503160b262df6a2f132" {
		t.Fatal("source payment snapshot hash differs")
	}
	var bundle struct {
		Tables map[string][]map[string]json.RawMessage `json:"tables"`
	}
	if json.Unmarshal(data, &bundle) != nil {
		t.Fatal("source payment snapshot invalid")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary payment key unavailable")
	}
	defer clear(key)
	ring, err := protected.New("offline-payment", map[string][]byte{"offline-payment": key})
	if err != nil {
		t.Fatal("temporary payment ring invalid")
	}
	counts := map[string]int{}
	for _, table := range []string{"payment_session", "payment"} {
		rows, exists := bundle.Tables[table]
		if !exists || len(rows) != 21 {
			t.Fatal("source payment coverage changed")
		}
		for _, row := range rows {
			identity := PaymentDataIdentity{SourceTable: table, Amount: row["amount"]}
			for field, destination := range map[string]*string{"id": &identity.SourceID, "provider_id": &identity.ProviderID, "currency_code": &identity.CurrencyCode} {
				if json.Unmarshal(row[field], destination) != nil {
					t.Fatal("source payment identity invalid")
				}
			}
			identity.SessionID = identity.SourceID
			if table == "payment" && json.Unmarshal(row["payment_session_id"], &identity.SessionID) != nil {
				t.Fatal("source payment session invalid")
			}
			raw, exists := row["data"]
			if !exists || jsonNull(raw) {
				t.Fatal("source data NULL provenance unresolved")
			}
			reviewed, err := ReviewPaymentData(identity, raw)
			if err != nil || !bytes.Equal(reviewed, raw) {
				t.Fatal("real provider evidence needs further review")
			}
			stored, err := SealPaymentData(ring, identity, raw)
			if err != nil {
				t.Fatal("real provider evidence seal failed")
			}
			opened, err := OpenPaymentData(ring, identity, stored)
			if err != nil || !bytes.Equal(opened, raw) {
				t.Fatal("real provider evidence roundtrip differs")
			}
			clear(opened)
			counts[table+".reviewed"]++
			if identity.ProviderID == "pp_system_default" {
				counts[table+".manual_empty"]++
			} else {
				counts[table+".mollie_exact_amount_and_references"]++
			}
			identity.SourceID += "-wrong"
			if table == "payment_session" {
				identity.SessionID = identity.SourceID
			}
			if got, err := OpenPaymentData(ring, identity, stored); err == nil || got != nil {
				t.Fatal("wrong row authentication accepted")
			}
			counts["wrong_row_rejections"]++
		}
	}
	report, _ := json.Marshal(counts)
	t.Log("PAYMENT_DATA_AUDIT " + string(report))
}
