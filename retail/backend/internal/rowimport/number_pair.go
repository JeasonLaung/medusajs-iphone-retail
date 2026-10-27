package rowimport

import (
	"encoding/json"
	"math/big"
	"regexp"
	"strconv"

	"github.com/eurorefurb/retail/internal/importvalue"
)

type numberPair struct {
	Raw    string `json:"raw"`
	Scalar string `json:"scalar"`
}

var rawDecimal = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE]([+-]?[0-9]+))?$`)

// raw.value is not bound to a DECIMAL column. Medusa toPrecision(20) may
// contain more than 18 fractional places consisting of trailing zeroes.
// Compare exact rationals without changing the retained JSON or rounding.
func rawRational(raw json.RawMessage) (*big.Rat, error) {
	if len(raw) > 1024 {
		return nil, ErrRow
	}
	text := string(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var err error
		text, err = importvalue.Text(raw, 0)
		if err != nil {
			return nil, ErrRow
		}
	}
	match := rawDecimal.FindStringSubmatch(text)
	if match == nil {
		return nil, ErrRow
	}
	if match[1] != "" {
		exponent, err := strconv.ParseInt(match[1], 10, 16)
		if err != nil || exponent < -1024 || exponent > 1024 {
			return nil, ErrRow
		}
	}
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, ErrRow
	}
	return value, nil
}

func checkNumberPairs(pairs []numberPair, row SourceRow) error {
	for _, pair := range pairs {
		raw, rawOK := row[pair.Raw]
		scalar, scalarOK := row[pair.Scalar]
		if !rawOK || !scalarOK || raw.SQLNull != scalar.SQLNull {
			return ErrRow
		}
		if raw.SQLNull {
			continue
		}
		if _, err := importvalue.JSONText(raw.Raw); err != nil {
			return ErrRow
		}
		document, err := object(raw.Raw)
		if err != nil || len(document) != 2 {
			return ErrRow
		}
		// Supported native representation. Extra/missing/duplicate keys are
		// held for explicit review, never silently discarded.
		precision, err := importvalue.Integer(document["precision"], 32)
		if err != nil || precision < 1 || precision > 1_000_000_000 {
			return ErrRow
		}
		rawValue, err := rawRational(document["value"])
		if err != nil {
			return ErrRow
		}
		decimal, err := importvalue.Decimal38_18(scalar.Raw)
		if err != nil {
			return ErrRow
		}
		scalarValue, ok := new(big.Rat).SetString(decimal)
		if !ok || rawValue.Cmp(scalarValue) != 0 {
			return ErrRow
		}
	}
	return nil
}
