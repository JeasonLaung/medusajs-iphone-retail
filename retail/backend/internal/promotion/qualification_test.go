package promotion

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestOriginalTargetQualification(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/promotion-qualification.json")
	if err != nil {
		t.Fatal(err)
	}
	type method struct {
		Target, Value, Currency string
		MinimumBuy              *int64
		BuyRules, TargetRules   []Rule
	}
	var fixture struct {
		Cases []struct {
			Name      string
			Promotion struct {
				Type   string
				Rules  []Rule
				Method *method
			}
			Context  json.RawMessage
			Expected TargetQualification
		}
		Sorting []struct {
			Name  string
			Input []struct {
				Code, Type                string
				Value                     json.RawMessage
				MinimumBuy, ApplyQuantity *int64
			}
			Expected []string
		}
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 23 || len(fixture.Sorting) != 4 {
		t.Fatal("qualification fixture coverage changed")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			p := ActiveConfiguration{Type: row.Promotion.Type, Rules: row.Promotion.Rules}
			if m := row.Promotion.Method; m != nil {
				p.Method = &ApplicationMethod{Target: m.Target, Value: m.Value, MinimumBuy: m.MinimumBuy, BuyRules: m.BuyRules, TargetRules: m.TargetRules}
				if m.Currency != "" {
					p.Method.Currency = &m.Currency
				}
			}
			before, _ := json.Marshal(p)
			got, err := QualifyTargets(p, row.Context)
			if err != nil || !reflect.DeepEqual(got, row.Expected) {
				t.Fatalf("got %+v %v want %+v", got, err, row.Expected)
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("qualification mutated configuration")
			}
		})
	}
	for _, row := range fixture.Sorting {
		t.Run(row.Name, func(t *testing.T) {
			in := []ActiveConfiguration{}
			for _, p := range row.Input {
				var value string
				if json.Unmarshal(p.Value, &value) != nil {
					value = string(p.Value)
				}
				in = append(in, ActiveConfiguration{Code: p.Code, Type: p.Type, Method: &ApplicationMethod{Value: value, MinimumBuy: p.MinimumBuy, ApplyQuantity: p.ApplyQuantity}})
			}
			before, _ := json.Marshal(in)
			out, err := SortCandidates(in)
			if err != nil {
				t.Fatal(err)
			}
			codes := []string{}
			for _, p := range out {
				codes = append(codes, p.Code)
			}
			if !reflect.DeepEqual(codes, row.Expected) {
				t.Fatalf("got %v want %v", codes, row.Expected)
			}
			after, _ := json.Marshal(in)
			if string(before) != string(after) {
				t.Fatal("sort mutated input order")
			}
		})
	}
}

func TestQualificationRejectsInvalidContext(t *testing.T) {
	p := ActiveConfiguration{Type: "standard", Method: &ApplicationMethod{Target: "items"}}
	for _, context := range []string{
		`[]`, `{} {}`, `{}`, `{"items":null}`, `{"items":[1]}`,
		`{"items":[{"id":""}]}`,
		`{"items":[{"id":"a","subtotal":"NaN","quantity":1}]}`,
		`{"items":[{"id":"a","subtotal":1,"quantity":1},{"id":"a","subtotal":1,"quantity":1}]}`,
	} {
		out, err := QualifyTargets(p, json.RawMessage(context))
		if err == nil || !reflect.DeepEqual(out, TargetQualification{}) {
			t.Fatalf("invalid context granted membership: %s %+v %v", context, out, err)
		}
	}
	p.Method.Target = "shipping_methods"
	if _, err := QualifyTargets(p, json.RawMessage(`{"shipping_methods":[null]}`)); err == nil {
		t.Fatal("null shipping accepted")
	}
	p.Method.Target = "unsupported"
	if _, err := QualifyTargets(p, json.RawMessage(`{}`)); err == nil {
		t.Fatal("unknown target accepted")
	}
}

func TestCandidateSortExactnessAndValidation(t *testing.T) {
	in := []ActiveConfiguration{
		{Code: "missing", Type: "buyget"},
		{Code: "lower", Type: "standard", Method: &ApplicationMethod{Value: "9007199254740992"}},
		{Code: "higher", Type: "standard", Method: &ApplicationMethod{Value: "9007199254740993"}},
	}
	out, err := SortCandidates(in)
	if err != nil || out[0].Code != "higher" || out[1].Code != "lower" || out[2].Code != "missing" {
		t.Fatal("sort lost precision or fabricated method")
	}
	for _, value := range []string{"NaN", "-1", "1e1000000"} {
		if out, err := SortCandidates([]ActiveConfiguration{{Type: "standard", Method: &ApplicationMethod{Value: value}}}); err == nil || out != nil {
			t.Fatal("invalid value accepted")
		}
	}
	if _, err := SortCandidates([]ActiveConfiguration{{Type: "unsupported"}}); err == nil {
		t.Fatal("unknown promotion type accepted")
	}
}
