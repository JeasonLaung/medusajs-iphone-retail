package payments

import "strings"

// Browser-safe SDK input. Secrets, access tokens and merchant API URLs never
// enter this projection. The browser cannot select the server settlement action.
type PayPalSDKConfig struct {
	ClientID    string `json:"client_id"`
	Currency    string `json:"currency"`
	Intent      string `json:"intent"`
	Environment string `json:"environment"`
}

func (a *PayPalCreateAdapter) SDKConfiguration(currency string) (PayPalSDKConfig, error) {
	if a == nil {
		return PayPalSDKConfig{}, ErrEffectInput
	}
	if _, e := PayPalSettlementAmount("1", currency); e != nil {
		return PayPalSDKConfig{}, e
	}
	return PayPalSDKConfig{ClientID: a.config.ClientID, Currency: strings.ToUpper(currency), Intent: strings.ToLower(a.config.Intent), Environment: a.config.Environment}, nil
}
