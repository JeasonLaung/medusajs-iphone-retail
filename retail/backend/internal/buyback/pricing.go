// Package buyback owns the existing retail buyback business rules. It does not
// bind historical customer IDs to new login identities.
package buyback

import (
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/eurorefurb/retail/internal/totals"
)

var ErrInput = errors.New("invalid buyback input")
var visuals = []string{"LIKE_NEW", "ALMOST_NEW", "GOOD", "MODERATE"}
var faults = []string{"BROKEN", "BROKEN_SCREEN", "BATTERY_ISSUE", "BROKEN_FRAME", "BROKEN_BACK_CAMERA"}

type Rules struct {
	Version             int                    `json:"version"`
	Enabled             bool                   `json:"enabled"`
	Bases               map[string]json.Number `json:"bases"`
	FaultCosts          map[string]json.Number `json:"fault_costs"`
	DamageFee           json.Number            `json:"damage_fee"`
	Floor               json.Number            `json:"floor"`
	BatteryThreshold    int                    `json:"battery_threshold"`
	UnknownBatteryIssue bool                   `json:"unknown_battery_issue"`
	Source              *Source                `json:"source,omitempty"`
}
type Source struct {
	Name      string `json:"name"`
	Country   string `json:"country"`
	FetchedAt string `json:"fetched_at"`
}

// Amounts remain major currency JSON numbers on the wire. Exact integer
// hundredths are used only inside this cent-denominated pricing policy.
func hundredths(n json.Number) (int64, error) {
	text, e := totals.Resolve(totals.Number{Decimal: string(n)})
	if e != nil {
		return 0, ErrInput
	}
	r, ok := new(big.Rat).SetString(text)
	if !ok || r.Sign() < 0 {
		return 0, ErrInput
	}
	r.Mul(r, big.NewRat(100, 1))
	if !r.IsInt() || !r.Num().IsInt64() || r.Num().Int64() > 100000000 {
		return 0, ErrInput
	}
	return r.Num().Int64(), nil
}
func major(n int64) json.Number {
	if n%100 == 0 {
		return json.Number(strconv.FormatInt(n/100, 10))
	}
	return json.Number(strings.TrimRight(strconv.FormatInt(n/100, 10)+"."+strconv.FormatInt(100+n%100, 10)[1:], "0"))
}
func (r Rules) Validate() error {
	if r.Version != 1 || r.BatteryThreshold < 1 || r.BatteryThreshold > 100 || len(r.Bases) != 4 || len(r.FaultCosts) != 5 {
		return ErrInput
	}
	floor, e := hundredths(r.Floor)
	if e != nil {
		return e
	}
	previous := int64(100000000)
	for _, v := range visuals {
		n, e := hundredths(r.Bases[v])
		if e != nil || n > previous || n < floor {
			return ErrInput
		}
		previous = n
	}
	for _, f := range faults {
		if _, e := hundredths(r.FaultCosts[f]); e != nil {
			return e
		}
	}
	if _, e := hundredths(r.DamageFee); e != nil {
		return e
	}
	if r.Source != nil && (len([]rune(r.Source.Name)) > 40 || len([]rune(r.Source.Country)) > 4 || len([]rune(r.Source.FetchedAt)) > 40) {
		return ErrInput
	}
	return nil
}

// Calculate deduplicates fault categories and charges the shared damage fee
// once. Public questionnaire visuals deliberately exclude sealed boxes.
func Calculate(r Rules, visual string, selected []string) (json.Number, error) {
	if r.Validate() != nil || !oneOf(visual, visuals...) || len(selected) > 32 {
		return "", ErrInput
	}
	base, _ := hundredths(r.Bases[visual])
	floor, _ := hundredths(r.Floor)
	seen := map[string]bool{}
	for _, f := range selected {
		if !oneOf(f, faults...) {
			return "", ErrInput
		}
		if !seen[f] {
			cost, _ := hundredths(r.FaultCosts[f])
			base -= cost
			seen[f] = true
		}
	}
	if len(seen) > 0 {
		fee, _ := hundredths(r.DamageFee)
		base -= fee
	}
	if base < floor {
		base = floor
	}
	return major(base), nil
}

type Assessment struct {
	Battery string   `json:"battery"`
	Checks  []*bool  `json:"checks"`
	Faults  []string `json:"faults"`
	Glass   []string `json:"glass"`
	Screen  string   `json:"screen"`
	Frame   string   `json:"frame"`
	Back    string   `json:"back"`
}
type Estimate struct {
	Amount      *json.Number `json:"amount"`
	Status      string       `json:"status"`
	Visual      *string      `json:"visual"`
	Faults      []string     `json:"faults"`
	Provisional bool         `json:"provisional"`
}

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

var batteryPattern = regexp.MustCompile(`^[0-9]{1,3}$`)

func validBattery(s string) bool {
	n, e := strconv.Atoi(s)
	return s == "unknown" || batteryPattern.MatchString(s) && e == nil && n >= 1 && n <= 100
}
func falseAt(a Assessment, i int) bool {
	return i < len(a.Checks) && a.Checks[i] != nil && !*a.Checks[i]
}
func (a Assessment) Eligible() bool { return !falseAt(a, 0) && !falseAt(a, 1) }
func (a Assessment) SkipScreen() bool {
	for _, set := range [][]string{a.Faults, a.Glass} {
		for _, v := range set {
			if v != "none" {
				return true
			}
		}
	}
	return false
}
func validMulti(v []string, maximum int, complete bool, allowed ...string) bool {
	if len(v) > maximum || complete && len(v) == 0 {
		return false
	}
	for _, s := range v {
		if !oneOf(s, allowed...) {
			return false
		}
		if complete && s == "none" && len(v) != 1 {
			return false
		}
	}
	return true
}

// ValidatePartial accepts the original estimate endpoint's seven required
// fields and nullable checks. Incomplete answers still produce a provisional
// estimate. Complete is stricter and is used when creating a request.
func (a Assessment) ValidatePartial() error {
	if a.Battery != "" && !validBattery(a.Battery) || len(a.Checks) != 5 || !validMulti(a.Faults, 4, false, "none", "bright", "pixels", "lines") || !validMulti(a.Glass, 4, false, "none", "cracked", "chipped", "scratched") || !oneOf(a.Screen, "", "visible", "some", "minimal", "none") || !oneOf(a.Frame, "", "damaged", "visible", "some", "minimal", "none") || !oneOf(a.Back, "", "damaged", "visible", "some", "minimal", "none") {
		return ErrInput
	}
	return nil
}
func (a Assessment) Complete() bool {
	if !validBattery(a.Battery) || !a.Eligible() || len(a.Checks) != 5 {
		return false
	}
	for _, v := range a.Checks {
		if v == nil {
			return false
		}
	}
	return validMulti(a.Faults, 3, true, "none", "bright", "pixels", "lines") && validMulti(a.Glass, 3, true, "none", "cracked", "chipped", "scratched") && (a.SkipScreen() || oneOf(a.Screen, "visible", "some", "minimal", "none")) && oneOf(a.Frame, "damaged", "visible", "some", "minimal", "none") && (a.Frame == "damaged" || oneOf(a.Back, "damaged", "visible", "some", "minimal", "none"))
}
func (a Assessment) RequiredPhotos() []string {
	out := []string{"front", "back"}
	if oneOf(a.Frame, "damaged", "visible") {
		out = append(out, "edges")
	}
	return out
}

// Assess also accepts internal partial answers, as the original pure function
// does. HTTP callers must parse/validate the complete wire shape separately.
func Assess(r *Rules, a Assessment, legacy *json.Number) (Estimate, error) {
	out := Estimate{Faults: []string{}}
	if !a.Eligible() {
		out.Status = "ineligible"
		return out, nil
	}
	if r == nil {
		out.Status = "unconfigured"
		if legacy != nil {
			value, e := totals.Resolve(totals.Number{Decimal: string(*legacy)})
			if e != nil || strings.HasPrefix(value, "-") {
				return out, ErrInput
			}
			n := json.Number(value)
			out.Amount = &n
			out.Status = "legacy"
		}
		return out, nil
	}
	if r.Validate() != nil {
		return out, ErrInput
	}
	if !r.Enabled {
		out.Status = "disabled"
		return out, nil
	}
	add := func(f string) { out.Faults = append(out.Faults, f) }
	for i := 2; i < len(a.Checks); i++ {
		if falseAt(a, i) {
			add("BROKEN")
			break
		}
	}
	screenBroken := a.SkipScreen()
	if screenBroken {
		add("BROKEN_SCREEN")
	}
	if a.Frame == "damaged" || a.Back == "damaged" {
		add("BROKEN_FRAME")
	}
	battery, _ := strconv.Atoi(a.Battery)
	if a.Battery == "unknown" && r.UnknownBatteryIssue || a.Battery != "" && a.Battery != "unknown" && battery < r.BatteryThreshold {
		add("BATTERY_ISSUE")
	}
	grades := []string{a.Frame}
	if !screenBroken {
		grades = append(grades, a.Screen)
	}
	if a.Frame != "damaged" {
		grades = append(grades, a.Back)
	}
	visual := "LIKE_NEW"
	for _, v := range []struct{ grade, visual string }{{"visible", "MODERATE"}, {"some", "GOOD"}, {"minimal", "ALMOST_NEW"}} {
		found := false
		for _, g := range grades {
			if g == v.grade {
				found = true
				break
			}
		}
		if found {
			visual = v.visual
			break
		}
	}
	out.Provisional = a.Battery == "" || a.Battery == "unknown" || len(a.Checks) != 5 || len(a.Faults) == 0 || len(a.Glass) == 0 || !screenBroken && a.Screen == "" || a.Frame == "" || a.Frame != "damaged" && a.Back == ""
	for _, v := range a.Checks {
		if v == nil {
			out.Provisional = true
		}
	}
	amount, e := Calculate(*r, visual, out.Faults)
	if e != nil {
		return out, e
	}
	out.Amount = &amount
	out.Visual = &visual
	out.Status = "estimated"
	return out, nil
}

func NormalizeCapacity(s string) string {
	// JavaScript \s includes BOM as well as Unicode whitespace.
	s = strings.ToUpper(strings.Map(func(r rune) rune {
		if strings.ContainsRune("\u0009\u000a\u000b\u000c\u000d\u0020\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff", r) {
			return -1
		}
		return r
	}, s))
	if s == "1024GB" {
		return "1TB"
	}
	if s == "2048GB" {
		return "2TB"
	}
	return s
}
