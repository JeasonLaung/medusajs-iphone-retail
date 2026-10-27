// Package importvalue validates source values before MySQL can coerce, round,
// truncate or replace them. Errors never include source values or identifiers.
package importvalue

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrValue = errors.New("invalid or unrepresentable source value")
var decimalPattern = regexp.MustCompile(`^(-?)(0|[1-9][0-9]*)(?:\.([0-9]+))?(?:[eE]([+-]?[0-9]+))?$`)

// Text accepts a JSON string only. No trimming, case-folding, numeric coercion
// or Unicode normalization. maxChars=0 means no column character limit here.
func Text(raw json.RawMessage, maxChars int) (string, error) {
	if !validJSON(raw) {
		return "", ErrValue
	}
	var value string
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || trim[0] != '"' || json.Unmarshal(trim, &value) != nil {
		return "", ErrValue
	}
	if maxChars < 0 || (maxChars > 0 && utf8.RuneCountInString(value) > maxChars) {
		return "", ErrValue
	}
	return value, nil
}

func Boolean(raw json.RawMessage) (bool, error) {
	switch string(bytes.TrimSpace(raw)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, ErrValue
	}
}

// Decimal38_18 expands scientific notation using strings only. The result has
// <=20 integer places and <=18 fractional places, with no rounding. Input scale
// exceeding the target is rejected even if the extra digits are all zero.
func Decimal38_18(raw json.RawMessage) (string, error) {
	if len(raw) > 1024 {
		return "", ErrValue
	}
	text := string(bytes.TrimSpace(raw))
	if strings.HasPrefix(text, "\"") {
		var err error
		text, err = Text(raw, 0)
		if err != nil {
			return "", err
		}
	} else if !json.Valid(raw) {
		return "", ErrValue
	}
	m := decimalPattern.FindStringSubmatch(text)
	if m == nil {
		return "", ErrValue
	}
	exponent := 0
	if m[4] != "" {
		n, err := strconv.ParseInt(m[4], 10, 16)
		if err != nil || n < -1024 || n > 1024 {
			return "", ErrValue
		}
		exponent = int(n)
	}
	digits := m[2] + m[3]
	point := len(m[2]) + exponent
	var whole, fraction string
	switch {
	case point <= 0:
		whole = "0"
		fraction = strings.Repeat("0", -point) + digits
	case point >= len(digits):
		whole = digits + strings.Repeat("0", point-len(digits))
	default:
		whole, fraction = digits[:point], digits[point:]
	}
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	if len(whole) > 20 || len(fraction) > 18 {
		return "", ErrValue
	}
	if fraction != "" {
		return m[1] + whole + "." + fraction, nil
	}
	return m[1] + whole, nil
}

func Integer(raw json.RawMessage, bits int) (int64, error) {
	if bits != 32 && bits != 64 {
		return 0, ErrValue
	}
	text := string(bytes.TrimSpace(raw))
	if len(text) > 21 || !json.Valid(raw) || strings.ContainsAny(text, ".eE\"") {
		return 0, ErrValue
	}
	v, err := strconv.ParseInt(text, 10, bits)
	if err != nil {
		return 0, ErrValue
	}
	return v, nil
}

// Instant converts explicit RFC3339 instants to UTC without changing precision.
// Reject all >6 fractional digits (including trailing zeroes), unknown-offset
// -00:00 and UTC values outside MySQL DATETIME's 1000..9999 range.
func Instant(raw json.RawMessage) (time.Time, error) {
	text, err := Text(raw, 0)
	if err != nil || strings.HasSuffix(text, "-00:00") {
		return time.Time{}, ErrValue
	}
	if len(text) < 20 || text[10] != 'T' || strings.Contains(text, ",") {
		return time.Time{}, ErrValue
	}
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		end := dot + 1
		for end < len(text) && text[end] >= '0' && text[end] <= '9' {
			end++
		}
		if end-dot-1 > 6 {
			return time.Time{}, ErrValue
		}
	}
	v, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}, ErrValue
	}
	_, offset := v.Zone()
	if offset <= -24*3600 || offset >= 24*3600 {
		return time.Time{}, ErrValue
	}
	v = v.UTC()
	if v.Year() < 1000 || v.Year() > 9999 {
		return time.Time{}, ErrValue
	}
	return v, nil
}

// JSONText retains exact original UTF-8 bytes, key order, duplicate keys and
// numeric lexemes. SQL NULL must be distinguished by the caller; JSON null here
// is a non-NULL document. No decode-to-interface{} float64 path is used.
func JSONText(raw json.RawMessage) (string, error) {
	if !validJSON(raw) {
		return "", ErrValue
	}
	return string(raw), nil
}

// Check UTF-16 escapes before encoding/json can replace unpaired surrogates.
// Escaped backslashes are skipped as pairs; literal U+FFFD is valid text.
func validJSON(raw []byte) bool {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
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
		v, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if v >= 0xDC00 && v <= 0xDFFF {
			return false
		}
		if v < 0xD800 || v > 0xDBFF {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		lo, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || lo < 0xDC00 || lo > 0xDFFF {
			return false
		}
		i += 6
	}
	return true
}
