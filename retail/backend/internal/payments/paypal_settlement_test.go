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

func TestPayPalSettlementKeyBinding(t *testing.T) {
	r := paypalRequest()
	first, e := PayPalSettlementRequestID(r, "5O190127TN364715T", "CAPTURE")
	if e != nil || len(first) != 36 || first == r.SessionID {
		t.Fatal("invalid settlement key")
	}
	if repeated, _ := PayPalSettlementRequestID(r, "5O190127TN364715T", "CAPTURE"); first != repeated {
		t.Fatal("key not stable")
	}
	for _, change := range []func(*SessionRequest){func(r *SessionRequest) { r.SessionID = "payses_other" }, func(r *SessionRequest) { r.CollectionID = "paycol_other" }, func(r *SessionRequest) { r.ProviderID = "pp_paypal_card_paypal_card" }} {
		changed := r
		change(&changed)
		key, e := PayPalSettlementRequestID(changed, "5O190127TN364715T", "CAPTURE")
		if e != nil || key == first {
			t.Fatal("key omits identity")
		}
	}
	for _, change := range []struct{ id, intent string }{{"7B123456789012345", "CAPTURE"}, {"5O190127TN364715T", "AUTHORIZE"}} {
		key, e := PayPalSettlementRequestID(r, change.id, change.intent)
		if e != nil || key == first {
			t.Fatal("key omits action/order")
		}
	}
}

func TestPayPalSettlementFixedActionAndExactEvidence(t *testing.T) {
	for _, provider := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
		for _, intent := range []string{"CAPTURE", "AUTHORIZE"} {
			for _, state := range []string{"complete", "pending"} {
				t.Run(provider+"/"+intent+"/"+state, func(t *testing.T) {
					config := paypalConfig()
					config.ProviderID = provider
					config.Intent = intent
					a, _ := NewPayPalCreateAdapter(config)
					request := paypalRequest()
					request.ProviderID = provider
					posts := 0
					gets := 0
					a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
						if r.URL.Host != "api-m.sandbox.paypal.com" {
							t.Fatal("foreign destination")
						}
						if r.URL.Path == "/v1/oauth2/token" {
							return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"artificial","token_type":"Bearer","expires_in":3600}`))}, nil
						}
						var order map[string]any
						if r.Method == "GET" {
							gets++
							order = paypalObservedFixture("approved")
							order["intent"] = intent
						} else {
							posts++
							action := map[string]string{"CAPTURE": "capture", "AUTHORIZE": "authorize"}[intent]
							key, _ := PayPalSettlementRequestID(request, "5O190127TN364715T", intent)
							body, _ := io.ReadAll(r.Body)
							if r.Method != "POST" || r.URL.Path != "/v2/checkout/orders/5O190127TN364715T/"+action || r.GetBody != nil || r.Header.Get("PayPal-Request-Id") != key || r.Header.Get("Prefer") != "return=representation" || string(body) != "{}" {
								t.Fatal("wrong settlement request")
							}
							status := "paid"
							if state == "pending" {
								status = "pending"
							}
							if intent == "AUTHORIZE" {
								status = "authorized"
								if state == "pending" {
									status = "authorization-pending"
								}
							}
							order = paypalObservedFixture(status)
						}
						raw, _ := json.Marshal(order)
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
					})
					observed, e := InvokePayPalSettlement(context.Background(), a, request, "5O190127TN364715T")
					want := "paid"
					if intent == "AUTHORIZE" {
						want = "authorized"
					}
					if state == "pending" {
						want = "pending"
					}
					if e != nil || observed.ProviderStatus != want || posts != 1 || gets != 1 {
						t.Fatal("settlement failed or repeated", e, posts, gets)
					}
				})
			}
		}
	}
}

func TestPayPalSettlementNoBlindRepeatOrUnapprovedMutation(t *testing.T) {
	for _, test := range []string{"created", "paid", "authorized", "pending", "authorization-pending", "voided", "foreign", "disconnect", "422", "redirect", "contradictory"} {
		t.Run(test, func(t *testing.T) {
			config := paypalConfig()
			if test == "authorized" || test == "authorization-pending" {
				config.Intent = "AUTHORIZE"
			}
			a, _ := NewPayPalCreateAdapter(config)
			posts := 0
			a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v1/oauth2/token" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"artificial","token_type":"Bearer","expires_in":3600}`))}, nil
				}
				if r.Method == "GET" {
					status := test
					if status == "foreign" || status == "disconnect" || status == "422" || status == "redirect" || status == "contradictory" {
						status = "approved"
					}
					order := paypalObservedFixture(status)
					if test == "foreign" {
						order["purchase_units"].([]any)[0].(map[string]any)["custom_id"] = "cart_foreign"
					}
					raw, _ := json.Marshal(order)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				}
				posts++
				if test == "disconnect" {
					return nil, errors.New("lost response after settlement")
				}
				code := 422
				if test == "redirect" {
					code = 302
				}
				if test == "contradictory" {
					code = 201
				}
				raw, _ := json.Marshal(paypalObservedFixture("created"))
				return &http.Response{StatusCode: code, Header: http.Header{"Location": []string{"https://foreign.invalid"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})
			observed, e := InvokePayPalSettlement(context.Background(), a, paypalRequest(), "5O190127TN364715T")
			if test == "paid" || test == "authorized" {
				if e != nil || posts != 0 || observed.ProviderStatus != test {
					t.Fatal("existing money must not be settled again")
				}
			} else {
				if e == nil || observed.PaymentID != "" {
					t.Fatal("unapproved/ambiguous settlement accepted", test)
				}
				if posts > 1 {
					t.Fatal("settlement automatically retried")
				}
			}
		})
	}
}
