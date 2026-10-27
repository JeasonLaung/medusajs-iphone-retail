// Package totals calculates reviewed historical and checkout amount arithmetic.
// It does not select prices/promotions/taxes, authorize purchases, settle money,
// mutate input, or confer ownership of orders. Callers supply trusted snapshots.
package totals

type TaxLine struct{ Rate Number }
type Adjustment struct {
	Amount         *Number
	IsTaxInclusive bool
}
type Quantities struct {
	Fulfilled, Shipped, ReturnRequested, ReturnReceived, ReturnDismissed, WrittenOff *Number
}
type Item struct {
	ID                  string
	UnitPrice, Quantity Number
	IsTaxInclusive      *bool
	TaxLines            []TaxLine
	Adjustments         []Adjustment
	Detail              Quantities
}
type Shipping struct {
	ID             string
	Amount         Number
	IsTaxInclusive *bool
	TaxLines       []TaxLine
	Adjustments    []Adjustment
}
type Credit struct{ Amount *Number }
type SummaryInput struct{ TransactionTotal *Number }
type SummaryResult struct {
	PendingDifference string `json:"pending_difference"`
}
type Input struct {
	Items                        []Item
	ShippingMethods              []Shipping
	CreditLines                  []Credit
	IncludeTaxes, AutomaticTaxes bool
	// Must come from the authoritative currency definition, never a guessed
	// default or client-selected rounding policy. Zero-decimal currencies use 0.
	CurrencyDecimals *int
	Summary          *SummaryInput
}
type Row struct {
	ID          string              `json:"id"`
	Totals      map[string]string   `json:"totals"`
	Adjustments []map[string]string `json:"adjustments,omitempty"`
}
type Result struct {
	Totals          map[string]string   `json:"totals"`
	Items           []Row               `json:"items"`
	ShippingMethods []Row               `json:"shipping_methods"`
	CreditLines     []map[string]string `json:"credit_lines"`
	Summary         *SummaryResult      `json:"summary,omitempty"`
}
type values map[string]decimal

func (v values) strings() map[string]string {
	out := map[string]string{}
	for k, d := range v {
		out[k] = d.text()
	}
	return out
}

type taxRates struct {
	sum   decimal
	parts []decimal
}

func (t taxRates) on(base decimal) decimal {
	total := integer(0)
	for _, r := range t.parts {
		total = total.add(base.mul(r))
	}
	return total
}
func taxes(lines []TaxLine) (taxRates, error) {
	if len(lines) > 100 {
		return taxRates{}, ErrInput
	}
	rate := integer(0)
	parts := make([]decimal, 0, len(lines))
	for _, line := range lines {
		n, err := line.Rate.value()
		if err != nil {
			return taxRates{}, err
		}
		rate = rate.add(n)
		p, err := n.div(integer(100))
		if err != nil {
			return taxRates{}, err
		}
		parts = append(parts, p)
	}
	sum, err := rate.div(integer(100))
	return taxRates{sum, parts}, err
}
func inclusive(flag *bool, fallback bool) bool {
	if flag != nil {
		return *flag
	}
	return fallback
}
func optional(n *Number) (decimal, error) {
	if n == nil {
		return integer(0), nil
	}
	return n.value()
}

type discounts struct {
	subtotal, total, tax, perUnit decimal
	rows                          []map[string]string
}

func adjustments(rows []Adjustment, rate, quantity decimal) (discounts, error) {
	d := discounts{subtotal: integer(0), total: integer(0), tax: integer(0), perUnit: integer(0), rows: make([]map[string]string, 0, len(rows))}
	if len(rows) > 100 {
		return d, ErrInput
	}
	for _, a := range rows {
		if a.Amount == nil {
			d.rows = append(d.rows, map[string]string{})
			continue
		}
		sub, err := a.Amount.value()
		if err != nil {
			return d, err
		}
		if a.IsTaxInclusive {
			sub, err = sub.div(integer(1).add(rate))
			if err != nil {
				return d, err
			}
		}
		tax := sub.mul(rate)
		total := sub.add(tax)
		d.subtotal = d.subtotal.add(sub)
		d.total = d.total.add(total)
		d.tax = d.tax.add(tax)
		d.rows = append(d.rows, map[string]string{"subtotal": sub.text(), "total": total.text()})
	}
	if !quantity.zero() {
		var err error
		d.perUnit, err = d.subtotal.div(quantity)
		if err != nil {
			return d, err
		}
	}
	return d, nil
}
func itemTotals(item Item, fallback bool) (values, []map[string]string, error) {
	price, err := item.UnitPrice.value()
	if err != nil {
		return nil, nil, err
	}
	quantity, err := item.Quantity.value()
	if err != nil {
		return nil, nil, err
	}
	rates, err := taxes(item.TaxLines)
	if err != nil {
		return nil, nil, err
	}
	rate := rates.sum
	taxInclusive := inclusive(item.IsTaxInclusive, fallback)
	fullPrice := price.mul(quantity)
	sub := fullPrice
	if taxInclusive {
		sub, err = fullPrice.div(integer(1).add(rate))
		if err != nil {
			return nil, nil, err
		}
	}
	d, err := adjustments(item.Adjustments, rate, quantity)
	if err != nil {
		return nil, nil, err
	}
	received, err := optional(item.Detail.ReturnReceived)
	if err != nil {
		return nil, nil, err
	}
	dismissed, err := optional(item.Detail.ReturnDismissed)
	if err != nil {
		return nil, nil, err
	}
	requested, err := optional(item.Detail.ReturnRequested)
	if err != nil {
		return nil, nil, err
	}
	currentQty := quantity.sub(received).sub(dismissed)
	currentPrice := price.mul(currentQty)
	currentSub := currentPrice
	if taxInclusive {
		currentSub, err = currentPrice.div(integer(1).add(rate))
		if err != nil {
			return nil, nil, err
		}
	}
	currentDiscount := d.perUnit.mul(currentQty)
	tax := rates.on(currentSub.sub(currentDiscount))
	originalTax := rates.on(currentSub)
	originalTotal := currentSub.add(originalTax)
	if taxInclusive {
		originalTotal = currentPrice
	}
	v := values{"subtotal": currentSub, "total": currentSub.sub(currentDiscount).add(tax), "tax_total": tax,
		"original_subtotal": originalTotal.sub(originalTax), "original_total": originalTotal, "original_tax_total": originalTax,
		"discount_subtotal": currentDiscount, "discount_tax_total": originalTax.sub(tax), "discount_total": currentDiscount.add(originalTax.sub(tax))}
	if item.Detail.ReturnRequested != nil || item.Detail.ReturnReceived != nil || item.Detail.ReturnDismissed != nil {
		refundQty := currentQty.sub(requested)
		// The original refundable branch tests the item flag itself, rather than
		// the inherited includeTax context. Preserve that distinction.
		refundInclusive := item.IsTaxInclusive != nil && *item.IsTaxInclusive
		discount := d.subtotal
		if refundInclusive {
			discount = d.total
		}
		perUnit, err := discount.div(quantity)
		if err != nil {
			return nil, nil, err
		}
		refundSub := refundQty.mul(price).sub(refundQty.mul(perUnit))
		refundTotal := refundSub
		if !refundInclusive {
			refundTotal = refundTotal.add(rates.on(refundSub))
		}
		refundPerUnit := integer(0)
		if !refundQty.zero() {
			refundPerUnit, err = refundTotal.div(refundQty)
			if err != nil {
				return nil, nil, err
			}
		}
		v["refundable_total"] = refundTotal
		v["refundable_total_per_unit"] = refundPerUnit
	}
	divisor := quantity
	if divisor.zero() {
		divisor = integer(1)
	}
	netFull := sub.sub(d.subtotal)
	perUnit, err := netFull.add(rates.on(netFull)).div(divisor)
	if err != nil {
		return nil, nil, err
	}
	for _, extra := range []struct {
		name string
		n    *Number
	}{
		{"fulfilled_total", item.Detail.Fulfilled}, {"shipped_total", item.Detail.Shipped}, {"return_requested_total", item.Detail.ReturnRequested},
		{"return_received_total", item.Detail.ReturnReceived}, {"return_dismissed_total", item.Detail.ReturnDismissed}, {"write_off_total", item.Detail.WrittenOff},
	} {
		if extra.n != nil {
			n, err := extra.n.value()
			if err != nil {
				return nil, nil, err
			}
			v[extra.name] = perUnit.mul(n)
		}
	}
	return v, d.rows, nil
}
func shippingTotals(s Shipping, fallback bool) (values, []map[string]string, error) {
	amount, err := s.Amount.value()
	if err != nil {
		return nil, nil, err
	}
	rates, err := taxes(s.TaxLines)
	if err != nil {
		return nil, nil, err
	}
	rate := rates.sum
	taxInclusive := inclusive(s.IsTaxInclusive, fallback)
	sub := amount
	if taxInclusive {
		sub, err = amount.div(integer(1).add(rate))
		if err != nil {
			return nil, nil, err
		}
	}
	d, err := adjustments(s.Adjustments, rate, integer(1))
	if err != nil {
		return nil, nil, err
	}
	tax := rates.on(sub.sub(d.subtotal))
	originalTax := rates.on(sub)
	originalTotal := sub.add(originalTax)
	if taxInclusive {
		originalTotal = amount
	}
	return values{"subtotal": sub, "total": sub.sub(d.subtotal).add(tax), "tax_total": tax,
		"original_subtotal": originalTotal.sub(originalTax), "original_total": originalTotal, "original_tax_total": originalTax,
		"discount_subtotal": d.subtotal, "discount_tax_total": d.tax, "discount_total": d.total}, d.rows, nil
}

func Calculate(input Input) (Result, error) {
	if input.CurrencyDecimals == nil || *input.CurrencyDecimals < 0 || *input.CurrencyDecimals > 18 || len(input.Items) > 1000 || len(input.ShippingMethods) > 100 || len(input.CreditLines) > 1000 {
		return Result{}, ErrInput
	}
	// Repeated source IDs would make legacy decoration silently reuse another
	// row's totals. An ambiguous projection cannot be used as financial evidence.
	for _, ids := range [][]string{itemIDs(input.Items), shippingIDs(input.ShippingMethods)} {
		seen := map[string]bool{}
		for _, id := range ids {
			if id != "" {
				if seen[id] {
					return Result{}, ErrInput
				}
				seen[id] = true
			}
		}
	}
	v := values{}
	for _, k := range []string{"subtotal", "tax_total", "discount_total", "discount_subtotal", "discount_tax_total", "original_total", "original_subtotal", "original_tax_total", "item_total", "item_subtotal", "item_tax_total", "item_discount_total", "original_item_total", "original_item_subtotal", "original_item_tax_total", "shipping_total", "shipping_subtotal", "shipping_tax_total", "shipping_discount_total", "original_shipping_total", "original_shipping_subtotal", "original_shipping_tax_total"} {
		v[k] = integer(0)
	}
	r := Result{Items: make([]Row, 0, len(input.Items)), ShippingMethods: make([]Row, 0, len(input.ShippingMethods)), CreditLines: make([]map[string]string, 0, len(input.CreditLines))}
	fallback := input.IncludeTaxes || input.AutomaticTaxes
	for _, item := range input.Items {
		t, adj, err := itemTotals(item, fallback)
		if err != nil {
			return Result{}, err
		}
		r.Items = append(r.Items, Row{ID: item.ID, Totals: t.strings(), Adjustments: adj})
		for _, k := range []string{"subtotal", "discount_total", "discount_subtotal", "discount_tax_total", "original_total", "original_tax_total"} {
			v[k] = v[k].add(t[k])
		}
		v["original_subtotal"] = v["original_subtotal"].add(t["subtotal"])
		for _, p := range [][2]string{{"item_total", "total"}, {"item_subtotal", "subtotal"}, {"item_tax_total", "tax_total"}, {"item_discount_total", "discount_total"}, {"original_item_total", "original_total"}, {"original_item_subtotal", "subtotal"}, {"original_item_tax_total", "original_tax_total"}} {
			v[p[0]] = v[p[0]].add(t[p[1]])
		}
		for _, k := range []string{"fulfilled_total", "shipped_total", "return_requested_total", "return_received_total", "return_dismissed_total", "write_off_total"} {
			if n, ok := t[k]; ok {
				if _, ok := v[k]; !ok {
					v[k] = integer(0)
				}
				v[k] = v[k].add(n)
			}
		}
	}
	for _, s := range input.ShippingMethods {
		t, adj, err := shippingTotals(s, fallback)
		if err != nil {
			return Result{}, err
		}
		r.ShippingMethods = append(r.ShippingMethods, Row{ID: s.ID, Totals: t.strings(), Adjustments: adj})
		for _, k := range []string{"subtotal", "discount_total", "discount_subtotal", "discount_tax_total", "original_total", "original_tax_total"} {
			v[k] = v[k].add(t[k])
		}
		v["original_subtotal"] = v["original_subtotal"].add(t["subtotal"])
		for _, p := range [][2]string{{"shipping_total", "total"}, {"shipping_subtotal", "subtotal"}, {"shipping_tax_total", "tax_total"}, {"shipping_discount_total", "discount_total"}, {"original_shipping_total", "original_total"}, {"original_shipping_subtotal", "subtotal"}, {"original_shipping_tax_total", "original_tax_total"}} {
			v[p[0]] = v[p[0]].add(t[p[1]])
		}
	}
	credits := integer(0)
	for _, cl := range input.CreditLines {
		if cl.Amount == nil {
			r.CreditLines = append(r.CreditLines, map[string]string{})
			continue
		}
		n, err := cl.Amount.value()
		if err != nil {
			return Result{}, err
		}
		credits = credits.add(n)
		r.CreditLines = append(r.CreditLines, map[string]string{"subtotal": n.text(), "total": n.text()})
	}
	// Legacy credit-line tolerance is a currency unit (<=, including one full
	// minor unit), not a rounding of every line or an epsilon for all totals.
	epsilon := newDecimalEpsilon(*input.CurrencyDecimals)
	if newAbs(credits).r.Cmp(epsilon.r) <= 0 {
		credits = integer(0)
	}
	v["credit_line_total"] = credits
	v["credit_line_subtotal"] = credits
	v["credit_line_tax_total"] = integer(0)
	v["tax_total"] = v["item_tax_total"].add(v["shipping_tax_total"])
	v["total"] = v["subtotal"].add(v["tax_total"]).sub(v["discount_subtotal"]).sub(credits)
	if input.Summary != nil {
		transaction, err := optional(input.Summary.TransactionTotal)
		if err != nil {
			return Result{}, err
		}
		pending := integer(0)
		if n, ok := v["return_requested_total"]; ok {
			pending = n
		}
		difference := v["total"].sub(pending).sub(transaction)
		if newAbs(difference).r.Cmp(epsilon.r) <= 0 {
			difference = integer(0)
		}
		r.Summary = &SummaryResult{PendingDifference: difference.text()}
	}
	r.Totals = v.strings()
	return r, nil
}
func itemIDs(items []Item) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}
func shippingIDs(rows []Shipping) []string {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}
