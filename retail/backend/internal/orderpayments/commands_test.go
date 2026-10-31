package orderpayments

import (
	"strings"
	"testing"
)

func TestSupplementalConsentAmount(t *testing.T) {
	in := Input{OperationID: "00000000-0000-4000-8000-000000000001", OrderID: "order_fixture", ExpectedVersion: 3, CollectionID: "paycol_fixture", Amount: "6.00", CurrencyCode: "eur"}
	if !valid(in) {
		t.Fatal("exact positive consent rejected")
	}
	for _, amount := range []string{"6", "6.0", "06.00", "6.000", "-6.00", "0.00", "1e8", "1/2", "NaN", strings.Repeat("9", 10000) + ".00"} {
		bad := in
		bad.Amount = amount
		if valid(bad) {
			t.Fatal("unsupported money accepted")
		}
	}
	bad := in
	bad.PreviousOperationID = in.OperationID
	if valid(bad) {
		t.Fatal("self predecessor accepted")
	}
	bad = in
	bad.ExpectedVersion = 0
	if valid(bad) {
		t.Fatal("missing consent version accepted")
	}
	bad = in
	bad.CurrencyCode = "EUR"
	if valid(bad) {
		t.Fatal("noncanonical currency accepted")
	}
}
