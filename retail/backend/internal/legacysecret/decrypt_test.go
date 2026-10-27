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
)

func synthetic(t *testing.T, value []byte) string {
	t.Helper()
	key := sha256.Sum256([]byte("artificial-source-key"))
	block, _ := aes.NewCipher(key[:])
	aead, _ := cipher.NewGCM(block)
	nonce := bytes.Repeat([]byte{9}, 12)
	sealed := aead.Seal(nil, nonce, value, nil)
	return base64.StdEncoding.EncodeToString(nonce) + "." + base64.StdEncoding.EncodeToString(sealed[len(sealed)-16:]) + "." + base64.StdEncoding.EncodeToString(sealed[:len(sealed)-16])
}

func TestLegacyAuthenticationAndStrictFormat(t *testing.T) {
	keys, err := New(map[string]string{"mail": "artificial-source-key"})
	if err != nil {
		t.Fatal(err)
	}
	valid := synthetic(t, []byte(" 保留 🚀 \x00 "))
	plain, err := keys.Open("mail", valid)
	if err != nil || string(plain) != " 保留 🚀 \x00 " {
		t.Fatal("legacy bytes changed")
	}
	for _, bad := range []string{"plaintext", valid + ".extra", strings.Replace(valid, ".", ".\n", 1), strings.Replace(valid, "=", "", 1), synthetic(t, []byte{0xff}), strings.Repeat("x", 2<<20+1)} {
		if out, err := keys.Open("mail", bad); err != ErrLegacy || out != nil {
			t.Fatal("malformed legacy value accepted")
		}
	}
	parts := strings.Split(valid, ".")
	for index := range parts {
		for offset := 0; offset < len(parts[index]); offset++ {
			copyParts := append([]string{}, parts...)
			mutated := []byte(copyParts[index])
			mutated[offset] ^= 1
			copyParts[index] = string(mutated)
			if out, err := keys.Open("mail", strings.Join(copyParts, ".")); err != ErrLegacy || out != nil {
				t.Fatal("mutated ciphertext accepted")
			}
		}
	}
	for _, config := range []map[string]string{{"unknown": "key"}, {"mail": ""}, {"mail": "supersecret"}} {
		if _, err := New(config); err != ErrLegacy {
			t.Fatal("invalid legacy key configuration accepted")
		}
	}
	wrong, _ := New(map[string]string{"mail": "wrong"})
	if _, err := wrong.Open("mail", valid); err != ErrLegacy {
		t.Fatal("wrong key accepted")
	}
	if _, err := keys.Open("seo", valid); err != ErrLegacy {
		t.Fatal("implicit domain fallback")
	}
	var absent *Keys
	if value, err := absent.Open("mail", ""); err != nil || len(value) != 0 {
		t.Fatal("empty legacy source changed")
	}
	if _, err := absent.Open("unknown", ""); err != ErrLegacy {
		t.Fatal("unknown empty domain accepted")
	}
}

func TestActualNodeLegacyFunctions(t *testing.T) {
	path := os.Getenv("LEGACY_SECRET_NODE_FIXTURE")
	if path == "" {
		t.Skip("requires artificial actual TypeScript source fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("artificial Node fixture missing")
	}
	var fixtures []struct{ Domain, Secret, Plaintext, Ciphertext string }
	if json.Unmarshal(data, &fixtures) != nil || len(fixtures) != 16 {
		t.Fatal("invalid artificial Node fixture")
	}
	for _, f := range fixtures {
		keys, err := New(map[string]string{f.Domain: f.Secret})
		if err != nil {
			t.Fatal("artificial source key rejected")
		}
		plain, err := keys.Open(f.Domain, f.Ciphertext)
		if err != nil || string(plain) != f.Plaintext {
			t.Fatal("Node to Go source authentication failed")
		}
	}
}
