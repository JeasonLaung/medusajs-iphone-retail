package protected

import (
	"strings"
	"testing"
)

func TestParseExplicitKeyring(t *testing.T) {
	key := strings.Repeat("71", 32)
	ring, err := ParseKeyring("first", `{"first":"`+key+`"}`)
	if err != nil || ring == nil {
		t.Fatal("explicit keyring rejected")
	}
	for _, input := range []string{"", `null`, `[]`, `{}`, `{"first":null}`, `{"first":"short"}`, `{"first":"` + strings.Repeat("AA", 32) + `"}`, `{"first":"` + key + `","first":"` + key + `"}`, `{"first":"` + key + `"} {}`, `{"first":"` + strings.Repeat("00", 32) + `"}`, `{"first":"` + key + `","second":"` + key + `"}`} {
		if _, err := ParseKeyring("first", input); err != ErrConfig {
			t.Fatal("invalid or ambiguous keyring accepted")
		}
	}
	if _, err := ParseKeyring("missing", `{"first":"`+key+`"}`); err != ErrConfig {
		t.Fatal("missing active key accepted")
	}
	if _, err := ParseKeyring("first", strings.Repeat(" ", 8193)); err != ErrConfig {
		t.Fatal("oversized keyring accepted")
	}
}
