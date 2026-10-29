package orders

import (
	"testing"
	"time"
)

func TestOrderNumberIntegerClock(t *testing.T) {
	for _, sample := range []struct {
		tick int64
		want string
	}{
		{0, "1970010108000000000"},
		{99999, "1970010108000099999"},
		{100000, "1970010108000100000"},
		{179328959999999, "2026102923595999999"},
		{179328960000000, "2026103000000000000"},
	} {
		got, err := customOrderNumber(sample.tick)
		if err != nil || got != sample.want {
			t.Fatalf("tick %d: %s %v", sample.tick, got, err)
		}
		back, err := parseCustomOrderNumber(got)
		if err != nil || back != sample.tick {
			t.Fatal("integer clock roundtrip")
		}
	}
	last := time.Date(9999, 12, 31, 23, 59, 59, 999990000, orderNumberZone).UnixMicro() / 10
	if _, err := customOrderNumber(last); err != nil {
		t.Fatal("last representable tick rejected")
	}
	if _, err := customOrderNumber(last + 1); err == nil {
		t.Fatal("year overflow accepted")
	}
	for _, bad := range []string{"", "202610300000000000", "20261030000000000000", "2026023000000000000", "20261030000000+0000", "20261030000000０0000", "1969123123595999999"} {
		if _, err := parseCustomOrderNumber(bad); err == nil {
			t.Fatalf("invalid order number accepted: %q", bad)
		}
	}
	if _, err := customOrderNumber(-1); err == nil {
		t.Fatal("negative clock accepted")
	}
}

func TestOrderNumberNeedsExplicitTransaction(t *testing.T) {
	if _, err := AllocateOrderNumbers(t.Context(), nil); err != ErrNumbering {
		t.Fatal("nil allocator accepted")
	}
	if err := SeedOrderNumbers(t.Context(), nil, OrderNumberSeed{}); err != ErrNumberSeed {
		t.Fatal("unreviewed seed accepted")
	}
}
