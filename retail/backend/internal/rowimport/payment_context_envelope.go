package rowimport

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/eurorefurb/retail/internal/protected"
)

// SealPaymentContext handles only the non-SQL-NULL payment_session.context
// column. Other payment JSON must still be reviewed; this does not enable a
// full Convert or an active provider session. Text encoding matches the
// existing LONGTEXT envelope target design, never arbitrary binary in UTF-8.
func SealPaymentContext(ring *protected.Ring, sourceID string, raw json.RawMessage) (string, map[string]int, error) {
	if ring == nil {
		return "", nil, ErrProtection
	}
	reviewed, err := ReviewPaymentContext(raw)
	if err != nil {
		return "", nil, err
	}
	envelope, err := ring.Seal(protected.Context{Purpose: "payment-context-history", Table: "payment_sessions", Column: "context", Row: sourceID}, reviewed.JSON)
	if err != nil {
		return "", nil, ErrProtection
	}
	return "ers1." + base64.StdEncoding.EncodeToString(envelope), reviewed.Removed, nil
}

// OpenPaymentContext authenticates the precise field/row and revalidates that
// excluded account-profile content is absent. No legacy/plaintext fallback.
// This internal decryptor grants no public access or new-customer ownership.
func OpenPaymentContext(ring *protected.Ring, sourceID, stored string) (json.RawMessage, error) {
	if ring == nil || !strings.HasPrefix(stored, "ers1.") || len(stored) > 2*protected.MaxPlaintext {
		return nil, ErrProtection
	}
	encoded := strings.TrimPrefix(stored, "ers1.")
	envelope, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(envelope) != encoded {
		return nil, ErrProtection
	}
	plain, err := ring.Open(protected.Context{Purpose: "payment-context-history", Table: "payment_sessions", Column: "context", Row: sourceID}, envelope)
	if err != nil {
		return nil, ErrProtection
	}
	reviewed, err := ReviewPaymentContext(plain)
	if err != nil || len(reviewed.Removed) != 0 {
		clear(plain)
		return nil, ErrHistoryPolicy
	}
	return plain, nil
}
