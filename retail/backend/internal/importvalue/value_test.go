package importvalue

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExactDecimals(t *testing.T) {
	for input, want := range map[string]string{`99999999999999999999.999999999999999999`: "99999999999999999999.999999999999999999", `"-0.000000000000000001"`: "-0.000000000000000001", `1e-18`: "0.000000000000000001", `1.2300e2`: "123.00", `1e19`: "10000000000000000000", `0`: "0", `"-0.000"`: "-0.000"} {
		got, err := Decimal38_18(json.RawMessage(input))
		if err != nil || got != want {
			t.Fatalf("decimal artificial case mismatch: %s => %s (%v)", input, got, err)
		}
	}
	for _, input := range []string{`1e20`, `1e-19`, `0.0000000000000000000`, `"1.0000000000000000000"`, `NaN`, `Infinity`, `true`, `null`, `" 1"`, `"+1"`, `"01"`, `1e1000000`, `"1/3"`, `""`, `1 2`} {
		if _, err := Decimal38_18(json.RawMessage(input)); err != ErrValue {
			t.Fatal("invalid decimal accepted", input)
		}
	}
}

func TestInstants(t *testing.T) {
	for input, want := range map[string]string{`"2026-10-27T08:00:00.123456+08:00"`: "2026-10-27T00:00:00.123456Z", `"2026-01-01T00:00:00+01:00"`: "2025-12-31T23:00:00Z"} {
		got, err := Instant(json.RawMessage(input))
		if err != nil || got.Format("2006-01-02T15:04:05.999999Z07:00") != want {
			t.Fatal("instant conversion mismatch", err)
		}
	}
	for _, input := range []string{`null`, `""`, `"2026-02-30T00:00:00Z"`, `"2026-01-01T00:00:00.1234567Z"`, `"2026-01-01T00:00:00.0000000Z"`, `"2026-01-01T00:00:00-00:00"`, `"2026-01-01T00:00:00+24:00"`, `"2026-01-01T00:00:00,1Z"`, `"0999-12-31T23:00:00Z"`, `"1000-01-01T00:00:00+01:00"`} {
		if _, err := Instant(json.RawMessage(input)); err != ErrValue {
			t.Fatal("invalid instant accepted", input)
		}
	}
}

func TestTextAndRawJSON(t *testing.T) {
	for _, value := range []string{" ID ", "é", "e\u0301", strings.Repeat("🚀", 191), "", "\ufffd"} {
		raw, _ := json.Marshal(value)
		got, err := Text(raw, 191)
		if err != nil || got != value {
			t.Fatal("identity changed", err)
		}
	}
	raw, _ := json.Marshal(strings.Repeat("🚀", 192))
	if _, err := Text(raw, 191); err != ErrValue {
		t.Fatal("overlength ID accepted")
	}
	for _, raw := range [][]byte{[]byte(`123`), []byte(`null`), []byte(`"\ud800"`), []byte(`"\udc00"`), {'"', 0xff, '"'}} {
		if _, err := Text(raw, 0); err != ErrValue {
			t.Fatal("invalid text accepted")
		}
	}
	for _, raw := range []string{` {"n":9007199254740993,"n":1.2300} `, `null`, `"\\ud800"`, `"\ud83d\ude80"`} {
		got, err := JSONText(json.RawMessage(raw))
		if err != nil || got != raw {
			t.Fatal("raw JSON changed", err)
		}
	}
	if _, err := JSONText(json.RawMessage(`{"secret":"\ud800"}`)); err != ErrValue {
		t.Fatal("unpaired JSON escape accepted")
	}
}

func TestBooleansIntegersAndErrors(t *testing.T) {
	if v, err := Boolean(json.RawMessage(`false`)); v || err != nil {
		t.Fatal("false lost")
	}
	for _, raw := range []string{`0`, `"false"`, `null`, `FALSE`, ``} {
		if _, err := Boolean(json.RawMessage(raw)); err != ErrValue {
			t.Fatal("boolean coercion accepted")
		}
	}
	for _, bits := range []int{32, 64} {
		for _, raw := range []string{`1.0`, `1e1`, `"1"`, `null`, `9223372036854775808`} {
			if _, err := Integer(json.RawMessage(raw), bits); err != ErrValue {
				t.Fatal("integer coercion accepted")
			}
		}
	}
	if v, err := Integer(json.RawMessage(`9223372036854775807`), 64); err != nil || v != 9223372036854775807 {
		t.Fatal("int64 precision lost")
	}
	if _, err := Integer(json.RawMessage(`2147483648`), 32); err != ErrValue {
		t.Fatal("int32 overflow")
	}
	if strings.Contains(ErrValue.Error(), "source-secret") {
		t.Fatal("error disclosed artificial data")
	}
}
