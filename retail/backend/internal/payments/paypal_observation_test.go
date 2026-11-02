package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func paypalObservedFixture(status string) map[string]any {
	order := paypalOrder()
	unit := order["purchase_units"].([]any)[0].(map[string]any)
	transaction := map[string]any{"id": "7B123456789012345", "status": "COMPLETED", "amount": map[string]any{"currency_code": "EUR", "value": "9007199254740993.01"}, "final_capture": true}
	switch status {
	case "approved":
		order["status"] = "APPROVED"
	case "created":
	case "voided":
		order["status"] = "VOIDED"
	case "authorized", "authorization-pending", "authorization-voided":
		order["intent"] = "AUTHORIZE"
		order["status"] = "COMPLETED"
		transaction["status"] = map[string]string{"authorized": "CREATED", "authorization-pending": "PENDING", "authorization-voided": "VOIDED"}[status]
		delete(transaction, "final_capture")
		unit["payments"] = map[string]any{"authorizations": []any{transaction}}
	default:
		order["status"] = "COMPLETED"
		transaction["status"] = map[string]string{"paid": "COMPLETED", "pending": "PENDING", "declined": "DECLINED", "refunded": "REFUNDED", "partial-refund": "PARTIALLY_REFUNDED"}[status]
		unit["payments"] = map[string]any{"captures": []any{transaction}}
	}
	return order
}

func TestPayPalAuthenticatedObservationStates(t *testing.T) {
	for _, provider := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
		for _, test := range []struct{ source, status, action, local string }{
			{"created", "pending", "pending", "pending"}, {"approved", "pending", "pending", "pending"}, {"voided", "canceled", "canceled", "canceled"},
			{"authorized", "authorized", "authorized", "authorized"}, {"authorization-pending", "pending", "pending", "pending"}, {"authorization-voided", "canceled", "canceled", "canceled"},
			{"paid", "paid", "successful", "authorized"}, {"pending", "pending", "pending", "pending"}, {"declined", "failed", "failed", "error"},
		} {
			t.Run(provider+"/"+test.source, func(t *testing.T) {
				order := paypalObservedFixture(test.source)
				config := paypalConfig()
				config.ProviderID = provider
				config.Intent = order["intent"].(string)
				a, e := NewPayPalCreateAdapter(config)
				if e != nil {
					t.Fatal(e)
				}
				r := paypalRequest()
				r.ProviderID = provider
				calls := 0
				a.client.Transport = mollieTransport(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.URL.Host != "api-m.sandbox.paypal.com" {
						t.Fatal("fixed endpoint changed")
					}
					if req.URL.Path == "/v1/oauth2/token" {
						return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"artificial","token_type":"Bearer","expires_in":3600}`))}, nil
					}
					if req.Method != "GET" || req.URL.Path != "/v2/checkout/orders/5O190127TN364715T" || req.Header.Get("Authorization") != "Bearer artificial" {
						t.Fatal("observation made a mutation or used a foreign locator")
					}
					raw, _ := json.Marshal(order)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
				})
				registry, _ := NewCreateAdapters(map[string]CreateSessionAdapter{provider: a})
				observer, e := registry.Observer(provider)
				if e != nil {
					t.Fatal(e)
				}
				observed, e := InvokeSessionObservation(context.Background(), observer, r, "5O190127TN364715T")
				if e != nil || observed.ProviderStatus != test.status || observed.Action != test.action || observed.Session.Status != test.local || observed.Amount != "9007199254740993.01" || calls != 2 {
					t.Fatalf("incorrect money evidence %+v %v", observed, e)
				}
				if ValidatePayPalObservation(r, observed.PaymentID, observed) != nil {
					t.Fatal("valid observation rejected")
				}
				for _, mutate := range []func(*SessionObservation){func(o *SessionObservation) { o.Amount = "9007199254740993.010000000000000001" }, func(o *SessionObservation) { o.Currency = "usd" }, func(o *SessionObservation) { o.PaymentID = "7B123456789012345" }, func(o *SessionObservation) { o.ProviderStatus = "forged" }, func(o *SessionObservation) { o.Action = "forged" }, func(o *SessionObservation) { o.Session.Status = "captured" }} {
					bad := observed
					mutate(&bad)
					if ValidatePayPalObservation(r, observed.PaymentID, bad) == nil {
						t.Fatal("tampered projection accepted")
					}
				}
			})
		}
	}
}

func TestPayPalObservationRejectsContradictoryMoney(t *testing.T) {
	for _, name := range []string{"merchant", "cart", "invoice", "currency", "amount", "id", "intent", "multi-unit", "multi-capture", "partial", "non-final", "missing-final", "capture-currency", "capture-amount", "capture-id", "completed-without-capture", "refunded", "partial-refund", "unknown-capture", "capture-with-voided-order", "authorization-for-capture", "ambiguous-auth", "bad-json", "duplicate", "oversize", "404", "redirect"} {
		t.Run(name, func(t *testing.T) {
			order := paypalObservedFixture("paid")
			unit := order["purchase_units"].([]any)[0].(map[string]any)
			capture := unit["payments"].(map[string]any)["captures"].([]any)[0].(map[string]any)
			switch name {
			case "merchant":
				unit["payee"].(map[string]any)["merchant_id"] = "ZZZ1234567890"
			case "cart":
				unit["custom_id"] = "cart_foreign"
			case "invoice":
				unit["invoice_id"] = "payses_foreign"
			case "currency":
				unit["amount"].(map[string]any)["currency_code"] = "USD"
			case "amount":
				unit["amount"].(map[string]any)["value"] = "1"
			case "id":
				order["id"] = "7B123456789012345"
			case "intent":
				order["intent"] = "AUTHORIZE"
			case "multi-unit":
				order["purchase_units"] = []any{unit, unit}
			case "multi-capture":
				unit["payments"].(map[string]any)["captures"] = []any{capture, capture}
			case "partial":
				capture["amount"].(map[string]any)["value"] = "1"
			case "non-final":
				capture["final_capture"] = false
			case "missing-final":
				delete(capture, "final_capture")
			case "capture-currency":
				capture["amount"].(map[string]any)["currency_code"] = "USD"
			case "capture-amount":
				capture["amount"].(map[string]any)["value"] = "9007199254740993.010000000000000001"
			case "capture-id":
				capture["id"] = "../bad"
			case "completed-without-capture":
				delete(unit, "payments")
			case "refunded":
				capture["status"] = "REFUNDED"
			case "partial-refund":
				capture["status"] = "PARTIALLY_REFUNDED"
			case "unknown-capture":
				capture["status"] = "NEW_STATUS"
			case "capture-with-voided-order":
				order["status"] = "VOIDED"
			case "authorization-for-capture":
				unit["payments"] = map[string]any{"authorizations": []any{capture}}
			case "ambiguous-auth":
				unit["payments"].(map[string]any)["authorizations"] = []any{capture, capture}
			}
			a, _ := NewPayPalCreateAdapter(paypalConfig())
			calls := 0
			a.client.Transport = mollieTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Path == "/v1/oauth2/token" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"artificial","token_type":"Bearer","expires_in":3600}`))}, nil
				}
				if req.Method != "GET" {
					t.Fatal("observation mutated provider")
				}
				raw, _ := json.Marshal(order)
				code := 200
				switch name {
				case "bad-json":
					raw = []byte(`{`)
				case "duplicate":
					raw = []byte(`{"id":"5O190127TN364715T","id":"7B123456789012345"}`)
				case "oversize":
					raw = []byte(`{"blob":"` + strings.Repeat("x", 1<<20) + `"}`)
				case "404":
					code = 404
				case "redirect":
					code = 302
				}
				return &http.Response{StatusCode: code, Header: http.Header{"Location": []string{"https://foreign.invalid"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})
			observed, e := InvokeSessionObservation(context.Background(), a, paypalRequest(), "5O190127TN364715T")
			if e == nil || observed.PaymentID != "" || calls != 2 {
				t.Fatal("contradictory payment accepted", name, e, calls)
			}
		})
	}
	for _, id := range []string{"", "../id", "https://foreign.invalid", "tr_mollie"} {
		a, _ := NewPayPalCreateAdapter(paypalConfig())
		a.client.Transport = mollieTransport(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid locator reached provider")
			return nil, nil
		})
		if _, e := a.ObserveSession(context.Background(), paypalRequest(), id); e == nil {
			t.Fatal("invalid locator accepted")
		}
	}
}
