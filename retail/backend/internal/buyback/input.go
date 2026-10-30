package buyback

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Strict object parsing is shared by future HTTP commands and persisted rules.
// It preserves numeric lexemes and rejects duplicate keys before Go's decoder
// could silently choose the last value. Only questionnaire check slots are null.
func decode(raw []byte, out any, required ...string) error {
	return decodeBounded(raw, out, 16384, required...)
}
func decodeBounded(raw []byte, out any, max int, required ...string) error {
	if len(raw) == 0 || len(raw) > max || !utf8.Valid(raw) {
		return ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int, bool) error
	walk = func(depth int, nullable bool) error {
		if depth > 8 {
			return ErrInput
		}
		v, e := d.Token()
		if e != nil || v == nil && !nullable {
			return ErrInput
		}
		if delimiter, ok := v.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					v, e := d.Token()
					if e != nil {
						return ErrInput
					}
					key, ok := v.(string)
					if !ok || seen[key] {
						return ErrInput
					}
					seen[key] = true
					// checks is the only array in these input shapes that permits null slots.
					if key == "checks" {
						start, e := d.Token()
						if e != nil || start != json.Delim('[') {
							return ErrInput
						}
						count := 0
						for d.More() {
							if count >= 5 || walk(depth+2, true) != nil {
								return ErrInput
							}
							count++
						}
						end, e := d.Token()
						if e != nil || end != json.Delim(']') {
							return ErrInput
						}
					} else if walk(depth+1, false) != nil {
						return ErrInput
					}
				}
				end, e := d.Token()
				if e != nil || end != json.Delim('}') {
					return ErrInput
				}
			case '[':
				count := 0
				for d.More() {
					if count >= 32 || walk(depth+1, false) != nil {
						return ErrInput
					}
					count++
				}
				end, e := d.Token()
				if e != nil || end != json.Delim(']') {
					return ErrInput
				}
			default:
				return ErrInput
			}
		}
		return nil
	}
	if walk(0, false) != nil {
		return ErrInput
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInput
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return ErrInput
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return ErrInput
		}
	}
	// encoding/json accepts case-insensitive struct field aliases. Our public
	// contract uses exact field names, including inside assessment/source.
	known := map[string]bool{}
	typ := reflect.TypeOf(out).Elem()
	for i := 0; i < typ.NumField(); i++ {
		known[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for key := range fields {
		if !known[key] {
			return ErrInput
		}
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrInput
	}
	return nil
}

func ParseRules(raw []byte) (Rules, error) {
	var r Rules
	if decode(raw, &r, "version", "enabled", "bases", "fault_costs", "damage_fee", "floor", "battery_threshold", "unknown_battery_issue") != nil || r.Validate() != nil {
		return Rules{}, ErrInput
	}
	// Source provenance has three required fields when present.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, key := range []string{"damage_fee", "floor"} {
		if bytes.HasPrefix(bytes.TrimSpace(fields[key]), []byte{'"'}) {
			return Rules{}, ErrInput
		}
	}
	for _, key := range []string{"bases", "fault_costs"} {
		var values map[string]json.RawMessage
		if json.Unmarshal(fields[key], &values) != nil {
			return Rules{}, ErrInput
		}
		for _, value := range values {
			if bytes.HasPrefix(bytes.TrimSpace(value), []byte{'"'}) {
				return Rules{}, ErrInput
			}
		}
	}
	if value, ok := fields["source"]; ok {
		var source Source
		if decode(value, &source, "name", "country", "fetched_at") != nil {
			return Rules{}, ErrInput
		}
	}
	return r, nil
}

type EstimateInput struct {
	ProductID  string     `json:"product_id"`
	Capacity   string     `json:"capacity"`
	Assessment Assessment `json:"assessment"`
}

var capacityPattern = regexp.MustCompile(`^[0-9]+(GB|TB)$`)

func ParseEstimate(raw []byte) (EstimateInput, error) {
	var in EstimateInput
	if decode(raw, &in, "product_id", "capacity", "assessment") != nil || len([]rune(in.ProductID)) < 1 || len([]rune(in.ProductID)) > 100 || !capacityPattern.MatchString(in.Capacity) {
		return EstimateInput{}, ErrInput
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	var a Assessment
	if decode(fields["assessment"], &a, "battery", "checks", "faults", "glass", "screen", "frame", "back") != nil || a.ValidatePartial() != nil {
		return EstimateInput{}, ErrInput
	}
	in.Capacity = NormalizeCapacity(in.Capacity)
	return in, nil
}
