package pricing

import (
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/totals"
)

func ptr[T any](v T) *T { return &v }

func TestOmittedShippingQuantity(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	base := testPrice("base", "100")
	smallMax := testPrice("small-max", "30")
	smallMax.Max = ptr(totals.Number{Decimal: "0.5"})
	largeMin := testPrice("large-min", "1")
	largeMin.Min = ptr(totals.Number{Decimal: "1.000000000000000001"})
	c := Context{Region: "europe", Currency: "eur", QuantityOmitted: true}
	q, err := Select([]Price{base, smallMax, largeMin}, nil, c, now)
	if err != nil || q == nil || q.Amount != "30" {
		t.Fatal("omitted quantity filter differs")
	}
	c.QuantityOmitted, c.Quantity = false, "1"
	q, err = Select([]Price{base, smallMax, largeMin}, nil, c, now)
	if err != nil || q == nil || q.Amount != "100" {
		t.Fatal("explicit quantity one changed")
	}
	c.Quantity = ""
	if _, err := Select([]Price{base}, nil, c, now); err != ErrInput {
		t.Fatal("missing product quantity accepted")
	}
	c.QuantityOmitted, c.Quantity = true, "1"
	if _, err := Select([]Price{base}, nil, c, now); err != ErrInput {
		t.Fatal("contradictory quantity accepted")
	}
}
func testPrice(id, amount string) Price {
	return Price{ID: id, SetID: "set", Currency: "eur", Amount: totals.Number{Decimal: amount}, RulesCount: ptr(0)}
}
func listPrice(id, amount, kind string) Price {
	p := testPrice(id, amount)
	p.ListID = ptr(id + "-list")
	p.List = &List{ID: *p.ListID, Type: kind, Status: "active", RulesCount: ptr(0)}
	return p
}
func TestSelectPriceSemantics(t *testing.T) {
	c := Context{Region: "europe", Currency: "eur", Channel: "web", Quantity: "1"}
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	base := testPrice("base", "100")
	regional := testPrice("regional", "120")
	regional.Rules = []Rule{{"region_id", "eq", "europe"}}
	regional.RulesCount = ptr(1)
	unknown := testPrice("unknown", "0.01")
	unknown.Rules = []Rule{{"internal", "eq", "secret"}}
	unknown.RulesCount = ptr(1)
	tier := testPrice("tier", "50")
	tier.Min = &totals.Number{Decimal: "2"}
	tier.Max = &totals.Number{Decimal: "5"}
	wrong := testPrice("wrong-currency", "1")
	wrong.Currency = "usd"
	future := listPrice("future", "1", "sale")
	future.List.StartsAt = ptr(now.Add(time.Second))
	draft := listPrice("draft", "1", "sale")
	draft.List.Status = "draft"
	group := listPrice("group", "5", "override")
	group.List.RulesCount = ptr(1)
	group.List.Rules = []ListRule{{"customer.groups.id", ptr(`["vip"]`)}}
	inconsistent := testPrice("bad-count", "1")
	inconsistent.RulesCount = ptr(1)
	cases := []struct {
		name                           string
		prices                         []Price
		quantity, amount, original, id string
	}{
		{"base", []Price{base}, "1", "100", "100", "base"},
		{"specificity-before-cheapest", []Price{base, regional, unknown}, "1", "120", "120", "regional"},
		{"quantity-too-low", []Price{base, tier}, "1", "100", "100", "base"},
		{"quantity-min-inclusive", []Price{base, tier}, "2", "50", "50", "tier"},
		{"quantity-max-inclusive", []Price{base, tier}, "5", "50", "50", "tier"},
		{"quantity-too-high", []Price{base, tier}, "6", "100", "100", "base"},
		{"active-sale", []Price{base, listPrice("sale", "80", "sale")}, "1", "80", "100", "sale"},
		{"sale-not-more-than-base", []Price{base, listPrice("sale", "120", "sale")}, "1", "100", "100", "base"},
		{"override-can-increase", []Price{base, listPrice("override", "150", "override")}, "1", "150", "150", "override"},
		{"stacked-sale-reference", []Price{base, listPrice("override", "90", "override"), listPrice("sale", "80", "sale")}, "1", "80", "90", "sale"},
		{"zero-base-is-real", []Price{testPrice("zero", "0"), listPrice("sale", "80", "sale")}, "1", "0", "0", "zero"},
		{"zero-sale-is-real", []Price{base, listPrice("sale", "0", "sale")}, "1", "0", "100", "sale"},
		{"no-base-sale", []Price{listPrice("sale", "80", "sale")}, "1", "80", "", "sale"},
		{"future-draft-currency-and-unknown-not-used", []Price{base, future, draft, wrong, unknown, group, inconsistent}, "1", "100", "100", "base"},
		{"stable-tie", []Price{testPrice("z", "100"), base, testPrice("a", "100")}, "1", "100", "100", "a"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			ctx := c
			ctx.Quantity = tt.quantity
			q, e := Select(tt.prices, nil, ctx, now)
			if e != nil || q == nil || q.Amount != tt.amount || q.PriceID != tt.id || tt.original == "" && q.OriginalAmount != nil || tt.original != "" && (q.OriginalAmount == nil || *q.OriginalAmount != tt.original) {
				t.Fatal("configured price selection mismatch")
			}
		})
	}
	for _, prices := range [][]Price{nil, {wrong}, {unknown}, {inconsistent}, {group}} {
		q, e := Select(prices, nil, c, now)
		if e != nil || q != nil {
			t.Fatal("unmatched price became a quote")
		}
	}
	c.CustomerGroups = []string{"vip"}
	q, e := Select([]Price{base, group}, nil, c, now)
	if e != nil || q == nil || q.Amount != "5" {
		t.Fatal("trusted customer group did not match")
	}
	group.List.StartsAt = ptr(now)
	group.List.EndsAt = ptr(now)
	q, e = Select([]Price{group}, nil, c, now)
	if e != nil || q == nil {
		t.Fatal("inclusive validity boundary failed")
	}
	group.List.EndsAt = ptr(now.Add(-time.Nanosecond))
	q, e = Select([]Price{group}, nil, c, now)
	if e != nil || q != nil {
		t.Fatal("expired price list applied")
	}
}
func TestExactNumbersAndTaxPreferences(t *testing.T) {
	c := Context{Region: "europe", Currency: "eur", Quantity: "1"}
	now := time.Now()
	low := testPrice("low", "1")
	low.Amount.Raw = &totals.RawValue{Value: "9007199254740993.123456789012345678", Precision: 38}
	high := testPrice("high", "1")
	high.Amount.Raw = &totals.RawValue{Value: "9007199254740993.123456789012345679", Precision: 38}
	prefs := []Preference{{"currency_code", ptr("eur"), true}, {"region_id", ptr("europe"), false}}
	q, e := Select([]Price{high, low}, prefs, c, now)
	if e != nil || q == nil || q.PriceID != "low" || q.Amount != "9007199254740993.123456789012345678" || !q.CalculatedTaxInclusive {
		t.Fatal("exact raw amount or currency tax preference lost")
	}
	low.Rules = []Rule{{"region_id", "eq", "europe"}}
	low.RulesCount = ptr(1)
	q, e = Select([]Price{low}, prefs, c, now)
	if e != nil || q == nil || q.CalculatedTaxInclusive || q.OriginalTaxInclusive {
		t.Fatal("regional tax preference did not override rule-bound price")
	}
	if _, e = Select([]Price{low}, append(prefs, prefs[0]), c, now); e == nil {
		t.Fatal("ambiguous tax preferences accepted")
	}
	low.Amount = totals.Number{Decimal: "-1"}
	if _, e = Select([]Price{low}, nil, c, now); e == nil {
		t.Fatal("negative price accepted")
	}
	for _, quantity := range []string{"0", "-1", "1.5", "NaN", "1/2", "1e999999999"} {
		c.Quantity = quantity
		if _, e = Select([]Price{high}, nil, c, now); e == nil {
			t.Fatal("invalid purchase quantity accepted")
		}
	}
}
func TestStrictSourceNumbers(t *testing.T) {
	for _, raw := range []string{`{"value":"1.000000000000000001","precision":20}`, `{"value":0,"precision":20}`, `{"value":"1e-20","precision":20}`} {
		if _, e := sourceNumber("1", &raw); e != nil {
			t.Fatal("valid exact source number rejected")
		}
	}
	for _, raw := range []string{`{}`, `null`, `{"value":"1","value":"2","precision":20}`, `{"value":"1","precision":20,"extra":true}`, `{"value":"1/2","precision":20}`, `{"value":"-1","precision":20}`, `{"value":"1","precision":0}`, `{"value":"1","precision":20} {}`} {
		if _, e := sourceNumber("1", &raw); e == nil {
			t.Fatal("malformed source number accepted")
		}
	}
}
