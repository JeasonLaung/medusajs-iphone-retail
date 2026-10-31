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

func refundList(items []any, next any) map[string]any {
	return map[string]any{"count": len(items), "_embedded": map[string]any{"refunds": items}, "_links": map[string]any{"next": next}}
}
func TestMollieRefundLostResponseRecoveryNeverPostsAgain(t *testing.T) {
	a, _ := NewMollieCreateAdapter(mollieCreateConfig())
	in := refundFixtureRequest()
	posts, reads := 0, 0
	committed := false
	a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			posts++
			committed = true
			return nil, errors.New("lost success PRIVATE")
		}
		reads++
		if r.Method != http.MethodGet {
			t.Fatal("recovery wrote provider")
		}
		switch r.URL.Path {
		case "/v2/payments/tr_owned":
			return refundResponse(r, 200, refundParent()), nil
		case "/v2/payments/tr_owned/refunds":
			if !committed {
				t.Fatal("not testing committed lost result")
			}
			return refundResponse(r, 200, refundList([]any{refundFixture(in, "pending")}, nil)), nil
		case "/v2/payments/tr_owned/refunds/re_owned":
			return refundResponse(r, 200, refundFixture(in, "refunded")), nil
		default:
			t.Fatal("untrusted URL")
			return nil, nil
		}
	})
	out, e := a.CreateRefund(context.Background(), in)
	if e != ErrRefundExecution || out != (RefundObservation{}) {
		t.Fatal("lost result claimed success")
	}
	out, e = a.DiscoverRefund(context.Background(), in)
	if e != nil || out.Status != "refunded" || out.RefundID != "re_owned" || posts != 1 || reads != 5 {
		t.Fatal("lost refund not recovered with GET only", e, posts, reads)
	}
}
func TestMollieRefundDiscoveryChecksWholeBoundedHistoryAndCandidate(t *testing.T) {
	a, _ := NewMollieCreateAdapter(mollieCreateConfig())
	in := refundFixtureRequest()
	pages := 0
	a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Fatal("discovery issued a write")
		}
		if r.URL.Path == "/v2/payments/tr_owned" {
			return refundResponse(r, 200, refundParent()), nil
		}
		if r.URL.Path == "/v2/payments/tr_owned/refunds/re_owned" {
			return refundResponse(r, 200, refundFixture(in, "processing")), nil
		}
		pages++
		if r.URL.Path != "/v2/payments/tr_owned/refunds" || r.URL.Query().Get("limit") != "25" {
			t.Fatal("fixed refund list request changed")
		}
		if pages == 1 {
			return refundResponse(r, 200, refundList([]any{map[string]any{"id": "re_other", "metadata": nil}}, map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?from=re_owned&limit=25"})), nil
		}
		if r.URL.Query().Get("from") != "re_owned" {
			t.Fatal("cursor changed")
		}
		return refundResponse(r, 200, refundList([]any{refundFixture(in, "pending")}, nil)), nil
	})
	out, e := a.DiscoverRefund(context.Background(), in)
	if e != nil || out.Status != "processing" || pages != 2 {
		t.Fatal("unique candidate not independently read", e, pages)
	}
}
func TestMollieRefundDiscoveryRefusesAmbiguousIncompleteOrForeignEvidence(t *testing.T) {
	for _, mode := range []string{"none", "ambiguous", "incomplete", "wrong-parent-link", "foreign-host", "userinfo", "fragment", "duplicate-cursor", "extra-query", "wrong-limit", "missing-cursor", "empty-page-next", "count", "missing-links", "missing-items", "invalid-id", "duplicate-id", "duplicate-json", "oversize", "http302", "http404", "http429", "transport", "panic", "get-parent", "get-operation", "get-amount", "get-id", "get-mode", "get-date", "get404"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			in := refundFixtureRequest()
			pages := 0
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.Host != "api.mollie.com" {
					t.Fatal("unsafe discovery request")
				}
				if r.URL.Path == "/v2/payments/tr_owned" {
					return refundResponse(r, 200, refundParent()), nil
				}
				if r.URL.Path == "/v2/payments/tr_owned/refunds/re_owned" {
					v := refundFixture(in, "refunded")
					code := 200
					switch mode {
					case "get-parent":
						v["paymentId"] = "tr_other"
					case "get-operation":
						v["metadata"] = map[string]string{"refund_operation_id": "other"}
					case "get-amount":
						v["amount"] = map[string]string{"value": "10.02", "currency": "EUR"}
					case "get-id":
						v["id"] = "re_other"
					case "get-mode":
						v["mode"] = "live"
					case "get-date":
						v["createdAt"] = "bad"
					case "get404":
						code = 404
					}
					return refundResponse(r, code, v), nil
				}
				pages++
				if pages > 4 {
					t.Fatal("unbounded discovery")
				}
				items := []any{refundFixture(in, "pending")}
				var next any
				code := 200
				switch mode {
				case "none":
					items = []any{}
				case "ambiguous":
					v := refundFixture(in, "queued")
					v["id"] = "re_second"
					items = append(items, v)
				case "incomplete":
					if pages > 1 {
						items = []any{map[string]any{"id": "re_page" + string(rune('0'+pages)), "metadata": nil}}
					}
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?from=re_page" + string(rune('1'+pages)) + "&limit=25"}
				case "wrong-parent-link":
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_other/refunds?from=re_next"}
				case "foreign-host":
					next = map[string]string{"href": "https://evil.invalid/v2/payments/tr_owned/refunds?from=re_next"}
				case "userinfo":
					next = map[string]string{"href": "https://user@api.mollie.com/v2/payments/tr_owned/refunds?from=re_next"}
				case "fragment":
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?from=re_next#fragment"}
				case "duplicate-cursor":
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?from=re_next&from=re_other"}
				case "extra-query":
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?from=re_next&secret=x"}
				case "wrong-limit":
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?from=re_next&limit=250"}
				case "missing-cursor":
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?limit=25"}
				case "empty-page-next":
					items = []any{}
					next = map[string]string{"href": "https://api.mollie.com/v2/payments/tr_owned/refunds?from=re_next"}
				case "invalid-id":
					items = []any{map[string]string{"id": "re_bad/path"}}
				case "duplicate-id":
					items = append(items, items[0])
				case "transport":
					return nil, errors.New("PRIVATE")
				case "panic":
					panic("PRIVATE")
				case "http302":
					code = 302
				case "http404":
					code = 404
				case "http429":
					code = 429
				}
				body := refundList(items, next)
				switch mode {
				case "count":
					body["count"] = 2
				case "missing-links":
					delete(body, "_links")
				case "missing-items":
					body["_embedded"] = map[string]any{}
				}
				response := refundResponse(r, code, body)
				if mode == "duplicate-json" {
					response.Body = io.NopCloser(strings.NewReader(`{"count":1,"count":0}`))
				}
				if mode == "oversize" {
					response.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1)))
				}
				return response, nil
			})
			out, e := a.DiscoverRefund(context.Background(), in)
			if e != ErrRefundExecution || out != (RefundObservation{}) {
				raw, _ := json.Marshal(out)
				t.Fatal("unverified discovery accepted", mode, string(raw), e)
			}
		})
	}
}
