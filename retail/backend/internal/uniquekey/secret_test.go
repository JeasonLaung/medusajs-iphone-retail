package uniquekey

import (
	"bytes"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func TestSecretIndexConfigurationAndDomainSeparation(t *testing.T) {
	ring, _ := protected.New("enc", map[string][]byte{"enc": bytes.Repeat([]byte{1}, 32)})
	a, err := NewSecretRegistry(ring, "index_a", bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewSecretRegistry(ring, "index_b", bytes.Repeat([]byte{2}, 32))
	c, _ := NewSecretRegistry(ring, "index_a", bytes.Repeat([]byte{3}, 32))
	value := []byte("artificial capability")
	if a.keyed("scope", value) != a.keyed("scope", value) {
		t.Fatal("nondeterministic locator")
	}
	if a.keyed("scope", value) == a.keyed("other", value) || a.keyed("scope", value) == b.keyed("scope", value) || a.keyed("scope", value) == c.keyed("scope", value) {
		t.Fatal("index domain/key isolation lost")
	}
	for _, key := range [][]byte{nil, make([]byte, 31), make([]byte, 32)} {
		if _, err := NewSecretRegistry(ring, "index_a", key); err != ErrProtected {
			t.Fatal("invalid index material accepted")
		}
	}
	if _, err := NewSecretRegistry(nil, "index_a", bytes.Repeat([]byte{2}, 32)); err != ErrProtected {
		t.Fatal("nil encryption ring accepted")
	}
	if _, err := NewSecretRegistry(ring, "BAD/ID", bytes.Repeat([]byte{2}, 32)); err != ErrProtected {
		t.Fatal("invalid index id accepted")
	}
}
