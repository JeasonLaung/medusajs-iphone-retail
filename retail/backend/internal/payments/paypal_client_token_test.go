package payments

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPayPalBrowserTokenIsNotOAuthOrFinancialRequest(t *testing.T) {
	for _, body := range []string{`{"client_token":"artificial.browser-token"}`, `{"client_token":"artificial.oauth-token"}`, `{"access_token":"private"}`, `{"client_token":"bad\nheader"}`, `{"client_token":""}`} {
		cfg := paypalConfig()
		cfg.ProviderID = "pp_paypal_card_paypal_card"
		a, _ := NewPayPalCreateAdapter(cfg)
		calls := 0
		a.client.Transport = mollieTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" || r.GetBody != nil || r.URL.Host != "api-m.sandbox.paypal.com" {
				t.Fatal("token endpoint or replay")
			}
			response := body
			if calls == 1 {
				if r.URL.Path != "/v1/oauth2/token" {
					t.Fatal("missing OAuth")
				}
				response = `{"access_token":"artificial.oauth-token","token_type":"Bearer","expires_in":3600}`
			} else if r.URL.Path != "/v1/identity/generate-token" || r.Header.Get("Authorization") != "Bearer artificial.oauth-token" || r.Body != nil {
				t.Fatal("browser token request accepted cart/card data")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
		})
		token, err := a.ClientToken(context.Background())
		if body == `{"client_token":"artificial.browser-token"}` {
			if err != nil || token.ClientToken != "artificial.browser-token" {
				t.Fatal(err)
			}
		} else if err == nil || token.ClientToken != "" {
			t.Fatal("private/malformed token returned")
		}
		if calls != 2 {
			t.Fatal("unexpected financial call")
		}
	}
}
