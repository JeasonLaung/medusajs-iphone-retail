package rowimport

import (
	"bytes"
	"encoding/json"

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/protected"
)

// These aliases are explicitly read by the installed provider. Multiple
// non-NULL aliases must agree; never silently choose the first conflicting
// merchant or decrypt only the alias currently preferred by JavaScript.
var paypalCredentialAliases = [][]string{
	{"client_id", "clientId"},
	{"client_secret", "clientSecret"},
	{"seller_merchant_id", "sellerMerchantId", "payer_id", "merchant_id", "merchantId"},
	{"seller_email", "sellerEmail"},
}

type paypalCredentialFacts struct {
	values [4][]byte
	set    [4]bool
}

func (f *paypalCredentialFacts) clear() {
	for _, value := range f.values {
		clear(value)
	}
}

// projectPayPalCredentials decrypts each reviewed secret alias before placing
// it in a NEW encrypted metadata document. No old ciphertext is nested in the
// target envelope. Source keys/contact text remain exact; no account is made.
func (c *ConfigSecrets) projectPayPalCredentials(raw json.RawMessage) (json.RawMessage, paypalCredentialFacts, error) {
	values, err := object(raw)
	if err != nil {
		return nil, paypalCredentialFacts{}, ErrProtection
	}
	facts := paypalCredentialFacts{}
	success := false
	defer func() {
		if !success {
			facts.clear()
		}
	}()
	for name, value := range values {
		group := -1
		for index, aliases := range paypalCredentialAliases {
			for _, alias := range aliases {
				if name == alias {
					group = index
				}
			}
		}
		if group < 0 {
			return nil, paypalCredentialFacts{}, ErrProtection
		}
		if jsonNull(value) {
			continue
		}
		text, err := importvalue.Text(value, 0)
		if err != nil {
			return nil, paypalCredentialFacts{}, ErrProtection
		}
		plain := []byte(text)
		if group == 1 {
			plain, err = c.paypal.Open(text)
			if err != nil {
				return nil, paypalCredentialFacts{}, ErrProtection
			}
			encoded, err := json.Marshal(string(plain))
			if err != nil {
				clear(plain)
				return nil, paypalCredentialFacts{}, ErrProtection
			}
			values[name] = encoded
		}
		// Empty strings participate in conflict checks too. SQL NULL / JSON
		// null / absent aliases are not rewritten to empty source strings.
		if facts.set[group] && !bytes.Equal(facts.values[group], plain) {
			clear(plain)
			return nil, paypalCredentialFacts{}, ErrProtection
		}
		if !facts.set[group] {
			facts.values[group] = bytes.Clone(plain)
			facts.set[group] = true
		}
		clear(plain)
	}
	success = true
	return reviewedObject(values), facts, nil
}

func (c *ConfigSecrets) convertPayPalMetadata(raw json.RawMessage, row SourceRow) (json.RawMessage, error) {
	if len(raw) > protected.MaxPlaintext {
		return nil, ErrProtection
	}
	if _, err := importvalue.JSONText(raw); err != nil {
		return nil, ErrProtection
	}
	if jsonNull(raw) {
		return bytes.Clone(raw), nil
	}
	values, err := object(raw)
	if err != nil {
		return nil, ErrProtection
	}
	for name := range values {
		if name != "credentials" && name != "active_environment" {
			return nil, ErrProtection
		}
	}
	if active, exists := values["active_environment"]; exists {
		environment, err := importvalue.Text(row["environment"].Raw, 0)
		selected, ok := paymentString(active)
		if err != nil || row["environment"].SQLNull || !ok || !paymentChoice(active, "live", "sandbox") || selected != environment {
			return nil, ErrProtection
		}
	}
	credentialsRaw, exists := values["credentials"]
	if !exists {
		return bytes.Clone(raw), nil
	}
	credentials, err := object(credentialsRaw)
	if err != nil {
		return nil, ErrProtection
	}
	if len(credentials) == 0 {
		return bytes.Clone(raw), nil
	}
	active, ok := paymentString(values["active_environment"])
	if !ok {
		return nil, ErrProtection
	}
	activeFacts := paypalCredentialFacts{}
	defer activeFacts.clear()
	for environment, document := range credentials {
		if environment != "live" && environment != "sandbox" {
			return nil, ErrProtection
		}
		projected, facts, err := c.projectPayPalCredentials(document)
		if err != nil {
			return nil, ErrProtection
		}
		credentials[environment] = projected
		if environment == active {
			activeFacts = facts
		} else {
			facts.clear()
		}
	}
	// An inactive-only connection may legitimately have no active credentials.
	// Its denormalized columns must then be empty/NULL, never another account.
	for index, column := range []string{"seller_client_id", "seller_client_secret", "seller_merchant_id", "seller_email"} {
		cell, exists := row[column]
		if !exists {
			return nil, ErrProtection
		}
		var plain []byte
		if !cell.SQLNull {
			text, err := importvalue.Text(cell.Raw, 0)
			if err != nil {
				return nil, ErrProtection
			}
			plain = []byte(text)
			if index == 1 {
				plain, err = c.paypal.Open(text)
				if err != nil {
					return nil, ErrProtection
				}
			}
		}
		equal := bytes.Equal(plain, activeFacts.values[index])
		clear(plain)
		if !equal {
			return nil, ErrProtection
		}
	}
	values["credentials"] = reviewedObject(credentials)
	projected := reviewedObject(values)
	if len(projected) > protected.MaxPlaintext {
		clear(projected)
		return nil, ErrProtection
	}
	return projected, nil
}
