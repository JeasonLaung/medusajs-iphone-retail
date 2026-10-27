package rowimport

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func artificial(f field) json.RawMessage {
	if strings.HasPrefix(f.Source, "raw_") {
		return json.RawMessage(`{"value":"1.2300000000000000000","precision":20}`)
	}
	switch f.Transform {
	case "strict_boolean":
		return json.RawMessage(`false`)
	case "checked_int32", "checked_int64", "minor_unit_integer":
		return json.RawMessage(`7`)
	case "utc_datetime6":
		return json.RawMessage(`"2026-10-27T08:00:00.123456+08:00"`)
	case "exact_json_text", "protected_exact_json":
		return json.RawMessage(` {"n":9007199254740993,"n":1.2300} `)
	case "text_array_json":
		return json.RawMessage(`["a",null,""]`)
	case "finite_float32_to_float64":
		return json.RawMessage(`1.25`)
	case "money_major_decimal", "quantity_decimal", "rate_decimal", "budget_decimal", "promotion_value_decimal", "legacy_float_rate_decimal":
		return json.RawMessage(`"1.2300"`)
	case "enum_text":
		value, _ := json.Marshal(f.Allowed[0])
		return value
	default:
		return json.RawMessage(`" Exact value "`)
	}
}

func satisfyFixtureChecks(plan tablePlan, row SourceRow) {
	for _, check := range plan.Checks {
		switch check.Kind {
		case "enum":
			raw, _ := json.Marshal(check.Allowed[0])
			row[check.Columns[0]] = Cell{Raw: raw}
		case "true":
			row[check.Columns[0]] = Cell{Raw: json.RawMessage(`true`)}
		case "null_or_null", "null_or_not_null":
			row[check.Columns[0]] = Cell{SQLNull: true}
		}
	}
}

func TestAllPinnedFields(t *testing.T) {
	var program struct {
		Tables []tablePlan `json:"tables"`
	}
	if err := json.Unmarshal(fieldPlans, &program); err != nil {
		t.Fatal(err)
	}
	if len(program.Tables) != 159 {
		t.Fatal("table coverage drift")
	}
	count := 0
	for _, plan := range program.Tables {
		t.Run(plan.Source, func(t *testing.T) {
			row := SourceRow{}
			blocked := false
			for _, f := range plan.Fields {
				count++
				raw := artificial(f)
				row[f.Source] = Cell{Raw: raw}
				_, err := convertValue(f, raw)
				if strings.HasPrefix(f.Transform, "protected_") {
					blocked = true
					if err != ErrProtection {
						t.Fatal("protected input not held")
					}
				} else if err != nil {
					t.Fatal("valid mapped field rejected", f.Source, err)
				}
			}
			satisfyFixtureChecks(plan, row)
			bound, err := Convert(plan.Source, row)
			if plan.Source == "customer" || plan.Source == "mail_message" || plan.Source == "payment_session" || plan.Source == "payment" || plan.Source == "account_holder" {
				if err != ErrHistoryPolicy || len(bound.Values) != 0 {
					t.Fatal("mandatory history policy bypassed")
				}
				return
			}
			if blocked {
				if err != ErrProtection {
					t.Fatal("protected row partially returned", err)
				}
				if len(bound.Values) != 0 {
					t.Fatal("partial row returned")
				}
			} else if err != nil || len(bound.Columns) != len(plan.Fields) || bound.Table != plan.Target {
				t.Fatal("field program mismatch", err)
			}
			delete(row, plan.Fields[0].Source)
			if _, err := Convert(plan.Source, row); err != ErrRow {
				t.Fatal("missing field not rejected")
			}
			row[plan.Fields[0].Source] = Cell{Raw: artificial(plan.Fields[0])}
			row["unknown_field"] = Cell{Raw: json.RawMessage(`1`)}
			if _, err := Convert(plan.Source, row); err != ErrRow {
				t.Fatal("unknown field not rejected")
			}
		})
	}
	if count != 1691 {
		t.Fatal("field coverage drift", count)
	}
}

func TestNullAndJSONDocumentAreDistinct(t *testing.T) {
	// Use a real small mapped table rather than an invented transform program.
	var program struct {
		Tables []tablePlan `json:"tables"`
	}
	_ = json.Unmarshal(fieldPlans, &program)
	for _, plan := range program.Tables {
		if plan.Source != "product_tag" {
			continue
		}
		row := SourceRow{}
		for _, f := range plan.Fields {
			row[f.Source] = Cell{Raw: artificial(f)}
		}
		for _, f := range plan.Fields {
			original := row[f.Source]
			row[f.Source] = Cell{SQLNull: true}
			bound, err := Convert(plan.Source, row)
			if !f.Nullable {
				if err != ErrRow {
					t.Fatal("required NULL accepted")
				}
			} else if err != nil {
				t.Fatal("explicit nullable SQL NULL rejected", err)
			} else {
				for i, col := range bound.Columns {
					if col == f.Target && bound.Values[i] != nil {
						t.Fatal("SQL NULL changed")
					}
				}
			}
			row[f.Source] = Cell{SQLNull: true, Raw: json.RawMessage(`null`)}
			if _, err := Convert(plan.Source, row); err != ErrRow {
				t.Fatal("ambiguous null representation accepted")
			}
			row[f.Source] = original
		}
	}
	f := field{Transform: "exact_json_text"}
	v, err := convertValue(f, json.RawMessage(`null`))
	if err != nil || v != "null" {
		t.Fatal("JSON null document changed")
	}
	if _, err := Convert("user", SourceRow{}); err != ErrRow {
		t.Fatal("excluded auth table accepted")
	}
}

func TestIndependentConversionCases(t *testing.T) {
	for _, c := range []struct {
		transform, input string
		want             any
	}{
		{"strict_boolean", `false`, false},
		{"checked_int64", `9223372036854775807`, int64(9223372036854775807)},
		{"money_major_decimal", `"99999999999999999999.000000000000000001"`, "99999999999999999999.000000000000000001"},
		{"utc_datetime6", `"2026-10-27T08:00:00.123456+08:00"`, "2026-10-27 00:00:00.123456"},
		{"text_array_json", `["1",null,"", "a,b"]`, `["1",null,"", "a,b"]`},
		{"finite_float32_to_float64", `1.5`, float64(1.5)},
		{"identity_string", `" ID "`, " ID "},
	} {
		t.Run(c.transform, func(t *testing.T) {
			v, err := convertValue(field{Transform: c.transform}, json.RawMessage(c.input))
			if err != nil || !reflect.DeepEqual(v, c.want) {
				t.Fatal("independent expected value mismatch", err)
			}
		})
	}
	for _, c := range []struct{ transform, input string }{
		{"strict_boolean", `0`}, {"checked_int32", `2147483648`}, {"money_major_decimal", `1e-19`},
		{"text_array_json", `[1]`}, {"text_array_json", `null`}, {"finite_float32_to_float64", `1e100`},
		{"utf8_text", `"\ud800"`}, {"utc_datetime6", `null`}, {"unknown", `"x"`},
	} {
		if _, err := convertValue(field{Transform: c.transform}, json.RawMessage(c.input)); err != ErrRow {
			t.Fatal("unsafe conversion accepted", c.transform)
		}
	}
	f := field{Transform: "enum_text", Allowed: []string{"pending", "completed"}}
	if _, err := convertValue(f, json.RawMessage(`"other"`)); err != ErrRow {
		t.Fatal("unknown enum accepted")
	}
}
