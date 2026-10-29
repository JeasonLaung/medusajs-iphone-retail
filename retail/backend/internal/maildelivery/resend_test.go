package maildelivery

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestVerificationResendTransport(t *testing.T) {
	key := "mail/cv_00000000-0000-4000-8000-000000000001"
	for _, fixture := range []struct {
		status     int
		body, code string
		retry      bool
	}{{200, `{"id":"00000000-0000-4000-8000-000000000002"}`, "", false}, {429, `PRIVATE`, "provider_temporary", true}, {500, `PRIVATE`, "provider_temporary", true}, {401, `PRIVATE_API_KEY`, "provider_rejected", false}, {302, `PRIVATE`, "provider_rejected", false}, {200, `{"id":"malformed"}`, "provider_response", true}, {200, `PRIVATE`, "provider_response", true}} {
		calls := 0
		sender, err := newResend("re_artificial_key", "EuroRefurb <mail@example.invalid>", "reply@example.invalid", transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" || r.URL.String() != "https://api.resend.com/emails" || r.Header.Get("Authorization") != "Bearer re_artificial_key" || r.Header.Get("Idempotency-Key") != key || r.Header.Get("Content-Type") != "application/json" {
				t.Fatal("mail transport request differed")
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"to":["new@example.invalid"]`) || !strings.Contains(string(body), "Verify your email to join EuroRefurb") {
				t.Fatal("mail transport body differed")
			}
			return &http.Response{StatusCode: fixture.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fixture.body))}, nil
		}))
		if err != nil {
			t.Fatal("sender fixture invalid")
		}
		payload, err := sender.Prepare("new@example.invalid", "https://shop.example/dk-en/verify-account?token="+strings.Repeat("a", 64), "en")
		if err != nil {
			t.Fatal("mail template preparation failed")
		}
		result := sender.Send(context.Background(), key, payload)
		if result.Code != fixture.code || result.Retry != fixture.retry || calls != 1 {
			t.Fatal("provider classification/retry differed")
		}
		if fixture.status == 200 && fixture.code == "" && result.ID != "00000000-0000-4000-8000-000000000002" {
			t.Fatal("provider ID absent")
		}
	}
	sender, _ := newResend("re_artificial_key", "mail@example.invalid", "", transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("SECRET") }))
	payload, _ := sender.Prepare("new@example.invalid", "https://shop.example/dk-zh/verify-account?token=abc", "zh")
	if !strings.Contains(string(payload), "验证邮箱，完成 EuroRefurb 注册") {
		t.Fatal("Chinese mail copy changed")
	}
	if result := sender.Send(context.Background(), key, payload); result.Code != "network_error" || !result.Retry {
		t.Fatal("network failure retry missing")
	}
	for _, args := range [][3]string{{"", "mail@example.invalid", ""}, {"re_fixture", "bad", ""}, {"re_fixture", "mail@example.invalid\r\nX: evil", ""}, {"re_fixture", "mail@example.invalid", "bad"}} {
		if _, err := NewResend(args[0], args[1], args[2]); err == nil {
			t.Fatal("invalid sender config accepted")
		}
	}
}
