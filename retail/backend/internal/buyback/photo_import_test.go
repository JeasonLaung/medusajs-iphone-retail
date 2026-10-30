package buyback

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestActualLegacyPhotoRead(t *testing.T) {
	path := os.Getenv("BUYBACK_LEGACY_PHOTO_ROOT")
	if path == "" {
		t.Skip("explicit read-only original private root required")
	}
	root, e := os.OpenRoot(path)
	if e != nil {
		t.Fatal("private root unavailable")
	}
	defer root.Close()
	entries, e := os.ReadDir(path)
	if e != nil {
		t.Fatal("private listing unavailable")
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		before, e := os.ReadFile(filepath.Join(path, entry.Name()))
		if e != nil {
			t.Fatal("source read")
		}
		copied, e := ReadLegacyPhoto(root, entry.Name())
		if e != nil || sha256.Sum256(before) != sha256.Sum256(copied) {
			t.Fatal("original private object mismatch")
		}
		count++
	}
	if count == 0 {
		t.Fatal("no original private objects")
	}
	t.Logf("LEGACY_PRIVATE_READ objects=%d source_mutated=false", count)
}

func TestLegacyPhotoConfinedRead(t *testing.T) {
	dir := t.TempDir()
	root, e := os.OpenRoot(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	key := "private-1792746765521-buyback-d23e1047-c8aa-4d66-95cc-f24e419e1fde.png"
	data := []byte{137, 80, 78, 71, 13, 10, 26, 10, 1}
	if os.WriteFile(filepath.Join(dir, key), data, 0600) != nil {
		t.Fatal("fixture")
	}
	got, e := ReadLegacyPhoto(root, key)
	if e != nil || !bytes.Equal(got, data) {
		t.Fatal("valid private file")
	}
	for _, bad := range []string{"../" + key, "/" + key, `C:\` + key, "nested/" + key, "file://" + key, "public.png", "private-1-buyback-abcdef.exe", "missing"} {
		if _, e := ReadLegacyPhoto(root, bad); e == nil {
			t.Fatal("unexpected file accepted")
		}
	}
	if os.WriteFile(filepath.Join(dir, key), []byte("not an image"), 0600) != nil {
		t.Fatal("fixture")
	}
	if _, e := ReadLegacyPhoto(root, key); e == nil {
		t.Fatal("invalid MIME")
	}
	if os.WriteFile(filepath.Join(dir, key), make([]byte, MaxPhotoBytes+1), 0600) != nil {
		t.Fatal("fixture")
	}
	if _, e := ReadLegacyPhoto(root, key); e == nil {
		t.Fatal("oversized file")
	}
	if _, e := ReadLegacyPhoto(nil, key); e == nil {
		t.Fatal("nil root")
	}
}
