package rowimport

import (
	"encoding/json"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
)

// HistoricalCustomer carries business-subject evidence only. The legacy account
// flag is an observed fact, never authority to create a login or claim an order.
// A future importer MUST persist the binding disposition with the row and must
// not infer ownership through email, phone, original ID or legacy_had_account.
type HistoricalCustomer struct {
	Row     BoundRow
	Binding EffectDisposition
}

type CustomerHistory struct{ ring *protected.Ring }

func NewCustomerHistory(ring *protected.Ring) (*CustomerHistory, error) {
	if ring == nil {
		return nil, ErrProtection
	}
	return &CustomerHistory{ring: ring}, nil
}

func customerHistoryContext(id string) (protected.Context, error) {
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 {
		return protected.Context{}, ErrHistoryPolicy
	}
	return protected.Context{Purpose: "customer-subject-history", Table: "historical_customer_subjects", Column: "metadata", Row: id}, nil
}

// ReviewCustomerMetadata accepts the inspected baseline only. Unknown fields
// need an explicit separate policy: encrypting arbitrary JSON would preserve
// old credentials or embedded account grants rather than remove them.
func ReviewCustomerMetadata(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > protected.MaxPlaintext {
		return nil, ErrHistoryPolicy
	}
	if _, err := importvalue.JSONText(raw); err != nil {
		return nil, ErrHistoryPolicy
	}
	if jsonNull(raw) {
		return raw, nil
	}
	values, err := object(raw)
	if err != nil || len(values) != 0 {
		return nil, ErrHistoryPolicy
	}
	return raw, nil
}

// OpenCustomerMetadata authenticates the exact historical subject and rechecks
// the reviewed baseline. SQL NULL is handled by the storage layer, not decoded
// as an empty ciphertext. No plaintext/legacy-key fallback is supported.
func OpenCustomerMetadata(ring *protected.Ring, id string, sealed []byte) (json.RawMessage, error) {
	aad, err := customerHistoryContext(id)
	if err != nil || ring == nil {
		return nil, ErrProtection
	}
	plain, err := ring.Open(aad, sealed)
	if err != nil {
		return nil, ErrProtection
	}
	if _, err := ReviewCustomerMetadata(plain); err != nil {
		clear(plain)
		return nil, err
	}
	return plain, nil
}

func (h *CustomerHistory) Convert(row SourceRow) (HistoricalCustomer, error) {
	if h == nil || h.ring == nil {
		return HistoricalCustomer{}, ErrProtection
	}
	cell, ok := row["id"]
	if !ok || cell.SQLNull {
		return HistoricalCustomer{}, ErrRow
	}
	id, err := importvalue.Text(cell.Raw, 191)
	if err != nil {
		return HistoricalCustomer{}, ErrRow
	}
	aad, err := customerHistoryContext(id)
	if err != nil {
		return HistoricalCustomer{}, err
	}
	bound, err := convertRow("customer", row, func(f field, raw json.RawMessage) (any, error) {
		if f.Source != "metadata" || f.Transform != "protected_exact_json" {
			return nil, ErrProtection
		}
		reviewed, err := ReviewCustomerMetadata(raw)
		if err != nil {
			return nil, err
		}
		sealed, err := h.ring.Seal(aad, reviewed)
		if err != nil {
			return nil, ErrProtection
		}
		return sealed, nil
	}, "customer-subject-history")
	if err != nil {
		return HistoricalCustomer{}, err
	}
	return HistoricalCustomer{Row: bound, Binding: EffectDisposition{SourceTable: "customer", SourceID: id,
		Effect: "customer-account-binding", Disposition: "excluded", Reason: "historical-subject-not-login"}}, nil
}
