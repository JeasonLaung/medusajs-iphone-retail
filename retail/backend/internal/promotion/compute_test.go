package promotion

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestOriginalComputeCandidates(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/promotion-compute.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name       string
			Promotions []struct {
				ID, Code, Type string
				Used           int64
				Limit          *int64
				TaxInclusive   bool `json:"is_tax_inclusive"`
				Rules          []Rule
				Method         *struct {
					Type, Value   string
					Target        string `json:"target_type"`
					Allocation    *string
					Currency      *string `json:"currency_code"`
					MaxQuantity   *int64  `json:"max_quantity"`
					ApplyQuantity *int64  `json:"apply_to_quantity"`
					MinimumBuy    *int64  `json:"buy_rules_min_quantity"`
					BuyRules      []Rule  `json:"buy_rules"`
					TargetRules   []Rule  `json:"target_rules"`
				} `json:"application_method"`
				Campaign *struct{ Budget *CampaignBudget }
			}
			Context         json.RawMessage
			AttributeUsages map[string]AttributeUsageSnapshot
			Expected        ComputeResult
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 19 {
		t.Fatal("compute fixture coverage changed")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			in := ComputeInput{Context: row.Context, Epsilon: "0.0001", AttributeUsages: row.AttributeUsages}
			for _, p := range row.Promotions {
				candidate := ActiveConfiguration{ID: p.ID, Code: p.Code, Type: p.Type, Used: p.Used, Limit: p.Limit, TaxInclusive: p.TaxInclusive, Rules: p.Rules}
				if m := p.Method; m != nil {
					candidate.Method = &ApplicationMethod{Type: m.Type, Value: m.Value, Target: m.Target, Allocation: m.Allocation, Currency: m.Currency, MaxQuantity: m.MaxQuantity, ApplyQuantity: m.ApplyQuantity, MinimumBuy: m.MinimumBuy, BuyRules: m.BuyRules, TargetRules: m.TargetRules}
				}
				if p.Campaign != nil {
					candidate.Budget = p.Campaign.Budget
				}
				in.Candidates = append(in.Candidates, candidate)
			}
			before, _ := json.Marshal(in)
			got, err := ComputeCandidates(in)
			if err != nil || !reflect.DeepEqual(got, row.Expected) {
				t.Fatalf("got %+v %v want %+v", got, err, row.Expected)
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("compute mutated configuration or context")
			}
		})
	}
}

func TestComputeRejectsUntrustedOrMissingFacts(t *testing.T) {
	allocation, attribute, limit := "each", "customer_id", "1"
	p := ActiveConfiguration{ID: "p", Code: "SAVE", Type: "standard", Method: &ApplicationMethod{Type: "fixed", Target: "items", Value: "1", Allocation: &allocation}}
	base := ComputeInput{Candidates: []ActiveConfiguration{p}, Context: json.RawMessage(`{"items":[{"id":"a","subtotal":"10","quantity":1}],"customer_id":"customer"}`), Epsilon: "0.0001"}
	expectError := func(in ComputeInput) {
		t.Helper()
		got, err := ComputeCandidates(in)
		if err == nil || !reflect.DeepEqual(got, ComputeResult{}) {
			t.Fatalf("invalid facts leaked partial result: %+v %v", got, err)
		}
	}
	for _, context := range []string{`[]`, `{}`, `{"items":[{"id":"a","subtotal":"10","quantity":0}]}`, `{"items":[{"id":"a","subtotal":"10","quantity":"1.5"}]}`} {
		in := base
		in.Context = json.RawMessage(context)
		expectError(in)
	}
	in := base
	in.Candidates = append([]ActiveConfiguration{p}, p)
	expectError(in)
	p.Budget = &CampaignBudget{ID: "budget", Type: "use_by_attribute", Used: "0", Limit: &limit, Attribute: &attribute}
	base.Candidates = []ActiveConfiguration{p}
	expectError(base) // missing lookup must not mean no usage
	for _, usage := range []AttributeUsageSnapshot{
		{Attribute: attribute, Value: "different", Found: false},
		{Attribute: "customer_email", Value: "customer", Found: false},
		{Attribute: attribute, Value: "customer", Found: true, Used: "NaN"},
	} {
		in := base
		in.AttributeUsages = map[string]AttributeUsageSnapshot{"budget": usage}
		expectError(in)
	}
	p.Budget.Type = "spend_by_attribute"
	expectError(base)
	p.Budget.Type = "spend"
	p.Budget.Used = "-1"
	expectError(base)
}
