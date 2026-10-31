package checkoutconfig

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitMollieCAFileFailsClosed(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	directory := t.TempDir()
	valid := filepath.Join(directory, "ca.pem")
	invalid := filepath.Join(directory, "invalid.pem")
	large := filepath.Join(directory, "large.pem")
	for path, body := range map[string][]byte{valid: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), invalid: []byte("not a certificate"), large: []byte(strings.Repeat("x", (1<<20)+1))} {
		if os.WriteFile(path, body, 0600) != nil {
			t.Fatal("CA fixture file")
		}
	}
	for _, path := range []string{"", valid, invalid, large, filepath.Join(directory, "missing.pem")} {
		values := fixture()
		values["MOLLIE_CA_FILE"] = path
		out, e := Load(func(k string) string { return values[k] })
		if path == "" || path == valid {
			if e != nil || out == nil {
				t.Fatal("valid trust configuration refused", e)
			}
		} else if e != ErrInvalid || out != nil {
			t.Fatal("invalid custom CA silently fell back")
		}
	}
}
