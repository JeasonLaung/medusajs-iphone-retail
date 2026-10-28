package totals

import (
	"strings"
	"testing"
)

func TestStoredNumberNeverRoundsOrFallsBack(t *testing.T) {
	for _, c := range []struct {
		scalar string
		raw    *string
		want   string
	}{
		{"9007199254740993.000000000000000001", nil, "9007199254740993.000000000000000001"},
		{"1", ptr(`{"value":"1.000000000000000000001","precision":38}`), "1.000000000000000000001"},
		{"1", ptr(`{"value":1e-20,"precision":20}`), "0.00000000000000000001"},
		{"-2", nil, "-2"}, // sign restrictions belong to the amount/tax calculator.
	} {
		n, err := StoredNumber(c.scalar, c.raw)
		if err != nil {
			t.Fatal("valid exact stored number rejected")
		}
		got, err := Resolve(n)
		if err != nil || got != c.want {
			t.Fatal("exact stored number changed")
		}
	}
	for _, raw := range []string{`{}`, `null`, `[]`, `{"value":"1"}`, `{"value":"1","precision":0}`, `{"value":"1","precision":101}`, `{"value":null,"precision":20}`, `{"value":true,"precision":20}`, `{"value":"NaN","precision":20}`, `{"value":"1","value":"2","precision":20}`, `{"value":"1","precision":20,"precision":30}`, `{"value":"1","precision":20,"private":"x"}`, `{"value":"1","precision":20} {}`, `{"value":"1","precision":2.5}`, `{"value":"1e999","precision":20}`, string([]byte{0xff}), strings.Repeat(" ", 4097)} {
		if _, err := StoredNumber("7", &raw); err != ErrInput {
			t.Fatal("malformed raw number silently used scalar")
		}
	}
	for _, scalar := range []string{"", "NaN", " 1", "1\n", "1/2"} {
		if _, err := StoredNumber(scalar, nil); err != ErrInput {
			t.Fatal("invalid stored scalar accepted")
		}
	}
}

func ptr(s string) *string { return &s }
