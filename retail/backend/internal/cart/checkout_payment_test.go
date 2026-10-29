package cart

import (
	"bytes"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"testing"
)

func TestInitialPaymentClosedConfiguration(t *testing.T) {
	ring, err := protected.New("test", map[string][]byte{"test": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := payments.NewMollieCreateAdapter(payments.MollieCreateConfig{ProviderID: hostedMollie, APIKey: "test_artificial", RedirectURL: "https://shop.example/api/mollie-return", BackendURL: "https://api.example"})
	if err != nil {
		t.Fatal(err)
	}
	adapters, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{hostedMollie: adapter})
	if err != nil {
		t.Fatal(err)
	}
	if initialOptions(InitialPaymentOptions{Ring: ring, Adapters: adapters, ReturnOrigin: "https://shop.example"}) != nil {
		t.Fatal("valid explicit options rejected")
	}
	for _, origin := range []string{"", "https://shop.example/path", "https://user@shop.example", "https://shop.example?", "https://shop.example#x", "http://shop.example"} {
		if initialOptions(InitialPaymentOptions{Ring: ring, Adapters: adapters, ReturnOrigin: origin}) == nil {
			t.Fatal("incomplete/foreign initial payment configuration accepted")
		}
	}
	for _, test := range []struct{ in, want string }{{"158.75", "158.75"}, {"9007199254740993.005", "9007199254740993.01"}, {"0.004", "0.00"}, {"0.005", "0.01"}} {
		got, err := payments.MollieSettlementAmount(test.in)
		if err != nil || got != test.want {
			t.Fatalf("settlement: %s %v", got, err)
		}
	}
}
