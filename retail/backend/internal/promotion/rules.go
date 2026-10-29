// Package promotion implements promotion qualification and allocation phases.
// A successful predicate is not a budget reservation or permission to discount.
package promotion

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrRules = errors.New("invalid promotion rules or context")

type RuleValue struct {
	Value json.RawMessage `json:"value"`
}
type Rule struct {
	Attribute string      `json:"attribute"`
	Operator  string      `json:"operator"`
	Values    []RuleValue `json:"values"`
}

// MatchRules is the original all-rules predicate over trusted server context.
// Scopes are order (empty), items and shipping_methods. Unlike shipping rules,
// eq checks ALL picked values, in checks ANY, and ne checks ALL not present.
// Missing values retain original undefined semantics, not a browser default.
// Invalid configuration/context returns an error and must never grant a coupon.
func MatchRules(context json.RawMessage, rules []Rule, scope string) (bool, error) {
	if len(context) > 1<<20 || len(rules) > 1000 || (scope != "" && scope != "items" && scope != "shipping_methods") {
		return false, ErrRules
	}
	value, err := decode(context)
	if err != nil {
		return false, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return false, ErrRules
	}
	prepared := make([][]string, len(rules))
	for i, rule := range rules {
		if rule.Attribute == "" || len(rule.Attribute) > 191 || !utf8.ValidString(rule.Attribute) || len(strings.Split(rule.Attribute, ".")) > 32 || len(rule.Values) > 1000 {
			return false, ErrRules
		}
		switch rule.Operator {
		case "eq", "in", "ne", "gt", "gte", "lt", "lte":
		default:
			return false, ErrRules
		}
		for _, v := range rule.Values {
			if len(v.Value) > 65536 {
				return false, ErrRules
			}
			parsed, err := decode(v.Value)
			if err != nil {
				return false, err
			}
			if text, ok := parsed.(string); ok {
				if rule.Operator != "eq" && rule.Operator != "in" && rule.Operator != "ne" {
					if _, ok := decimal(text); !ok {
						return false, ErrRules
					}
				}
				prepared[i] = append(prepared[i], text)
			}
		}
		if len(prepared[i]) == 0 {
			return false, ErrRules
		}
	}
	for i, rule := range rules {
		attribute := rule.Attribute
		if scope != "" {
			attribute = strings.Replace(attribute, scope+".", "", 1)
		}
		picked, err := pick(object, strings.Split(attribute, "."))
		if err != nil {
			return false, err
		}
		values, ok := picked.([]any)
		if !ok {
			values = []any{picked}
		}
		if len(values) == 0 {
			if rule.Operator == "ne" {
				continue
			}
			return false, nil
		}
		valid := map[string]bool{}
		for _, text := range prepared[i] {
			valid[text] = true
		}
		matched := rule.Operator != "in"
		for _, v := range values {
			current := false
			switch rule.Operator {
			case "eq", "in":
				current = valid[text(v, false)]
			case "ne":
				current = !valid[text(v, false)]
			default:
				left, ok := decimal(v)
				if !ok {
					return false, ErrRules
				}
				for _, candidate := range prepared[i] {
					right, ok := decimal(candidate)
					if !ok {
						return false, ErrRules
					}
					cmp := left.Cmp(right)
					if rule.Operator == "gt" && cmp > 0 || rule.Operator == "gte" && cmp >= 0 || rule.Operator == "lt" && cmp < 0 || rule.Operator == "lte" && cmp <= 0 {
						current = true
						break
					}
				}
			}
			if rule.Operator == "in" {
				matched = matched || current
				if matched {
					break
				}
			} else {
				matched = matched && current
				if !matched {
					break
				}
			}
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func decode(raw json.RawMessage) (any, error) {
	if !utf8.Valid(raw) {
		return nil, ErrRules
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value any
	if d.Decode(&value) != nil {
		return nil, ErrRules
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, ErrRules
	}
	return value, nil
}

type missing struct{}

func pick(value any, path []string) (any, error) {
	if len(path) == 0 {
		return value, nil
	}
	if value == nil {
		return nil, ErrRules
	}
	object, ok := value.(map[string]any)
	if !ok {
		return missing{}, nil
	}
	v, ok := object[path[0]]
	if !ok {
		return missing{}, nil
	}
	if len(path) == 1 {
		return v, nil
	}
	if array, ok := v.([]any); ok {
		out := []any{}
		for _, item := range array {
			next, err := pick(item, path[1:])
			if err != nil {
				return nil, err
			}
			if nested, ok := next.([]any); ok {
				out = append(out, nested...)
			} else {
				out = append(out, next)
			}
		}
		return out, nil
	}
	if _, ok := v.(map[string]any); ok {
		return pick(v, path[1:])
	}
	return v, nil
}

func text(value any, element bool) string {
	switch v := value.(type) {
	case missing:
		if element {
			return ""
		}
		return "undefined"
	case nil:
		if element {
			return ""
		}
		return "null"
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		f, err := v.Float64()
		if math.IsInf(f, 1) {
			return "Infinity"
		}
		if math.IsInf(f, -1) {
			return "-Infinity"
		}
		if err != nil {
			return "NaN"
		}
		if f == 0 {
			return "0"
		}
		b, err := json.Marshal(f)
		if err != nil {
			return "NaN"
		}
		return string(b)
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = text(item, true)
		}
		return strings.Join(parts, ",")
	default:
		return "[object Object]"
	}
}

var decimalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func decimal(value any) (*big.Rat, bool) {
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case json.Number:
		s = string(v)
	default:
		return nil, false
	}
	s = strings.TrimSpace(s)
	if len(s) > 256 || !decimalPattern.MatchString(s) {
		return nil, false
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e, err := strconv.Atoi(s[i+1:])
		if err != nil || e > 1000 || e < -1000 {
			return nil, false
		}
	}
	n, ok := new(big.Rat).SetString(s)
	return n, ok
}
