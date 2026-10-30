package buyback

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/google/uuid"
)

type ReturnAddress struct {
	Name        string `json:"name"`
	Address1    string `json:"address_1"`
	Address2    string `json:"address_2"`
	City        string `json:"city"`
	PostalCode  string `json:"postal_code"`
	CountryCode string `json:"country_code"`
}
type SubmissionItem struct {
	Model      string     `json:"model"`
	Capacity   string     `json:"capacity"`
	Assessment Assessment `json:"assessment"`
}
type Submission struct {
	SubmissionKey         string           `json:"submission_key"`
	ContactName           string           `json:"contact_name"`
	Phone                 string           `json:"phone"`
	ReturnAddress         ReturnAddress    `json:"return_address"`
	ReferenceAcknowledged bool             `json:"reference_acknowledged"`
	PrivacyAcknowledged   bool             `json:"privacy_acknowledged"`
	Items                 []SubmissionItem `json:"items"`
}

const jsWhitespace = "\u0009\u000a\u000b\u000c\u000d\u0020\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

var countryPattern = regexp.MustCompile(`^[a-z]{2}$`)

// Zod string lengths count JavaScript UTF-16 units; Go rune counts would admit
// twice as many astral characters. trim matches String.trim, including BOM.
func trimSubmission(s string, min, max int) (string, error) {
	s = strings.Trim(s, jsWhitespace)
	n := len(utf16.Encode([]rune(s)))
	if n < min || n > max {
		return "", ErrInput
	}
	return s, nil
}

// ParseSubmission validates only the original public command contract. Product
// eligibility, customer identity, current prices and deduplication are resolved
// by the transactional command; none may be accepted from browser snapshots.
func ParseSubmission(raw []byte) (Submission, error) {
	var in Submission
	if decode(raw, &in, "submission_key", "contact_name", "phone", "return_address", "reference_acknowledged", "privacy_acknowledged", "items") != nil || !in.ReferenceAcknowledged || !in.PrivacyAcknowledged || len(in.Items) < 1 || len(in.Items) > 10 {
		return Submission{}, ErrInput
	}
	id, err := uuid.Parse(in.SubmissionKey)
	// z.uuid accepts RFC UUID versions 1..8 and the nil/max sentinel values.
	canonical := strings.ToLower(in.SubmissionKey)
	if err != nil || len(in.SubmissionKey) != 36 || id.String() != canonical || !(id == uuid.Nil || canonical == "ffffffff-ffff-ffff-ffff-ffffffffffff" || id.Version() >= 1 && id.Version() <= 8 && id.Variant() == uuid.RFC4122) {
		return Submission{}, ErrInput
	}
	if in.ContactName, err = trimSubmission(in.ContactName, 2, 100); err != nil {
		return Submission{}, err
	}
	if in.Phone, err = trimSubmission(in.Phone, 5, 50); err != nil {
		return Submission{}, err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if decode(fields["return_address"], &in.ReturnAddress, "name", "address_1", "city", "postal_code", "country_code") != nil {
		return Submission{}, ErrInput
	}
	a := &in.ReturnAddress
	for _, field := range []struct {
		value    *string
		min, max int
	}{{&a.Name, 2, 100}, {&a.Address1, 3, 200}, {&a.Address2, 0, 200}, {&a.City, 1, 100}, {&a.PostalCode, 1, 30}} {
		if *field.value, err = trimSubmission(*field.value, field.min, field.max); err != nil {
			return Submission{}, err
		}
	}
	if !countryPattern.MatchString(a.CountryCode) {
		return Submission{}, ErrInput
	}
	var items []json.RawMessage
	_ = json.Unmarshal(fields["items"], &items)
	for i, item := range items {
		var parsed SubmissionItem
		if decode(item, &parsed, "model", "capacity", "assessment") != nil {
			return Submission{}, ErrInput
		}
		if parsed.Model, err = trimSubmission(parsed.Model, 1, 150); err != nil {
			return Submission{}, err
		}
		if !capacityPattern.MatchString(parsed.Capacity) {
			return Submission{}, ErrInput
		}
		var f map[string]json.RawMessage
		_ = json.Unmarshal(item, &f)
		if decode(f["assessment"], &parsed.Assessment, "battery", "checks", "faults", "glass", "screen", "frame", "back") != nil {
			return Submission{}, ErrInput
		}
		v := parsed.Assessment
		// Complete assessments permit irrelevant skipped grade strings, bounded
		// by the original wire schema. Do not replace this with partial validation.
		if len(v.Battery) > 7 || len(utf16.Encode([]rune(v.Screen))) > 10 || len(utf16.Encode([]rune(v.Frame))) > 10 || len(utf16.Encode([]rune(v.Back))) > 10 || !v.Complete() {
			return Submission{}, ErrInput
		}
		in.Items[i] = parsed
	}
	return in, nil
}
