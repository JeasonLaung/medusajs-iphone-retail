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

func TestMollieAuthoritativeObservation(t *testing.T) {
	for status, action := range map[string]string{"open": "not_supported", "pending": "pending", "paid": "successful", "authorized": "authorized", "canceled": "canceled", "expired": "failed", "failed": "failed"} {
		t.Run(status, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			calls := 0
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.URL.String() != "https://api.mollie.com/v2/payments/tr_owned" || r.Header.Get("Authorization") != "Bearer test_artificial" || r.Header.Get("Idempotency-Key") != "" {
					t.Fatal("observation issued an incorrect or mutating request")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 8*time.Second {
					t.Fatal("unbounded observation")
				}
				fixture := mollieCreateFixture()
				fixture["status"] = status
				if status != "open" && status != "pending" {
					delete(fixture, "_links")
				}
				raw, _ := json.Marshal(fixture)
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
			})
			observed, err := a.ObserveSession(context.Background(), mollieCreateRequest(), "tr_owned")
			if err != nil || calls != 1 || observed.PaymentID != "tr_owned" || observed.ProviderStatus != status || observed.Action != action || observed.Amount != "9007199254740993.01" || observed.Currency != "eur" {
				t.Fatal("authoritative fact/status/amount lost")
			}
			if !strings.Contains(string(observed.Session.Data), "9007199254740993.000000000000000000001") {
				t.Fatal("observation rounded provider JSON")
			}
			if status == "paid" || status == "authorized" {
				if observed.Session.Status != "authorized" {
					t.Fatal("paid fact not protected")
				}
			}
		})
	}
}

func TestMollieObservationDoesNotTrustCandidate(t *testing.T) {
	for _, mode := range []string{"id", "session", "cart", "amount", "currency", "mode", "redirect", "status", "404", "302", "network", "json"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := NewMollieCreateAdapter(mollieCreateConfig())
			calls := 0
			a.transport.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" {
					t.Fatal("reconciliation performed a mutation")
				}
				fixture := mollieCreateFixture()
				code := 200
				header := http.Header{}
				switch mode {
				case "id":
					fixture["id"] = "tr_other"
				case "session":
					fixture["metadata"].(map[string]any)["session_id"] = "payses_other"
				case "cart":
					fixture["metadata"].(map[string]any)["cart_id"] = "cart_other"
				case "amount":
					fixture["amount"].(map[string]any)["value"] = "9007199254740993.02"
				case "currency":
					fixture["amount"].(map[string]any)["currency"] = "USD"
				case "mode":
					fixture["mode"] = "live"
				case "redirect":
					fixture["redirectUrl"] = "https://untrusted.example/"
				case "status":
					fixture["status"] = "unexpected"
				case "404":
					code = 404
				case "302":
					code = 302
					header.Set("Location", "https://untrusted.example/")
				case "network":
					return nil, errors.New("private-provider-token")
				}
				raw, _ := json.Marshal(fixture)
				if mode == "json" {
					raw = []byte(`{"id":"tr_owned","id":"tr_other"}`)
				}
				return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
			})
			observed, err := a.ObserveSession(context.Background(), mollieCreateRequest(), "tr_owned")
			if err != ErrCreateExecution || observed.PaymentID != "" || len(observed.Session.Data) != 0 || calls != 1 {
				t.Fatal("unverified observation accepted or followed up")
			}
		})
	}
	a, _ := NewMollieCreateAdapter(mollieCreateConfig())
	a.transport.client.Transport = mollieTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid locator or request reached HTTP")
		return nil, nil
	})
	if _, err := a.ObserveSession(context.Background(), mollieCreateRequest(), "tr_owned/../other"); err != ErrCreateExecution {
		t.Fatal("invalid locator accepted")
	}
	r := mollieCreateRequest()
	r.ProviderID = "other"
	if _, err := a.ObserveSession(context.Background(), r, "tr_owned"); err != ErrCreateExecution {
		t.Fatal("wrong request reached merchant")
	}
}
