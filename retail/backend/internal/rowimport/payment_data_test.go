package rowimport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func paymentDataFixture() (PaymentDataIdentity, json.RawMessage) {
	identity := PaymentDataIdentity{SourceTable: "payment_session", SourceID: "session-1", SessionID: "session-1", ProviderID: "pp_mollie-hosted-checkout_mollie", CurrencyCode: "eur", Amount: json.RawMessage(`"9007199254740993.123456789012345678"`)}
	raw := json.RawMessage(` {"id":"payment-provider-1","mode":"test","resource":"payment","status":"paid","amount":{"currency":"EUR","value":"9007199254740993.123456789012345678"},"metadata":{"session_id":"session-1","cart_id":"cart-1"},"session_id":"session-1","cart_id":"cart-1","details":{"consumerName":"历史客户","paypalFee":{"currency":"EUR","value":"0.01"}},"billingAddress":{"email":"history@example.invalid"},"_links":{"checkout":{"href":"https://example.invalid/checkout/business-capability","type":"text/html"}},"createdAt":"2026-10-27T00:00:00Z","method":null} `)
	return identity, raw
}

func TestPaymentDataExactPrivateEvidence(t *testing.T) {
	identity, raw := paymentDataFixture()
	original := bytes.Clone(raw)
	reviewed, err := ReviewPaymentData(identity, raw)
	if err != nil || !bytes.Equal(raw, reviewed) || !bytes.Equal(raw, original) {
		t.Fatal("exact provider JSON evidence changed")
	}
	// Equal rationals need not share JSON representations or scales.
	identity.Amount = json.RawMessage(`"9007199254740993123456789012345678e-18"`)
	if _, err := ReviewPaymentData(identity, raw); err != nil {
		t.Fatal("equivalent exact row amount rejected")
	}
	identity.Amount = json.RawMessage(`"9007199254740993.123456789012345679"`)
	if got, err := ReviewPaymentData(identity, raw); err != ErrHistoryPolicy || got != nil {
		t.Fatal("one unit in final decimal place escaped comparison")
	}
}

func TestPaymentDataHoldsUnknownOrConflictingFacts(t *testing.T) {
	identity, raw := paymentDataFixture()
	for _, replacement := range [][2]string{
		{`"mode":"test"`, `"mode":"future"`},
		{`"resource":"payment"`, `"resource":"refund"`},
		{`"status":"paid"`, `"status":"future"`},
		{`"status":"paid"`, `"status":"paid","status":"open"`},
		{`"id":"payment-provider-1"`, `"id":"payment-provider-1","token":"unreviewed"`},
		{`"id":"payment-provider-1"`, `"id":"payment-provider-1","last_refund":{}`},
		{`"value":"9007199254740993.123456789012345678"`, `"value":9007199254740993.123456789012345678`},
		{`"value":"9007199254740993.123456789012345678"`, `"value":"-1"`},
		{`"value":"9007199254740993.123456789012345678"`, `"value":"1.1234567890123456789"`},
		{`"currency":"EUR"`, `"currency":"DKK"`},
		{`"metadata":{"session_id":"session-1"`, `"metadata":{"session_id":"session-other"`},
		{`"metadata":{"session_id":"session-1"`, `"metadata":{"token":"old-login","session_id":"session-1"`},
		{`"cart_id":"cart-1"},"session_id"`, `"cart_id":"cart-other"},"session_id"`},
		{`"paypalFee":{"currency":"EUR","value":"0.01"}`, `"unknown":true`},
		{`"email":"history@example.invalid"`, `"password_hash":"old"`},
		{`https://example.invalid/checkout/business-capability`, `javascript:alert(1)`},
		{`https://example.invalid/checkout/business-capability`, `https://user:password@example.invalid/`},
		{`"createdAt":"2026-10-27T00:00:00Z"`, `"createdAt":"invalid"`},
		{`"method":null`, `"method":"\ud800"`},
	} {
		changed := strings.Replace(string(raw), replacement[0], replacement[1], 1)
		if changed == string(raw) {
			t.Fatal("invalid negative fixture")
		}
		if got, err := ReviewPaymentData(identity, json.RawMessage(changed)); err != ErrHistoryPolicy || got != nil {
			t.Fatal("unreviewed or conflicting data returned evidence")
		}
	}
	for _, raw := range []string{`null`, `[]`, `{}`, `{"id":"missing-required"}`} {
		if got, err := ReviewPaymentData(identity, json.RawMessage(raw)); err != ErrHistoryPolicy || got != nil {
			t.Fatal("invalid source shape accepted")
		}
	}
	for _, change := range []func(*PaymentDataIdentity){
		func(i *PaymentDataIdentity) { i.SourceID = "other" },
		func(i *PaymentDataIdentity) { i.SessionID = "other" },
		func(i *PaymentDataIdentity) { i.SourceTable = "account_holder" },
		func(i *PaymentDataIdentity) { i.ProviderID = "unreviewed-provider" },
		func(i *PaymentDataIdentity) { i.CurrencyCode = "EUR" },
		func(i *PaymentDataIdentity) { i.CurrencyCode = "usd" },
		func(i *PaymentDataIdentity) { i.Amount = json.RawMessage(`"-1"`) },
		func(i *PaymentDataIdentity) { i.Amount = json.RawMessage(`null`) },
	} {
		changed := identity
		change(&changed)
		if got, err := ReviewPaymentData(changed, raw); err != ErrHistoryPolicy || got != nil {
			t.Fatal("invalid source identity accepted")
		}
	}
}

func TestHistoricalManualDataAndPaymentRow(t *testing.T) {
	identity, raw := paymentDataFixture()
	identity.SourceTable = "payment"
	identity.SourceID = "payment-row-1"
	if _, err := ReviewPaymentData(identity, raw); err != nil {
		t.Fatal("payment row references rejected")
	}
	identity.ProviderID = "pp_system_default"
	if _, err := ReviewPaymentData(identity, json.RawMessage(` { } `)); err != nil {
		t.Fatal("empty historical manual evidence rejected")
	}
	if got, err := ReviewPaymentData(identity, raw); err != ErrHistoryPolicy || got != nil {
		t.Fatal("manual provider accepted Mollie evidence")
	}
}

func TestPaymentDataEnvelopeIdentityAndRevalidation(t *testing.T) {
	identity, raw := paymentDataFixture()
	ring, _ := protected.New("test-data", map[string][]byte{"test-data": bytes.Repeat([]byte{37}, 32)})
	stored, err := SealPaymentData(ring, identity, raw)
	if err != nil || strings.Contains(stored, "business-capability") || strings.Contains(stored, "history@example.invalid") {
		t.Fatal("private provider evidence not sealed")
	}
	opened, err := OpenPaymentData(ring, identity, stored)
	if err != nil || !bytes.Equal(opened, raw) {
		t.Fatal("provider envelope roundtrip differs")
	}
	for _, change := range []func(*PaymentDataIdentity){
		func(i *PaymentDataIdentity) { i.SourceID, i.SessionID = "other", "other" },
		func(i *PaymentDataIdentity) { i.SourceTable = "payment" },
		func(i *PaymentDataIdentity) { i.ProviderID = "pp_system_default" },
		func(i *PaymentDataIdentity) { i.Amount = json.RawMessage(`"1"`) },
		func(i *PaymentDataIdentity) { i.CurrencyCode = "usd" },
	} {
		changed := identity
		change(&changed)
		if got, err := OpenPaymentData(ring, changed, stored); err == nil || got != nil {
			t.Fatal("wrong envelope identity/source facts accepted")
		}
	}
	for _, invalid := range []string{string(raw), "ers1.====", stored + "\n", stored[:len(stored)-8]} {
		if got, err := OpenPaymentData(ring, identity, invalid); err == nil || got != nil {
			t.Fatal("invalid envelope accepted")
		}
	}
	if got, err := SealPaymentData(nil, identity, raw); err != ErrProtection || got != "" {
		t.Fatal("missing key accepted")
	}
}
