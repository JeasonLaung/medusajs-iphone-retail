package uniquekey

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"strconv"
	"time"
	"unicode/utf8"
)

//go:embed source_plans.json
var sourcePlans []byte

var ErrProtected = errors.New("protected unique identity needs an authenticated comparator")

type sourceField struct {
	Column   string `json:"column"`
	Kind     string `json:"kind"`
	Nullable bool   `json:"nullable"`
}
type sourceCondition struct {
	sourceField
	Operation string `json:"operation"`
}
type sourcePlan struct {
	TargetTable string            `json:"target_table"`
	Scope       string            `json:"scope"`
	Owner       sourceField       `json:"owner"`
	Keys        []sourceField     `json:"keys"`
	Conditions  []sourceCondition `json:"conditions"`
	Protected   bool              `json:"protected"`
}

// TargetRow is an internal full row projection, never an HTTP request binding.
// All key and predicate columns must be present, even when explicitly NULL.
// Domain validation and row locking remain the caller's responsibility.
type TargetRow map[string]json.RawMessage

func fieldValue(row TargetRow, f sourceField) (*string, error) {
	raw, exists := row[f.Column]
	if !exists || len(raw) == 0 || !json.Valid(raw) {
		return nil, ErrInvalid
	}
	raw = bytes.TrimSpace(raw)
	if string(raw) == "null" {
		if !f.Nullable {
			return nil, ErrInvalid
		}
		return nil, nil
	}
	if f.Kind == "bool" {
		var value bool
		if json.Unmarshal(raw, &value) != nil {
			return nil, ErrInvalid
		}
		text := strconv.FormatBool(value)
		return &text, nil
	}
	var value string
	// encoding/json replaces invalid UTF-8; reject before decoding.
	if !utf8.Valid(raw) || !validSurrogates(raw) || json.Unmarshal(raw, &value) != nil {
		return nil, ErrInvalid
	}
	if f.Kind == "time" {
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil || parsed.Nanosecond()%1000 != 0 {
			return nil, ErrInvalid
		}
	} else if f.Kind != "text" {
		return nil, ErrInvalid
	}
	return &value, nil
}

// JSON decoding replaces unpaired UTF-16 surrogate escapes with U+FFFD. Reject
// those inputs instead of collapsing distinct malformed source identities.
func validSurrogates(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		code, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if code >= 0xDC00 && code <= 0xDFFF {
			return false
		}
		if code < 0xD800 || code > 0xDBFF {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || low < 0xDC00 || low > 0xDFFF {
			return false
		}
		i += 6
	}
	return true
}

// SourceChanges builds all registry claims for a mapped target row. Unknown
// tables fail closed. Protected token identities fail closed rather than
// persisting plaintext in the registry or comparing randomized ciphertext.
// Historical customer predicates use legacy_had_account and grant no login.
func SourceChanges(table string, row TargetRow) ([]Change, error) {
	return sourceChanges(table, row, nil)
}

func sourceChanges(table string, row TargetRow, secret func(sourcePlan, string, sourceField) (*string, error)) ([]Change, error) {
	var document struct {
		Plans []sourcePlan `json:"plans"`
	}
	if json.Unmarshal(sourcePlans, &document) != nil {
		return nil, ErrInvalid
	}
	changes := []Change{}
	for _, plan := range document.Plans {
		if plan.TargetTable != table {
			continue
		}
		if plan.Protected && secret == nil {
			return nil, ErrProtected
		}
		owner, err := fieldValue(row, plan.Owner)
		if err != nil || owner == nil || *owner == "" {
			return nil, ErrInvalid
		}
		change := Change{Scope: plan.Scope, Owner: *owner, Active: true}
		for _, field := range plan.Keys {
			var value *string
			if plan.Protected {
				value, err = secret(plan, *owner, field)
			} else {
				value, err = fieldValue(row, field)
			}
			if err != nil {
				return nil, err
			}
			change.Components = append(change.Components, value)
		}
		for _, condition := range plan.Conditions {
			value, err := fieldValue(row, condition.sourceField)
			if err != nil {
				return nil, err
			}
			switch condition.Operation {
			case "is_null":
				change.Active = change.Active && value == nil
			case "is_false":
				change.Active = change.Active && value != nil && *value == "false"
			default:
				return nil, ErrInvalid
			}
		}
		changes = append(changes, change)
	}
	if len(changes) == 0 {
		return nil, ErrInvalid
	}
	return changes, nil
}

func protectedScope(scope string) bool {
	var document struct {
		Plans []sourcePlan `json:"plans"`
	}
	if json.Unmarshal(sourcePlans, &document) != nil {
		return true
	}
	for _, plan := range document.Plans {
		if plan.Scope == scope {
			return plan.Protected
		}
	}
	return false
}

// ApplySourceRow joins registry changes to a caller-owned business transaction.
// It does not write the domain row or provide authorization/version checks.
func ApplySourceRow(ctx context.Context, tx *sql.Tx, table string, row TargetRow) error {
	changes, err := SourceChanges(table, row)
	if err != nil {
		return err
	}
	return Apply(ctx, tx, changes)
}
