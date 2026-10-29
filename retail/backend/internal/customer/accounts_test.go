package customer

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInputAndUnavailableDependenciesFailClosed(t *testing.T) {
	var store *Store
	input := Registration{Email: "user@example.invalid", Password: "valid", FirstName: "Test", LastName: "Customer", Country: "dk", Locale: "en"}
	if _, err := store.Register(context.Background(), input); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing dependencies accepted")
	}
	cases := []Registration{input, input, input, input, input, input, input}
	cases[0].Email = "Name <user@example.invalid>"
	cases[1].Password = strings.Repeat("密", 25)
	cases[2].FirstName = "\n"
	cases[3].Country = "dk/../"
	cases[4].Locale = "fr"
	cases[5].Phone = "x\x00y"
	cases[6].LastName = string([]byte{255})
	for _, v := range cases {
		if _, err := store.Register(context.Background(), v); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid registration accepted")
		}
	}
	if email, ok := normalizedEmail("  USER@Example.invalid "); !ok || email != "user@example.invalid" {
		t.Fatal("source email normalization changed")
	}
	if !passwordOK("密碼") || passwordOK(strings.Repeat("x", 73)) || passwordOK("") {
		t.Fatal("password byte boundary differs")
	}
	for _, raw := range []string{"", strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if err := store.Verify(context.Background(), raw); !errors.Is(err, ErrVerification) {
			t.Fatal("malformed capability accepted")
		}
	}
	if err := store.Verify(context.Background(), strings.Repeat("a", 64)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing verification dependency accepted")
	}
	if _, err := store.Current(context.Background(), strings.Repeat("a", 64)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing session dependency accepted")
	}
}
