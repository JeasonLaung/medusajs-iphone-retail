package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func mollieCreateFixture() map[string]any {
	body := mollieFixture("open", true)
	body["resource"] = "payment"
	body["mode"] = "test"
	body["redirectUrl"] = "https://shop.example/api/mollie-return?cart_id=cart_owned"
	body["_links"] = map[string]any{"checkout": map[string]any{"href": "https://www.mollie.com/checkout/owned"}}
	body["large"] = json.RawMessage(`9007199254740993.000000000000000000001`)
	return body
}

func mollieCreateConfig() MollieCreateConfig {
	return MollieCreateConfig{ProviderID: "pp_mollie-hosted-checkout_mollie", APIKey: "test_artificial",
		RedirectURL: "https://shop.example/api/mollie-return?cart_id=cart_owned", BackendURL: "https://api.example/base?ignored=1"}
}

func mollieCreateRequest() SessionRequest {
	r := mollieRequest()
	r.ProviderID = "pp_mollie-hosted-checkout_mollie"
	r.Context = json.RawMessage(`{"idempotency_key":"payses_owned","private":"not-for-provider"}`)
	return r
}

func TestMollieCreateRequestAndResult(t *testing.T) {
	for provider, method := range mollieMethods {
		for _, manual := range []bool{false, true} {
			t.Run(provider+map[bool]string{false: "automatic", true: "manual"}[manual], func(t *testing.T) {
				config := mollieCreateConfig()
				config.ProviderID = provider
				config.ManualCapture = manual
				a, err := NewMollieCreateAdapter(config)
				if err != nil {
					t.Fatal(err)
				}
				request := mollieCreateRequest()
				request.ProviderID = provider
				calls := 0
				var firstBody string
				a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.Method != "POST" || r.URL.String() != "https://api.mollie.com/v2/payments" ||
						r.GetBody != nil ||
						r.Header.Get("Authorization") != "Bearer test_artificial" || r.Header.Get("Idempotency-Key") != request.SessionID ||
						r.Header.Get("Content-Type") != "application/json" {
						t.Fatal("incorrect fixed API request")
					}
					deadline, ok := r.Context().Deadline()
					if !ok || time.Until(deadline) > 8*time.Second {
						t.Fatal("HTTP deadline missing")
					}
					raw, _ := io.ReadAll(r.Body)
					if calls == 1 {
						firstBody = string(raw)
					} else if firstBody != string(raw) {
						t.Fatal("retry request changed")
					}
					var params map[string]json.RawMessage
					if json.Unmarshal(raw, &params) != nil || strings.Contains(string(raw), "not-for-provider") || strings.Contains(string(raw), "tr_owned") {
						t.Fatal("private context or old payment leaked")
					}
					if string(params["amount"]) != `{"currency":"EUR","value":"9007199254740993.01"}` ||
						string(params["webhookUrl"]) != `"https://api.example/hooks/payment/`+strings.TrimPrefix(provider, "pp_")+`"` ||
						string(params["redirectUrl"]) != `"https://shop.example/api/mollie-return?cart_id=cart_owned"` ||
						string(params["description"]) != `"iPhone retail order"` {
						t.Fatal("legacy create parameters differ")
					}
					capture := "automatic"
					if manual {
						capture = "manual"
					}
					if string(params["captureMode"]) != `"`+capture+`"` {
						t.Fatal("capture configuration lost")
					}
					if method == "" {
						if _, ok := params["method"]; ok {
							t.Fatal("hosted method must be omitted")
						}
					} else if string(params["method"]) != `"`+method+`"` {
						t.Fatal("provider method changed")
					}
					response, _ := json.Marshal(mollieCreateFixture())
					return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(response))), Request: r}, nil
				})
				for i := 0; i < 2; i++ {
					outcome, result, err := InvokeCreate(context.Background(), a, request)
					if outcome != CreateSucceeded || err != nil || result.Status != "pending" {
						t.Fatalf("create failed %s %v", outcome, err)
					}
					var saved map[string]json.RawMessage
					if json.Unmarshal(result.Data, &saved) != nil || string(saved["large"]) != `9007199254740993.000000000000000000001` || string(saved["session_id"]) != `"payses_owned"` || string(saved["cart_id"]) != `"cart_owned"` {
						t.Fatal("provider result lost identity or exact JSON")
					}
				}
				if calls != 2 {
					t.Fatal("unexpected automatic retries")
				}
			})
		}
	}
}

func TestMollieCreateResponseFailures(t *testing.T) {
	for _, mode := range []string{"id", "session", "cart", "amount", "currency", "resource", "mode", "redirect", "checkout-user", "checkout-host", "checkout-scheme", "checkout-port", "checkout-missing", "status", "json", "duplicate", "oversize", "transport", "deadline", "http302", "http200", "http401", "http422", "http429", "http500"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			calls := 0
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				fixture := mollieCreateFixture()
				code := 201
				header := http.Header{}
				switch mode {
				case "id":
					fixture["id"] = "../../other"
				case "session":
					fixture["metadata"].(map[string]any)["session_id"] = "payses_other"
				case "cart":
					fixture["metadata"].(map[string]any)["cart_id"] = "cart_other"
				case "amount":
					fixture["amount"].(map[string]any)["value"] = "9007199254740993.02"
				case "currency":
					fixture["amount"].(map[string]any)["currency"] = "USD"
				case "resource":
					fixture["resource"] = "refund"
				case "mode":
					fixture["mode"] = "live"
				case "redirect":
					fixture["redirectUrl"] = "https://untrusted.example/"
				case "checkout-user":
					fixture["_links"] = map[string]any{"checkout": map[string]string{"href": "https://user:secret@www.mollie.com/"}}
				case "checkout-host":
					fixture["_links"] = map[string]any{"checkout": map[string]string{"href": "https://mollie.com.untrusted.example/"}}
				case "checkout-scheme":
					fixture["_links"] = map[string]any{"checkout": map[string]string{"href": "http://www.mollie.com/"}}
				case "checkout-port":
					fixture["_links"] = map[string]any{"checkout": map[string]string{"href": "https://www.mollie.com:9443/"}}
				case "checkout-missing":
					delete(fixture, "_links")
				case "status":
					fixture["status"] = "unexpected"
				case "transport":
					return nil, errors.New("secret-private-provider-error")
				case "deadline":
					<-r.Context().Done()
					return nil, r.Context().Err()
				case "http302":
					code = 302
					header.Set("Location", "https://untrusted.example/")
				case "http200":
					code = 200
				case "http401":
					code = 401
				case "http422":
					code = 422
				case "http429":
					code = 429
				case "http500":
					code = 500
				}
				raw, _ := json.Marshal(fixture)
				if mode == "json" {
					raw = []byte(`{"id":`)
				}
				if mode == "duplicate" {
					raw = []byte(`{"id":"tr_owned","id":"tr_other"}`)
				}
				if mode == "oversize" {
					raw = []byte(`{"large":"` + strings.Repeat("x", 1<<20) + `"}`)
				}
				return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			outcome, result, err := InvokeCreate(ctx, a, mollieCreateRequest())
			if outcome != CreateUncertain || err != ErrCreateExecution || result.Status != "" || len(result.Data) != 0 || calls != 1 {
				t.Fatal("unverified result escaped or repeated HTTP")
			}
		})
	}
}

func TestMollieCreateStatuses(t *testing.T) {
	for remote, local := range map[string]string{"open": "pending", "pending": "pending", "paid": "authorized", "authorized": "authorized", "canceled": "canceled", "expired": "error", "failed": "error"} {
		t.Run(remote, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				fixture := mollieCreateFixture()
				fixture["status"] = remote
				if local != "pending" {
					delete(fixture, "_links")
				}
				raw, _ := json.Marshal(fixture)
				return &http.Response{StatusCode: 201, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
			})
			outcome, result, err := InvokeCreate(context.Background(), a, mollieCreateRequest())
			if outcome != CreateSucceeded || err != nil || result.Status != local {
				t.Fatal("verified external status lost")
			}
		})
	}
}

func TestMollieCreateLocalRefusals(t *testing.T) {
	for _, mutate := range []func(*SessionRequest){
		func(r *SessionRequest) { r.ProviderID = "other" }, func(r *SessionRequest) { r.Amount = "-1" }, func(r *SessionRequest) { r.Currency = "EUR" },
		func(r *SessionRequest) { r.SessionID = "other" }, func(r *SessionRequest) { r.CollectionID = "" },
		func(r *SessionRequest) { r.Data = json.RawMessage(`{"cart_id":"invalid"}`) },
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","session_id":"payses_other"}`)
		},
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","mollie_attempt":"changing-attempt"}`)
		},
		func(r *SessionRequest) { r.Context = json.RawMessage(`{"idempotency_key":"changing-attempt"}`) },
		func(r *SessionRequest) {
			r.Context = json.RawMessage(`{"idempotency_key":"payses_owned","idempotency_key":"other"}`)
		},
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","redirect_url":"https://shop.example:9443/api/mollie-return?cart_id=cart_owned"}`)
		},
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","redirect_url":"https://other.example/api/mollie-return?cart_id=cart_owned"}`)
		},
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","redirect_url":"https://user:pass@shop.example/api/mollie-return?cart_id=cart_owned"}`)
		},
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","redirect_url":"https://shop.example/other?cart_id=cart_owned"}`)
		},
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","redirect_url":"https://shop.example/api%2Fmollie-return?cart_id=cart_owned"}`)
		},
		func(r *SessionRequest) {
			r.Data = json.RawMessage(`{"cart_id":"cart_owned","redirect_url":"https://shop.example/api/mollie-return?cart_id=cart_other"}`)
		},
	} {
		a, _ := NewMollieCreateAdapter(mollieCreateConfig())
		a.transport.client.Transport = mollieTransport(func(*http.Request) (*http.Response, error) { t.Fatal("invalid input issued HTTP"); return nil, nil })
		request := mollieCreateRequest()
		mutate(&request)
		outcome, result, err := InvokeCreate(context.Background(), a, request)
		if outcome != CreateFailed || err != nil || result.Status != "" {
			t.Fatal("local refusal classification wrong")
		}
	}
	for _, mutate := range []func(*MollieCreateConfig){
		func(c *MollieCreateConfig) { c.ProviderID = "browser-arbitrary-method" }, func(c *MollieCreateConfig) { c.APIKey = "secret\nheader" },
		func(c *MollieCreateConfig) { c.RedirectURL = "https://shop.example/other" }, func(c *MollieCreateConfig) { c.BackendURL = "file:///private" },
		func(c *MollieCreateConfig) { c.Description = strings.Repeat("字", 256) },
	} {
		config := mollieCreateConfig()
		mutate(&config)
		if _, err := NewMollieCreateAdapter(config); err == nil {
			t.Fatal("invalid server config accepted")
		}
	}
}

func TestMollieExactSettlement(t *testing.T) {
	for raw, want := range map[string]string{"0": "0.00", "0.0049": "0.00", "0.005": "0.01", "1.005": "1.01", "2.675": "2.68", "9.999": "10.00", "1e-2": "0.01", "9007199254740993.015": "9007199254740993.02"} {
		got, err := mollieAmount(raw)
		if err != nil || got != want {
			t.Fatalf("%s -> %s, want %s", raw, got, want)
		}
		payment := mollieFixture("open", true)
		payment["amount"].(map[string]any)["value"] = want
		encoded, _ := json.Marshal(payment)
		var parsed molliePayment
		_ = json.Unmarshal(encoded, &parsed)
		request := mollieRequest()
		request.Amount = raw
		if !verifyMolliePayment(parsed, request, "tr_owned", "cart_owned") {
			t.Fatal("create/delete settlement rules diverged")
		}
	}
}

func TestMollieCreateAcceptsNewCartUUID(t *testing.T) {
	a, err := NewMollieCreateAdapter(mollieCreateConfig())
	if err != nil {
		t.Fatal(err)
	}
	request := mollieCreateRequest()
	request.Data = json.RawMessage(`{"cart_id":"cart_12345678-1234-4234-8234-123456789012","redirect_url":"https://shop.example/api/mollie-return?cart_id=cart_12345678-1234-4234-8234-123456789012"}`)
	if _, _, _, err = a.createBody(request); err != nil {
		t.Fatal("new runtime cart UUID rejected")
	}
}
