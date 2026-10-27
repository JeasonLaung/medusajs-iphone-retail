package rowimport

import (
	"encoding/json"
	"testing"
)

func TestSourceCheckSQLNullAndExactNumericSemantics(t *testing.T) {
	for _, c := range []struct {
		kind, input string
		valid       bool
	}{
		{"true", `true`, true}, {"true", `false`, false}, {"true", `1`, false},
		{"nonnegative_integer", `0`, true}, {"nonnegative_integer", `9223372036854775807`, true},
		{"nonnegative_integer", `-1`, false}, {"nonnegative_integer", `9223372036854775808`, false},
		{"nonnegative_decimal", `"0.000000000000000001"`, true},
		{"nonnegative_decimal", `"-0.000000000000000001"`, false},
		{"nonnegative_decimal", `"-0.000000000000000000"`, true},
		{"nonnegative_decimal", `"99999999999999999999.000000000000000001"`, true},
		{"enum", `"draft"`, true}, {"enum", `"Draft"`, false}, {"enum", `"draft "`, false},
		{"enum", `null`, false}, {"enum", `7`, false}, {"unknown", `true`, false},
	} {
		check := sourceCheck{Name: "fixture", Kind: c.kind, Columns: []string{"value"}, Allowed: []string{"draft", "active"}}
		err := checkSourceConstraints([]sourceCheck{check}, SourceRow{"value": {Raw: json.RawMessage(c.input)}})
		if (err == nil) != c.valid {
			t.Fatal("CHECK acceptance mismatch", c.kind)
		}
	}
	for _, kind := range []string{"true", "nonnegative_integer", "nonnegative_decimal", "enum"} {
		check := sourceCheck{Name: "fixture", Kind: kind, Columns: []string{"value"}, Allowed: []string{"draft"}}
		if checkSourceConstraints([]sourceCheck{check}, SourceRow{"value": {SQLNull: true}}) != nil {
			t.Fatal("CHECK UNKNOWN rejected")
		}
		if checkSourceConstraints([]sourceCheck{check}, SourceRow{}) != ErrRow {
			t.Fatal("missing value accepted")
		}
	}
	for _, kind := range []string{"null_or_null", "null_or_not_null"} {
		for _, firstNull := range []bool{false, true} {
			for _, secondNull := range []bool{false, true} {
				check := sourceCheck{Name: "fixture", Kind: kind, Columns: []string{"parent", "other"}}
				row := SourceRow{"parent": {SQLNull: firstNull, Raw: nil}, "other": {SQLNull: secondNull, Raw: nil}}
				want := firstNull || secondNull
				if kind == "null_or_not_null" {
					want = firstNull || !secondNull
				}
				if err := checkSourceConstraints([]sourceCheck{check}, row); (err == nil) != want {
					t.Fatal("IS NULL disjunction mismatch")
				}
			}
		}
	}
}

func TestAllPinnedSourceChecks(t *testing.T) {
	plans, err := compiledPlans()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, plan := range plans {
		for _, check := range plan.Checks {
			count++
			row := SourceRow{}
			for _, f := range plan.Fields {
				row[f.Source] = Cell{Raw: artificial(f)}
			}
			satisfyFixtureChecks(plan, row)
			if checkSourceConstraints(plan.Checks, row) != nil {
				t.Fatal("valid source CHECK fixture rejected")
			}
			switch check.Kind {
			case "enum":
				row[check.Columns[0]] = Cell{Raw: json.RawMessage(`"unsupported"`)}
			case "true":
				row[check.Columns[0]] = Cell{Raw: json.RawMessage(`false`)}
			case "nonnegative_integer", "nonnegative_decimal":
				row[check.Columns[0]] = Cell{Raw: json.RawMessage(`-1`)}
			case "null_or_null":
				row[check.Columns[0]] = Cell{Raw: json.RawMessage(`"parent"`)}
				row[check.Columns[1]] = Cell{Raw: json.RawMessage(`"provider"`)}
			case "null_or_not_null":
				row[check.Columns[0]] = Cell{Raw: json.RawMessage(`"parent"`)}
				row[check.Columns[1]] = Cell{SQLNull: true}
			}
			if checkSourceConstraints([]sourceCheck{check}, row) != ErrRow {
				t.Fatal("invalid pinned CHECK accepted")
			}
		}
	}
	if count != 47 {
		t.Fatal("source CHECK coverage drift", count)
	}
}

func TestSourceChecksIntegratedBeforeBoundRowReturns(t *testing.T) {
	plans, _ := compiledPlans()
	for _, c := range []struct{ table, column, invalid string }{
		{"product", "status", `"not_a_status"`},
		{"price_list", "type", `"not_a_type"`},
		{"price_list", "status", `"not_a_status"`},
		{"price_rule", "operator", `"not_an_operator"`},
		{"order_number_clock", "id", `false`},
		{"sku_number_counter", "last_id", `-1`},
		{"cart_line_item", "unit_price", `-1`},
		{"cart_line_item_adjustment", "amount", `-1`},
		{"cart_shipping_method", "amount", `-1`},
	} {
		plan := plans[c.table]
		row := SourceRow{}
		for _, f := range plan.Fields {
			row[f.Source] = Cell{Raw: artificial(f)}
		}
		satisfyFixtureChecks(plan, row)
		if _, err := Convert(c.table, row); err != nil {
			t.Fatal("valid integration fixture rejected", err)
		}
		row[c.column] = Cell{Raw: json.RawMessage(c.invalid)}
		if _, paired := row["raw_"+c.column]; paired {
			row["raw_"+c.column] = Cell{Raw: json.RawMessage(`{"value":"-1.0000000000000000000","precision":20}`)}
			if checkNumberPairs(plan.RawPairs, row) != nil {
				t.Fatal("negative fixture pair is inconsistent")
			}
		}
		bound, err := Convert(c.table, row)
		if err != ErrRow || len(bound.Values) != 0 {
			t.Fatal("invalid source CHECK returned bound row")
		}
	}
}
