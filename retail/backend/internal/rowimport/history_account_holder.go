package rowimport

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
)

// HistoricalAccountHolder is private migration evidence. It carries neither a
// customer ownership link nor a grant to reuse payment methods/provider actors.
// Its external_id may name a legacy customer; never match it to a new login.
type HistoricalAccountHolder struct {
	Row        BoundRow
	Operations EffectDisposition
}

type AccountHolderHistory struct{ ring *protected.Ring }

func NewAccountHolderHistory(ring *protected.Ring) (*AccountHolderHistory, error) {
	if ring == nil {
		return nil, ErrProtection
	}
	return &AccountHolderHistory{ring}, nil
}

func accountHolderContext(id, provider, column string) (protected.Context, error) {
	if id == "" || provider != "pp_system_default" || (column != "data" && column != "metadata") {
		return protected.Context{}, ErrHistoryPolicy
	}
	if !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 {
		return protected.Context{}, ErrHistoryPolicy
	}
	return protected.Context{Purpose: "account-holder-history." + provider, Table: "payment_account_holders", Column: column, Row: id}, nil
}

// ReviewAccountHolderJSON accepts only the inspected system-provider baseline.
// data must be an empty object. metadata may additionally be JSON null; SQL
// NULL is handled by complete row validation. Tokens, saved methods, account
// bindings and all unknown nonempty objects require a separate explicit review.
func ReviewAccountHolderJSON(provider, column string, raw json.RawMessage) (json.RawMessage, error) {
	if provider != "pp_system_default" || (column != "data" && column != "metadata") || len(raw) > protected.MaxPlaintext {
		return nil, ErrHistoryPolicy
	}
	if _, err := importvalue.JSONText(raw); err != nil {
		return nil, ErrHistoryPolicy
	}
	if column == "metadata" && jsonNull(raw) {
		return raw, nil
	}
	values, err := object(raw)
	if err != nil || len(values) != 0 {
		return nil, ErrHistoryPolicy
	}
	return raw, nil
}

func sealAccountHolderJSON(ring *protected.Ring, id, provider, column string, raw json.RawMessage) (string, error) {
	aad, err := accountHolderContext(id, provider, column)
	if err != nil || ring == nil {
		return "", ErrProtection
	}
	reviewed, err := ReviewAccountHolderJSON(provider, column, raw)
	if err != nil {
		return "", err
	}
	sealed, err := ring.Seal(aad, reviewed)
	if err != nil {
		return "", ErrProtection
	}
	return "ers1." + base64.StdEncoding.EncodeToString(sealed), nil
}

// Open authenticates provider, table, column and original ID and rechecks the
// baseline. No permissive legacy/plaintext fallback or ownership lookup exists.
func OpenAccountHolderJSON(ring *protected.Ring, id, provider, column, stored string) (json.RawMessage, error) {
	aad, err := accountHolderContext(id, provider, column)
	if err != nil || ring == nil || !strings.HasPrefix(stored, "ers1.") || len(stored) > 2*protected.MaxPlaintext {
		return nil, ErrProtection
	}
	encoded := strings.TrimPrefix(stored, "ers1.")
	sealed, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(sealed) != encoded {
		return nil, ErrProtection
	}
	plain, err := ring.Open(aad, sealed)
	if err != nil {
		return nil, ErrProtection
	}
	if _, err := ReviewAccountHolderJSON(provider, column, plain); err != nil {
		clear(plain)
		return nil, err
	}
	return plain, nil
}

func (h *AccountHolderHistory) Convert(row SourceRow) (HistoricalAccountHolder, error) {
	if h == nil || h.ring == nil {
		return HistoricalAccountHolder{}, ErrProtection
	}
	var id, provider string
	for column, dest := range map[string]*string{"id": &id, "provider_id": &provider} {
		cell, present := row[column]
		if !present || cell.SQLNull {
			return HistoricalAccountHolder{}, ErrRow
		}
		value, err := importvalue.Text(cell.Raw, 191)
		if err != nil || value == "" {
			return HistoricalAccountHolder{}, ErrRow
		}
		*dest = value
	}
	if _, err := accountHolderContext(id, provider, "data"); err != nil {
		return HistoricalAccountHolder{}, err
	}
	bound, err := convertRow("account_holder", row, func(f field, raw json.RawMessage) (any, error) {
		if f.Transform != "protected_exact_json" {
			return nil, ErrProtection
		}
		return sealAccountHolderJSON(h.ring, id, provider, f.Source, raw)
	}, "account-holder-history")
	if err != nil {
		return HistoricalAccountHolder{}, err
	}
	return HistoricalAccountHolder{Row: bound, Operations: EffectDisposition{SourceTable: "account_holder", SourceID: id, Effect: "payment-account-operations", Disposition: "hold", Reason: "legacy-account-holder-reconciliation"}}, nil
}
