package legacysecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/protected"
	"golang.org/x/crypto/scrypt"
)

// PayPal is deliberately separate from the four dot-separated source ciphers.
// It never reads env or uses a cookie key. The caller selects the source key.
type PayPal struct{ raw string }

func NewPayPal(raw string) (*PayPal, error) {
	if !utf8.ValidString(raw) || len(raw) > 4096 {
		return nil, ErrLegacy
	}
	// ECMAScript String.trim as used by the pinned source, including BOM but
	// excluding NEL. Go TrimSpace has a different Unicode whitespace set.
	raw = strings.Trim(raw, "\u0009\u000a\u000b\u000c\u000d\u0020\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff")
	return &PayPal{raw: raw}, nil
}

// Open explicitly supports source legacy plaintext, enc:v1 (SHA256) and
// enc:v2 (scrypt N=16384,r=8,p=1,salt=16). An encrypted value never falls back
// to plaintext when its key, encoding or authentication fails. Unknown enc:
// versions are held, unlike the old plugin's permissive identity path.
func (p *PayPal) Open(value string) ([]byte, error) {
	if !utf8.ValidString(value) || len(value) > 2*protected.MaxPlaintext {
		return nil, ErrLegacy
	}
	if !strings.HasPrefix(value, "enc:") {
		if len(value) > protected.MaxPlaintext || strings.HasPrefix(value, "ers1.") {
			return nil, ErrLegacy
		}
		return []byte(value), nil
	}
	if p == nil || p.raw == "" {
		return nil, ErrLegacy
	}
	parts := strings.Split(value, ":")
	if len(parts) < 2 || parts[0] != "enc" || !((parts[1] == "v1" && len(parts) == 5) || (parts[1] == "v2" && len(parts) == 6)) {
		return nil, ErrLegacy
	}
	var decoded [][]byte
	for _, part := range parts[2:] {
		data, err := base64.StdEncoding.Strict().DecodeString(part)
		if err != nil || base64.StdEncoding.EncodeToString(data) != part {
			return nil, ErrLegacy
		}
		decoded = append(decoded, data)
	}
	var key []byte
	if parts[1] == "v2" {
		if len(decoded[0]) != 16 {
			return nil, ErrLegacy
		}
	}
	dataIndex := 0
	if parts[1] == "v2" {
		dataIndex = 1
	}
	if len(decoded[dataIndex]) != 12 || len(decoded[dataIndex+1]) != 16 || len(decoded[dataIndex+2]) == 0 || len(decoded[dataIndex+2]) > protected.MaxPlaintext {
		return nil, ErrLegacy
	}
	if parts[1] == "v2" {
		var err error
		key, err = scrypt.Key([]byte(p.raw), decoded[0], 16384, 8, 1, 32)
		if err != nil {
			return nil, ErrLegacy
		}
		decoded = decoded[1:]
	} else {
		digest := sha256.Sum256([]byte(p.raw))
		key = append([]byte{}, digest[:]...)
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrLegacy
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrLegacy
	}
	plain, err := aead.Open(nil, decoded[0], append(decoded[2], decoded[1]...), nil)
	if err != nil || !utf8.Valid(plain) {
		clear(plain)
		return nil, ErrLegacy
	}
	return plain, nil
}
