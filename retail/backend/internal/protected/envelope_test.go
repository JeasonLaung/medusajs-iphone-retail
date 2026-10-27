package protected

import (
	"bytes"
	"sync"
	"testing"
)

func testRing(t *testing.T) (*Ring, Context) {
	t.Helper()
	r, err := New("test-key-1", map[string][]byte{"test-key-1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	return r, Context{"merchant-config", "mail_accounts", "api_key", "fixture-row"}
}

func TestRoundtripBoundaries(t *testing.T) {
	r, c := testRing(t)
	for _, value := range [][]byte{nil, {}, []byte(" UTF-8 密钥 🚀 "), {0, 0xff, 0xfe, 1}, bytes.Repeat([]byte{9}, MaxPlaintext)} {
		sealed, err := r.Seal(c, value)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := r.Open(c, sealed)
		if err != nil || !bytes.Equal(opened, value) {
			t.Fatal("bytes changed", err)
		}
	}
	if _, err := r.Seal(c, make([]byte, MaxPlaintext+1)); err != ErrEnvelope {
		t.Fatal("oversize accepted")
	}
	for _, value := range [][]byte{nil, {}, []byte("plaintext"), []byte("legacy.base64.ciphertext")} {
		if _, err := r.Open(c, value); err != ErrEnvelope {
			t.Fatal("fallback accepted")
		}
	}
}

func TestAuthenticatedHeaderAndCiphertext(t *testing.T) {
	r, c := testRing(t)
	sealed, _ := r.Seal(c, []byte("artificial secret"))
	for i := range sealed {
		mutated := bytes.Clone(sealed)
		mutated[i] ^= 1
		if _, err := r.Open(c, mutated); err == nil {
			t.Fatal("tampered byte authenticated", i)
		}
	}
	for i := 0; i < len(sealed); i++ {
		if _, err := r.Open(c, sealed[:i]); err == nil {
			t.Fatal("truncated envelope accepted", i)
		}
	}
	if _, err := r.Open(c, append(bytes.Clone(sealed), 0)); err == nil {
		t.Fatal("extra suffix accepted")
	}
}

func TestContextIsolation(t *testing.T) {
	r, c := testRing(t)
	sealed, _ := r.Seal(c, []byte("artificial secret"))
	for _, wrong := range []Context{
		{"other", c.Table, c.Column, c.Row}, {c.Purpose, "seo_providers", c.Column, c.Row},
		{c.Purpose, c.Table, "webhook_secret", c.Row}, {c.Purpose, c.Table, c.Column, "Fixture-row"},
		{c.Purpose, c.Table, c.Column, "fixture-row "}, {c.Purpose, c.Table, c.Column, ""},
	} {
		if _, err := r.Open(wrong, sealed); err == nil {
			t.Fatal("wrong context accepted")
		}
	}
	a := Context{"ab", "c", "field", "row"}
	b := Context{"a", "bc", "field", "row"}
	sealed, _ = r.Seal(a, nil)
	if _, err := r.Open(b, sealed); err == nil {
		t.Fatal("context concatenation ambiguity")
	}
}

func TestRotationAndKeyOwnership(t *testing.T) {
	key1 := bytes.Repeat([]byte{1}, 32)
	key2 := bytes.Repeat([]byte{2}, 32)
	old, _ := New("key1", map[string][]byte{"key1": key1})
	c := Context{"purpose", "table", "column", "row"}
	before, _ := old.Seal(c, []byte("fixture"))
	// Mutating caller-owned config after construction cannot alter the key.
	key1[0] = 99
	if _, err := old.Open(c, before); err != nil {
		t.Fatal("key config not copied")
	}
	key1[0] = 1
	next, _ := New("key2", map[string][]byte{"key1": key1, "key2": key2})
	plain, err := next.Open(c, before)
	if err != nil {
		t.Fatal(err)
	}
	after, err := next.Seal(c, plain)
	if err != nil {
		t.Fatal(err)
	}
	onlyNew, _ := New("key2", map[string][]byte{"key2": key2})
	if _, err := onlyNew.Open(c, after); err != nil {
		t.Fatal(err)
	}
	if _, err := onlyNew.Open(c, before); err != ErrKeyUnavailable {
		t.Fatal("missing old key fallback")
	}
	wrong, _ := New("key1", map[string][]byte{"key1": key2})
	if _, err := wrong.Open(c, before); err != ErrEnvelope {
		t.Fatal("wrong material accepted")
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, keys := range []map[string][]byte{nil, {"test": nil}, {"test": make([]byte, 32)}, {"test": make([]byte, 31)}, {"other": bytes.Repeat([]byte{1}, 32)}, {"test": bytes.Repeat([]byte{1}, 32), "duplicate": bytes.Repeat([]byte{1}, 32)}} {
		if _, err := New("test", keys); err != ErrConfig {
			t.Fatal("bad config accepted")
		}
	}
	if _, err := New("bad/id", map[string][]byte{"bad/id": bytes.Repeat([]byte{1}, 32)}); err != ErrConfig {
		t.Fatal("bad id accepted")
	}
}

func TestRandomNonceAndConcurrentReads(t *testing.T) {
	r, c := testRing(t)
	const count = 64
	results := make(chan []byte, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			encrypted, err := r.Seal(c, []byte("same value"))
			if err != nil {
				t.Error(err)
				return
			}
			opened, err := r.Open(c, encrypted)
			if err != nil || string(opened) != "same value" {
				t.Error("concurrent roundtrip failed")
			}
			results <- encrypted
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for result := range results {
		if seen[string(result)] {
			t.Fatal("nonce repeated in artificial sample")
		}
		seen[string(result)] = true
	}
	if len(seen) != count {
		t.Fatal("incomplete concurrent operations")
	}
}
