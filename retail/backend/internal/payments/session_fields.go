package payments

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/eurorefurb/retail/internal/protected"
)

func sessionFieldContext(in SessionRequest, column string) protected.Context {
	identity, _ := json.Marshal([]string{in.SessionID, in.ProviderID, in.CollectionID, in.Amount, in.Currency})
	digest := sha256.Sum256(identity)
	return protected.Context{Purpose: "payment-session-runtime-v1", Table: "payment_sessions", Column: column, Row: hex.EncodeToString(digest[:])}
}

// SealSessionFields protects NEW runtime session data/context with explicit
// business keys. Amount normalization and JSON validation match stored requests.
// NULL context stays SQL NULL. Never apply this function to legacy plaintext or
// historic envelopes as an implicit conversion; historical sessions are held.
func SealSessionFields(ring *protected.Ring, in SessionRequest) (string, *string, error) {
	key := EffectKey{ResourceID: in.SessionID}
	raw, err := encodeSessionRequest(key, in)
	if err != nil {
		return "", nil, err
	}
	defer clear(raw)
	var normalized storedSessionRequest
	if json.Unmarshal(raw, &normalized) != nil {
		return "", nil, ErrEffectInput
	}
	in = normalized.Request
	seal := func(column string, value []byte) (string, error) {
		envelope, err := ring.Seal(sessionFieldContext(in, column), value)
		if err != nil {
			return "", ErrEffectProtection
		}
		return "ers1." + base64.StdEncoding.EncodeToString(envelope), nil
	}
	data, err := seal("data", in.Data)
	if err != nil {
		return "", nil, err
	}
	if string(in.Context) == "null" {
		return data, nil, nil
	}
	value, err := seal("context", in.Context)
	if err != nil {
		return "", nil, err
	}
	return data, &value, nil
}

// OpenSessionFields accepts only new-runtime authenticated envelopes at the
// exact session/provider/collection/amount/currency. No plaintext or historical
// purpose fallback. The caller must first authorize and lock the source row.
func OpenSessionFields(ring *protected.Ring, in SessionRequest, data string, context *string) (SessionRequest, error) {
	// Validate/normalize source scalar identity before constructing AAD.
	in.Data = json.RawMessage("{}")
	in.Context = nil
	raw, err := encodeSessionRequest(EffectKey{ResourceID: in.SessionID}, in)
	if err != nil {
		return SessionRequest{}, err
	}
	defer clear(raw)
	var normalized storedSessionRequest
	if json.Unmarshal(raw, &normalized) != nil {
		return SessionRequest{}, ErrEffectInput
	}
	in = normalized.Request
	open := func(column, stored string) (json.RawMessage, error) {
		if len(stored) > 2*protected.MaxPlaintext || !strings.HasPrefix(stored, "ers1.") {
			return nil, ErrEffectProtection
		}
		encoded := strings.TrimPrefix(stored, "ers1.")
		envelope, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || base64.StdEncoding.EncodeToString(envelope) != encoded {
			return nil, ErrEffectProtection
		}
		plain, err := ring.Open(sessionFieldContext(in, column), envelope)
		if err != nil {
			return nil, ErrEffectProtection
		}
		defer clear(plain)
		value, err := requestObject(plain, false)
		if err != nil {
			return nil, ErrEffectProtection
		}
		return value, nil
	}
	in.Data, err = open("data", data)
	if err != nil {
		return SessionRequest{}, err
	}
	if context != nil {
		in.Context, err = open("context", *context)
		if err != nil {
			return SessionRequest{}, err
		}
	}
	return in, nil
}
