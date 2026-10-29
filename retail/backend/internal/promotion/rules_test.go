package promotion

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestOriginalPromotionPredicates(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/promotion-rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name, Scope string
			Context     json.RawMessage
			Rules       []Rule
			Expected    bool
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 34 {
		t.Fatal("promotion fixture coverage changed")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			before, _ := json.Marshal(row)
			got, err := MatchRules(row.Context, row.Rules, row.Scope)
			if err != nil || got != row.Expected {
				t.Fatalf("got %v %v want %v", got, err, row.Expected)
			}
			after, _ := json.Marshal(row)
			if string(before) != string(after) {
				t.Fatal("promotion input changed")
			}
		})
	}
}

func TestPromotionRulesValidationAndExactness(t *testing.T) {
	rule := func(attribute, operator, value string) Rule {
		return Rule{Attribute: attribute, Operator: operator, Values: []RuleValue{{Value: json.RawMessage(value)}}}
	}
	for _, row := range []struct {
		context string
		rules   []Rule
		scope   string
	}{
		{`[]`, nil, ""}, {`{} {}`, nil, ""}, {`{}`, nil, "bad"},
		{`{}`, []Rule{rule("", "eq", `"a"`)}, ""},
		{`{}`, []Rule{rule("x", "nin", `"a"`)}, ""},
		{`{}`, []Rule{rule("x", "eq", `1`)}, ""},
		{`{}`, []Rule{rule("x", "eq", `null`)}, ""},
		{`{"x":"a"}`, []Rule{rule("x", "gt", `"10"`)}, ""},
		{`{"x":"1"}`, []Rule{rule("x", "gt", `"1e1000000"`)}, ""},
		{`{"items":[null]}`, []Rule{rule("items.x", "ne", `"a"`)}, ""},
		{`{"x":"a"}`, []Rule{rule("x", "eq", `"mismatch"`), rule("x", "invalid", `"a"`)}, ""},
		{`{}`, make([]Rule, 1001), ""},
		{`{"x":"a"}`, []Rule{rule("x", "eq", `"mismatch"`), rule("x", "gt", `"invalid"`)}, ""},
		{`{}`, []Rule{rule(strings.Repeat("x", 192), "eq", `"a"`)}, ""},
	} {
		if _, err := MatchRules(json.RawMessage(row.context), row.rules, row.scope); err != ErrRules {
			t.Fatalf("malformed rule/context accepted: %v", err)
		}
	}
	for _, row := range []struct {
		context, value string
		want           bool
	}{
		{`{"x":"0.10000000000000000001"}`, `"0.1"`, true},
		{`{"x":"1e-30"}`, `"0.0000000000000000000000000000009"`, true},
		{`{"x":"9007199254740993"}`, `"9007199254740992"`, true},
		{`{"x":9007199254740993}`, `"9007199254740992"`, true},
	} {
		got, err := MatchRules(json.RawMessage(row.context), []Rule{rule("x", "gt", row.value)}, "")
		if err != nil || got != row.want {
			t.Fatal("promotion decimal comparison rounded")
		}
	}
}

func TestPromotionJSONNumberStringCoercion(t *testing.T) {
	for _, row := range []struct{ raw, want string }{
		{"-0", "0"}, {"1e-7", "1e-7"}, {"1e-6", "0.000001"},
		{"1e20", "100000000000000000000"}, {"1e21", "1e+21"},
		{"1e309", "Infinity"}, {"-1e309", "-Infinity"},
	} {
		encoded, _ := json.Marshal(row.want)
		got, err := MatchRules(json.RawMessage(`{"x":`+row.raw+`}`), []Rule{{Attribute: "x", Operator: "eq", Values: []RuleValue{{Value: encoded}}}}, "")
		if err != nil || !got {
			t.Fatalf("number %s coercion: %v %v", row.raw, got, err)
		}
	}
}
