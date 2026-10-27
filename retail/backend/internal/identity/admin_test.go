package identity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/sessionstore"
)

func TestExplicitOriginAndCookieConfiguration(t *testing.T) {
	for _, sample := range []struct {
		origin        string
		secure, valid bool
	}{
		{"", false, false}, {"https://admin.example", false, false}, {"https://admin.example", true, true},
		{"http://admin.example", false, false}, {"http://localhost:9200", false, true}, {"http://127.0.0.1:9200", false, true},
		{"https://admin.example/", true, false}, {"https://admin.example?token=x", true, false}, {"https://user@admin.example", true, false},
		{"https://admin.example#x", true, false}, {"http://localhost:9200", true, false},
	} {
		h, err := NewWithOptions(nil, Options{Sessions: &sessionstore.Store{}, Origin: sample.origin, Secure: sample.secure})
		if (err == nil) != sample.valid || (h != nil) != sample.valid {
			t.Fatal("origin configuration boundary incorrect")
		}
	}
}

func TestAmbiguousLoginJSONRejected(t *testing.T) {
	for _, raw := range []string{`{"email":"one","email":"two","password":"x"}`, `{"email":"x","password":"x","role":"admin"}`, `null`, `[]`, `{} {}`, `{"email":1}`, `{"password":true}`} {
		var email, password string
		if decodeLogin(json.NewDecoder(strings.NewReader(raw)), &email, &password) == nil {
			t.Fatal("ambiguous login accepted")
		}
	}
}

func TestSessionGenerationIncludesAuthorityFields(t *testing.T) {
	u := User{ID: "new-user", PasswordHash: "hash", Role: "admin", Enabled: true, AuthVersion: 1}
	before := authVersion(u)
	for _, mutate := range []func(*User){func(u *User) { u.AuthVersion++ }, func(u *User) { u.PasswordHash = "changed" }, func(u *User) { u.Role = "warehouse" }, func(u *User) { u.Enabled = false }, func(u *User) { u.ID = "other" }} {
		changed := u
		mutate(&changed)
		if authVersion(changed) == before {
			t.Fatal("authority edit did not change session generation")
		}
	}
}
