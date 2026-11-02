package catalogimport

import "testing"

func TestCartHistoryScopeCannotGrantOrdinaryImports(t *testing.T) {
	names := CartHistoryTables()
	if len(names) != 10 || names[0] != "cart_address" || names[1] != "cart" {
		t.Fatal("cart dependency order changed")
	}
	names[0] = "customer"
	if CartHistoryTables()[0] != "cart_address" {
		t.Fatal("caller changed cart scope")
	}
	for _, name := range []string{"cart", "cart_ownership", "customer", "payment", "cart_checkout_attempts"} {
		if ImportCartHistory(t.Context(), nil, "", name, nil) != ErrPolicy {
			t.Fatal("unreviewed cart import accepted")
		}
	}
	all, err := historyLayouts()
	if err != nil {
		t.Fatal("compiled cart layouts unavailable")
	}
	columns := 0
	for _, name := range CartHistoryTables() {
		columns += len(all[name].Columns)
	}
	if columns != 136 {
		t.Fatal("reviewed cart source shape changed")
	}
}
