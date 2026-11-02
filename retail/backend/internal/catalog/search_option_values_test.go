package catalog

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSearchSourceOptionValues(t *testing.T) {
	data, err := os.ReadFile("testdata/search-option-values.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Options []AdminOption
		Values  []string
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 12 {
		t.Fatal("source option oracle coverage changed")
	}
	for i, row := range rows {
		if got := searchOptionValues(row.Options); !reflect.DeepEqual(got, row.Values) {
			t.Fatalf("source case %d: got %v want %v", i, got, row.Values)
		}
	}
}

// The boundary is in characters, matching option editor validation, not bytes.
func TestSearchQualifiedOptionLength(t *testing.T) {
	for _, tc := range []struct {
		attribute, value string
		valid            bool
	}{
		{"option_values", strings.Repeat("型", 100) + ":" + strings.Repeat("值", 500), true},
		{"option_values", strings.Repeat("x", 602), false},
		{"model", strings.Repeat("x", 192), false},
		{"model", strings.Repeat("x", 191), true},
	} {
		raw, _ := json.Marshal(StoreSearchRequest{RegionID: "europe", Queries: []StoreSearchQuery{{Key: "main", Filters: map[string][]string{tc.attribute: {tc.value}}}}})
		_, err := parseSearch(raw)
		if (err == nil) != tc.valid {
			t.Fatalf("filter %s length=%d valid=%v error=%v", tc.attribute, len([]rune(tc.value)), tc.valid, err)
		}
	}
}
