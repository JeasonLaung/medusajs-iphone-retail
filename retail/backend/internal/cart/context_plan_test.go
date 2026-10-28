package cart

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type contextOracleRegion struct {
	ID        string `json:"id"`
	Currency  string `json:"currency_code"`
	Countries []struct {
		Code string `json:"iso_2"`
	} `json:"countries"`
}

func (r contextOracleRegion) region() ContextRegion {
	out := ContextRegion{ID: r.ID, Currency: r.Currency}
	for _, country := range r.Countries {
		out.Countries = append(out.Countries, country.Code)
	}
	return out
}

type contextOracleState struct {
	Region  contextOracleRegion `json:"region"`
	Locale  *string             `json:"locale"`
	Address ContextAddress      `json:"shipping_address"`
}

func oracleChange(t *testing.T, input map[string]json.RawMessage) ContextChange {
	t.Helper()
	var change ContextChange
	if value, ok := input["region_id"]; ok {
		if err := json.Unmarshal(value, &change.RegionID); err != nil {
			t.Fatal(err)
		}
	}
	if value, ok := input["locale"]; ok {
		change.Locale.Present = true
		if err := json.Unmarshal(value, &change.Locale.Value); err != nil {
			t.Fatal(err)
		}
	}
	if value, ok := input["shipping_address"]; ok {
		change.Address.Present = true
		if err := json.Unmarshal(value, &change.Address.Value); err != nil {
			t.Fatal(err)
		}
	}
	return change
}

func planProjection(plan ContextPlan) map[string]any {
	out := map[string]any{"region_id": plan.RegionID, "currency_code": plan.Currency}
	if plan.Locale.Present {
		out["locale"] = plan.Locale.Value
	}
	if plan.Address.Present {
		out["shipping_address"] = plan.Address.Value
	}
	return out
}

func TestContextPlanMatchesOriginalWorkflow(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/legacy-cart-region.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version int `json:"format_version"`
		Cases   []struct {
			Name  string `json:"name"`
			Input struct {
				Input  map[string]json.RawMessage `json:"input"`
				Region contextOracleRegion        `json:"region"`
				Cart   contextOracleState         `json:"cartToUpdate"`
			} `json:"input"`
			Output map[string]json.RawMessage `json:"output"`
			Error  *struct {
				Type string `json:"type"`
			} `json:"error"`
		} `json:"cases"`
		Workflow []struct {
			Name   string                     `json:"name"`
			Input  map[string]json.RawMessage `json:"input"`
			Cart   contextOracleState         `json:"cartToUpdate"`
			Output struct {
				RegionChanged      bool `json:"region_changed"`
				RemoveCustomPrices bool `json:"remove_custom_prices"`
				ForceRefresh       bool `json:"force_refresh"`
			} `json:"output"`
		} `json:"workflow_cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 2 || len(fixture.Cases) != 12 || len(fixture.Workflow) != 16 {
		t.Fatal("original workflow oracle coverage changed")
	}
	for _, test := range fixture.Cases {
		t.Run("prepare/"+test.Name, func(t *testing.T) {
			current := ContextState{Region: test.Input.Cart.Region.region(), Locale: test.Input.Cart.Locale, Address: test.Input.Cart.Address}
			change := oracleChange(t, test.Input.Input)
			before, _ := json.Marshal([]any{current, change})
			plan, err := PlanContext(current, change, test.Input.Region.region())
			after, _ := json.Marshal([]any{current, change})
			if string(before) != string(after) {
				t.Fatal("planning changed caller input")
			}
			if test.Error != nil {
				if err != ErrRegion || test.Error.Type != "invalid_data" {
					t.Fatal("original invalid address was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			// Only the region/locale/address projection is in this command's scope.
			want := map[string]json.RawMessage{}
			for _, key := range []string{"region_id", "currency_code", "locale", "shipping_address"} {
				if value, ok := test.Output[key]; ok {
					want[key] = value
				}
			}
			encoded, _ := json.Marshal(planProjection(plan))
			var actual, expected any
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			encoded, _ = json.Marshal(want)
			if err := json.Unmarshal(encoded, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("original preparation differs: got %v want %v", actual, expected)
			}
		})
	}
	for _, test := range fixture.Workflow {
		t.Run("refresh/"+test.Name, func(t *testing.T) {
			change := oracleChange(t, test.Input)
			target := test.Cart.Region.region()
			if change.RegionID == "region-gb" {
				target = ContextRegion{ID: "region-gb", Currency: "gbp", Countries: []string{"gb"}}
			}
			plan, err := PlanContext(ContextState{Region: test.Cart.Region.region(), Locale: test.Cart.Locale, Address: test.Cart.Address}, change, target)
			if err != nil {
				t.Fatal(err)
			}
			if plan.RegionChanged != test.Output.RegionChanged || plan.RemoveCustomPrices != test.Output.RemoveCustomPrices || plan.ForceRefresh != test.Output.ForceRefresh {
				t.Fatalf("original workflow decision differs: %+v", plan)
			}
		})
	}
}

func TestContextPlanDoesNotAliasCaller(t *testing.T) {
	locale, city, country := "zh-CN", "sample", "dk"
	address := ContextAddress{"city": &city, "country_code": &country, "province": nil}
	region := ContextRegion{ID: "region-eu", Currency: "eur", Countries: []string{"dk", "de"}}
	change := ContextChange{Locale: ContextField[string]{Present: true, Value: &locale}, Address: ContextField[ContextAddress]{Present: true, Value: &address}}
	plan, err := PlanContext(ContextState{Region: region}, change, ContextRegion{})
	if err != nil {
		t.Fatal(err)
	}
	*plan.Locale.Value = "en"
	*(*plan.Address.Value)["city"] = "changed"
	delete(*plan.Address.Value, "country_code")
	if locale != "zh-CN" || city != "sample" || address["country_code"] == nil {
		t.Fatal("result aliases caller's data")
	}
	if _, err := PlanContext(ContextState{Region: region}, ContextChange{RegionID: "region-gb"}, region); err != ErrRegion {
		t.Fatal("unresolved target region accepted")
	}
}
