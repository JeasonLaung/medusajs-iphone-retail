// Package rowimport executes explicit, versioned source field mappings. It has
// no database or file access and never invents IDs, defaults or missing values.
package rowimport

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/eurorefurb/retail/internal/importvalue"
)

//go:embed field_plans.json
var fieldPlans []byte

var (
	ErrRow        = errors.New("source row does not match explicit field program")
	ErrProtection = errors.New("source field requires protected transformation")
)

type field struct {
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Transform  string   `json:"transform"`
	Nullable   bool     `json:"nullable"`
	TargetType string   `json:"target_type"`
	MaxChars   int      `json:"max_chars"`
	Allowed    []string `json:"allowed"`
}
type tablePlan struct {
	Source   string       `json:"source"`
	Target   string       `json:"target"`
	Fields   []field      `json:"fields"`
	RawPairs []numberPair `json:"raw_pairs"`
}

// Cell explicitly distinguishes SQL NULL from JSON null. JSON fields containing
// the document null use SQLNull=false, Raw="null". Snapshot adapters must prove
// this distinction; a JSON-object export which conflates them is insufficient.
type Cell struct {
	SQLNull bool
	Raw     json.RawMessage
}
type SourceRow map[string]Cell
type BoundRow struct {
	Table   string
	Columns []string
	Values  []any
}

var compiledPlans = sync.OnceValues(func() (map[string]tablePlan, error) {
	var program struct {
		Version int         `json:"version"`
		Tables  []tablePlan `json:"tables"`
	}
	if json.Unmarshal(fieldPlans, &program) != nil || program.Version != 1 || len(program.Tables) != 159 {
		return nil, ErrRow
	}
	plans := make(map[string]tablePlan, len(program.Tables))
	columns := 0
	for _, plan := range program.Tables {
		if _, exists := plans[plan.Source]; exists {
			return nil, ErrRow
		}
		plans[plan.Source] = plan
		columns += len(plan.Fields)
	}
	if columns != 1691 {
		return nil, ErrRow
	}
	return plans, nil
})

// Convert rejects unknown/missing fields and only returns a COMPLETE ordered row.
// Returned values are for bound SQL arguments, never string interpolation. It
// checks exact raw-number pairs, but not references, source CHECK constraints, identity
// claims or domain states. Protected non-NULL fields deliberately fail closed
// until their source-specific authenticated conversion is provided.
func Convert(sourceTable string, row SourceRow) (BoundRow, error) {
	plans, err := compiledPlans()
	if err != nil {
		return BoundRow{}, ErrRow
	}
	plan, exists := plans[sourceTable]
	if !exists {
		return BoundRow{}, ErrRow
	}
	if len(row) != len(plan.Fields) {
		return BoundRow{}, ErrRow
	}
	bound := BoundRow{Table: plan.Target}
	for _, f := range plan.Fields {
		cell, present := row[f.Source]
		if !present {
			return BoundRow{}, ErrRow
		}
		var value any
		if cell.SQLNull {
			if !f.Nullable || len(cell.Raw) != 0 {
				return BoundRow{}, ErrRow
			}
		} else {
			var err error
			value, err = convertValue(f, cell.Raw)
			if err != nil {
				return BoundRow{}, err
			}
		}
		bound.Columns = append(bound.Columns, f.Target)
		bound.Values = append(bound.Values, value)
	}
	if err := checkNumberPairs(plan.RawPairs, row); err != nil {
		return BoundRow{}, ErrRow
	}
	return bound, nil
}

func convertValue(f field, raw json.RawMessage) (any, error) {
	var value any
	var err error
	switch f.Transform {
	case "identity_string", "historical_subject_ref", "utf8_text", "binary_text", "code_string", "device_identity_text", "enum_text":
		value, err = importvalue.Text(raw, f.MaxChars)
		if err == nil && f.Transform == "enum_text" {
			found := false
			for _, allowed := range f.Allowed {
				if value == allowed {
					found = true
				}
			}
			if !found {
				return nil, ErrRow
			}
		}
	case "strict_boolean":
		value, err = importvalue.Boolean(raw)
	case "checked_int32", "minor_unit_integer":
		value, err = importvalue.Integer(raw, 32)
	case "checked_int64":
		value, err = importvalue.Integer(raw, 64)
	case "utc_datetime6":
		instant, e := importvalue.Instant(raw)
		err = e
		if err == nil {
			value = instant.Format("2006-01-02 15:04:05.000000")
		}
	case "money_major_decimal", "quantity_decimal", "rate_decimal", "budget_decimal", "promotion_value_decimal", "legacy_float_rate_decimal":
		value, err = importvalue.Decimal38_18(raw)
	case "exact_json_text":
		value, err = importvalue.JSONText(raw)
	case "text_array_json":
		value, err = importvalue.JSONText(raw)
		if err == nil {
			trim := bytes.TrimSpace(raw)
			if len(trim) == 0 || trim[0] != '[' {
				return nil, ErrRow
			}
			var elements []json.RawMessage
			if json.Unmarshal(raw, &elements) != nil {
				return nil, ErrRow
			}
			for _, element := range elements {
				if string(bytes.TrimSpace(element)) == "null" {
					continue
				}
				if _, e := importvalue.Text(element, 0); e != nil {
					return nil, ErrRow
				}
			}
		}
	case "finite_float32_to_float64":
		// Only dimension fields have this transform. Money never reaches it.
		text := string(bytes.TrimSpace(raw))
		if !json.Valid(raw) || strings.Contains(text, "\"") {
			return nil, ErrRow
		}
		v, e := strconv.ParseFloat(text, 32)
		if e != nil || math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, ErrRow
		}
		value = v
	case "protected_string", "protected_exact_json":
		return nil, ErrProtection
	default:
		return nil, ErrRow
	}
	if err != nil {
		return nil, ErrRow
	}
	return value, nil
}
