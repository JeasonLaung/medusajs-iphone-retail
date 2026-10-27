package totals

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func num(value string) *Number { return &Number{Decimal: value} }
func baseInput() Input {
	digits := 2
	flag := false
	return Input{CurrencyDecimals: &digits, Items: []Item{{ID: "validation-item", UnitPrice: *num("100"), Quantity: *num("2"), IsTaxInclusive: &flag, TaxLines: []TaxLine{{Rate: *num("25")}}}}}
}
func TestInvalidInputsRejectWholeResult(t *testing.T) {
	checks := []struct {
		name   string
		change func(*Input)
	}{
		{"missing-currency", func(i *Input) { i.CurrencyDecimals = nil }},
		{"negative-currency-digits", func(i *Input) { *i.CurrencyDecimals = -1 }},
		{"excess-currency-digits", func(i *Input) { *i.CurrencyDecimals = 19 }},
		{"missing-price", func(i *Input) { i.Items[0].UnitPrice = Number{} }},
		{"NaN-price", func(i *Input) { i.Items[0].UnitPrice = *num("NaN") }},
		{"infinite-tax", func(i *Input) { i.Items[0].TaxLines[0].Rate = *num("Infinity") }},
		{"bad-quantity", func(i *Input) { i.Items[0].Quantity = *num("1/2") }},
		{"space-price", func(i *Input) { i.Items[0].UnitPrice = *num(" 1") }},
		{"locale-price", func(i *Input) { i.Items[0].UnitPrice = *num("1,25") }},
		{"huge-exponent", func(i *Input) { i.Items[0].UnitPrice = *num("1e999999") }},
		{"huge-value", func(i *Input) { i.Items[0].UnitPrice = *num(strings.Repeat("9", 129)) }},
		{"huge-fraction", func(i *Input) { i.Items[0].UnitPrice = *num("0." + strings.Repeat("1", 65)) }},
		{"raw-invalid-no-fallback", func(i *Input) { i.Items[0].UnitPrice.Raw = &RawValue{Value: "NaN", Precision: 38} }},
		{"raw-zero-precision", func(i *Input) { i.Items[0].Quantity.Raw = &RawValue{Value: "1", Precision: 0} }},
		{"raw-excess-precision", func(i *Input) { i.Items[0].Quantity.Raw = &RawValue{Value: "1", Precision: 101} }},
		{"bad-adjustment", func(i *Input) { i.Items[0].Adjustments = []Adjustment{{Amount: num("bad")}} }},
		{"bad-shipping", func(i *Input) { i.ShippingMethods = []Shipping{{Amount: *num("bad")}} }},
		{"bad-credit", func(i *Input) { i.CreditLines = []Credit{{Amount: num("bad")}} }},
		{"bad-summary", func(i *Input) { i.Summary = &SummaryInput{TransactionTotal: num("bad")} }},
		{"bad-extra-quantity", func(i *Input) { i.Items[0].Detail.Shipped = num("bad") }},
		{"singular-tax-inclusive", func(i *Input) {
			flag := true
			i.Items[0].IsTaxInclusive = &flag
			i.Items[0].TaxLines[0].Rate = *num("-100")
		}},
		{"zero-quantity-refund-undefined", func(i *Input) { i.Items[0].Quantity = *num("0"); i.Items[0].Detail.ReturnRequested = num("0") }},
		{"duplicate-item-id", func(i *Input) { i.Items = append(i.Items, i.Items[0]) }},
		{"duplicate-shipping-id", func(i *Input) {
			i.ShippingMethods = []Shipping{{ID: "same", Amount: *num("1")}, {ID: "same", Amount: *num("1")}}
		}},
		{"too-many-items", func(i *Input) { i.Items = make([]Item, 1001) }},
		{"too-many-shipping", func(i *Input) { i.ShippingMethods = make([]Shipping, 101) }},
		{"too-many-credits", func(i *Input) { i.CreditLines = make([]Credit, 1001) }},
		{"too-many-taxes", func(i *Input) { i.Items[0].TaxLines = make([]TaxLine, 101) }},
		{"too-many-adjustments", func(i *Input) { i.Items[0].Adjustments = make([]Adjustment, 101) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			input := baseInput()
			check.change(&input)
			before, _ := json.Marshal(input)
			result, err := Calculate(input)
			after, _ := json.Marshal(input)
			if !errors.Is(err, ErrInput) || !reflect.DeepEqual(result, Result{}) {
				t.Fatal("invalid arithmetic returned a partial or successful result")
			}
			if string(before) != string(after) {
				t.Fatal("rejected input mutated")
			}
		})
	}
	fmt.Printf("TOTALS_REJECTIONS {\"invalid_cases\":%d,\"partial_results\":false,\"rejected_input_unchanged\":true}\n", len(checks))
}
func TestDecimalRoundingAndExactOperations(t *testing.T) {
	for _, row := range []struct{ n, d, want string }{
		{"1", "3", "0.33333333333333333333"}, {"2", "3", "0.66666666666666666667"},
		{"-2", "3", "-0.66666666666666666667"}, {"2", "-3", "-0.66666666666666666667"},
		{"0.000000000000000000005", "1", "0.00000000000000000001"},
		{"-0.000000000000000000005", "1", "-0.00000000000000000001"},
		{"0.0000000000000000000049", "1", "0"},
		{"9007199254740993.000000000000000001", "1", "9007199254740993.000000000000000001"},
	} {
		n, e := parse(row.n)
		if e != nil {
			t.Fatal(e)
		}
		d, e := parse(row.d)
		if e != nil {
			t.Fatal(e)
		}
		got, e := n.div(d)
		if e != nil || got.text() != row.want {
			t.Fatalf("division %s/%s got %s want %s", row.n, row.d, got.text(), row.want)
		}
	}
	n, _ := parse("9007199254740993.000000000000000001")
	d, _ := parse("1.000000000000000001")
	if got := n.mul(d).text(); got != "9007199254740993.009007199254740994000000000000000001" {
		t.Fatalf("exact product lost digits: %s", got)
	}
	if got := n.add(d).sub(d).text(); got != "9007199254740993.000000000000000001" {
		t.Fatal("addition/subtraction lost digits")
	}
	if got := n.text(); got != "9007199254740993.000000000000000001" {
		t.Fatal("operand mutated")
	}
	if _, err := n.div(integer(0)); !errors.Is(err, ErrInput) {
		t.Fatal("zero division accepted")
	}
}
func TestParallelCalculationAndIndependentResults(t *testing.T) {
	input := baseInput()
	input.Items[0].Adjustments = []Adjustment{{Amount: num("20")}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 25 {
				result, err := Calculate(input)
				if err != nil || result.Totals["total"] != "225" {
					t.Error("parallel arithmetic differs")
				}
				result.Totals["total"] = "changed"
				result.Items[0].Totals["total"] = "changed"
			}
		})
	}
	wg.Wait()
	result, err := Calculate(input)
	if err != nil || result.Totals["total"] != "225" || result.Items[0].Totals["total"] != "225" {
		t.Fatal("prior result mutation leaked")
	}
}
