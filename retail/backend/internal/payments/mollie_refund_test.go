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

func refundFixtureRequest() RefundRequest {
	return RefundRequest{Session: mollieCreateRequest(), RemotePaymentID: "tr_owned", OperationID: "281ea420-4422-4bb6-86ee-f0c1b6cb09f5", OrderID: "order_owned", RequestID: "asr_owned", Amount: "10.01"}
}
func refundFixture(in RefundRequest, status string) map[string]any {
	return map[string]any{"resource": "refund", "id": "re_owned", "mode": "test", "status": status, "paymentId": in.RemotePaymentID, "amount": map[string]string{"value": in.Amount, "currency": "EUR"}, "metadata": refundMetadata(in), "createdAt": "2026-10-31T05:00:00Z", "private": "must-not-escape"}
}
func refundParent() map[string]any {
	p := mollieCreateFixture()
	p["status"] = "paid"
	p["amountRemaining"] = map[string]string{"value": "9007199254740993.01", "currency": "EUR"}
	return p
}
func refundResponse(r *http.Request, code int, body any) *http.Response {
	raw, _ := json.Marshal(body)
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}
}
func TestMollieRefundCreatePreservesIdentityAndIndependentStatus(t *testing.T) {
	for _, status := range []string{"queued", "pending", "processing", "refunded", "failed", "canceled"} {
		t.Run(status, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			in := refundFixtureRequest()
			calls := 0
			var first string
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Scheme != "https" || r.URL.Host != "api.mollie.com" || r.Header.Get("Authorization") != "Bearer test_artificial" {
					t.Fatal("fixed merchant transport lost")
				}
				if d, ok := r.Context().Deadline(); !ok || time.Until(d) > 8*time.Second {
					t.Fatal("HTTP request unbounded")
				}
				if r.Method == http.MethodGet {
					if r.URL.Path != "/v2/payments/tr_owned" {
						t.Fatal("wrong parent")
					}
					return refundResponse(r, 200, refundParent()), nil
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v2/payments/tr_owned/refunds" || r.Header.Get("Idempotency-Key") != in.OperationID || r.Header.Get("Content-Type") != "application/json" {
					t.Fatal("refund identity changed")
				}
				raw, _ := io.ReadAll(r.Body)
				if first != "" && first != string(raw) {
					t.Fatal("stable UUID changed request body")
				}
				first = string(raw)
				var body struct {
					Amount      struct{ Value, Currency string }
					Description string
					Metadata    map[string]string
				}
				if json.Unmarshal(raw, &body) != nil || body.Amount.Value != "10.01" || body.Amount.Currency != "EUR" || body.Description != "after-sales:asr_owned" || body.Metadata["refund_operation_id"] != in.OperationID || strings.Contains(first, "not-for-provider") || strings.Contains(first, "cart_owned") || strings.Contains(first, "redirect") {
					t.Fatal("private input or wrong refund body")
				}
				return refundResponse(r, 201, refundFixture(in, status)), nil
			})
			for i := 0; i < 2; i++ {
				out, e := a.CreateRefund(context.Background(), in)
				if e != nil || out.Status != status || out.RefundID != "re_owned" || out.Amount != "10.01" || out.Currency != "eur" {
					t.Fatalf("refund status projection differs: %v %v", out, e)
				}
				raw, _ := json.Marshal(out)
				if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "metadata") {
					t.Fatal("provider raw data leaked")
				}
			}
			if calls != 4 {
				t.Fatal("unexpected retry or extra provider call", calls)
			}
		})
	}
}
func TestMollieRefundRejectsInvalidInputBeforeNetwork(t *testing.T) {
	for _, mode := range []string{"provider", "operation", "order", "request", "remote", "zero", "negative", "fraction", "exponent", "leading-zero", "over-total", "currency"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			in := refundFixtureRequest()
			switch mode {
			case "provider":
				in.Session.ProviderID = "other"
			case "operation":
				in.OperationID = "not-a-uuid"
			case "order":
				in.OrderID = "../other"
			case "request":
				in.RequestID = ""
			case "remote":
				in.RemotePaymentID = "tr_owned/other"
			case "zero":
				in.Amount = "0.00"
			case "negative":
				in.Amount = "-1"
			case "fraction":
				in.Amount = "1.001"
			case "exponent":
				in.Amount = "1e1"
			case "leading-zero":
				in.Amount = "01"
			case "over-total":
				in.Amount = "9007199254740993.02"
			case "currency":
				in.Session.Currency = "EUR"
			}
			a.transport.client.Transport = mollieTransport(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid request reached provider")
				return nil, nil
			})
			if out, e := a.CreateRefund(context.Background(), in); e != ErrRefundExecution || out != (RefundObservation{}) {
				t.Fatal("invalid refund input accepted")
			}
		})
	}
}
func TestMollieRefundCreateNeverRetriesOrConvertsErrorsToRefunds(t *testing.T) {
	modes := []string{"parent-authorized", "parent-currency", "parent-session", "parent-mode", "parent-id", "missing-remaining", "remaining-currency", "remaining-insufficient", "remaining-negative", "remaining-over-total", "remaining-json", "resource", "refund-id", "refund-parent", "refund-mode", "refund-amount", "refund-currency", "operation", "order", "request", "status", "date", "json", "duplicate", "oversize", "transport", "panic", "http302", "http200", "http404", "http409", "http422", "http429", "http500"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			in := refundFixtureRequest()
			posts := 0
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodGet {
					p := refundParent()
					switch mode {
					case "parent-authorized":
						p["status"] = "authorized"
					case "parent-currency":
						p["amount"] = map[string]string{"value": "9007199254740993.01", "currency": "USD"}
					case "parent-session":
						p["metadata"] = map[string]string{"session_id": "foreign", "cart_id": "cart_owned"}
					case "parent-mode":
						p["mode"] = "live"
					case "parent-id":
						p["id"] = "tr_other"
					case "missing-remaining":
						delete(p, "amountRemaining")
					case "remaining-currency":
						p["amountRemaining"] = map[string]string{"value": "20", "currency": "USD"}
					case "remaining-insufficient":
						p["amountRemaining"] = map[string]string{"value": "10.00", "currency": "EUR"}
					case "remaining-negative":
						p["amountRemaining"] = map[string]string{"value": "-1", "currency": "EUR"}
					case "remaining-over-total":
						p["amountRemaining"] = map[string]string{"value": "9007199254740993.02", "currency": "EUR"}
					case "remaining-json":
						p["amountRemaining"] = "bad"
					}
					return refundResponse(r, 200, p), nil
				}
				posts++
				v := refundFixture(in, "pending")
				code := 201
				switch mode {
				case "resource":
					v["resource"] = "payment"
				case "refund-id":
					v["id"] = "re_owned/path"
				case "refund-parent":
					v["paymentId"] = "tr_other"
				case "refund-mode":
					v["mode"] = "live"
				case "refund-amount":
					v["amount"] = map[string]string{"value": "10.02", "currency": "EUR"}
				case "refund-currency":
					v["amount"] = map[string]string{"value": "10.01", "currency": "USD"}
				case "operation":
					v["metadata"] = map[string]string{"refund_operation_id": "other"}
				case "order":
					v["metadata"] = map[string]string{"refund_operation_id": in.OperationID, "order_id": "other", "after_sales_request_id": in.RequestID}
				case "request":
					v["metadata"] = map[string]string{"refund_operation_id": in.OperationID, "order_id": in.OrderID, "after_sales_request_id": "other"}
				case "status":
					v["status"] = "paid"
				case "date":
					v["createdAt"] = "no"
				case "transport":
					return nil, errors.New("PRIVATE")
				case "panic":
					panic("PRIVATE")
				case "http302":
					code = 302
				case "http200":
					code = 200
				case "http404":
					code = 404
				case "http409":
					code = 409
				case "http422":
					code = 422
				case "http429":
					code = 429
				case "http500":
					code = 500
				}
				response := refundResponse(r, code, v)
				if mode == "json" {
					response.Body = io.NopCloser(strings.NewReader("{"))
				}
				if mode == "duplicate" {
					response.Body = io.NopCloser(strings.NewReader(`{"id":"re_owned","id":"re_other"}`))
				}
				if mode == "oversize" {
					response.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1)))
				}
				return response, nil
			})
			out, e := a.CreateRefund(context.Background(), in)
			if e != ErrRefundExecution || out != (RefundObservation{}) {
				t.Fatal("refund failure exposed or accepted", mode, e)
			}
			expected := 1
			if strings.HasPrefix(mode, "parent-") || strings.HasPrefix(mode, "remaining-") || mode == "missing-remaining" {
				expected = 0
			}
			if posts != expected {
				t.Fatal("unexpected refund POST or retry", posts, expected)
			}
		})
	}
}
func TestMollieRefundObserveDoesNotRequireUnrefundedBalance(t *testing.T) {
	a, _ := NewMollieCreateAdapter(mollieCreateConfig())
	in := refundFixtureRequest()
	calls := 0
	a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodGet {
			t.Fatal("observation wrote provider")
		}
		if r.URL.Path == "/v2/payments/tr_owned" {
			p := refundParent()
			p["amountRemaining"] = map[string]string{"value": "0.00", "currency": "EUR"}
			return refundResponse(r, 200, p), nil
		}
		if r.URL.Path != "/v2/payments/tr_owned/refunds/re_owned" {
			t.Fatal("refund ID not fixed")
		}
		return refundResponse(r, 200, refundFixture(in, "refunded")), nil
	})
	out, e := a.ObserveRefund(context.Background(), in, "re_owned")
	if e != nil || out.Status != "refunded" || calls != 2 {
		t.Fatal("full refund observation failed", e, calls)
	}
	if _, e = a.ObserveRefund(context.Background(), in, "re_owned/foreign"); e != ErrRefundExecution || calls != 2 {
		t.Fatal("invalid ID sent")
	}
}

func TestMollieRefundUsesExplicitOriginalProviderRegistry(t *testing.T) {
	a, _ := NewMollieCreateAdapter(mollieCreateConfig())
	registry, e := NewCreateAdapters(map[string]CreateSessionAdapter{mollieCreateConfig().ProviderID: a, "unsupported": artificialCreate(func(context.Context, SessionRequest) (CreateOutcome, CreatedSession, error) {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	})})
	if e != nil {
		t.Fatal(e)
	}
	if refunder, e := registry.Refunder(mollieCreateConfig().ProviderID); e != nil || refunder != a {
		t.Fatal("original provider not reused")
	}
	for _, id := range []string{"unsupported", "unknown"} {
		if _, e := registry.Refunder(id); e != ErrEffectInput {
			t.Fatal("refund fallback permitted")
		}
	}
	var missing *CreateAdapters
	if _, e := missing.Refunder("unknown"); e != ErrEffectInput {
		t.Fatal("unconfigured refund provider enabled")
	}
}
