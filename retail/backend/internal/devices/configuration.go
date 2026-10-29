package devices

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"regexp"
	"strconv"
	"strings"
)

type VariantOption struct {
	Title string `json:"title"`
	Value string `json:"value"`
}
type Configuration struct {
	Grade         *string `json:"grade"`
	BatteryHealth *int    `json:"battery_health"`
}

var percentagePattern = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)
var nonPercentagePattern = regexp.MustCompile(`[^0-9.]`)

func gradeOption(title string) bool {
	v := strings.ToLower(title)
	return strings.Contains(v, "grade") || strings.Contains(v, "condition") || strings.Contains(v, "成色")
}
func batteryOption(title string) bool {
	v := strings.ToLower(title)
	return strings.Contains(v, "battery") || strings.Contains(v, "电池")
}

// ResolveConfiguration preserves SKU-derived defaults and exact grade/minimum
// battery checks. Fractional defaults cannot fit the reviewed integer device
// column; reject those rather than truncate a measured fact.
func ResolveConfiguration(options []VariantOption, input Configuration) (Configuration, error) {
	out := input
	var grade *string
	var battery *int
	fractional := false
	for _, o := range options {
		value := strings.TrimSpace(o.Value)
		if gradeOption(o.Title) && value != "" {
			v := cases.Upper(language.Und).String(value)
			grade = &v
		}
		if batteryOption(o.Title) && value != "" {
			if raw := percentagePattern.FindString(value); raw != "" {
				v, e := strconv.ParseFloat(raw, 64)
				if e == nil && v <= 100 {
					fractional = v != float64(int(v))
					n := int(v)
					battery = &n
				}
			}
		}
	}
	if out.Grade == nil {
		out.Grade = grade
	}
	if out.BatteryHealth == nil {
		if fractional {
			return out, ErrInput
		}
		out.BatteryHealth = battery
	}
	if input.Grade != nil && *input.Grade != "S" && *input.Grade != "A" && *input.Grade != "B" && *input.Grade != "C" {
		return out, ErrInput
	}
	if out.BatteryHealth != nil && (*out.BatteryHealth < 0 || *out.BatteryHealth > 100) {
		return out, ErrInput
	}
	for _, o := range options {
		if gradeOption(o.Title) && (out.Grade == nil || cases.Upper(language.Und).String(o.Value) != *out.Grade) {
			return out, ErrInput
		}
		if batteryOption(o.Title) {
			raw := nonPercentagePattern.ReplaceAllString(o.Value, "")
			minimum, e := strconv.ParseFloat(raw, 64)
			if e == nil && minimum > 0 && (out.BatteryHealth == nil || float64(*out.BatteryHealth) < minimum) {
				return out, ErrInput
			}
		}
	}
	return out, nil
}
