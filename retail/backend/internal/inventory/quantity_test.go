package inventory

import "testing"

func TestExactInventoryQuantities(t *testing.T) {
	for _, c := range []struct{ a, b, want string }{
		{"9007199254740993.000000000000000001", "9007199254740993", "0.000000000000000001"},
		{"2.5", "3.75", "-1.250000000000000000"}, {"0", "-0.1", "0.100000000000000000"},
		{"99999999999999999999.999999999999999999", "-0.000000000000000001", "100000000000000000000.000000000000000000"},
	} {
		got, err := Difference(c.a, c.b)
		if err != nil || got != c.want {
			t.Fatal("quantity arithmetic lost precision")
		}
	}
	eq, err := Equal("1.00", "1.000000000000000000")
	if err != nil || !eq {
		t.Fatal("equivalent quantities differ")
	}
	for _, bad := range []string{"NaN", "1e3", "+1", "01", " 1", "1.0000000000000000001", ""} {
		if _, err := Difference(bad, "0"); err == nil {
			t.Fatal("invalid quantity accepted")
		}
	}
}
