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

type mollieTransport func(*http.Request) (*http.Response, error)

func (f mollieTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mollieFixture(status string, cancelable any) map[string]any {
	return map[string]any{
		"id": "tr_owned", "status": status, "isCancelable": cancelable,
		"amount":   map[string]any{"value": "9007199254740993.01", "currency": "EUR"},
		"metadata": map[string]any{"session_id": "payses_owned", "cart_id": "cart_owned"},
	}
}

func mollieRequest() SessionRequest {
	return SessionRequest{ProviderID: "pp_mollie_mollie", CollectionID: "paycol_owned", SessionID: "payses_owned",
		Amount: "9007199254740993.010", Currency: "eur",
		Data: json.RawMessage(`{"id":"tr_owned","session_id":"payses_owned","cart_id":"cart_owned"}`)}
}

func TestMollieDeleteDecisions(t *testing.T) {
	for _, test := range []struct {
		name       string
		get        map[string]any
		deleted    map[string]any
		want       DeleteOutcome
		wantCalls  int
		deleteCode int
	}{
		{"open cancellation", mollieFixture("open", true), mollieFixture("canceled", false), DeleteSucceeded, 2, 200},
		{"processing cancellation", mollieFixture("pending", true), mollieFixture("canceled", false), DeleteSucceeded, 2, 200},
		{"paid protected", mollieFixture("paid", true), nil, DeleteFailed, 1, 0},
		{"authorized protected", mollieFixture("authorized", true), nil, DeleteFailed, 1, 0},
		{"already canceled", mollieFixture("canceled", nil), nil, DeleteSucceeded, 1, 0},
		{"already expired", mollieFixture("expired", nil), nil, DeleteSucceeded, 1, 0},
		{"already failed", mollieFixture("failed", nil), nil, DeleteSucceeded, 1, 0},
		{"legacy open local discard", mollieFixture("open", false), nil, DeleteSucceeded, 1, 0},
		{"processing preserved", mollieFixture("pending", false), nil, DeleteFailed, 1, 0},
		{"missing cancelability preserved", mollieFixture("open", nil), nil, DeleteFailed, 1, 0},
		{"unknown status", mollieFixture("surprise", true), nil, DeleteUncertain, 1, 0},
		{"raced paid", mollieFixture("open", true), mollieFixture("paid", false), DeleteUncertain, 2, 200},
		{"still processing", mollieFixture("open", true), mollieFixture("pending", true), DeleteUncertain, 2, 200},
		{"delete 422", mollieFixture("open", true), nil, DeleteUncertain, 2, 422},
		{"delete 404", mollieFixture("open", true), nil, DeleteUncertain, 2, 404},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, err := NewMollieDeleteAdapter("pp_mollie_mollie", "test_artificial")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://api.mollie.com/v2/payments/tr_owned" || r.Header.Get("Authorization") != "Bearer test_artificial" || r.Header.Get("Accept") != "application/hal+json" {
					t.Fatal("incorrect fixed origin or authentication")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 8*time.Second {
					t.Fatal("missing bounded HTTP context")
				}
				body, code := test.get, 200
				if calls == 1 {
					if r.Method != http.MethodGet {
						t.Fatal("must read before cancel")
					}
				} else {
					if r.Method != http.MethodDelete || calls != 2 {
						t.Fatal("unexpected extra operation")
					}
					body, code = test.deleted, test.deleteCode
				}
				raw, _ := json.Marshal(body)
				return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
			})
			outcome, err := InvokeDelete(context.Background(), a, mollieRequest())
			if outcome != test.want || calls != test.wantCalls || (err != nil) != (test.want == DeleteUncertain) {
				t.Fatalf("outcome %s err %v calls %d", outcome, err, calls)
			}
		})
	}
}

func TestMollieDeleteResponseBinding(t *testing.T) {
	for _, field := range []string{"id", "session", "cart", "currency", "amount"} {
		for _, at := range []int{1, 2} {
			t.Run(field+string(rune('0'+at)), func(t *testing.T) {
				a, _ := NewMollieDeleteAdapter("pp_mollie_mollie", "test_artificial")
				calls := 0
				a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
					calls++
					body := mollieFixture("open", true)
					if calls == 2 {
						body = mollieFixture("canceled", false)
					}
					if calls == at {
						switch field {
						case "id":
							body["id"] = "tr_other"
						case "session":
							body["metadata"].(map[string]any)["session_id"] = "payses_other"
						case "cart":
							body["metadata"].(map[string]any)["cart_id"] = "cart_other"
						case "currency":
							body["amount"].(map[string]any)["currency"] = "USD"
						case "amount":
							body["amount"].(map[string]any)["value"] = "9007199254740993.02"
						}
					}
					raw, _ := json.Marshal(body)
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw))), Request: r}, nil
				})
				outcome, err := InvokeDelete(context.Background(), a, mollieRequest())
				if outcome != DeleteUncertain || err != ErrDeleteExecution || calls != at {
					t.Fatal("response binding did not fail closed")
				}
			})
		}
	}
}

func TestMollieDeleteFailureBoundaries(t *testing.T) {
	for _, mode := range []string{"redirect", "404", "429", "500", "transport", "duplicate", "oversize", "malformed", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := NewMollieDeleteAdapter("pp_mollie_mollie", "test_artificial")
			calls := 0
			a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				code, body := 200, `{}`
				header := http.Header{}
				switch mode {
				case "redirect":
					code = 302
					header.Set("Location", "https://untrusted.invalid/private")
				case "404":
					code = 404
				case "429":
					code = 429
				case "500":
					code = 500
				case "transport":
					return nil, errors.New("private-api-key-provider-error")
				case "deadline":
					<-r.Context().Done()
					return nil, r.Context().Err()
				case "duplicate":
					body = `{"id":"tr_owned","id":"tr_other"}`
				case "oversize":
					body = `{"a":"` + strings.Repeat("x", 1<<20) + `"}`
				case "malformed":
					body = `{"id":`
				}
				return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			outcome, err := InvokeDelete(ctx, a, mollieRequest())
			if outcome != DeleteUncertain || err != ErrDeleteExecution || calls != 1 {
				t.Fatal("failure escaped uncertain boundary or caused a retry")
			}
		})
	}
	for _, mutate := range []func(*SessionRequest){
		func(r *SessionRequest) { r.ProviderID = "other" },
		func(r *SessionRequest) { r.Amount = "-1" },
		func(r *SessionRequest) { r.Currency = "EUR" },
		func(r *SessionRequest) { r.Data = json.RawMessage(`{"id":"tr_owned/../../other"}`) },
		func(r *SessionRequest) { r.Data = json.RawMessage(`{"id":"tr_owned","session_id":"payses_other"}`) },
	} {
		a, _ := NewMollieDeleteAdapter("pp_mollie_mollie", "test_artificial")
		a.client.Transport = mollieTransport(func(*http.Request) (*http.Response, error) { t.Fatal("invalid input issued HTTP"); return nil, nil })
		request := mollieRequest()
		mutate(&request)
		if outcome, err := InvokeDelete(context.Background(), a, request); outcome != DeleteUncertain || err != ErrDeleteExecution {
			t.Fatal("invalid input accepted")
		}
	}
	if _, err := NewMollieDeleteAdapter("bad/provider", "test_artificial"); err == nil {
		t.Fatal("invalid provider accepted")
	}
	if _, err := NewMollieDeleteAdapter("pp_mollie_mollie", "test_private\nheader"); err == nil {
		t.Fatal("invalid credential accepted")
	}
}
