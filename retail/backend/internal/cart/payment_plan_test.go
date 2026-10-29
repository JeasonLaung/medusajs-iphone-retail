package cart

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestPaymentRefreshMatchesOriginalCallbacks(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/cart-payment-refresh.json")
	var fixture struct {
		Cases []struct {
			Name     string
			Input    PaymentRefreshFacts
			Expected PaymentRefreshPlan
		}
	}
	if err != nil || json.Unmarshal(data, &fixture) != nil || len(fixture.Cases) != 15 {
		t.Fatal("original payment refresh fixtures missing")
	}
	for _, row := range fixture.Cases {
		t.Run(row.Name, func(t *testing.T) {
			got, err := PlanPaymentRefresh(row.Input)
			if err != nil || !reflect.DeepEqual(got, row.Expected) {
				t.Fatalf("got %+v %v want %+v", got, err, row.Expected)
			}
		})
	}
}

func TestPaymentRefreshInvalidFacts(t *testing.T) {
	for _, input := range []PaymentRefreshFacts{
		{Amount: "NaN", Currency: "eur"}, {Amount: "-1", Currency: "eur"}, {Amount: "0", Currency: "EUR"},
		{Amount: "1", Currency: "eur", Collection: &PaymentRefreshCollection{ID: "c", Currency: "eur", Amount: "bad"}},
		{Amount: "1", Currency: "eur", Collection: &PaymentRefreshCollection{ID: "c", Currency: "eur", Amount: "0", SessionIDs: []string{"a", "a"}}},
		{Amount: "1", Currency: "eur", Collection: &PaymentRefreshCollection{ID: "c", Currency: "eur", Amount: "0", SessionIDs: make([]string, 101)}},
	} {
		if got, err := PlanPaymentRefresh(input); err == nil || !reflect.DeepEqual(got, PaymentRefreshPlan{}) {
			t.Fatal("invalid payment facts returned plan")
		}
	}
}
