package rowimport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func TestPaymentContextExcludesOldProfileBindings(t *testing.T) {
	raw := json.RawMessage(` {"customer":{"id":"旧ID ","email":" HISTORICAL@example.invalid ","metadata":null,"addresses":[{"token":"excluded-address-capability"}],"account_holders":[{"data":{"token":"excluded-account-binding"}}],"billing_address":{"id":"billing-history","customer_id":"旧ID ","address_1":" Historic address ","metadata":{},"is_default_billing":true,"is_default_shipping":false}},"account_holder": {"id":"provider-holder", "data":{}, "metadata":null}} `)
	original := bytes.Clone(raw)
	result, err := ReviewPaymentContext(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{"excluded-address-capability", "excluded-account-binding", "addresses", "account_holders", "is_default_billing", "is_default_shipping"} {
		if bytes.Contains(result.JSON, []byte(excluded)) {
			t.Fatal("excluded profile material retained")
		}
	}
	for _, preserved := range []string{`"旧ID "`, `" HISTORICAL@example.invalid "`, `" Historic address "`, `{"id":"provider-holder", "data":{}, "metadata":null}`} {
		if !bytes.Contains(result.JSON, []byte(preserved)) {
			t.Fatal("reviewed historical tokens changed")
		}
	}
	if len(result.Removed) != 4 || !bytes.Equal(raw, original) {
		t.Fatal("removal evidence or source mutation mismatch")
	}
}

func TestUnknownPaymentContextCapabilitiesAreHeld(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `{"token":"old-login"}`, `{"customer":{"password_hash":"old"}}`,
		`{"customer":{"metadata":{"verified":true}}}`, `{"customer":{"metadata":{"token":"old"}}}`,
		`{"customer":{"addresses":{}}}`, `{"customer":{"account_holders":"bad"}}`,
		`{"customer":{"billing_address":{"is_default_billing":1}}}`,
		`{"customer":{"billing_address":{"metadata":{"old_grant":true}}}}`,
		`{"account_holder":{"data":{"unreviewed_secret":"value"}}}`,
		`{"account_holder":{"auth_identity_id":"old"}}`,
		`{"customer":{"id":"\ud800"}}`, `{"customer":{"id":"a","id":"b"}}`,
		`{"customer":{},"customer":{}}`, `{"customer":{"billing_address":{"unknown":true}}}`,
		`{"account_holder":{"created_at":"invalid"}}`,
	} {
		result, err := ReviewPaymentContext(json.RawMessage(raw))
		if err != ErrHistoryPolicy || result.JSON != nil || result.Removed != nil {
			t.Fatal("unknown context returned partial review")
		}
	}
	if _, err := ReviewPaymentContext(json.RawMessage(strings.Repeat(" ", (1<<20)+1))); err != ErrHistoryPolicy {
		t.Fatal("context size bound ignored")
	}
}

func TestEmptyAndNullableReviewedContextShapes(t *testing.T) {
	for _, raw := range []string{`{}`, `{"customer":null}`, `{"customer":{"metadata":{},"addresses":null,"account_holders":[]}}`, `{"account_holder":null}`} {
		if _, err := ReviewPaymentContext(json.RawMessage(raw)); err != nil {
			t.Fatal("supported context shape rejected")
		}
	}
}

func TestReviewedPaymentContextEnvelopeAndNoBypass(t *testing.T) {
	ring, _ := protected.New("test-context", map[string][]byte{"test-context": bytes.Repeat([]byte{19}, 32)})
	raw := json.RawMessage(`{"customer":{"id":"historical-id","addresses":[{"token":"old-login"}],"account_holders":[]}}`)
	stored, removed, err := SealPaymentContext(ring, "old-session", raw)
	if err != nil || len(removed) != 2 || strings.Contains(stored, "historical-id") || strings.Contains(stored, "old-login") {
		t.Fatal("context envelope failed")
	}
	opened, err := OpenPaymentContext(ring, "old-session", stored)
	if err != nil || !bytes.Equal(opened, []byte(`{"customer":{"id":"historical-id"}}`)) {
		t.Fatal("reviewed context roundtrip differs")
	}
	if _, err := OpenPaymentContext(ring, "another-session", stored); err != ErrProtection {
		t.Fatal("context row not authenticated")
	}
	for _, bad := range []string{string(raw), strings.TrimPrefix(stored, "ers1."), stored + "\n", "ers1."} {
		if _, err := OpenPaymentContext(ring, "old-session", bad); err != ErrProtection {
			t.Fatal("text envelope fallback accepted")
		}
	}
	context := protected.Context{Purpose: "payment-context-history", Table: "payment_sessions", Column: "context", Row: "old-session"}
	bad, _ := ring.Seal(context, raw)
	if _, err := OpenPaymentContext(ring, "old-session", "ers1."+base64.StdEncoding.EncodeToString(bad)); err != ErrHistoryPolicy {
		t.Fatal("sealed excluded profile passed content policy")
	}
	if _, _, err := SealPaymentContext(nil, "old-session", raw); err != ErrProtection {
		t.Fatal("missing ring accepted")
	}
}
