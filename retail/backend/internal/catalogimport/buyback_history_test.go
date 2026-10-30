package catalogimport

import (
	"context"
	"testing"
)

func TestBuybackHistoricalItemOwnership(t *testing.T) {
	for _, c := range []struct {
		raw, id string
		want    bool
	}{
		{`[{"id":"one"},{"id":"two"}]`, "two", true},
		{`[{"id":"one"}]`, "ONE", false},
		{`[{"id":"one"}]`, "foreign", false},
		{`[{"id":"one"},{"id":"one"}]`, "one", false},
		{`[{"id":"one"},{}]`, "one", false},
		{`null`, "one", false},
		{`{"id":"one"}`, "one", false},
		{`[{"id":1}]`, "1", false},
	} {
		if buybackHasItem(c.raw, c.id) != c.want {
			t.Fatal("historical item relationship differs")
		}
	}
	if ImportBuybackHistory(context.Background(), nil, "", "buyback_request", nil) != ErrPolicy {
		t.Fatal("nil transaction accepted")
	}
}
