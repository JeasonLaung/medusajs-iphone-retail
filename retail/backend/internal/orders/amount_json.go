package orders

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/totals"
)

// Only an explicit numeric projection is returned. Historical JSON remains
// untouched; duplicate top-level fields never silently overwrite financial data.
func amountObject(text string) (map[string]json.RawMessage, error) {
	if len(text) > 1<<20 || !utf8.ValidString(text) {
		return nil, ErrSchema
	}
	d := json.NewDecoder(bytes.NewBufferString(text))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrSchema
	}
	out := map[string]json.RawMessage{}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, ErrSchema
		}
		key, ok := t.(string)
		if !ok {
			return nil, ErrSchema
		}
		if _, ok := out[key]; ok || len(out) >= 1000 {
			return nil, ErrSchema
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return nil, ErrSchema
		}
		out[key] = raw
	}
	if t, err := d.Token(); err != nil || t != json.Delim('}') {
		return nil, ErrSchema
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrSchema
	}
	return out, nil
}
func numericText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", ErrSchema
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return "", ErrSchema
		}
		return text, nil
	}
	var value json.Number
	if json.Unmarshal(raw, &value) != nil {
		return "", ErrSchema
	}
	return value.String(), nil
}
func amountNumber(decimal string, raw *string) (totals.Number, error) {
	n := totals.Number{Decimal: decimal}
	if raw != nil {
		obj, err := amountObject(*raw)
		if err != nil || len(obj) != 2 {
			return n, ErrSchema
		}
		value, err := numericText(obj["value"])
		if err != nil {
			return n, ErrSchema
		}
		precision, err := strconv.Atoi(string(obj["precision"]))
		if err != nil {
			return n, ErrSchema
		}
		n.Raw = &totals.RawValue{Value: value, Precision: precision}
	}
	if _, err := totals.Resolve(n); err != nil {
		return n, ErrSchema
	}
	return n, nil
}
func transactionSummary(text *string) (*totals.SummaryInput, *string, error) {
	if text == nil || bytes.Equal(bytes.TrimSpace([]byte(*text)), []byte("null")) {
		return nil, nil, nil
	}
	obj, err := amountObject(*text)
	if err != nil {
		return nil, nil, err
	}
	// Missing or JSON null transaction_total uses the reviewed legacy zero.
	decimal := "0"
	if raw, ok := obj["transaction_total"]; ok && string(raw) != "null" {
		decimal, err = numericText(raw)
		if err != nil {
			return nil, nil, err
		}
	}
	var raw *string
	if r, ok := obj["raw_transaction_total"]; ok && string(r) != "null" {
		s := string(r)
		raw = &s
	}
	n, err := amountNumber(decimal, raw)
	if err != nil {
		return nil, nil, err
	}
	effective, err := totals.Resolve(n)
	if err != nil {
		return nil, nil, ErrSchema
	}
	return &totals.SummaryInput{TransactionTotal: &n}, &effective, nil
}
