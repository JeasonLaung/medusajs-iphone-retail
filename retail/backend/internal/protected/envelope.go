// Package protected seals business secrets. It has no environment, SQL, HTTP,
// cookie-key fallback, logging, or legacy-ciphertext detection side effects.
package protected

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"regexp"
	"unicode/utf8"
)

const MaxPlaintext = 1 << 20

var (
	ErrConfig         = errors.New("invalid protected key configuration")
	ErrEnvelope       = errors.New("invalid protected envelope or context")
	ErrKeyUnavailable = errors.New("protected envelope key unavailable")
	identifier        = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
	magic             = []byte{'E', 'R', 'S', 1}
)

// Context binds each value to its use and exact row. Copying an envelope to a
// different purpose/table/column/row must fail even under the same key.
type Context struct {
	Purpose string
	Table   string
	Column  string
	Row     string
}

// Ring is immutable and safe to share. Keys must be independently generated
// 32-byte business encryption keys; never password strings or cookie secrets.
// Keep old decryption keys available while persisted envelopes reference them.
type Ring struct {
	active string
	keys   map[string]cipher.AEAD
}

func New(active string, keys map[string][]byte) (*Ring, error) {
	if !identifier.MatchString(active) || len(keys) == 0 || len(keys) > 32 {
		return nil, ErrConfig
	}
	if _, exists := keys[active]; !exists {
		return nil, ErrConfig
	}
	r := &Ring{active: active, keys: make(map[string]cipher.AEAD, len(keys))}
	seen := make(map[[32]byte]bool)
	for id, key := range keys {
		if !identifier.MatchString(id) || len(key) != 32 {
			return nil, ErrConfig
		}
		var copyKey [32]byte
		copy(copyKey[:], key)
		if copyKey == [32]byte{} || seen[copyKey] {
			return nil, ErrConfig
		}
		seen[copyKey] = true
		block, err := aes.NewCipher(copyKey[:])
		if err != nil {
			return nil, ErrConfig
		}
		aead, err := cipher.NewGCMWithRandomNonce(block)
		if err != nil {
			return nil, ErrConfig
		}
		r.keys[id] = aead
	}
	return r, nil
}

func associated(header []byte, c Context) ([]byte, error) {
	if !identifier.MatchString(c.Purpose) || !identifier.MatchString(c.Table) || !identifier.MatchString(c.Column) ||
		c.Row == "" || !utf8.ValidString(c.Row) || utf8.RuneCountInString(c.Row) > 191 {
		return nil, ErrEnvelope
	}
	var aad bytes.Buffer
	for _, value := range [][]byte{[]byte("eurorefurb.business-secret.v1"), header, []byte(c.Purpose), []byte(c.Table), []byte(c.Column), []byte(c.Row)} {
		_ = binary.Write(&aad, binary.BigEndian, uint32(len(value)))
		aad.Write(value)
	}
	return aad.Bytes(), nil
}

// Seal uses Go's AES-GCM random-nonce implementation (96-bit nonce prepended
// to ciphertext, 128-bit tag). Rotate a key BEFORE 2^32 encryptions under it;
// key use counting/rotation operations are the caller's responsibility.
// nil and empty plaintext both seal empty bytes. SQL NULL is handled outside
// this API; neither NULL nor empty persisted ciphertext is an empty secret.
func (r *Ring) Seal(c Context, plaintext []byte) ([]byte, error) {
	if r == nil || len(plaintext) > MaxPlaintext {
		return nil, ErrEnvelope
	}
	aead, exists := r.keys[r.active]
	if !exists {
		return nil, ErrKeyUnavailable
	}
	header := append(append([]byte{}, magic...), byte(len(r.active)))
	header = append(header, r.active...)
	aad, err := associated(header, c)
	if err != nil {
		return nil, err
	}
	return aead.Seal(header, nil, plaintext, aad), nil
}

// Open authenticates the entire header and caller-provided context before
// returning plaintext. No plaintext/legacy fallback or automatic key lookup.
func (r *Ring) Open(c Context, envelope []byte) ([]byte, error) {
	if r == nil || len(envelope) < len(magic)+1 || len(envelope) > MaxPlaintext+4+1+64+28 || !bytes.Equal(envelope[:4], magic) {
		return nil, ErrEnvelope
	}
	size := int(envelope[4])
	end := 5 + size
	if size < 1 || size > 64 || len(envelope) < end+28 {
		return nil, ErrEnvelope
	}
	id := string(envelope[5:end])
	if !identifier.MatchString(id) {
		return nil, ErrEnvelope
	}
	aead, exists := r.keys[id]
	if !exists {
		return nil, ErrKeyUnavailable
	}
	aad, err := associated(envelope[:end], c)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nil, envelope[end:], aad)
	if err != nil || len(plaintext) > MaxPlaintext {
		return nil, ErrEnvelope
	}
	return plaintext, nil
}
