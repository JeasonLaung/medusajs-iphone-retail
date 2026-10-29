package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

var (
	ErrEffectProtection = errors.New("payment effect request protection unavailable")
	requestCurrency     = regexp.MustCompile(`^[a-z]{3}$`)
)

// SessionRequest is private provider input/recovery material. It is NEVER a
// browser DTO or log value. The server coordinator supplies all fields from its
// authorized snapshot. SessionID is the local request ID, NOT a provider token.
// A create adapter must use this same SessionID as its stable idempotency key
// and data.session_id on every attempt; it must never use the attempt UUID.
// Delete adapters pass only Data to the provider; the remaining fields support
// compensation. This type does not grant provider or cart authority.
type SessionRequest struct {
	ProviderID   string          `json:"provider_id"`
	CollectionID string          `json:"collection_id"`
	SessionID    string          `json:"session_id"`
	Amount       string          `json:"amount"`
	Currency     string          `json:"currency"`
	Data         json.RawMessage `json:"data"`
	Context      json.RawMessage `json:"context"`
}

type storedSessionRequest struct {
	Version int            `json:"version"`
	Request SessionRequest `json:"request"`
}

// JSON objects preserve exact number spellings/large integers. Reject duplicate
// keys at every depth rather than silently changing the provider's parameters.
func requestObject(raw json.RawMessage, nullable bool) (json.RawMessage, error) {
	if len(raw) > protected.MaxPlaintext || !utf8.Valid(raw) {
		return nil, ErrEffectInput
	}
	if nullable && (len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
		return json.RawMessage("null"), nil
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, ErrEffectInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 64 {
			return ErrEffectInput
		}
		token, err := d.Token()
		if err != nil {
			return ErrEffectInput
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				token, err := d.Token()
				name, ok := token.(string)
				if err != nil || !ok || seen[name] {
					return ErrEffectInput
				}
				seen[name] = true
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim('}') {
				return ErrEffectInput
			}
		case '[':
			for d.More() {
				if err := visit(depth + 1); err != nil {
					return err
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim(']') {
				return ErrEffectInput
			}
		default:
			return ErrEffectInput
		}
		return nil
	}
	if bytes.TrimSpace(raw)[0] != '{' || visit(0) != nil {
		return nil, ErrEffectInput
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrEffectInput
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return nil, ErrEffectInput
	}
	return compact.Bytes(), nil
}

func encodeSessionRequest(key EffectKey, in SessionRequest) ([]byte, error) {
	if !effectID.MatchString(in.ProviderID) || !effectID.MatchString(in.CollectionID) || !effectID.MatchString(in.SessionID) || in.SessionID != key.ResourceID || !requestCurrency.MatchString(in.Currency) {
		return nil, ErrEffectInput
	}
	amount, err := totals.Resolve(totals.Number{Decimal: in.Amount})
	if err != nil || strings.HasPrefix(amount, "-") {
		return nil, ErrEffectInput
	}
	in.Amount = amount
	in.Data, err = requestObject(in.Data, false)
	if err != nil {
		return nil, err
	}
	in.Context, err = requestObject(in.Context, true)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(storedSessionRequest{Version: 1, Request: in})
	if err != nil || len(raw) > protected.MaxPlaintext {
		return nil, ErrEffectInput
	}
	return raw, nil
}

func requestContext(key EffectKey) protected.Context {
	// Fixed-order JSON supplies unambiguous boundaries for arbitrary ID lengths.
	identity, _ := json.Marshal([]string{key.ScopeID, key.OperationID, key.Kind, key.ResourceID, hex.EncodeToString(key.Digest[:])})
	digest := sha256.Sum256(identity)
	return protected.Context{Purpose: "payment-effect-request-v1", Table: "payment_effect_journal", Column: "request_envelope", Row: hex.EncodeToString(digest[:])}
}

func openSessionRequest(key EffectKey, ring *protected.Ring, envelope []byte) (SessionRequest, error) {
	raw, err := ring.Open(requestContext(key), envelope)
	if err != nil {
		return SessionRequest{}, ErrEffectProtection
	}
	defer clear(raw)
	digest := sha256.Sum256(raw)
	if subtle.ConstantTimeCompare(digest[:], key.Digest[:]) != 1 {
		return SessionRequest{}, ErrEffectProtection
	}
	var stored storedSessionRequest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&stored) != nil || stored.Version != 1 {
		return SessionRequest{}, ErrEffectProtection
	}
	if d.Decode(new(any)) != io.EOF {
		return SessionRequest{}, ErrEffectProtection
	}
	canonical, err := encodeSessionRequest(key, stored.Request)
	if err != nil {
		return SessionRequest{}, ErrEffectProtection
	}
	defer clear(canonical)
	if !bytes.Equal(raw, canonical) {
		return SessionRequest{}, ErrEffectProtection
	}
	return stored.Request, nil
}

// StageSessionRequest atomically binds a journal receipt and an encrypted full
// request. A zero input Digest is filled; a nonzero one MUST match. Keys are
// explicit business keys, never env/cookie fallback. Replay authenticates the
// persisted envelope and retains it byte-for-byte (including after rotation).
// The enclosing authorized transaction must roll back on any error. This is an
// internal persistence primitive, not a complete cart operation or job worker.
func StageSessionRequest(ctx context.Context, tx *gorm.DB, key EffectKey, ring *protected.Ring, in SessionRequest) (EffectKey, EffectReceipt, error) {
	fail := func(err error) (EffectKey, EffectReceipt, error) { return EffectKey{}, EffectReceipt{}, err }
	raw, err := encodeSessionRequest(key, in)
	if err != nil {
		return fail(err)
	}
	defer clear(raw)
	digest := sha256.Sum256(raw)
	if key.Digest != ([32]byte{}) && key.Digest != digest {
		return fail(ErrEffectConflict)
	}
	key.Digest = digest
	envelope, err := ring.Seal(requestContext(key), raw)
	if err != nil {
		return fail(ErrEffectProtection)
	}
	defer clear(envelope)
	receipt, err := StageEffect(ctx, tx, key)
	if err != nil {
		return fail(err)
	}
	q, cancel, err := effectTx(ctx, tx, key)
	if err != nil {
		return fail(err)
	}
	defer cancel()
	var saved struct{ RequestEnvelope []byte }
	if effectWhere(q, key).Select("request_envelope").Take(&saved).Error != nil {
		return fail(ErrEffectStorage)
	}
	if saved.RequestEnvelope != nil {
		if _, err := openSessionRequest(key, ring, saved.RequestEnvelope); err != nil {
			return fail(err)
		}
	} else {
		// Never attach recovery material retroactively to an already dispatched job.
		if receipt.State != "pending" {
			return fail(ErrEffectConflict)
		}
		r := effectWhere(q, key).Where("request_envelope IS NULL AND state='pending'").Update("request_envelope", envelope)
		if r.Error != nil || r.RowsAffected != 1 {
			return fail(ErrEffectStorage)
		}
	}
	return key, receipt, nil
}

// ReadSessionRequest authenticates the exact bound row, including its digest,
// before returning private recovery data. It requires an authorized real
// transaction and holds the receipt lock until the caller commits/rolls back.
func ReadSessionRequest(ctx context.Context, tx *gorm.DB, key EffectKey, ring *protected.Ring) (SessionRequest, error) {
	q, cancel, err := effectTx(ctx, tx, key)
	if err != nil {
		return SessionRequest{}, err
	}
	defer cancel()
	if _, err := lockEffect(q, key); err != nil {
		return SessionRequest{}, err
	}
	var saved struct{ RequestEnvelope []byte }
	if effectWhere(q, key).Select("request_envelope").Take(&saved).Error != nil {
		return SessionRequest{}, ErrEffectStorage
	}
	return openSessionRequest(key, ring, saved.RequestEnvelope)
}

// ClaimSessionRequest decrypts and verifies BEFORE granting dispatch. A missing
// key, corrupt/swapped envelope or missing snapshot never advances the attempt.
// Call external providers ONLY after a successful commit and Dispatch=true.
// A returned uncertain request is for reconciliation, never automatic dispatch.
func ClaimSessionRequest(ctx context.Context, tx *gorm.DB, key EffectKey, ring *protected.Ring, lease time.Duration) (EffectReceipt, SessionRequest, error) {
	in, err := ReadSessionRequest(ctx, tx, key, ring)
	if err != nil {
		return EffectReceipt{}, SessionRequest{}, err
	}
	receipt, err := ClaimEffect(ctx, tx, key, lease)
	if err != nil {
		return EffectReceipt{}, SessionRequest{}, err
	}
	return receipt, in, nil
}
