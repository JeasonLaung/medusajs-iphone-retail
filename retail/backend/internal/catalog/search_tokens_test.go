package catalog

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Captured from the local source PostgreSQL 16.15 in a read-only transaction.
// This is a lexical oracle, not evidence that all parser classes are migrated.
func TestSearchPostgresLexemeOracle(t *testing.T) {
	verifySearchLexemeOracle(t, "testdata/search-postgres-lexemes.json", 29)
}

func TestSearchPostgresLinkOracle(t *testing.T) {
	verifySearchLexemeOracle(t, "testdata/search-postgres-links.json", 55)
}

func verifySearchLexemeOracle(t *testing.T, path string, count int) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input string
		Terms []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != count {
		t.Fatal("source oracle coverage changed")
	}
	for _, tc := range cases {
		t.Run(tc.Input, func(t *testing.T) {
			if got := searchTerms(tc.Input); !reflect.DeepEqual(got, tc.Terms) {
				t.Fatalf("lexemes %q: got %v, source %v", tc.Input, got, tc.Terms)
			}
		})
	}
}

func TestSearchCompoundAndNumericBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text, query string
		match       bool
	}{
		{"space gray", "space-gray", false},
		{"space-gray", "space gray", true},
		{"space-gray", "gray", true},
		{"6.1 inches", "6.1", true},
		{"6 1 inches", "6.1", false},
		{"a@example.test", "a@example.test", true},
		{"a example test", "a@example.test", false},
		{"phones128", "phone128", false},
		{"verified-quantities", "verifying quantities", true},
	} {
		h := StoreSearchHit{description: tc.text}
		if _, got := searchRank(h, tc.query); got != tc.match {
			t.Fatalf("text %q query %q: %v want %v", tc.text, tc.query, got, tc.match)
		}
	}
}
