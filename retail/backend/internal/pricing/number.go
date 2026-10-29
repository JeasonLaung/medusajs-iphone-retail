package pricing

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/totals"
)

// Raw numbers retain source precision. Scalar columns are not a fallback for a
// malformed raw value. Duplicate/unknown JSON fields fail closed.
func sourceNumber(scalar string, raw *string) (totals.Number, error) {
	n := totals.Number{Decimal: scalar}
	if raw != nil {
		if len(*raw) > 4096 || !utf8.ValidString(*raw) {
			return n, ErrInput
		}
		d := json.NewDecoder(bytes.NewBufferString(*raw))
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return n, ErrInput
		}
		fields := map[string]json.RawMessage{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return n, ErrInput
			}
			name, ok := key.(string)
			if !ok || (name != "value" && name != "precision") {
				return n, ErrInput
			}
			if _, exists := fields[name]; exists {
				return n, ErrInput
			}
			var v json.RawMessage
			if d.Decode(&v) != nil {
				return n, ErrInput
			}
			fields[name] = v
		}
		if token, err = d.Token(); err != nil || token != json.Delim('}') || len(fields) != 2 {
			return n, ErrInput
		}
		if _, err = d.Token(); err != io.EOF {
			return n, ErrInput
		}
		v := fields["value"]
		value := string(v)
		if len(v) > 0 && v[0] == '"' {
			if json.Unmarshal(v, &value) != nil {
				return n, ErrInput
			}
		}
		precision, err := strconv.Atoi(string(fields["precision"]))
		if err != nil {
			return n, ErrInput
		}
		n.Raw = &totals.RawValue{Value: value, Precision: precision}
	}
	if _, _, err := number(n); err != nil {
		return n, err
	}
	return n, nil
}

// ConfiguredAmount resolves the authoritative raw value for an operations form.
// It shares quote validation and never substitutes a rounded scalar for raw data.
func ConfiguredAmount(scalar, raw string) (string, error) {
	n, e := sourceNumber(scalar, &raw)
	if e != nil {
		return "", e
	}
	return totals.Resolve(n)
}
