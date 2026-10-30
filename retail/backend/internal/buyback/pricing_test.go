package buyback

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestOriginalPricing(t *testing.T) {
	raw, e := os.ReadFile("../../../../docs/migration/fixtures/buyback-pricing.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixture struct {
		Rules  []Rules
		Matrix []struct {
			RulesIndex int `json:"rules_index"`
			Visual     string
			Faults     []string
			Amount     json.Number
		} `json:"matrix_cases"`
		Assessments []struct {
			Name       string
			Rules      *Rules
			Assessment Assessment
			Legacy     *json.Number
			Expected   Estimate
			Complete   bool
			Required   []string `json:"required_photos"`
		} `json:"assessment_cases"`
		Normalize []struct{ Input, Expected string }
	}
	if e = json.Unmarshal(raw, &fixture); e != nil {
		t.Fatal(e)
	}
	if len(fixture.Matrix) != 8576 || len(fixture.Assessments) < 200 || len(fixture.Normalize) != 7 {
		t.Fatalf("unexpected fixture coverage: %d/%d/%d", len(fixture.Matrix), len(fixture.Assessments), len(fixture.Normalize))
	}
	for i, row := range fixture.Matrix {
		amount, e := Calculate(fixture.Rules[row.RulesIndex], row.Visual, row.Faults)
		if e != nil || amount != row.Amount {
			t.Fatalf("matrix %d: %s %v want %s", i, amount, e, row.Amount)
		}
	}
	for _, row := range fixture.Assessments {
		t.Run(row.Name, func(t *testing.T) {
			before, _ := json.Marshal(row)
			got, e := Assess(row.Rules, row.Assessment, row.Legacy)
			if e != nil || !reflect.DeepEqual(got, row.Expected) {
				t.Fatalf("estimate %#v %v want %#v", got, e, row.Expected)
			}
			if row.Assessment.Complete() != row.Complete || !reflect.DeepEqual(row.Assessment.RequiredPhotos(), row.Required) {
				t.Fatal("assessment/photo requirements differ")
			}
			after, _ := json.Marshal(row)
			if string(before) != string(after) {
				t.Fatal("mutated input")
			}
		})
	}
	for _, row := range fixture.Normalize {
		if NormalizeCapacity(row.Input) != row.Expected {
			t.Fatal("capacity normalization")
		}
	}
}

func testRules() Rules {
	return Rules{Version: 1, Enabled: true, Bases: map[string]json.Number{"LIKE_NEW": "294", "ALMOST_NEW": "270", "GOOD": "248", "MODERATE": "236"}, FaultCosts: map[string]json.Number{"BROKEN": "43", "BROKEN_SCREEN": "105", "BATTERY_ISSUE": "23", "BROKEN_FRAME": "99", "BROKEN_BACK_CAMERA": "58"}, DamageFee: "13.98", Floor: "25", BatteryThreshold: 80, UnknownBatteryIssue: true}
}
func TestPricingBoundaries(t *testing.T) {
	r := testRules()
	got, e := Calculate(r, "LIKE_NEW", []string{"BROKEN_SCREEN", "BROKEN_SCREEN"})
	if e != nil || got != "175.02" {
		t.Fatal(got, e)
	}
	for _, invalid := range []string{"-1", "1.001", "1000000.01", "NaN", "", "1/2"} {
		bad := testRules()
		bad.DamageFee = json.Number(invalid)
		if bad.Validate() == nil {
			t.Fatalf("accepted amount %q", invalid)
		}
	}
	for _, valid := range []string{"0", "1.00", "1e2", "1000000"} {
		r.DamageFee = json.Number(valid)
		if r.Validate() != nil {
			t.Fatal("rejected valid amount", valid)
		}
	}
	r = testRules()
	r.Bases["GOOD"] = "300"
	if r.Validate() == nil {
		t.Fatal("unordered grades")
	}
	r = testRules()
	r.Bases["UNKNOWN"] = "0"
	if r.Validate() == nil {
		t.Fatal("unknown grade")
	}
	r = testRules()
	r.Floor = "237"
	if r.Validate() == nil {
		t.Fatal("floor above moderate")
	}
	r = testRules()
	r.BatteryThreshold = 101
	if r.Validate() == nil {
		t.Fatal("battery threshold")
	}
	r = testRules()
	if _, e := Calculate(r, "SEALED_BOX", nil); e == nil {
		t.Fatal("sealed questionnaire grade")
	}
	if _, e := Calculate(r, "GOOD", []string{"UNKNOWN"}); e == nil {
		t.Fatal("unknown fault")
	}
	a := Assessment{Battery: "unknown", Checks: make([]*bool, 5), Faults: []string{}, Glass: []string{}}
	if a.ValidatePartial() != nil {
		t.Fatal("valid unanswered questionnaire")
	}
	a.Battery = "101"
	if a.ValidatePartial() == nil {
		t.Fatal("invalid battery")
	}
	a.Battery = "90"
	a.Checks = nil
	if a.ValidatePartial() == nil {
		t.Fatal("missing checks")
	}
}
