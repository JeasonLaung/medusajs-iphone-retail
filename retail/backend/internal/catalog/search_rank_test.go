package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
)

// Source scores are compared bit-for-bit as float4, not with a tolerance that
// could hide a tie becoming a different public result order.
func TestSearchPostgresRankOracle(t *testing.T) {
	data, err := os.ReadFile("testdata/search-postgres-rank.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Title, Description, Query, Expanded string
		ScoreBits                           string `json:"score_bits"`
		Options                             []string
		Matched                             bool
		Last                                bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 264 {
		t.Fatal("source rank oracle coverage changed")
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%03d_%s", i, tc.Query), func(t *testing.T) {
			if expanded := normalizeSearchQuery(tc.Query); expanded != tc.Expanded || (expanded == tc.Query) != tc.Last {
				t.Fatal("source query strategy differs")
			}
			h := StoreSearchHit{Title: tc.Title, description: tc.Description, Values: map[string][]string{"option_values": tc.Options}}
			score, matched := searchRank(h, tc.Query)
			if matched != tc.Matched || fmt.Sprintf("%08x", math.Float32bits(score)) != tc.ScoreBits {
				t.Fatalf("match=%v score=%g (%08x); source match=%v score bits=%s", matched, score, math.Float32bits(score), tc.Matched, tc.ScoreBits)
			}
		})
	}
}
