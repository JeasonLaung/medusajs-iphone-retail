package uniquekey

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
)

func testPaymentContextSQL(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	// Minimal artificial column with the exact reviewed LONGTEXT definition;
	// this is not a full payment_session row or an imported real session.
	_, err := db.ExecContext(ctx, `CREATE TABLE fixture_payment_context_envelopes (
	 id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
	 context LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL) ENGINE=InnoDB`)
	if err != nil {
		t.Fatal("context fixture DDL unavailable")
	}
	ring, _ := protected.New("fixture-context", map[string][]byte{"fixture-context": bytes.Repeat([]byte{15}, 32)})
	raw := json.RawMessage(`{"customer":{"id":"旧ID","email":" historical@example.invalid ","addresses":[{"token":"excluded-old-capability"}],"billing_address":{"address_1":" Historic address ","is_default_billing":true}}}`)
	stored, _, err := rowimport.SealPaymentContext(ring, "historical-session", raw)
	if err != nil {
		t.Fatal("context sealing failed")
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO fixture_payment_context_envelopes(id,context) VALUES(?,?),(?,NULL)", "historical-session", stored, "null-session"); err != nil {
		t.Fatal("context ASCII envelope insert failed")
	}
	var value string
	if err := db.QueryRowContext(ctx, "SELECT context FROM fixture_payment_context_envelopes WHERE id='historical-session'").Scan(&value); err != nil || value != stored {
		t.Fatal("context envelope changed in LONGTEXT")
	}
	plain, err := rowimport.OpenPaymentContext(ring, "historical-session", value)
	if err != nil || bytes.Contains(plain, []byte("excluded-old-capability")) || !bytes.Contains(plain, []byte(" Historic address ")) {
		t.Fatal("persisted context review failed")
	}
	var null sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT context FROM fixture_payment_context_envelopes WHERE id='null-session'").Scan(&null); err != nil || null.Valid {
		t.Fatal("SQL NULL became context envelope")
	}
	if _, err := rowimport.OpenPaymentContext(ring, "other-session", value); err != rowimport.ErrProtection {
		t.Fatal("persisted context row not bound")
	}
}
