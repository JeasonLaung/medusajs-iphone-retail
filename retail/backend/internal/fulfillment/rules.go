package fulfillment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/totals"
)

var ErrRules = errors.New("invalid shipping rules or context")

type Rule struct {
	Attribute string          `json:"attribute"`
	Operator  string          `json:"operator"`
	Value     json.RawMessage `json:"value"`
}

// MatchRules is a pure predicate over trusted, server-assembled context, not
// shipping eligibility or a public filter. It validates every rule before
// evaluation (including rules after a short-circuit). No input is mutated.
// some=true is the original optional OR mode; shipping selection uses false.
func MatchRules(context json.RawMessage, rules []Rule, some bool) (bool, error) {
	if len(context) > 1<<20 || len(rules) > 1000 {
		return false, ErrRules
	}
	value, err := decodeRuleJSON(context)
	if err != nil {
		return false, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false, ErrRules
	}
	values := make([]any, len(rules))
	for i, rule := range rules {
		if rule.Attribute == "" || len(rule.Attribute) > 191 || !utf8.ValidString(rule.Attribute) || len(strings.Split(rule.Attribute, ".")) > 32 || len(rule.Value) > 65536 {
			return false, ErrRules
		}
		v, err := decodeRuleJSON(rule.Value)
		if err != nil || v == nil {
			return false, ErrRules
		}
		switch rule.Operator {
		case "in", "nin":
			if _, ok := v.([]any); !ok {
				return false, ErrRules
			}
		case "eq", "ne", "gt", "gte", "lt", "lte":
			switch v.(type) {
			case string, bool, json.Number:
			default:
				return false, ErrRules
			}
			if v == "" {
				return false, ErrRules
			}
			// Correct the old validation bug: false and 0 are valid values.
			if b, ok := v.(bool); ok {
				v = strconv.FormatBool(b)
			}
		default:
			return false, ErrRules
		}
		values[i] = v
	}
	result := !some
	for i, rule := range rules {
		v, err := pickRuleValue(object, strings.Split(rule.Attribute, "."))
		if err != nil {
			return false, err
		}
		text := ruleText(v, false)
		matched := false
		switch rule.Operator {
		case "eq", "ne":
			s, isString := values[i].(string)
			matched = isString && text == s
			if rule.Operator == "ne" {
				matched = !matched
			}
		case "in", "nin":
			for _, entry := range values[i].([]any) {
				if s, ok := entry.(string); ok && s == text {
					matched = true
				}
			}
			if rule.Operator == "nin" {
				matched = !matched
			}
		default:
			right := ruleText(values[i], false)
			if n, ok := values[i].(json.Number); ok {
				right = string(n) // Do not round a numeric rule threshold through float64.
			}
			comparison, comparable := compareRuleValues(text, right)
			if comparable {
				switch rule.Operator {
				case "gt":
					matched = comparison > 0
				case "gte":
					matched = comparison >= 0
				case "lt":
					matched = comparison < 0
				case "lte":
					matched = comparison <= 0
				}
			}
		}
		if some {
			result = result || matched
		} else {
			result = result && matched
		}
	}
	return result, nil
}

func decodeRuleJSON(raw []byte) (any, error) {
	if !utf8.Valid(raw) {
		return nil, ErrRules
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, ErrRules
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, ErrRules
	}
	return v, nil
}

type missingRuleValue struct{}

// The original picker stops at a scalar even if path segments remain, and
// maps/one-level flattens arrays before the predicate stringifies the result.
func pickRuleValue(value any, path []string) (any, error) {
	if len(path) == 0 {
		return value, nil
	}
	// Accessing a property of null throws in the source. Do not turn that
	// malformed nested context into a successful negative rule.
	if value == nil {
		return nil, ErrRules
	}
	object, ok := value.(map[string]any)
	if !ok {
		return missingRuleValue{}, nil
	}
	v, exists := object[path[0]]
	if !exists {
		return missingRuleValue{}, nil
	}
	if len(path) == 1 {
		return v, nil
	}
	if array, ok := v.([]any); ok {
		out := []any{}
		for _, item := range array {
			picked, err := pickRuleValue(item, path[1:])
			if err != nil {
				return nil, err
			}
			if nested, ok := picked.([]any); ok {
				out = append(out, nested...)
			} else {
				out = append(out, picked)
			}
		}
		return out, nil
	}
	if _, ok := v.(map[string]any); ok {
		return pickRuleValue(v, path[1:])
	}
	return v, nil
}

func ruleText(value any, arrayElement bool) string {
	switch v := value.(type) {
	case missingRuleValue:
		if arrayElement {
			return ""
		}
		return "undefined"
	case nil:
		if arrayElement {
			return ""
		}
		return "null"
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		// JSON numeric context follows the old JS Number string coercion.
		// Monetary comparisons should use exact decimal strings instead.
		f, err := v.Float64()
		if err != nil {
			return "NaN"
		}
		b, err := json.Marshal(f)
		if err != nil {
			return "NaN"
		}
		return string(b)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = ruleText(item, true)
		}
		return strings.Join(parts, ",")
	default:
		return "[object Object]"
	}
}

var ruleDecimal = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func ruleNumber(s string) (*big.Rat, bool) {
	s = strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '\ufeff' })
	if s == "" {
		s = "0"
	}
	if len(s) > 256 {
		return nil, false
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") || strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") || strings.HasPrefix(s, "0o") || strings.HasPrefix(s, "0O") {
		// Number accepts unsigned radix literals; signed ones are NaN.
		base := map[byte]int{'x': 16, 'X': 16, 'b': 2, 'B': 2, 'o': 8, 'O': 8}[s[1]]
		if len(s[2:]) == 0 || len(s[2:]) > 128 {
			return nil, false
		}
		n, ok := new(big.Int).SetString(s[2:], base)
		if !ok || n.Sign() < 0 || strings.ContainsAny(s[2:], "_+-") {
			return nil, false
		}
		return new(big.Rat).SetInt(n), true
	}
	if !ruleDecimal.MatchString(s) {
		return nil, false
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	exponent := ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exponent = s[i:]
		s = s[:i]
	}
	parts := strings.SplitN(s, ".", 2)
	whole := strings.TrimLeft(parts[0], "0")
	if whole == "" {
		whole = "0"
	}
	s = whole
	if len(parts) == 2 && parts[1] != "" {
		s += "." + parts[1]
	}
	if negative {
		s = "-" + s
	}
	resolved, err := totals.Resolve(totals.Number{Decimal: s + exponent})
	if err != nil {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(resolved)
	return n, ok
}

func compareRuleValues(left, right string) (int, bool) {
	// Preserve exact decimals instead of the original lossy Number comparison.
	// Normalize Number's whitespace/empty-string zero behavior explicitly.
	a, aok := ruleNumber(left)
	b, bok := ruleNumber(right)
	if aok && bok {
		return a.Cmp(b), true
	}
	// ISO dates use an explicit timezone (date-only is UTC). Never use the
	// machine's locale/timezone for ambiguous date strings.
	date := func(s string) (time.Time, bool) {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
			if t, err := time.Parse(layout, s); err == nil {
				return t.Truncate(time.Millisecond), true // JS Date has millisecond precision.
			}
		}
		return time.Time{}, false
	}
	if !aok && !bok {
		x, xok := date(left)
		y, yok := date(right)
		if xok && yok {
			return x.Compare(y), true
		}
	}
	return 0, false
}
