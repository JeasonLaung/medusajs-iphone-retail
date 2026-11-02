package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

var ErrOptionDefinitionAssociated = errors.New("option value associated with products")
var ErrOptionDefinitionTitle = errors.New("shared option title already exists")

// Shared definitions are edited independently of a product's selected values.
// Values use their source text as identity, matching updateOptions_ in the
// original product module. Renaming a value therefore removes the old value.
type OptionDefinitionInput struct {
	OperationID      string         `json:"operation_id"`
	ExpectedRevision string         `json:"expected_revision"`
	Title            string         `json:"title"`
	Values           []string       `json:"values"`
	Ranks            map[string]int `json:"ranks,omitempty"`
}

type definitionValue struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Rank  *int   `json:"rank"`
	// Memberships include every non-deleted product/value association, not
	// merely values used by variants or associations to published products.
	Memberships int64 `json:"memberships"`
}

type definitionValuePlan struct {
	Keep   []definitionValue
	Remove []string
}

func validOptionDefinition(in OptionDefinitionInput) bool {
	if !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) || !createText(in.Title, 100) || len(in.Values) == 0 || len(in.Values) > 100 {
		return false
	}
	seen := make(map[string]bool, len(in.Values))
	for _, value := range in.Values {
		if !createText(value, 500) || seen[value] {
			return false
		}
		seen[value] = true
	}
	for value, rank := range in.Ranks {
		// The source column is a signed INT. Zero, negative and repeated ranks
		// are valid; a rank must name a value in the desired definition.
		if !seen[value] || int64(rank) < -2147483648 || int64(rank) > 2147483647 {
			return false
		}
	}
	return true
}

// imageObject requires every allowed field. Shared definitions additionally
// allow an omitted ranks map and a subset of values with explicit ranks.
func definitionObject(raw []byte, required []string, optional ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errEditInput
	}
	allowed := map[string]bool{}
	for _, key := range append(append([]string{}, required...), optional...) {
		allowed[key] = true
	}
	for _, key := range required {
		if fields[key] == nil {
			return nil, errEditInput
		}
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if !allowed[key] {
			return nil, errEditInput
		}
		keys = append(keys, key)
	}
	return imageObject(raw, keys...)
}

func decodeOptionDefinition(r io.Reader) (OptionDefinitionInput, error) {
	var in OptionDefinitionInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil {
		return in, e
	}
	if len(raw) > 262144 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	fields, e := definitionObject(raw, []string{"operation_id", "expected_revision", "title", "values"}, "ranks")
	if e != nil {
		return in, e
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return in, errEditInput
		}
	}
	if ranks, ok := fields["ranks"]; ok {
		// Decode member names first so duplicate rank keys cannot silently
		// override each other during ordinary JSON unmarshalling.
		var values []string
		if json.Unmarshal(fields["values"], &values) != nil {
			return in, errEditInput
		}
		members, e := definitionObject(ranks, nil, values...)
		if e != nil {
			return in, e
		}
		for _, rank := range members {
			if bytes.Equal(bytes.TrimSpace(rank), []byte("null")) {
				return in, errEditInput
			}
		}
	}
	if json.Unmarshal(raw, &in) != nil || !validOptionDefinition(in) {
		return in, errEditInput
	}
	return in, nil
}

// Plan before issuing any SQL writes so a blocked removal cannot partially
// rename the definition, add values or change ranks. IDs for new values are
// assigned by the transaction only after the complete plan is accepted.
func planDefinitionValues(current []definitionValue, in OptionDefinitionInput) (definitionValuePlan, error) {
	plan := definitionValuePlan{Keep: []definitionValue{}, Remove: []string{}}
	if !validOptionDefinition(in) {
		return plan, errEditInput
	}
	byText := make(map[string]definitionValue, len(current))
	ids := make(map[string]bool, len(current))
	for _, value := range current {
		if value.ID == "" || ids[value.ID] || value.Memberships < 0 {
			return definitionValuePlan{}, ErrSchema
		}
		if _, exists := byText[value.Value]; exists {
			return definitionValuePlan{}, ErrSchema
		}
		byText[value.Value] = value
		ids[value.ID] = true
	}
	for _, text := range in.Values {
		value, exists := byText[text]
		if !exists {
			value = definitionValue{Value: text}
		}
		if rank, present := in.Ranks[text]; present {
			value.Rank = &rank
		}
		plan.Keep = append(plan.Keep, value)
		delete(byText, text)
	}
	// Iterate the original order for deterministic receipts, rather than a map.
	for _, value := range current {
		if _, removed := byText[value.Value]; !removed {
			continue
		}
		if value.Memberships != 0 {
			return definitionValuePlan{}, ErrOptionDefinitionAssociated
		}
		plan.Remove = append(plan.Remove, value.ID)
	}
	return plan, nil
}
