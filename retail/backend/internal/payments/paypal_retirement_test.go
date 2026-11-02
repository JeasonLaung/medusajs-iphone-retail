package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPayPalRetirementOnlyObservesAnUnsettledBoundOrder(t *testing.T) {
	for _, provider := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
		for _, state := range []string{"created", "approved", "voided", "paid", "pending", "authorized", "authorization-pending", "authorization-voided", "declined", "refunded", "wrong-merchant", "wrong-money", "unknown-order", "network"} {
			t.Run(provider+"/"+state, func(t *testing.T) {
				order := paypalObservedFixture(state)
				if state == "wrong-merchant" || state == "wrong-money" || state == "unknown-order" || state == "network" {
					order = paypalObservedFixture("created")
				}
				unit := order["purchase_units"].([]any)[0].(map[string]any)
				if state == "wrong-merchant" {
					unit["payee"] = map[string]string{"merchant_id": "ZZZ1234567890"}
				}
				if state == "wrong-money" {
					unit["amount"] = map[string]string{"value": "0.01", "currency_code": "EUR"}
				}
				cfg := paypalConfig()
				cfg.ProviderID = provider
				cfg.Intent = order["intent"].(string)
				a, err := NewPayPalCreateAdapter(cfg)
				if err != nil {
					t.Fatal(err)
				}
				request := paypalRequest()
				request.ProviderID = provider
				request.Data = json.RawMessage(`{"cart_id":"cart_owned","session_id":"payses_owned","paypal":{"order_id":"5O190127TN364715T"}}`)
				calls := 0
				a.client.Transport = mollieTransport(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.URL.Host != "api-m.sandbox.paypal.com" {
						t.Fatal("foreign endpoint")
					}
					if req.URL.Path == "/v1/oauth2/token" {
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"artificial","token_type":"Bearer","expires_in":3600}`))}, nil
					}
					if req.Method != "GET" || req.URL.Path != "/v2/checkout/orders/5O190127TN364715T" || req.Header.Get("Authorization") != "Bearer artificial" {
						t.Fatal("retirement mutated a remote order or used a foreign locator")
					}
					if state == "network" {
						return nil, errors.New("owned lost query")
					}
					if state == "unknown-order" {
						return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
					}
					raw, _ := json.Marshal(order)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				})
				adapter, err := a.RetirementAdapter()
				if err != nil {
					t.Fatal(err)
				}
				out, err := adapter.DeleteSession(context.Background(), request)
				if calls != 2 {
					t.Fatal("unexpected calls", calls)
				}
				switch state {
				case "created", "approved", "voided":
					if err != nil || out != DeleteSucceeded {
						t.Fatal("unsettled retirement failed", out, err)
					}
				case "paid", "pending", "authorized", "authorization-pending", "authorization-voided", "declined":
					if err != nil || out != DeleteFailed {
						t.Fatal("transaction history discarded", out, err)
					}
				default:
					if err == nil || out != DeleteUncertain {
						t.Fatal("unverified retirement", out, err)
					}
				}
				calls = 0
				request.Data = json.RawMessage(`{"cart_id":"cart_owned","paypal":{"order_id":"foreign"}}`)
				if out, err = adapter.DeleteSession(context.Background(), request); err == nil || out != DeleteUncertain || calls != 0 {
					t.Fatal("bad locator called provider")
				}
			})
		}
	}
}
