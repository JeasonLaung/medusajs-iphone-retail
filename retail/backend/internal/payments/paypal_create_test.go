package payments

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func paypalConfig() PayPalCreateConfig {
	return PayPalCreateConfig{ProviderID: "pp_paypal_paypal", ClientID: "artificial_client", ClientSecret: "artificial_secret", MerchantID: "ABC1234567890",
		Environment: "sandbox", Intent: "CAPTURE", ReturnURL: "https://shop.example/api/paypal-return", CancelURL: "https://shop.example/dk-en/checkout"}
}
func paypalRequest() SessionRequest {
	return SessionRequest{ProviderID: "pp_paypal_paypal", CollectionID: "paycol_owned", SessionID: "payses_owned", Amount: "9007199254740993.005", Currency: "eur",
		Data: json.RawMessage(`{"cart_id":"cart_owned","session_id":"payses_owned","client_secret":"never-send","paypal":{"order_id":"OLD"}}`), Context: json.RawMessage(`{"idempotency_key":"payses_owned","private":"never-send"}`)}
}
func paypalOrder() map[string]any {
	return map[string]any{"id": "5O190127TN364715T", "intent": "CAPTURE", "status": "CREATED", "purchase_units": []any{map[string]any{
		"reference_id": "default", "custom_id": "cart_owned", "invoice_id": "payses_owned", "amount": map[string]any{"currency_code": "EUR", "value": "9007199254740993.01"}, "payee": map[string]any{"merchant_id": "ABC1234567890"}}}, "large": json.RawMessage(`9007199254740993.000000000000000000001`)}
}
func TestPayPalSettlementAmount(t *testing.T) {
	for _, c := range []struct{ amount, currency, want string }{
		{"9007199254740993.005", "eur", "9007199254740993.01"}, {"1.004999999999999999", "usd", "1.00"},
		{"1.005", "eur", "1.01"}, {"0.005", "cad", "0.01"}, {"0", "usd", "0.00"},
		{"9007199254740993.5", "jpy", "9007199254740994"}, {"10.49999999999999999", "huf", "10"}, {"10.5", "twd", "11"},
	} {
		got, e := paypalAmount(c.amount, c.currency)
		if e != nil || got != c.want {
			t.Fatalf("%s %s: %q %v", c.amount, c.currency, got, e)
		}
	}
	for _, c := range []struct{ amount, currency string }{{"-1", "eur"}, {"NaN", "eur"}, {"1", "EUR"}, {"1", "cny"}, {"1", "xxx"}} {
		if _, e := paypalAmount(c.amount, c.currency); e == nil {
			t.Fatal("invalid settlement accepted", c)
		}
	}
}

func TestPayPalCreateRequestAndResult(t *testing.T) {
	for _, provider := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
		for _, environment := range []string{"sandbox", "live"} {
			for _, intent := range []string{"CAPTURE", "AUTHORIZE"} {
				t.Run(provider+"/"+environment+"/"+intent, func(t *testing.T) {
					config := paypalConfig()
					config.ProviderID = provider
					config.Environment = environment
					config.Intent = intent
					a, e := NewPayPalCreateAdapter(config)
					if e != nil {
						t.Fatal(e)
					}
					request := paypalRequest()
					request.ProviderID = provider
					calls, creates := 0, 0
					var first string
					a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						if r.Method != "POST" || !strings.HasPrefix(r.URL.String(), a.base+"/") || r.GetBody != nil {
							t.Fatal("endpoint/replay mismatch")
						}
						deadline, ok := r.Context().Deadline()
						if !ok || time.Until(deadline) > 8*time.Second {
							t.Fatal("missing shared timeout")
						}
						body, _ := io.ReadAll(r.Body)
						if r.URL.Path == "/v1/oauth2/token" {
							id, secret, ok := r.BasicAuth()
							if !ok || id != config.ClientID || secret != config.ClientSecret || string(body) != "grant_type=client_credentials" {
								t.Fatal("OAuth request mismatch")
							}
							return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"artificial.token","token_type":"Bearer","expires_in":3600}`))}, nil
						}
						creates++
						if r.URL.Path != "/v2/checkout/orders" || r.Header.Get("PayPal-Request-Id") != request.SessionID || r.Header.Get("Authorization") != "Bearer artificial.token" || r.Header.Get("Prefer") != "return=representation" {
							t.Fatal("order request mismatch")
						}
						if creates == 1 {
							first = string(body)
						} else if first != string(body) {
							t.Fatal("immutable retry changed")
						}
						if strings.Contains(string(body), "never-send") || strings.Contains(string(body), "OLD") {
							t.Fatal("private or stale data leaked")
						}
						var payload map[string]json.RawMessage
						_ = json.Unmarshal(body, &payload)
						var units []map[string]json.RawMessage
						_ = json.Unmarshal(payload["purchase_units"], &units)
						if len(units) != 1 || string(units[0]["amount"]) != `{"currency_code":"EUR","value":"9007199254740993.01"}` || string(units[0]["invoice_id"]) != `"payses_owned"` || string(units[0]["custom_id"]) != `"cart_owned"` || string(payload["intent"]) != `"`+intent+`"` {
							t.Fatal("amount/binding/intent mismatch")
						}
						var source map[string]any
						_ = json.Unmarshal(payload["payment_source"], &source)
						kind := "paypal"
						if provider == "pp_paypal_card_paypal_card" {
							kind = "card"
						}
						if source[kind] == nil || len(source) != 1 {
							t.Fatal("SDK source changed")
						}
						order := paypalOrder()
						order["intent"] = intent
						if creates == 2 {
							order["status"] = "APPROVED"
						}
						raw, _ := json.Marshal(order)
						status := 201
						if creates == 2 {
							status = 200
						}
						return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
					})
					registry, e := NewCreateAdapters(map[string]CreateSessionAdapter{provider: a})
					if e != nil {
						t.Fatal(e)
					}
					adapter, _ := registry.Lookup(provider)
					for i := 0; i < 2; i++ {
						status, result, e := InvokeCreate(context.Background(), adapter, request)
						if e != nil || status != CreateSucceeded || result.Status != "pending" || !strings.Contains(string(result.Data), "9007199254740993.000000000000000000001") {
							t.Fatal("verified result lost; approval must not be paid", status, e)
						}
					}
					if calls != 4 || creates != 2 {
						t.Fatal("unexpected automatic calls")
					}
				})
			}
		}
	}
}

func TestPayPalCreateRefusesBadInputBeforeHTTP(t *testing.T) {
	for name, change := range map[string]func(*SessionRequest){
		"provider": func(r *SessionRequest) { r.ProviderID = "pp_system_default" },
		"currency": func(r *SessionRequest) { r.Currency = "cny" }, "zero": func(r *SessionRequest) { r.Amount = "0.001" },
		"negative": func(r *SessionRequest) { r.Amount = "-1" }, "session": func(r *SessionRequest) { r.SessionID = strings.Repeat("x", 109) },
		"cart":      func(r *SessionRequest) { r.Data = json.RawMessage(`{"cart_id":""}`) },
		"duplicate": func(r *SessionRequest) { r.Data = json.RawMessage(`{"cart_id":"cart_owned","cart_id":"cart_foreign"}`) },
		"identity":  func(r *SessionRequest) { r.Data = json.RawMessage(`{"cart_id":"cart_owned","session_id":"foreign"}`) },
		"key":       func(r *SessionRequest) { r.Context = json.RawMessage(`{"idempotency_key":"foreign"}`) },
	} {
		t.Run(name, func(t *testing.T) {
			a, _ := NewPayPalCreateAdapter(paypalConfig())
			a.client.Transport = mollieTransport(func(*http.Request) (*http.Response, error) { t.Fatal("invalid input reached network"); return nil, nil })
			r := paypalRequest()
			change(&r)
			status, _, e := a.CreateSession(context.Background(), r)
			if e != nil || status != CreateFailed {
				t.Fatal("local rejection must be definitive")
			}
		})
	}
	for name, change := range map[string]func(*PayPalCreateConfig){
		"provider": func(c *PayPalCreateConfig) { c.ProviderID = "pp_paypal-card_paypal" }, "environment": func(c *PayPalCreateConfig) { c.Environment = "https://other.invalid" },
		"intent": func(c *PayPalCreateConfig) { c.Intent = "sale" }, "merchant": func(c *PayPalCreateConfig) { c.MerchantID = "foreign" },
		"secret": func(c *PayPalCreateConfig) { c.ClientSecret = "secret\r\n" }, "return": func(c *PayPalCreateConfig) { c.ReturnURL = "javascript:alert(1)" },
		"cancel": func(c *PayPalCreateConfig) { c.CancelURL = "https://user:password@shop.example" },
	} {
		t.Run(name, func(t *testing.T) {
			c := paypalConfig()
			change(&c)
			if _, e := NewPayPalCreateAdapter(c); e == nil {
				t.Fatal("invalid server configuration accepted")
			}
		})
	}
}

func TestPayPalCreateUncertainResponseNeverBecomesSuccess(t *testing.T) {
	for _, test := range []string{"merchant", "cart", "invoice", "currency", "amount", "units", "intent", "id", "completed", "denied", "duplicate", "oversize", "status", "redirect", "disconnect", "bad-token", "token-redirect"} {
		t.Run(test, func(t *testing.T) {
			a, _ := NewPayPalCreateAdapter(paypalConfig())
			posts := 0
			a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/v1/oauth2/token" {
					status := 200
					body := `{"access_token":"artificial","token_type":"Bearer","expires_in":3600}`
					if test == "bad-token" {
						body = `{"access_token":"evil\r\nheader","token_type":"Bearer","expires_in":3600}`
					}
					if test == "token-redirect" {
						status = 302
					}
					return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.invalid/steal"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
				}
				posts++
				order := paypalOrder()
				unit := order["purchase_units"].([]any)[0].(map[string]any)
				switch test {
				case "merchant":
					unit["payee"] = map[string]string{"merchant_id": "OTHER12345678"}
				case "cart":
					unit["custom_id"] = "cart_other"
				case "invoice":
					unit["invoice_id"] = "payses_other"
				case "currency":
					unit["amount"].(map[string]any)["currency_code"] = "USD"
				case "amount":
					unit["amount"].(map[string]any)["value"] = "9007199254740993.01000000000000001"
				case "units":
					order["purchase_units"] = []any{unit, unit}
				case "intent":
					order["intent"] = "AUTHORIZE"
				case "id":
					order["id"] = "../orders"
				case "completed":
					order["status"] = "COMPLETED"
				case "denied":
					order["status"] = "VOIDED"
				case "disconnect":
					return nil, errors.New("artificial network loss after remote create")
				}
				raw, _ := json.Marshal(order)
				status := 201
				if test == "duplicate" {
					raw = []byte(`{"id":"5O190127TN364715T","id":"FOREIGN"}`)
				}
				if test == "oversize" {
					raw = []byte(`{"blob":"` + strings.Repeat("x", 1<<20) + `"}`)
				}
				if test == "status" {
					status = 422
				}
				if test == "redirect" {
					status = 302
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.invalid/steal"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})
			status, result, e := InvokeCreate(context.Background(), a, paypalRequest())
			if e == nil || status != CreateUncertain || len(result.Data) != 0 || posts > 1 {
				t.Fatal("uncertain response accepted or POST replayed", status, e, posts)
			}
			if (test == "bad-token" || test == "token-redirect") && posts != 0 {
				t.Fatal("orders requested after failed authentication")
			}
		})
	}
}

func TestPayPalCreateActualTLSHostAndRedirect(t *testing.T) {
	for _, hostname := range []string{"api-m.sandbox.paypal.com", "other.invalid"} {
		public, private, _ := ed25519.GenerateKey(rand.Reader)
		template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: hostname}, DNSNames: []string{hostname}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		der, e := x509.CreateCertificate(rand.Reader, template, template, public, private)
		if e != nil {
			t.Fatal(e)
		}
		cert, _ := x509.ParseCertificate(der)
		for _, mode := range []string{"valid", "untrusted", "redirect"} {
			t.Run(hostname+"/"+mode, func(t *testing.T) {
				calls := 0
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Host != "api-m.sandbox.paypal.com" {
						t.Error("fixed host changed")
					}
					if mode == "redirect" {
						http.Redirect(w, r, "https://other.invalid/steal", 302)
						return
					}
					if r.URL.Path == "/v1/oauth2/token" {
						io.WriteString(w, `{"access_token":"artificial","token_type":"Bearer","expires_in":3600}`)
						return
					}
					if r.Method == "GET" {
						_ = json.NewEncoder(w).Encode(paypalObservedFixture("paid"))
						return
					}
					w.WriteHeader(201)
					_ = json.NewEncoder(w).Encode(paypalOrder())
				}))
				server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: private}}, MinVersion: tls.VersionTLS12}
				server.StartTLS()
				defer server.Close()
				target, _ := url.Parse(server.URL)
				config := paypalConfig()
				config.RootCAs = x509.NewCertPool()
				if mode != "untrusted" {
					config.RootCAs.AddCert(cert)
				}
				a, e := NewPayPalCreateAdapter(config)
				if e != nil {
					t.Fatal(e)
				}
				transport := a.client.Transport.(*http.Transport)
				defer transport.CloseIdleConnections()
				if transport.TLSClientConfig.RootCAs == config.RootCAs || transport.TLSClientConfig.InsecureSkipVerify {
					t.Fatal("mutable/insecure trust roots")
				}
				transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					if address != "api-m.sandbox.paypal.com:443" {
						return nil, errors.New("foreign destination")
					}
					return (&net.Dialer{}).DialContext(ctx, network, target.Host)
				}
				status, _, e := a.CreateSession(context.Background(), paypalRequest())
				valid := hostname == "api-m.sandbox.paypal.com" && mode == "valid"
				if valid && (status != CreateSucceeded || e != nil || calls != 2) {
					t.Fatal("valid HTTPS flow failed", status, e, calls)
				}
				if !valid && (status != CreateUncertain || e == nil || calls > 1) {
					t.Fatal("untrusted host or redirect accepted", status, e, calls)
				}
				before := calls
				observed, e := a.ObserveSession(context.Background(), paypalRequest(), "5O190127TN364715T")
				if valid && (e != nil || observed.ProviderStatus != "paid" || calls != before+2) {
					t.Fatal("valid TLS GET evidence failed", e)
				}
				if !valid && (e == nil || calls > before+1) {
					t.Fatal("TLS GET trusted invalid hostname/root/redirect")
				}
			})
		}
	}
}
