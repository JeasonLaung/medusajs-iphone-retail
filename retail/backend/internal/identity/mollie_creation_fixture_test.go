package identity_test

import (
	"encoding/json"
	"github.com/eurorefurb/retail/internal/payments"
	"strings"
)

// Reflect the real adapter contract: unique remote identity, exact settlement
// amount/currency and original cart/session metadata, even in isolated tests.
func ownedMollieCreation(r payments.SessionRequest, prefix, checkout string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"id": prefix + "_" + strings.ReplaceAll(r.SessionID, "-", ""), "status": "open", "private": "hidden",
		"amount":   map[string]string{"value": r.Amount, "currency": "EUR"},
		"metadata": map[string]string{"session_id": r.SessionID, "cart_id": inScope(r)},
		"_links":   map[string]any{"checkout": map[string]string{"href": checkout}},
	})
	return raw
}
