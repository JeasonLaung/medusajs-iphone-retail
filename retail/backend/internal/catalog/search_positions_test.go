package catalog

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Source PostgreSQL captures include stopword gaps, compounds, URLs and markup.
// Comparing positions catches differences invisible to lexeme-only matching.
func TestSearchPostgresPositionOracle(t *testing.T) {
	data, err := os.ReadFile("testdata/search-postgres-positions.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input     string
		Positions []searchLexeme
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 95 {
		t.Fatal("source position oracle coverage changed")
	}
	for _, tc := range cases {
		t.Run(tc.Input, func(t *testing.T) {
			if got := searchLexemes(tc.Input); !reflect.DeepEqual(got, tc.Positions) {
				t.Fatalf("positions: got %v, source %v", got, tc.Positions)
			}
		})
	}
}
