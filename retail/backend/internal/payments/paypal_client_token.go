package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"
)

var paypalBrowserToken = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]+$`)

// This short-lived browser token is distinct from the private OAuth access token.
type PayPalClientToken struct {
	ClientToken string `json:"client_token"`
}

func (a *PayPalCreateAdapter) ClientToken(ctx context.Context) (out PayPalClientToken, err error) {
	defer func() {
		if recover() != nil || err != nil {
			out = PayPalClientToken{}
			err = ErrCreateExecution
		}
	}()
	if a == nil || a.config.ProviderID != "pp_paypal_card_paypal_card" {
		return out, ErrCreateExecution
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	token, e := a.accessToken(ctx)
	if e != nil {
		return out, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/v1/identity/generate-token", nil)
	if e != nil {
		return out, e
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, e := a.client.Do(req)
	if e != nil {
		return out, e
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		response.Body.Close()
		return out, ErrCreateExecution
	}
	raw, e := paypalJSON(response)
	if e != nil {
		return out, e
	}
	defer clear(raw)
	if json.Unmarshal(raw, &out) != nil || len(out.ClientToken) > 16384 || !paypalBrowserToken.MatchString(out.ClientToken) || out.ClientToken == token {
		return PayPalClientToken{}, ErrCreateExecution
	}
	return out, nil
}
