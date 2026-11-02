package payments

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
)

var paypalCredential = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
var paypalMerchant = regexp.MustCompile(`^[A-Z0-9]{13}$`)
var paypalOrderID = regexp.MustCompile(`^[A-Z0-9]{17}$`)
var paypalAccessToken = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+$`)

func PayPalSettlementAmount(amount, currency string) (string, error) {
	return paypalAmount(amount, currency)
}

// PayPalCreatedOrderID projects only the SDK identifier from protected adapter
// output. It grants no payment status and never exposes the full provider order.
func PayPalCreatedOrderID(request SessionRequest, result CreatedSession) (string, error) {
	if (request.ProviderID != "pp_paypal_paypal" && request.ProviderID != "pp_paypal_card_paypal_card") || result.Status != "pending" {
		return "", ErrEffectInput
	}
	raw, e := requestObject(result.Data, false)
	if e != nil {
		return "", e
	}
	var data struct {
		SessionID string `json:"session_id"`
		CartID    string `json:"cart_id"`
		PayPal    struct {
			OrderID string `json:"order_id"`
		} `json:"paypal"`
	}
	var original struct {
		CartID string `json:"cart_id"`
	}
	if json.Unmarshal(raw, &data) != nil || json.Unmarshal(request.Data, &original) != nil || data.CartID != original.CartID || data.SessionID != request.SessionID || !paypalOrderID.MatchString(data.PayPal.OrderID) {
		return "", ErrEffectInput
	}
	return data.PayPal.OrderID, nil
}

// PayPalCreateConfig is trusted merchant configuration. Orders are created for
// the original wallet/card SDK flow, not captured or marked paid by this adapter.
// No API base URL, merchant, credential or intent is taken from customer input.
type PayPalCreateConfig struct {
	ProviderID, ClientID, ClientSecret, MerchantID string
	Environment, Intent, ReturnURL, CancelURL      string
	RootCAs                                        *x509.CertPool
}

type PayPalCreateAdapter struct {
	config PayPalCreateConfig
	base   string
	client *http.Client
}

func NewPayPalCreateAdapter(config PayPalCreateConfig) (*PayPalCreateAdapter, error) {
	if (config.ProviderID != "pp_paypal_paypal" && config.ProviderID != "pp_paypal_card_paypal_card") ||
		!paypalCredential.MatchString(config.ClientID) || !paypalCredential.MatchString(config.ClientSecret) ||
		!paypalMerchant.MatchString(config.MerchantID) || (config.Intent != "CAPTURE" && config.Intent != "AUTHORIZE") {
		return nil, ErrEffectInput
	}
	base := map[string]string{"sandbox": "https://api-m.sandbox.paypal.com", "live": "https://api-m.paypal.com"}[config.Environment]
	if base == "" {
		return nil, ErrEffectInput
	}
	for _, raw := range []string{config.ReturnURL, config.CancelURL} {
		u, err := mollieHTTPURL(raw)
		if err != nil || u.Fragment != "" {
			return nil, ErrEffectInput
		}
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, ErrEffectInput
	}
	owned := transport.Clone()
	owned.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if config.RootCAs != nil {
		owned.TLSClientConfig.RootCAs = config.RootCAs.Clone()
	}
	// Avoid environment HTTP proxies for authenticated merchant requests.
	owned.Proxy = nil
	return &PayPalCreateAdapter{config: config, base: base, client: &http.Client{
		Timeout: 8 * time.Second, Transport: owned,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

// PayPal settlement uses zero decimals for HUF/JPY/TWD, two for the other
// supported currencies. Keep the source SQL amount exact; round only the API
// value with the same exact half-away rule as the existing Mollie adapter.
func paypalAmount(amount, currency string) (string, error) {
	if !requestCurrency.MatchString(currency) || !strings.Contains("|aud|brl|cad|chf|czk|dkk|eur|gbp|hkd|huf|ils|jpy|mxn|myr|nok|nzd|php|pln|sek|sgd|thb|twd|usd|", "|"+currency+"|") {
		return "", ErrEffectInput
	}
	normalized, err := totals.Resolve(totals.Number{Decimal: amount})
	if err != nil || strings.HasPrefix(normalized, "-") {
		return "", ErrEffectInput
	}
	n, ok := new(big.Rat).SetString(normalized)
	if !ok {
		return "", ErrEffectInput
	}
	zero := currency == "huf" || currency == "jpy" || currency == "twd"
	if !zero {
		n.Mul(n, big.NewRat(100, 1))
	}
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(n.Num(), n.Denom(), rem)
	if new(big.Int).Lsh(rem, 1).Cmp(n.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	s := q.String()
	if zero {
		return s, nil
	}
	if len(s) < 3 {
		s = strings.Repeat("0", 3-len(s)) + s
	}
	return s[:len(s)-2] + "." + s[len(s)-2:], nil
}

func (a *PayPalCreateAdapter) createBody(request SessionRequest) ([]byte, string, string, error) {
	if a == nil || request.ProviderID != a.config.ProviderID || !effectID.MatchString(request.CollectionID) ||
		!effectID.MatchString(request.SessionID) || len(request.SessionID) > 108 {
		return nil, "", "", ErrEffectInput
	}
	amount, err := paypalAmount(request.Amount, request.Currency)
	if err != nil || strings.Trim(amount, "0.") == "" {
		return nil, "", "", ErrEffectInput
	}
	raw, err := requestObject(request.Data, false)
	var data struct {
		CartID    string  `json:"cart_id"`
		SessionID *string `json:"session_id"`
	}
	if err != nil || json.Unmarshal(raw, &data) != nil || !mollieCartID.MatchString(data.CartID) || (data.SessionID != nil && *data.SessionID != request.SessionID) {
		return nil, "", "", ErrEffectInput
	}
	raw, err = requestObject(request.Context, true)
	var ctx struct {
		IdempotencyKey *string `json:"idempotency_key"`
	}
	if err != nil || json.Unmarshal(raw, &ctx) != nil || (ctx.IdempotencyKey != nil && *ctx.IdempotencyKey != request.SessionID) {
		return nil, "", "", ErrEffectInput
	}
	experience := map[string]string{"return_url": a.config.ReturnURL, "cancel_url": a.config.CancelURL}
	source := "card"
	if a.config.ProviderID == "pp_paypal_paypal" {
		source = "paypal"
		experience["landing_page"] = "LOGIN"
		experience["user_action"] = "PAY_NOW"
	}
	body, err := json.Marshal(map[string]any{
		"intent": a.config.Intent,
		"purchase_units": []any{map[string]any{"reference_id": "default", "custom_id": data.CartID, "invoice_id": request.SessionID,
			"payee":  map[string]string{"merchant_id": a.config.MerchantID},
			"amount": map[string]string{"currency_code": strings.ToUpper(request.Currency), "value": amount}}},
		"payment_source": map[string]any{source: map[string]any{"experience_context": experience}},
	})
	return body, data.CartID, amount, err
}

func paypalJSON(resp *http.Response) (json.RawMessage, error) {
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(protected.MaxPlaintext)+1))
	if err != nil {
		return nil, ErrCreateExecution
	}
	defer clear(raw)
	validated, err := requestObject(raw, false)
	if err != nil {
		return nil, ErrCreateExecution
	}
	return validated, nil
}

func (a *PayPalCreateAdapter) accessToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/v1/oauth2/token", strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", ErrCreateExecution
	}
	req.GetBody = nil
	req.SetBasicAuth(a.config.ClientID, a.config.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", ErrCreateExecution
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return "", ErrCreateExecution
	}
	raw, err := paypalJSON(resp)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.Unmarshal(raw, &token) != nil || len(token.AccessToken) > 8192 || !paypalAccessToken.MatchString(token.AccessToken) || !strings.EqualFold(token.TokenType, "bearer") || token.ExpiresIn <= 0 {
		return "", ErrCreateExecution
	}
	return token.AccessToken, nil
}

func (a *PayPalCreateAdapter) CreateSession(ctx context.Context, request SessionRequest) (CreateOutcome, CreatedSession, error) {
	body, cartID, amount, err := a.createBody(request)
	if err != nil {
		return CreateFailed, CreatedSession{}, nil
	}
	defer clear(body)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	token, err := a.accessToken(ctx)
	if err != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/v2/checkout/orders", bytes.NewReader(body))
	if err != nil {
		return CreateFailed, CreatedSession{}, nil
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("PayPal-Request-Id", request.SessionID)
	req.Header.Set("Prefer", "return=representation")
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	raw, err := paypalJSON(resp)
	if err != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	defer clear(raw)
	var order struct {
		ID, Intent, Status string
		PurchaseUnits      []struct {
			ReferenceID string `json:"reference_id"`
			CustomID    string `json:"custom_id"`
			InvoiceID   string `json:"invoice_id"`
			Amount      struct {
				Value        string
				CurrencyCode string `json:"currency_code"`
			}
			Payee struct {
				MerchantID string `json:"merchant_id"`
			}
		} `json:"purchase_units"`
	}
	if json.Unmarshal(raw, &order) != nil || !paypalOrderID.MatchString(order.ID) || order.Intent != a.config.Intent || len(order.PurchaseUnits) != 1 ||
		(order.Status != "CREATED" && order.Status != "PAYER_ACTION_REQUIRED" && order.Status != "APPROVED") {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	unit := order.PurchaseUnits[0]
	actual, ae := totals.Resolve(totals.Number{Decimal: unit.Amount.Value})
	expected, ee := totals.Resolve(totals.Number{Decimal: amount})
	if ae != nil || ee != nil || actual != expected || unit.Amount.CurrencyCode != strings.ToUpper(request.Currency) || unit.ReferenceID != "default" ||
		unit.CustomID != cartID || unit.InvoiceID != request.SessionID || unit.Payee.MerchantID != a.config.MerchantID {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	// APPROVED is buyer approval, not capture or authorization of money. Keep
	// the SDK/provider order private; a separate authenticated operation settles it.
	data, _ := json.Marshal(map[string]any{"session_id": request.SessionID, "cart_id": cartID, "paypal": map[string]any{"order_id": order.ID, "order": raw}})
	return CreateSucceeded, CreatedSession{Status: "pending", Data: data}, nil
}
