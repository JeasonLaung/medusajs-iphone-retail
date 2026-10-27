package totals

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type wire map[string]json.RawMessage

func numberAt(row wire, key string) (Number, error) {
	n := Number{}
	if raw, ok := row["raw_"+key]; ok && string(raw) != "null" {
		n.Raw = &RawValue{}
		if json.Unmarshal(raw, n.Raw) != nil {
			return n, ErrInput
		}
		return n, nil
	}
	raw, ok := row[key]
	if !ok || string(raw) == "null" {
		return n, ErrInput
	}
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(raw, &n.Decimal) != nil {
			return n, ErrInput
		}
	} else {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var v any
		if dec.Decode(&v) != nil {
			return n, ErrInput
		}
		j, ok := v.(json.Number)
		if !ok {
			return n, ErrInput
		}
		n.Decimal = j.String()
	}
	return n, nil
}
func optionalAt(row wire, key string) (*Number, error) {
	raw, hasRaw := row["raw_"+key]
	value, hasValue := row[key]
	if (!hasRaw || string(raw) == "null") && (!hasValue || string(value) == "null") {
		return nil, nil
	}
	n, err := numberAt(row, key)
	return &n, err
}
func itemFrom(row wire) (Item, error) {
	var out Item
	json.Unmarshal(row["id"], &out.ID)
	var err error
	out.UnitPrice, err = numberAt(row, "unit_price")
	if err != nil {
		return out, err
	}
	out.Quantity, err = numberAt(row, "quantity")
	if err != nil {
		return out, err
	}
	if v, ok := row["is_tax_inclusive"]; ok && json.Unmarshal(v, &out.IsTaxInclusive) != nil {
		return out, ErrInput
	}
	out.TaxLines, out.Adjustments, err = relationsFrom(row)
	if err != nil {
		return out, err
	}
	var detail wire
	if v, ok := row["detail"]; ok && json.Unmarshal(v, &detail) != nil {
		return out, ErrInput
	}
	for _, p := range []struct {
		k  string
		to **Number
	}{{"fulfilled_quantity", &out.Detail.Fulfilled}, {"shipped_quantity", &out.Detail.Shipped}, {"return_requested_quantity", &out.Detail.ReturnRequested}, {"return_received_quantity", &out.Detail.ReturnReceived}, {"return_dismissed_quantity", &out.Detail.ReturnDismissed}, {"written_off_quantity", &out.Detail.WrittenOff}} {
		*p.to, err = optionalAt(detail, p.k)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func relationsFrom(row wire) ([]TaxLine, []Adjustment, error) {
	var taxesWire, adjWire []wire
	if v, ok := row["tax_lines"]; ok && json.Unmarshal(v, &taxesWire) != nil {
		return nil, nil, ErrInput
	}
	if v, ok := row["adjustments"]; ok && json.Unmarshal(v, &adjWire) != nil {
		return nil, nil, ErrInput
	}
	taxes := []TaxLine{}
	adjustments := []Adjustment{}
	for _, r := range taxesWire {
		n, err := numberAt(r, "rate")
		if err != nil {
			return nil, nil, err
		}
		taxes = append(taxes, TaxLine{n})
	}
	for _, r := range adjWire {
		n, err := optionalAt(r, "amount")
		if err != nil {
			return nil, nil, err
		}
		var flag bool
		if v, ok := r["is_tax_inclusive"]; ok && json.Unmarshal(v, &flag) != nil {
			return nil, nil, ErrInput
		}
		adjustments = append(adjustments, Adjustment{n, flag})
	}
	return taxes, adjustments, nil
}
func inputFrom(row wire, precision int) (Input, error) {
	out := Input{CurrencyDecimals: &precision}
	var region struct {
		AutomaticTaxes bool `json:"automatic_taxes"`
	}
	if v, ok := row["region"]; ok && json.Unmarshal(v, &region) != nil {
		return out, ErrInput
	}
	out.AutomaticTaxes = region.AutomaticTaxes
	var itemsWire, shippingWire, creditsWire []wire
	for _, p := range []struct {
		k  string
		to *[]wire
	}{{"items", &itemsWire}, {"shipping_methods", &shippingWire}, {"credit_lines", &creditsWire}} {
		if v, ok := row[p.k]; ok && json.Unmarshal(v, p.to) != nil {
			return out, ErrInput
		}
	}
	for _, r := range itemsWire {
		n, err := itemFrom(r)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, n)
	}
	for _, r := range shippingWire {
		var s Shipping
		json.Unmarshal(r["id"], &s.ID)
		var err error
		s.Amount, err = numberAt(r, "amount")
		if err != nil {
			return out, err
		}
		if v, ok := r["is_tax_inclusive"]; ok && json.Unmarshal(v, &s.IsTaxInclusive) != nil {
			return out, ErrInput
		}
		s.TaxLines, s.Adjustments, err = relationsFrom(r)
		if err != nil {
			return out, err
		}
		out.ShippingMethods = append(out.ShippingMethods, s)
	}
	for _, r := range creditsWire {
		n, err := optionalAt(r, "amount")
		if err != nil {
			return out, err
		}
		out.CreditLines = append(out.CreditLines, Credit{n})
	}
	if v, ok := row["summary"]; ok && string(v) != "null" {
		var summary wire
		if json.Unmarshal(v, &summary) != nil {
			return out, ErrInput
		}
		n, err := optionalAt(summary, "transaction_total")
		if err != nil {
			return out, err
		}
		out.Summary = &SummaryInput{TransactionTotal: n}
	}
	return out, nil
}

func TestLegacyParity(t *testing.T) {
	type expected map[string]struct {
		Decimal string `json:"decimal"`
	}
	var cases, fields int
	compare := func(t *testing.T, actual map[string]string, want expected) {
		t.Helper()
		for key, value := range want {
			fields++
			if got, ok := actual[key]; !ok || got != value.Decimal {
				t.Fatalf("field %s: got %q want %q", key, got, value.Decimal)
			}
		}
	}
	for _, filename := range []string{"legacy-totals.json", "legacy-totals-boundaries.json"} {
		data, err := os.ReadFile(filepath.Join("../../../..", "docs/migration/fixtures", filename))
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			Cases []struct {
				Name             string
				Input            wire
				CurrencyDecimals *int `json:"currency_decimals"`
				Totals           expected
				Summary          expected
				Items            []struct {
					ID          string
					Totals      expected
					Adjustments []expected
				}
				Shipping []struct {
					ID     string
					Totals expected
				} `json:"shipping_methods"`
			}
		}
		if json.Unmarshal(data, &fixture) != nil {
			t.Fatal("invalid oracle fixture")
		}
		for _, c := range fixture.Cases {
			cases++
			t.Run(c.Name, func(t *testing.T) {
				digits := 2
				if c.CurrencyDecimals != nil {
					digits = *c.CurrencyDecimals
				}
				input, err := inputFrom(c.Input, digits)
				if err != nil {
					t.Fatal("oracle input translation failed")
				}
				before, _ := json.Marshal(input)
				result, err := Calculate(input)
				if err != nil {
					t.Fatal("valid oracle input rejected")
				}
				after, _ := json.Marshal(input)
				if !bytes.Equal(before, after) {
					t.Fatal("input mutated")
				}
				compare(t, result.Totals, c.Totals)
				if c.Summary != nil {
					if result.Summary == nil {
						t.Fatal("summary missing")
					}
					compare(t, map[string]string{"pending_difference": result.Summary.PendingDifference}, c.Summary)
				} else if result.Summary != nil {
					t.Fatal("summary invented")
				}
				if len(result.Items) != len(c.Items) || len(result.ShippingMethods) != len(c.Shipping) {
					t.Fatal("row count differs")
				}
				for i, row := range c.Items {
					if result.Items[i].ID != row.ID || len(result.Items[i].Adjustments) != len(row.Adjustments) {
						t.Fatal("item identity or adjustment count differs")
					}
					compare(t, result.Items[i].Totals, row.Totals)
					for j, adj := range row.Adjustments {
						compare(t, result.Items[i].Adjustments[j], adj)
					}
				}
				for i, row := range c.Shipping {
					if result.ShippingMethods[i].ID != row.ID {
						t.Fatal("shipping identity differs")
					}
					compare(t, result.ShippingMethods[i].Totals, row.Totals)
				}
			})
		}
	}
	fmt.Printf("TOTALS_PARITY {\"cases\":%d,\"exact_decimal_fields\":%d,\"input_unchanged\":true}\n", cases, fields)
}
