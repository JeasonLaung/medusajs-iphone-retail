package seo

import (
	"errors"
	"testing"
)

func TestSourcePathNormalization(t *testing.T) {
	for in, want := range map[string]string{"/": "/", "/dk-en/about///": "/dk-en/about", "/a%20b": "/a%20b", "/中文": "/中文"} {
		got, err := NormalizePath(in)
		if err != nil || got != want {
			t.Errorf("normalize %q", in)
		}
	}
	for _, in := range []string{"", "relative", "//external", "/path?query", "/path#fragment", "/path\\other", "/a\n", "/\x00", string([]byte{'/', 255})} {
		if _, err := NormalizePath(in); !errors.Is(err, ErrInput) {
			t.Errorf("accepted %q", in)
		}
	}
}
func TestOriginRejectsCredentialsAndNonHTTP(t *testing.T) {
	for _, in := range []string{"", "file:///tmp", "http://user:pass@example.com", "//example.com", "http:///missing"} {
		if _, err := siteOrigin(in); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
	got, err := siteOrigin("https://example.com/path?q=1#fragment")
	if err != nil || got != "https://example.com" {
		t.Fatal("origin parity")
	}
}
