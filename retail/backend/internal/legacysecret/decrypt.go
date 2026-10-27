// Package legacysecret reads the pinned Medusa merchant cipher format for
// offline migration only. It never reads environment. The four dot-separated
// domains do not fall back to plaintext; PayPal has a separate explicit reader.
package legacysecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/protected"
)

var ErrLegacy = errors.New("legacy merchant secret cannot be authenticated")

type Keys struct{ domains map[string]cipher.AEAD }

func known(domain string) bool {
	switch domain {
	case "mail", "seo", "webhook", "mollie":
		return true
	}
	return false
}

// New requires explicitly selected source domain secrets. COOKIE_SECRET fallback
// selection, if needed for the source, must happen in an offline caller. These
// SHA256-derived legacy keys must never be used as new business encryption keys.
func New(secrets map[string]string) (*Keys, error) {
	k := &Keys{domains: map[string]cipher.AEAD{}}
	for domain, secret := range secrets {
		if !known(domain) || secret == "" || secret == "supersecret" || !utf8.ValidString(secret) || len(secret) > protected.MaxPlaintext {
			return nil, ErrLegacy
		}
		digest := sha256.Sum256([]byte(secret))
		block, err := aes.NewCipher(digest[:])
		if err != nil {
			return nil, ErrLegacy
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, ErrLegacy
		}
		k.domains[domain] = aead
	}
	return k, nil
}

// Open strictly accepts canonical standard-base64(nonce).base64(tag).base64(data).
// Source empty strings retain their meaning, without requiring a source key.
// Noncanonical encodings accepted by Node's permissive decoder are held rather
// than normalized. Invalid UTF-8 is rejected, never replaced with U+FFFD.
func (k *Keys) Open(domain, value string) ([]byte, error) {
	if !known(domain) || len(value) > 2*protected.MaxPlaintext {
		return nil, ErrLegacy
	}
	if value == "" {
		return []byte{}, nil
	}
	if k == nil {
		return nil, ErrLegacy
	}
	aead, exists := k.domains[domain]
	if !exists {
		return nil, ErrLegacy
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return nil, ErrLegacy
	}
	decoded := make([][]byte, 3)
	for i, part := range parts {
		bytes, err := base64.StdEncoding.Strict().DecodeString(part)
		if err != nil || base64.StdEncoding.EncodeToString(bytes) != part {
			return nil, ErrLegacy
		}
		decoded[i] = bytes
	}
	if len(decoded[0]) != 12 || len(decoded[1]) != 16 || len(decoded[2]) > protected.MaxPlaintext {
		return nil, ErrLegacy
	}
	combined := append(decoded[2], decoded[1]...)
	plaintext, err := aead.Open(nil, decoded[0], combined, nil)
	if err != nil || !utf8.Valid(plaintext) {
		clear(plaintext)
		return nil, ErrLegacy
	}
	return plaintext, nil
}
