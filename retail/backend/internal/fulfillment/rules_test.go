package fulfillment

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestOriginalShippingRules(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/shipping-rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name     string
			Context  json.RawMessage
			Rules    []Rule
			Some     bool
			Expected bool
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 45 {
		t.Fatal("shipping evidence coverage changed")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			before, _ := json.Marshal(row)
			got, err := MatchRules(row.Context, row.Rules, row.Some)
			if err != nil || got != row.Expected {
				t.Fatalf("got %v, %v; want %v", got, err, row.Expected)
			}
			after, _ := json.Marshal(row)
			if string(before) != string(after) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestShippingRulesExactAndValidation(t *testing.T) {
	if _, err := MatchRules(json.RawMessage(`{"items":[null]}`), []Rule{{Attribute: "items.country", Operator: "nin", Value: json.RawMessage(`["dk"]`)}}, false); err != ErrRules {
		t.Fatal("malformed nested null must not grant negative-rule eligibility")
	}
	for _, test := range []struct {
		context, operator, value string
		expected                 bool
	}{
		{`{"x":"9007199254740993"}`, "gt", `"9007199254740992"`, true},
		{`{"x":"0.10000000000000000001"}`, "gt", `"0.1"`, true},
		{`{"x":"9007199254740993"}`, "gt", `9007199254740992`, true},
		{`{"x":"0.10000000000000000001"}`, "gt", `0.1`, true},
		{`{"x":false}`, "eq", `false`, true},
		{`{"x":0}`, "gte", `0`, true},
		{`{"x":0}`, "eq", `0`, false}, // strict string-vs-number equality remains
		{`{"x":"2026-10-28T00:00:00Z"}`, "gt", `"2026-10-27T23:59:59Z"`, true},
	} {
		got, err := MatchRules(json.RawMessage(test.context), []Rule{{Attribute: "x", Operator: test.operator, Value: json.RawMessage(test.value)}}, false)
		if err != nil || got != test.expected {
			t.Fatalf("%s/%s: %v %v", test.context, test.operator, got, err)
		}
	}
	for _, rule := range []Rule{
		{Attribute: "x", Operator: "unknown", Value: json.RawMessage(`"x"`)},
		{Attribute: "", Operator: "eq", Value: json.RawMessage(`"x"`)},
		{Attribute: "x", Operator: "eq", Value: json.RawMessage(`null`)},
		{Attribute: "x", Operator: "eq", Value: json.RawMessage(`{}`)},
		{Attribute: "x", Operator: "eq", Value: json.RawMessage(`[]`)},
		{Attribute: "x", Operator: "in", Value: json.RawMessage(`"x"`)},
		{Attribute: "x", Operator: "eq", Value: json.RawMessage(`""`)},
		{Attribute: "x", Operator: "eq", Value: json.RawMessage(`"x" true`)},
		{Attribute: strings.Repeat("a.", 33), Operator: "eq", Value: json.RawMessage(`"x"`)},
	} {
		// A false first rule must not hide an invalid later rule.
		_, err := MatchRules(json.RawMessage(`{}`), []Rule{{Attribute: "missing", Operator: "eq", Value: json.RawMessage(`"yes"`)}, rule}, false)
		if err != ErrRules {
			t.Fatalf("invalid rule accepted: %v", err)
		}
	}
	for _, context := range []string{`null`, `[]`, `{} {}`, `{`, strings.Repeat(" ", 1<<20) + `{}`} {
		if _, err := MatchRules(json.RawMessage(context), nil, false); err != ErrRules {
			t.Fatal("invalid context accepted")
		}
	}
}
