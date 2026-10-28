package totals

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"
)

// StoredNumber decodes an explicitly selected scalar/raw pair. A malformed raw
// value never falls back to its rounded SQL scalar. No private JSON is returned.
func StoredNumber(scalar string, raw *string) (Number, error) {
	n := Number{Decimal: scalar}
	if raw != nil {
		if len(*raw) > 4096 || !utf8.ValidString(*raw) {
			return Number{}, ErrInput
		}
		d := json.NewDecoder(bytes.NewBufferString(*raw))
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return Number{}, ErrInput
		}
		fields := map[string]json.RawMessage{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return Number{}, ErrInput
			}
			name, ok := key.(string)
			if !ok || name != "value" && name != "precision" {
				return Number{}, ErrInput
			}
			if _, found := fields[name]; found {
				return Number{}, ErrInput
			}
			var value json.RawMessage
			if d.Decode(&value) != nil {
				return Number{}, ErrInput
			}
			fields[name] = value
		}
		if token, err = d.Token(); err != nil || token != json.Delim('}') || len(fields) != 2 {
			return Number{}, ErrInput
		}
		if _, err = d.Token(); err != io.EOF {
			return Number{}, ErrInput
		}
		value := string(fields["value"])
		if len(value) > 0 && value[0] == '"' {
			if json.Unmarshal(fields["value"], &value) != nil {
				return Number{}, ErrInput
			}
		}
		precision, err := strconv.Atoi(string(fields["precision"]))
		if err != nil {
			return Number{}, ErrInput
		}
		n.Raw = &RawValue{Value: value, Precision: precision}
	}
	if _, err := Resolve(n); err != nil {
		return Number{}, ErrInput
	}
	return n, nil
}
