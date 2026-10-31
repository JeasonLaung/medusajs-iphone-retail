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

func discoveryPage(items []any, next any) map[string]any {
	return map[string]any{"count": len(items), "_embedded": map[string]any{"payments": items}, "_links": map[string]any{"next": next}}
}
func TestMollieDiscoveryReadsAndVerifies(t *testing.T) {
	a, _ := NewMollieCreateAdapter(mollieCreateConfig())
	calls := 0
	a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Host != "api.mollie.com" || r.Header.Get("Authorization") != "Bearer test_artificial" || r.Header.Get("Idempotency-Key") != "" {
			t.Fatal("discovery escaped read-only merchant API")
		}
		var body any
		switch calls {
		case 1:
			if r.URL.Path != "/v2/payments" || r.URL.Query().Get("limit") != "25" || r.URL.Query().Get("sort") != "desc" || r.URL.Query().Get("from") != "" {
				t.Fatal("unexpected first page")
			}
			body = discoveryPage([]any{map[string]any{"id": "tr_unrelated", "metadata": "legacy string"}}, map[string]string{"href": "https://api.mollie.com/v2/payments?from=tr_owned&limit=25"})
		case 2:
			if r.URL.Query().Get("from") != "tr_owned" {
				t.Fatal("cursor lost")
			}
			body = discoveryPage([]any{mollieCreateFixture()}, nil)
		case 3:
			if r.URL.Path != "/v2/payments/tr_owned" || r.URL.RawQuery != "" {
				t.Fatal("candidate not independently read")
			}
			body = mollieCreateFixture()
		default:
			t.Fatal("unexpected extra network call")
		}
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: http.Header{}, Request: r}, nil
	})
	out, err := InvokeDiscovery(context.Background(), a, mollieCreateRequest())
	if err != nil || out.PaymentID != "tr_owned" || out.ProviderStatus != "open" || calls != 3 {
		t.Fatal("unique discovery failed", calls, err)
	}
}

func TestMollieDiscoveryFailsClosed(t *testing.T) {
	for _, mode := range []string{"none", "ambiguous", "incomplete", "cursor-loop", "foreign-origin", "userinfo", "fragment", "extra-query", "duplicate-cursor", "count", "missing-links", "missing-payments", "bad-id", "duplicate-json", "oversize", "http302", "http429", "transport", "get-amount", "get-mode", "get-cart", "get-session", "get-id"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			calls := 0
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.Host != "api.mollie.com" {
					t.Fatal("unsafe recovery request")
				}
				if mode == "transport" {
					return nil, errors.New("private upstream error")
				}
				fixture := mollieCreateFixture()
				page := discoveryPage([]any{fixture}, nil)
				code := 200
				if r.URL.Path != "/v2/payments" {
					switch mode {
					case "get-amount":
						fixture["amount"].(map[string]any)["value"] = "1.00"
					case "get-mode":
						fixture["mode"] = "live"
					case "get-cart":
						fixture["metadata"].(map[string]any)["cart_id"] = "cart_other"
					case "get-session":
						fixture["metadata"].(map[string]any)["session_id"] = "payses_other"
					case "get-id":
						fixture["id"] = "tr_other"
					default:
						t.Fatal("untrusted list reached final GET")
					}
				} else {
					next := ""
					switch mode {
					case "none":
						page = discoveryPage([]any{}, nil)
					case "ambiguous":
						second := mollieCreateFixture()
						second["id"] = "tr_second"
						page = discoveryPage([]any{fixture, second}, nil)
					case "incomplete":
						fixture["id"] = "tr_page" + string(rune('a'+calls))
						fixture["metadata"] = nil
						next = "https://api.mollie.com/v2/payments?from=tr_cursor" + string(rune('a'+calls))
					case "cursor-loop":
						next = "https://api.mollie.com/v2/payments?from=tr_owned"
					case "foreign-origin":
						next = "https://attacker.example/v2/payments?from=tr_next"
					case "userinfo":
						next = "https://user@api.mollie.com/v2/payments?from=tr_next"
					case "fragment":
						next = "https://api.mollie.com/v2/payments?from=tr_next#bad"
					case "extra-query":
						next = "https://api.mollie.com/v2/payments?from=tr_next&profileId=pfl_other"
					case "duplicate-cursor":
						next = "https://api.mollie.com/v2/payments?from=tr_next&from=tr_other"
					case "count":
						page["count"] = 2
					case "missing-links":
						delete(page, "_links")
					case "missing-payments":
						page["_embedded"] = map[string]any{}
					case "bad-id":
						fixture["id"] = "../../bad"
					case "http302":
						code = 302
					case "http429":
						code = 429
					}
					if next != "" {
						page["_links"] = map[string]any{"next": map[string]string{"href": next}}
					}
				}
				var raw []byte
				if r.URL.Path == "/v2/payments" {
					raw, _ = json.Marshal(page)
				} else {
					raw, _ = json.Marshal(fixture)
				}
				if mode == "duplicate-json" {
					raw = []byte(`{"count":1,"count":0,"_embedded":{"payments":[]},"_links":{"next":null}}`)
				}
				if mode == "oversize" {
					raw = []byte(strings.Repeat("x", (1<<20)+1))
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(raw))), Header: http.Header{}, Request: r}, nil
			})
			out, err := InvokeDiscovery(context.Background(), a, mollieCreateRequest())
			if err != ErrCreateExecution || out.PaymentID != "" || calls > 4 && mode != "incomplete" {
				t.Fatal("unsafe recovery accepted", calls, err)
			}
			if mode == "incomplete" && calls != 4 {
				t.Fatal("scan was not bounded", calls)
			}
		})
	}
}
