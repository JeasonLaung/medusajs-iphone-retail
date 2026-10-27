package rowimport

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
)

// PaymentContextReview is private migration material, never a public DTO or a
// logging value. JSON is a deliberately projected historical payment snapshot,
// not an account profile, address book, login grant or complete payment import.
type PaymentContextReview struct {
	JSON    json.RawMessage
	Removed map[string]int
}

// Emit sorted reviewed object names while retaining exact value JSON tokens.
// json.Marshal(map[string]RawMessage) compacts values, so do not use it here.
func reviewedObject(values map[string]json.RawMessage) json.RawMessage {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	out.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(name)
		out.Write(key)
		out.WriteByte(':')
		out.Write(values[name])
	}
	out.WriteByte('}')
	return out.Bytes()
}

func jsonNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func emptyMetadata(raw json.RawMessage) bool {
	if jsonNull(raw) {
		return true
	}
	values, err := object(raw)
	return err == nil && len(values) == 0
}

func nullableText(raw json.RawMessage, max int) bool {
	if jsonNull(raw) {
		return true
	}
	_, err := importvalue.Text(raw, max)
	return err == nil
}

func historicalBillingAddress(raw json.RawMessage, removed map[string]int) (json.RawMessage, error) {
	if jsonNull(raw) {
		return raw, nil
	}
	values, err := object(raw)
	if err != nil {
		return nil, ErrHistoryPolicy
	}
	for name, value := range values {
		switch name {
		case "id", "customer_id":
			if !nullableText(value, 191) {
				return nil, ErrHistoryPolicy
			}
		case "address_1", "address_2", "address_name", "city", "company", "country_code", "first_name", "last_name", "phone", "postal_code", "province":
			if !nullableText(value, 0) {
				return nil, ErrHistoryPolicy
			}
		case "created_at", "updated_at", "deleted_at":
			if !jsonNull(value) {
				if _, err := importvalue.Instant(value); err != nil {
					return nil, ErrHistoryPolicy
				}
			}
		case "metadata":
			if !emptyMetadata(value) {
				return nil, ErrHistoryPolicy
			}
		case "is_default_billing", "is_default_shipping":
			if _, err := importvalue.Boolean(value); err != nil {
				return nil, ErrHistoryPolicy
			}
			delete(values, name)
			removed["customer.billing_address."+name]++
		default:
			return nil, ErrHistoryPolicy
		}
	}
	return reviewedObject(values), nil
}

func historicalPaymentCustomer(raw json.RawMessage, removed map[string]int) (json.RawMessage, error) {
	if jsonNull(raw) {
		return raw, nil
	}
	values, err := object(raw)
	if err != nil {
		return nil, ErrHistoryPolicy
	}
	for name, value := range values {
		switch name {
		case "id":
			if !nullableText(value, 191) {
				return nil, ErrHistoryPolicy
			}
		case "company_name", "email", "first_name", "last_name", "phone":
			if !nullableText(value, 0) {
				return nil, ErrHistoryPolicy
			}
		case "metadata":
			if !emptyMetadata(value) {
				return nil, ErrHistoryPolicy
			}
		case "addresses", "account_holders":
			if !jsonNull(value) {
				trim := bytes.TrimSpace(value)
				if len(trim) == 0 || trim[0] != '[' {
					return nil, ErrHistoryPolicy
				}
			}
			delete(values, name)
			removed["customer."+name]++
		case "billing_address":
			values[name], err = historicalBillingAddress(value, removed)
			if err != nil {
				return nil, err
			}
		default:
			return nil, ErrHistoryPolicy
		}
	}
	return reviewedObject(values), nil
}

func historicalPaymentAccountHolder(raw json.RawMessage) (json.RawMessage, error) {
	if jsonNull(raw) {
		return raw, nil
	}
	values, err := object(raw)
	if err != nil {
		return nil, ErrHistoryPolicy
	}
	for name, value := range values {
		switch name {
		case "id", "external_id", "provider_id":
			if !nullableText(value, 191) {
				return nil, ErrHistoryPolicy
			}
		case "email":
			if !nullableText(value, 0) {
				return nil, ErrHistoryPolicy
			}
		case "data", "metadata":
			if !emptyMetadata(value) {
				return nil, ErrHistoryPolicy
			}
		case "created_at", "updated_at", "deleted_at":
			if !jsonNull(value) {
				if _, err := importvalue.Instant(value); err != nil {
					return nil, ErrHistoryPolicy
				}
			}
		default:
			return nil, ErrHistoryPolicy
		}
	}
	// This is the transaction's provider holder, not the excluded customer to
	// holder account-link array. Keep its reviewed original lexical form.
	return raw, nil
}

// ReviewPaymentContext removes excluded customer address-book/account bindings
// and default-profile flags while retaining reviewed transaction evidence.
// Unknown keys, nonempty metadata/provider holder payloads and invalid types
// are explicit holds, not recursive token-name guessing or blind sealing.
// SQL NULL handling belongs to the cell caller; JSON null is rejected here.
func ReviewPaymentContext(raw json.RawMessage) (PaymentContextReview, error) {
	if len(raw) > protected.MaxPlaintext {
		return PaymentContextReview{}, ErrHistoryPolicy
	}
	if _, err := importvalue.JSONText(raw); err != nil {
		return PaymentContextReview{}, ErrHistoryPolicy
	}
	values, err := object(raw)
	if err != nil {
		return PaymentContextReview{}, ErrHistoryPolicy
	}
	removed := map[string]int{}
	for name, value := range values {
		switch name {
		case "customer":
			values[name], err = historicalPaymentCustomer(value, removed)
		case "account_holder":
			values[name], err = historicalPaymentAccountHolder(value)
		default:
			return PaymentContextReview{}, ErrHistoryPolicy
		}
		if err != nil {
			return PaymentContextReview{}, ErrHistoryPolicy
		}
	}
	return PaymentContextReview{JSON: reviewedObject(values), Removed: removed}, nil
}
