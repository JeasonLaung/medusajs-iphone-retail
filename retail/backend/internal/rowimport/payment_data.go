package rowimport

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
)

// PaymentDataIdentity contains private migration facts, never a customer DTO.
// Amount is the source scalar in major currency units, without float decoding.
// This component does not import a complete row or enable provider operations.
type PaymentDataIdentity struct {
	SourceTable, SourceID, ProviderID, SessionID, CurrencyCode string
	Amount                                                     json.RawMessage
}

var paymentCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func paymentString(raw json.RawMessage) (string, bool) {
	s, err := importvalue.Text(raw, 0)
	return s, err == nil && s != ""
}

func paymentChoice(raw json.RawMessage, choices ...string) bool {
	s, ok := paymentString(raw)
	if !ok {
		return false
	}
	for _, choice := range choices {
		if s == choice {
			return true
		}
	}
	return false
}

func paymentMoney(raw json.RawMessage) (string, *big.Rat, bool) {
	values, err := object(raw)
	if err != nil || len(values) != 2 {
		return "", nil, false
	}
	currency, ok := paymentString(values["currency"])
	if !ok || !paymentCurrency.MatchString(currency) {
		return "", nil, false
	}
	// Provider amount.value must be a JSON string. Never round a float or
	// silently accept precision outside the target DECIMAL contract.
	text, ok := paymentString(values["value"])
	if !ok {
		return "", nil, false
	}
	encoded, _ := json.Marshal(text)
	decimal, err := importvalue.Decimal38_18(encoded)
	if err != nil {
		return "", nil, false
	}
	value, ok := new(big.Rat).SetString(decimal)
	if !ok || value.Sign() < 0 {
		return "", nil, false
	}
	return currency, value, true
}

func paymentURL(raw json.RawMessage) bool {
	s, ok := paymentString(raw)
	if !ok || strings.ContainsAny(s, "\r\n\t") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}

func paymentLinks(raw json.RawMessage) bool {
	values, err := object(raw)
	if err != nil {
		return false
	}
	for name, value := range values {
		switch name {
		case "self", "checkout", "dashboard", "documentation", "changePaymentState":
		default:
			return false
		}
		link, err := object(value)
		if err != nil || len(link) != 2 || !paymentURL(link["href"]) {
			return false
		}
		if _, ok := paymentString(link["type"]); !ok {
			return false
		}
	}
	return true
}

func paymentDetails(raw json.RawMessage) bool {
	values, err := object(raw)
	if err != nil {
		return false
	}
	for name, value := range values {
		switch name {
		case "consumerAccount", "consumerName", "paypalPayerId", "paypalReference", "sellerProtection":
			if !nullableText(value, 0) {
				return false
			}
		case "paypalFee":
			if _, _, ok := paymentMoney(value); !ok {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func paymentDataContext(identity PaymentDataIdentity) (protected.Context, error) {
	target := ""
	switch identity.SourceTable {
	case "payment_session":
		target = "payment_sessions"
		if identity.SessionID != identity.SourceID {
			return protected.Context{}, ErrHistoryPolicy
		}
	case "payment":
		target = "payments"
	default:
		return protected.Context{}, ErrHistoryPolicy
	}
	for _, id := range []string{identity.SourceID, identity.SessionID} {
		if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 {
			return protected.Context{}, ErrHistoryPolicy
		}
	}
	if identity.ProviderID != "pp_system_default" && identity.ProviderID != "pp_mollie-hosted-checkout_mollie" {
		return protected.Context{}, ErrHistoryPolicy
	}
	return protected.Context{Purpose: "payment-data-history." + identity.ProviderID, Table: target, Column: "data", Row: identity.SourceID}, nil
}

// ReviewPaymentData accepts only reviewed provider shapes and checks the
// embedded session, currency and exact amount against source row facts.
// It preserves original JSON bytes, including transaction contact details and
// checkout links as encrypted historical evidence. No links are followed and
// no login, ownership, capture, refund or resumable payment grant is created.
func ReviewPaymentData(identity PaymentDataIdentity, raw json.RawMessage) (json.RawMessage, error) {
	if _, err := paymentDataContext(identity); err != nil || len(raw) > protected.MaxPlaintext {
		return nil, ErrHistoryPolicy
	}
	if _, err := importvalue.JSONText(raw); err != nil {
		return nil, ErrHistoryPolicy
	}
	values, err := object(raw)
	if err != nil {
		return nil, ErrHistoryPolicy
	}
	decimal, err := importvalue.Decimal38_18(identity.Amount)
	rowAmount, ok := new(big.Rat).SetString(decimal)
	if err != nil || !ok || rowAmount.Sign() < 0 || !paymentCurrency.MatchString(strings.ToUpper(identity.CurrencyCode)) || identity.CurrencyCode != strings.ToLower(identity.CurrencyCode) {
		return nil, ErrHistoryPolicy
	}
	if identity.ProviderID == "pp_system_default" {
		if len(values) != 0 {
			return nil, ErrHistoryPolicy
		}
		return bytes.Clone(raw), nil
	}
	for _, name := range []string{"id", "mode", "resource", "status", "amount", "metadata", "session_id", "cart_id"} {
		if _, exists := values[name]; !exists {
			return nil, ErrHistoryPolicy
		}
	}
	session, sessionOK := paymentString(values["session_id"])
	cart, cartOK := paymentString(values["cart_id"])
	metadata, metaErr := object(values["metadata"])
	metaSession, msOK := paymentString(metadata["session_id"])
	metaCart, mcOK := paymentString(metadata["cart_id"])
	currency, amount, moneyOK := paymentMoney(values["amount"])
	if !sessionOK || !cartOK || metaErr != nil || len(metadata) != 2 || !msOK || !mcOK || session != identity.SessionID || session != metaSession || cart != metaCart || !moneyOK || currency != strings.ToUpper(identity.CurrencyCode) || amount.Cmp(rowAmount) != 0 {
		return nil, ErrHistoryPolicy
	}
	for name, value := range values {
		switch name {
		case "amount", "metadata", "session_id", "cart_id": // checked together above
		case "id", "profileId", "description":
			if _, ok := paymentString(value); !ok {
				return nil, ErrHistoryPolicy
			}
		case "mode":
			if !paymentChoice(value, "live", "test") {
				return nil, ErrHistoryPolicy
			}
		case "resource":
			if !paymentChoice(value, "payment") {
				return nil, ErrHistoryPolicy
			}
		case "status":
			if !paymentChoice(value, "open", "pending", "authorized", "paid", "canceled", "expired", "failed") {
				return nil, ErrHistoryPolicy
			}
		case "sequenceType":
			if !paymentChoice(value, "oneoff", "first", "recurring") {
				return nil, ErrHistoryPolicy
			}
		case "method", "locale", "countryCode":
			if !nullableText(value, 0) {
				return nil, ErrHistoryPolicy
			}
		case "createdAt", "expiresAt", "paidAt":
			if _, err := importvalue.Instant(value); err != nil {
				return nil, ErrHistoryPolicy
			}
		case "webhookUrl", "redirectUrl", "redirect_url":
			if !paymentURL(value) {
				return nil, ErrHistoryPolicy
			}
		case "isCancelable":
			if _, err := importvalue.Boolean(value); err != nil {
				return nil, ErrHistoryPolicy
			}
		case "_links":
			if !paymentLinks(value) {
				return nil, ErrHistoryPolicy
			}
		case "details":
			if !paymentDetails(value) {
				return nil, ErrHistoryPolicy
			}
		case "amountRefunded", "amountRemaining":
			otherCurrency, _, ok := paymentMoney(value)
			if !ok || otherCurrency != currency {
				return nil, ErrHistoryPolicy
			}
		case "billingAddress":
			address, err := object(value)
			if err != nil {
				return nil, ErrHistoryPolicy
			}
			for key, field := range address {
				if (key != "email" && key != "givenName" && key != "familyName") || !nullableText(field, 0) {
					return nil, ErrHistoryPolicy
				}
			}
		default:
			return nil, ErrHistoryPolicy
		}
	}
	return bytes.Clone(raw), nil
}

// SealPaymentData creates a text-safe envelope for the reviewed LONGTEXT data
// column. SQL NULL is handled separately by the future full-row importer.
func SealPaymentData(ring *protected.Ring, identity PaymentDataIdentity, raw json.RawMessage) (string, error) {
	if ring == nil {
		return "", ErrProtection
	}
	reviewed, err := ReviewPaymentData(identity, raw)
	if err != nil {
		return "", err
	}
	defer clear(reviewed)
	context, _ := paymentDataContext(identity)
	envelope, err := ring.Seal(context, reviewed)
	if err != nil {
		return "", ErrProtection
	}
	return "ers1." + base64.StdEncoding.EncodeToString(envelope), nil
}

// OpenPaymentData authenticates provider/table/field/row and rechecks source
// facts. It is internal migration evidence access, never a public read API.
func OpenPaymentData(ring *protected.Ring, identity PaymentDataIdentity, stored string) (json.RawMessage, error) {
	context, err := paymentDataContext(identity)
	if err != nil || ring == nil || !strings.HasPrefix(stored, "ers1.") || len(stored) > 2*protected.MaxPlaintext {
		return nil, ErrProtection
	}
	encoded := strings.TrimPrefix(stored, "ers1.")
	envelope, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(envelope) != encoded {
		return nil, ErrProtection
	}
	plain, err := ring.Open(context, envelope)
	if err != nil {
		return nil, ErrProtection
	}
	defer clear(plain)
	return ReviewPaymentData(identity, plain)
}
