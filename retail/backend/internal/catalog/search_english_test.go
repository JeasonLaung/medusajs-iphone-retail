package catalog

import "testing"

func TestSearchEnglishSourceRegressions(t *testing.T) {
	h := StoreSearchHit{Title: "iPhone 13 Pro Max", description: "Stock quantity has not been verified. Supplied catalog snapshot.", Values: map[string][]string{"option_values": {"Refurbished", "128 GB"}, "category": {"iPhone"}}}
	for _, tc := range []struct {
		query string
		match bool
	}{
		{"quantities", true}, {"verifying", true}, {"supplies", true},
		{"the", false}, {"THE AND OF", false}, {"", true}, {"   ", true},
		{"the iphone", true}, {"iphone the", true}, {"iphone pr", true},
		{"iphone unfindable", false}, {"i13 unfindable", false},
		{"i13pm", true}, {"i13p", true}, {"i13mini", false},
		{"refurbishing 128", true}, {"苹果", false}, {"%", false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			if _, got := searchRank(h, tc.query); got != tc.match {
				t.Fatalf("query %q: match %v, want %v", tc.query, got, tc.match)
			}
		})
	}
}

func TestSearchEnglishStopwordsBeforeStemming(t *testing.T) {
	if len(searchEnglishStopwords) != 127 {
		t.Fatal("PostgreSQL English stopword list changed")
	}
	// 'only' stems to 'onli', so filtering after stemming would retain it.
	if got := searchTerms("ONLY the verifying quantities"); len(got) != 2 || got[0] != "verifi" || got[1] != "quantiti" {
		t.Fatalf("unexpected lexemes: %v", got)
	}
}
