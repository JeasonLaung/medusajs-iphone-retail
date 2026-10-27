package uniquekey

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"hash"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/protected"
)

// SecretRegistry mixes ordinary source claims with protected source claims in
// one transaction. The caller supplies an independent fixed 32-byte HMAC key,
// its stable ID, and a business encryption ring. This API does NOT rotate the
// HMAC key: changing it requires an explicit exclusive rebuild of all claims.
type SecretRegistry struct {
	ring     *protected.Ring
	keyID    string
	indexKey [32]byte
	// Injectable only in package tests to force full-value collisions.
	testDigest func(string, []byte) [32]byte
}

func NewSecretRegistry(ring *protected.Ring, keyID string, indexKey []byte) (*SecretRegistry, error) {
	if ring == nil || !scopePattern.MatchString(keyID) || len(indexKey) != 32 {
		return nil, ErrProtected
	}
	s := &SecretRegistry{ring: ring, keyID: keyID}
	copy(s.indexKey[:], indexKey)
	if s.indexKey == [32]byte{} {
		return nil, ErrProtected
	}
	return s, nil
}

func macField(mac hash.Hash, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = mac.Write(size[:])
	_, _ = mac.Write(value)
}

func (s *SecretRegistry) keyed(scope string, value []byte) [32]byte {
	mac := hmac.New(sha256.New, s.indexKey[:])
	for _, v := range [][]byte{[]byte("eurorefurb.secret-unique-index.v1"), []byte(s.keyID), []byte(scope), value} {
		macField(mac, v)
	}
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

func (s *SecretRegistry) digest(scope string, value []byte) [32]byte {
	if !protectedScope(scope) {
		return sha256.Sum256(value)
	}
	if s.testDigest != nil {
		return s.testDigest(scope, value)
	}
	return s.keyed(scope, value)
}

func claimContext(scope, owner string) protected.Context {
	return protected.Context{Purpose: "claim." + scope, Table: "unique_key_claims", Column: "full_value", Row: owner}
}

func configContext(scope string) protected.Context {
	return protected.Context{Purpose: "unique-index-config", Table: "unique_key_secret_scopes", Column: "verification", Row: scope}
}

func (s *SecretRegistry) seal(scope, owner string, value []byte) ([]byte, error) {
	if !protectedScope(scope) {
		return value, nil
	}
	return s.ring.Seal(claimContext(scope, owner), value)
}

func (s *SecretRegistry) open(scope, owner string, value []byte) ([]byte, error) {
	if !protectedScope(scope) {
		return value, nil
	}
	return s.ring.Open(claimContext(scope, owner), value)
}

func (s *SecretRegistry) prepare(ctx context.Context, tx *sql.Tx, claims []claim) error {
	if s == nil || s.ring == nil {
		return ErrProtected
	}
	last := ""
	for _, c := range claims {
		if !protectedScope(c.scope) || c.scope == last {
			continue
		}
		last = c.scope
		// This scope lock deliberately serializes protected-key changes; do not
		// cache verification outside the transaction or ignore its failure.
		expected := s.keyed(c.scope, []byte("configuration-check.v1"))
		sealed, err := s.ring.Seal(configContext(c.scope), expected[:])
		if err != nil {
			return ErrProtected
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO unique_key_secret_scopes(scope,key_id,verification) VALUES(?,?,?) ON DUPLICATE KEY UPDATE scope=unique_key_secret_scopes.scope", c.scope, s.keyID, sealed); err != nil {
			return ErrStorage
		}
		var keyID string
		var stored []byte
		if err = tx.QueryRowContext(ctx, "SELECT key_id,verification FROM unique_key_secret_scopes WHERE scope=? FOR UPDATE", c.scope).Scan(&keyID, &stored); err != nil {
			return ErrStorage
		}
		actual, err := s.ring.Open(configContext(c.scope), stored)
		if err != nil || keyID != s.keyID || !hmac.Equal(actual, expected[:]) {
			return ErrProtected
		}
	}
	return nil
}

// Apply accepts internal changes only. All callers must roll back the whole
// transaction on any error. Ordinary Apply refuses protected source scopes.
func (s *SecretRegistry) Apply(ctx context.Context, tx *sql.Tx, changes []Change) error {
	if s == nil || s.ring == nil {
		return ErrProtected
	}
	return applyCodec(ctx, tx, changes, s)
}

// ApplySourceRow consumes the ordinary target projection plus encrypted domain
// fields separately. Secret plaintext is transient and is never returned or
// placed in the ordinary target-row map. It still requires domain authorization,
// version checks and an actual business-row write in the SAME transaction.
func (s *SecretRegistry) ApplySourceRow(ctx context.Context, tx *sql.Tx, table string, row TargetRow, secrets map[string][]byte) error {
	if s == nil || s.ring == nil || table != "mail_contacts" || len(secrets) != 1 || secrets["unsubscribe_token"] == nil {
		return ErrProtected
	}
	if _, exists := row["unsubscribe_token"]; exists {
		return ErrProtected
	}
	changes, err := sourceChanges(table, row, func(plan sourcePlan, owner string, field sourceField) (*string, error) {
		if plan.TargetTable != "mail_contacts" || len(plan.Keys) != 1 || field.Column != "unsubscribe_token" {
			return nil, ErrProtected
		}
		plain, err := s.ring.Open(protected.Context{Purpose: "business-capability", Table: table, Column: field.Column, Row: owner}, secrets[field.Column])
		if err != nil || !utf8.Valid(plain) {
			return nil, ErrProtected
		}
		value := string(plain)
		return &value, nil
	})
	if err != nil {
		return err
	}
	return s.Apply(ctx, tx, changes)
}
