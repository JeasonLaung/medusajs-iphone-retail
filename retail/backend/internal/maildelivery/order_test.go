package maildelivery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOrderMailTemplateAndTransport(t *testing.T) {
	calls := 0
	s, err := newResend("re_artificial_key", "EuroRefurb <mail@example.invalid>", "reply@example.invalid", transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.resend.com/emails" || r.Header.Get("Idempotency-Key") != "mail/op_00000000-0000-4000-8000-000000000001" {
			t.Fatal("order transport differs")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"id":"00000000-0000-4000-8000-000000000002"}`))}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "zh"} {
		body, e := s.PrepareOrder(OrderPlaced{To: "buyer@example.invalid", Reference: "2026103000000000001", Amount: "9007199254740993.01", Currency: "eur", Locale: locale})
		if e != nil {
			t.Fatal(e)
		}
		var v request
		if json.Unmarshal(body, &v) != nil || !strings.Contains(v.Text, "9007199254740993.01 EUR") || strings.Contains(v.HTML, "href=") {
			t.Fatal("order content differs")
		}
		if got := s.Send(context.Background(), "mail/op_00000000-0000-4000-8000-000000000001", body); got.ID == "" {
			t.Fatal("order rejected")
		}
	}
	for _, in := range []OrderPlaced{{To: "bad", Reference: "order", Amount: "1", Currency: "eur", Locale: "en"}, {To: "buyer@example.invalid", Reference: "<script>", Amount: "1", Currency: "eur", Locale: "en"}, {To: "buyer@example.invalid", Reference: "order", Amount: "1e3", Currency: "eur", Locale: "en"}, {To: "buyer@example.invalid", Reference: "order", Amount: "1", Currency: "eur\n", Locale: "en"}} {
		if _, e := s.PrepareOrder(in); e == nil {
			t.Fatal("invalid order mail accepted")
		}
	}
	if calls != 2 {
		t.Fatal("unexpected mail calls")
	}
}
