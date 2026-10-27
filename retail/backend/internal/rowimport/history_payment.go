package rowimport

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
)

// HistoricalPayment is migration material, not a live payment command or DTO.
// A future SQL importer must persist Operations with Row atomically and enforce
// the hold before making imported rows available to any payment worker.
type HistoricalPayment struct {
	Row            BoundRow
	Operations     EffectDisposition
	ContextRemoved map[string]int
}

type PaymentHistory struct{ ring *protected.Ring }

func NewPaymentHistory(ring *protected.Ring) (*PaymentHistory, error) {
	if ring == nil {
		return nil, ErrProtection
	}
	return &PaymentHistory{ring: ring}, nil
}

func paymentRowIdentity(table string, row SourceRow) (PaymentDataIdentity, error) {
	identity := PaymentDataIdentity{SourceTable: table}
	for source, destination := range map[string]*string{
		"id": &identity.SourceID, "provider_id": &identity.ProviderID, "currency_code": &identity.CurrencyCode,
	} {
		cell, exists := row[source]
		if !exists || cell.SQLNull {
			return PaymentDataIdentity{}, ErrRow
		}
		value, err := importvalue.Text(cell.Raw, 191)
		if err != nil || value == "" {
			return PaymentDataIdentity{}, ErrRow
		}
		*destination = value
	}
	identity.SessionID = identity.SourceID
	if table == "payment" {
		cell, exists := row["payment_session_id"]
		if !exists || cell.SQLNull {
			return PaymentDataIdentity{}, ErrRow
		}
		var err error
		identity.SessionID, err = importvalue.Text(cell.Raw, 191)
		if err != nil {
			return PaymentDataIdentity{}, ErrRow
		}
	}
	amount, exists := row["amount"]
	if !exists || amount.SQLNull {
		return PaymentDataIdentity{}, ErrRow
	}
	identity.Amount = amount.Raw
	if _, err := paymentDataContext(identity); err != nil {
		return PaymentDataIdentity{}, err
	}
	// Also validate row facts when nullable data is SQL NULL. This does not
	// synthesize provider data or imply remote reconciliation.
	if _, err := importvalue.Decimal38_18(identity.Amount); err != nil || !paymentCurrency.MatchString(strings.ToUpper(identity.CurrencyCode)) || identity.CurrencyCode != strings.ToLower(identity.CurrencyCode) {
		return PaymentDataIdentity{}, ErrRow
	}
	return identity, nil
}

func paymentMetadataContext(identity PaymentDataIdentity) (protected.Context, error) {
	context, err := paymentDataContext(identity)
	if err != nil {
		return protected.Context{}, err
	}
	context.Purpose = "payment-metadata-history." + identity.ProviderID
	context.Column = "metadata"
	return context, nil
}

// The pinned source contains empty metadata/SQL NULL only. Do not permit
// arbitrary extensions to smuggle excluded authentication material into rows.
// JSON null is a supported JSON document, distinct from SQL NULL handled above.
func sealPaymentMetadata(ring *protected.Ring, identity PaymentDataIdentity, raw json.RawMessage) (string, error) {
	if len(raw) > protected.MaxPlaintext {
		return "", ErrHistoryPolicy
	}
	if _, err := importvalue.JSONText(raw); err != nil || !emptyMetadata(raw) {
		return "", ErrHistoryPolicy
	}
	context, err := paymentMetadataContext(identity)
	if err != nil {
		return "", err
	}
	envelope, err := ring.Seal(context, raw)
	if err != nil {
		return "", ErrProtection
	}
	return "ers1." + base64.StdEncoding.EncodeToString(envelope), nil
}

// OpenPaymentMetadata authenticates a reviewed historical metadata document.
// No plaintext fallback, arbitrary extension schema or public access is added.
func OpenPaymentMetadata(ring *protected.Ring, identity PaymentDataIdentity, stored string) (json.RawMessage, error) {
	context, err := paymentMetadataContext(identity)
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
	if _, err := importvalue.JSONText(plain); err != nil || !emptyMetadata(plain) {
		clear(plain)
		return nil, ErrHistoryPolicy
	}
	return plain, nil
}

// Convert validates the ENTIRE row (including NULL provenance, raw money pairs,
// enum/check rules, missing/unknown columns) and protects each reviewed JSON.
// Any error returns no partial row, disposition or redaction evidence.
func (p *PaymentHistory) Convert(table string, row SourceRow) (HistoricalPayment, error) {
	if p == nil || p.ring == nil {
		return HistoricalPayment{}, ErrProtection
	}
	identity, err := paymentRowIdentity(table, row)
	if err != nil {
		return HistoricalPayment{}, err
	}
	removed := map[string]int{}
	bound, err := convertRow(table, row, func(f field, raw json.RawMessage) (any, error) {
		if f.Transform != "protected_exact_json" {
			return nil, ErrProtection
		}
		switch f.Source {
		case "data":
			return SealPaymentData(p.ring, identity, raw)
		case "context":
			if table != "payment_session" {
				return nil, ErrProtection
			}
			stored, projection, err := SealPaymentContext(p.ring, identity.SourceID, raw)
			if err != nil {
				return nil, err
			}
			removed = projection
			return stored, nil
		case "metadata":
			return sealPaymentMetadata(p.ring, identity, raw)
		default:
			return nil, ErrProtection
		}
	}, "payment-history")
	if err != nil {
		return HistoricalPayment{}, err
	}
	return HistoricalPayment{Row: bound, ContextRemoved: removed,
		Operations: EffectDisposition{SourceTable: table, SourceID: identity.SourceID, Effect: "payment-operations", Disposition: "hold", Reason: "legacy-payment-reconciliation"}}, nil
}
