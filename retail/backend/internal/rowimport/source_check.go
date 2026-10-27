package rowimport

import (
	"math/big"

	"github.com/eurorefurb/retail/internal/importvalue"
)

type sourceCheck struct {
	Name    string   `json:"source_constraint"`
	Kind    string   `json:"kind"`
	Columns []string `json:"columns"`
	Allowed []string `json:"allowed"`
}

// Runs after complete field type/nullability validation. SQL CHECK rejects
// FALSE, but accepts UNKNOWN (NULL); NOT NULL is a separate field constraint.
// The program is generated from reviewed source expressions, not runtime SQL.
func checkSourceConstraints(checks []sourceCheck, row SourceRow) error {
	for _, check := range checks {
		arity := 1
		if check.Kind == "null_or_not_null" || check.Kind == "null_or_null" {
			arity = 2
		}
		if check.Name == "" || len(check.Columns) != arity {
			return ErrRow
		}
		cells := make([]Cell, arity)
		for i, name := range check.Columns {
			var exists bool
			cells[i], exists = row[name]
			if !exists {
				return ErrRow
			}
		}
		first := cells[0]
		switch check.Kind {
		case "enum":
			if len(check.Allowed) == 0 {
				return ErrRow
			}
			if first.SQLNull {
				continue
			}
			value, err := importvalue.Text(first.Raw, 0)
			if err != nil {
				return ErrRow
			}
			found := false
			for _, allowed := range check.Allowed {
				if value == allowed {
					found = true
					break
				}
			}
			if !found {
				return ErrRow
			}
		case "true":
			if first.SQLNull {
				continue
			}
			value, err := importvalue.Boolean(first.Raw)
			if err != nil || !value {
				return ErrRow
			}
		case "nonnegative_integer":
			if first.SQLNull {
				continue
			}
			value, err := importvalue.Integer(first.Raw, 64)
			if err != nil || value < 0 {
				return ErrRow
			}
		case "nonnegative_decimal":
			if first.SQLNull {
				continue
			}
			value, err := importvalue.Decimal38_18(first.Raw)
			if err != nil {
				return ErrRow
			}
			number, ok := new(big.Rat).SetString(value)
			if !ok || number.Sign() < 0 {
				return ErrRow
			}
		case "null_or_not_null":
			if !first.SQLNull && cells[1].SQLNull {
				return ErrRow
			}
		case "null_or_null":
			if !first.SQLNull && !cells[1].SQLNull {
				return ErrRow
			}
		default:
			return ErrRow
		}
	}
	return nil
}
