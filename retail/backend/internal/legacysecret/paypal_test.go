package legacysecret

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/scrypt"
)

func paypalFixture(version, secret string, plain []byte) string {
	salt, nonce := bytes.Repeat([]byte{13}, 16), bytes.Repeat([]byte{17}, 12)
	digest := sha256.Sum256([]byte(secret))
	key := digest[:]
	prefix := "enc:v1:"
	if version == "v2" {
		key, _ = scrypt.Key([]byte(secret), salt, 16384, 8, 1, 32)
		prefix = "enc:v2:" + base64.StdEncoding.EncodeToString(salt) + ":"
	}
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	sealed := aead.Seal(nil, nonce, plain, nil)
	return prefix + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(sealed[len(sealed)-16:]) + ":" + base64.StdEncoding.EncodeToString(sealed[:len(sealed)-16])
}

func TestPayPalFormatsAndExplicitPlaintext(t *testing.T) {
	reader, _ := NewPayPal("\ufeff  source-key\u00a0")
	for _, version := range []string{"v1", "v2"} {
		stored := paypalFixture(version, "source-key", []byte(" 中文 merchant 😀 \x00 "))
		plain, err := reader.Open(stored)
		if err != nil || string(plain) != " 中文 merchant 😀 \x00 " {
			t.Fatal("PayPal versioned source authentication failed")
		}
		wrong, _ := NewPayPal("different")
		for _, bad := range []*PayPal{nil, {}, wrong} {
			if got, err := bad.Open(stored); err != ErrLegacy || got != nil {
				t.Fatal("encrypted source fell back to plaintext")
			}
		}
		invalidUTF := paypalFixture(version, "source-key", []byte{0xff})
		if got, err := reader.Open(invalidUTF); err != ErrLegacy || got != nil {
			t.Fatal("invalid decrypted UTF8 replaced")
		}
	}
	var missing *PayPal
	for _, value := range []string{"", " source plaintext \x00 ", "\u0085key\u0085"} {
		if got, err := missing.Open(value); err != nil || string(got) != value {
			t.Fatal("explicit source plaintext changed")
		}
	}
	nel, _ := NewPayPal("\u0085key\u0085")
	if got, err := nel.Open(paypalFixture("v1", "\u0085key\u0085", []byte("ok"))); err != nil || string(got) != "ok" {
		t.Fatal("Go whitespace rules replaced ECMAScript key trimming")
	}
}

func TestPayPalRejectsMalformedCiphertext(t *testing.T) {
	reader, _ := NewPayPal("source-key")
	valid := paypalFixture("v2", "source-key", []byte("synthetic"))
	for _, value := range []string{
		"enc:v3:future", "enc:v1:", "enc:v2:", "ers1.new-envelope", valid + ":extra", valid + "\n",
		strings.Replace(valid, ":", ".", 1), "enc:v1:AA==:AA==:AA==", "enc:v2:AA==:AA==:AA==:AA==",
		strings.Replace(valid, "enc:v2:", "enc:v1:", 1), strings.Repeat("x", (1<<20)+1), string([]byte{0xff}),
	} {
		// A malformed separator removes the recognized prefix; make its
		// remaining enc: marker explicit to exercise version framing.
		if strings.HasPrefix(value, "enc.") {
			value = "enc:" + value
		}
		if got, err := reader.Open(value); err != ErrLegacy || got != nil {
			t.Fatal("malformed source accepted")
		}
	}
	parts := strings.Split(valid, ":")
	ct, _ := base64.StdEncoding.DecodeString(parts[5])
	ct[0] ^= 1
	parts[5] = base64.StdEncoding.EncodeToString(ct)
	if got, err := reader.Open(strings.Join(parts, ":")); err != ErrLegacy || got != nil {
		t.Fatal("tampered source authenticated")
	}
	if _, err := NewPayPal(string([]byte{0xff})); err != ErrLegacy {
		t.Fatal("invalid source key accepted")
	}
}

func TestPayPalActualNodeFixtures(t *testing.T) {
	path := os.Getenv("PAYPAL_NODE_FIXTURE")
	if path == "" {
		t.Skip("explicit artificial Node fixture required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("Node fixture unavailable")
	}
	var rows []struct{ Key, Plaintext, Stored string }
	if json.Unmarshal(data, &rows) != nil || len(rows) != 9 {
		t.Fatal("Node fixture invalid")
	}
	for _, row := range rows {
		reader, err := NewPayPal(row.Key)
		if err != nil {
			t.Fatal("Node source key invalid")
		}
		plain, err := reader.Open(row.Stored)
		if err != nil || string(plain) != row.Plaintext {
			t.Fatal("actual Node PayPal interop failed")
		}
	}
}
