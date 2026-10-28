package cart

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/totals"
)

func TestItemAmountStoresExactRawAndExplicitScalar(t *testing.T) {
	for _, c := range []struct{ value, scalar string }{
		{"0", "0"}, {"100.000000000000000000001", "100.000000000000000000"},
		{"99999999999999999999.999999999999999999999", "99999999999999999999.999999999999999999"},
		{"0.000000000000000000001", "0.000000000000000000"},
	} {
		scalar, raw, err := sqlAmount(c.value)
		if err != nil || scalar != c.scalar {
			t.Fatal("stored cart scalar rounded or rejected")
		}
		n, err := totals.StoredNumber(scalar, &raw)
		if err != nil {
			t.Fatal("stored cart raw invalid")
		}
		effective, err := totals.Resolve(n)
		if err != nil || effective != c.value {
			t.Fatal("stored cart effective precision lost")
		}
		var object map[string]any
		if json.Unmarshal([]byte(raw), &object) != nil || len(object) != 2 {
			t.Fatal("raw money projection differs")
		}
	}
	for _, value := range []string{"-1", "01", "1.00", "1e2", "100000000000000000000", "NaN", "1\n", ""} {
		if _, _, err := sqlAmount(value); err != ErrItemUnavailable {
			t.Fatal("unrepresentable cart price accepted")
		}
	}
}

func TestDecodeItemRejectsAmbiguousInput(t *testing.T) {
	good := `{"version":"1","variant_id":"v","quantity":"2"}`
	got, err := decodeItem(strings.NewReader(good))
	if err != nil || got.Version != "1" || got.VariantID != "v" || got.Quantity != "2" || got.OperationID != "" {
		t.Fatal("canonical item wire input differs")
	}
	for _, bad := range []string{`null`, `[]`, `{}`, `{"version":null,"variant_id":"v","quantity":"2"}`, `{"version":"1","variant_id":"v","quantity":2}`, `{"version":"1","variant_id":"v","quantity":"2","quantity":"3"}`, `{"version":"1","variant_id":"v","quantity":"2","price":"0"}`, good + ` {}`} {
		if _, err := decodeItem(strings.NewReader(bad)); err == nil {
			t.Fatal("ambiguous item wire input accepted")
		}
	}
}
