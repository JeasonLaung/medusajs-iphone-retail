package rowimport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExactNumberPairs(t *testing.T) {
	pairs := []numberPair{{Raw: "raw_amount", Scalar: "amount"}}
	for _, c := range []struct {
		scalar, raw string
		valid       bool
	}{
		{`1.23`, `{"value":"1.2300000000000000000","precision":20}`, true},
		{`"9007199254740993"`, `{"value":"9007199254740993.0000","precision":20}`, true},
		{`"99999999999999999999.000000000000000001"`, `{"value":"99999999999999999999.000000000000000001","precision":38}`, true},
		{`0`, `{"precision":20,"value":"-0.0000000000000000000"}`, true},
		{`"0.000000000000000001"`, `{"value":"1.0000000000000000000e-18","precision":20}`, true},
		{`123`, `{"value":1.23e2,"precision":20}`, true},
		{`9007199254740992`, `{"value":"9007199254740993","precision":20}`, false},
		{`1.23`, `{"value":"1.2300000000000000001","precision":20}`, false},
		{`1.23`, `{"value":"1.23","value":"1.23","precision":20}`, false},
		{`1.23`, `{"value":"1.23","precision":20,"other":0}`, false},
		{`1.23`, `{"value":"1.23"}`, false},
		{`1.23`, `{"value":"1.23","precision":"20"}`, false},
		{`1.23`, `{"value":"1.23","precision":0}`, false},
		{`1.23`, `{"value":"1.23","precision":20.0}`, false},
		{`1.23`, `{"value":"1.23","precision":1000000001}`, false},
		{`1.23`, `{"value":"NaN","precision":20}`, false},
		{`1.23`, `{"value":"1/2","precision":20}`, false},
		{`1.23`, `{"value":"1e999999999","precision":20}`, false},
		{`1.23`, `{"value":null,"precision":20}`, false},
		{`1.23`, `null`, false},
	} {
		row := SourceRow{"amount": {Raw: json.RawMessage(c.scalar)}, "raw_amount": {Raw: json.RawMessage(c.raw)}}
		if err := checkNumberPairs(pairs, row); (err == nil) != c.valid {
			t.Fatal("exact pair acceptance mismatch")
		}
	}
	for _, scalarNull := range []bool{true, false} {
		for _, rawNull := range []bool{true, false} {
			row := SourceRow{"amount": {SQLNull: scalarNull}, "raw_amount": {SQLNull: rawNull}}
			if err := checkNumberPairs(pairs, row); (err == nil) != (scalarNull && rawNull) {
				t.Fatal("NULL pair acceptance mismatch")
			}
		}
	}
}

func TestEveryMappedPairAndProtectedBoundary(t *testing.T) {
	plans, err := compiledPlans()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, plan := range plans {
		for _, pair := range plan.RawPairs {
			count++
			row := SourceRow{}
			blocked := false
			for _, f := range plan.Fields {
				if strings.HasPrefix(f.Transform, "protected_") && !f.Nullable {
					blocked = true
				}
				if strings.HasPrefix(f.Transform, "protected_") && f.Nullable {
					row[f.Source] = Cell{SQLNull: true}
				} else {
					row[f.Source] = Cell{Raw: artificial(f)}
				}
			}
			if err := checkNumberPairs(plan.RawPairs, row); err != nil {
				t.Fatal("valid pair rejected", err)
			}
			if blocked {
				if _, err := Convert(plan.Source, row); err != ErrProtection {
					t.Fatal("protected row not held")
				}
				row[pair.Raw] = Cell{Raw: json.RawMessage(`{"value":"1.2300000000000000001","precision":20}`)}
				if err := checkNumberPairs(plan.RawPairs, row); err != ErrRow {
					t.Fatal("protected table pair mismatch accepted")
				}
				continue
			}
			bound, err := Convert(plan.Source, row)
			if err != nil {
				t.Fatal("paired table fixture rejected", plan.Source, err)
			}
			for i, col := range bound.Columns {
				for _, f := range plan.Fields {
					if f.Source == pair.Raw && f.Target == col && bound.Values[i] != string(row[pair.Raw].Raw) {
						t.Fatal("raw JSON changed")
					}
				}
			}
			row[pair.Raw] = Cell{Raw: json.RawMessage(`{"value":"1.2300000000000000001","precision":20}`)}
			bound, err = Convert(plan.Source, row)
			if err != ErrRow || len(bound.Values) != 0 {
				t.Fatal("mismatch returned partial row")
			}
		}
	}
	if count != 59 {
		t.Fatal("pair coverage drift", count)
	}
}
