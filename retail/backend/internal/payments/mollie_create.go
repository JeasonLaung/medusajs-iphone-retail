package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
)

// These seven identifiers/methods are from the installed provider's services,
// not client-selected method parameters. Empty method is hosted checkout.
var mollieMethods = map[string]string{
	"pp_mollie-hosted-checkout_mollie": "", "pp_mollie-ideal_mollie": "ideal",
	"pp_mollie-card_mollie": "creditcard", "pp_mollie-bancontact_mollie": "bancontact",
	"pp_mollie-giftcard_mollie": "giftcard", "pp_mollie-paypal_mollie": "paypal",
	"pp_mollie-apple-pay_mollie": "applepay",
}

var mollieCartID = regexp.MustCompile(`^cart_[A-Za-z0-9_-]{1,120}$`)

type MollieCreateConfig struct {
	ProviderID    string
	APIKey        string
	RedirectURL   string
	BackendURL    string
	Description   string
	ManualCapture bool
}

// MollieCreateAdapter must be configured explicitly by trusted server code.
// No startup registration, SQL writes, environment lookup or automatic retries.
type MollieCreateAdapter struct {
	transport   *MollieDeleteAdapter
	redirect    *url.URL
	webhook     string
	description string
	method      string
	captureMode string
}

func mollieHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || !u.IsAbs() || u.Opaque != "" || u.Hostname() == "" || u.User != nil ||
		(u.Scheme != "https" && u.Scheme != "http") {
		return nil, ErrEffectInput
	}
	return u, nil
}

func mollieOrigin(u *url.URL) string {
	port := u.Port()
	if u.Scheme == "https" && port == "443" || u.Scheme == "http" && port == "80" {
		port = ""
	}
	return u.Scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

func NewMollieCreateAdapter(config MollieCreateConfig) (*MollieCreateAdapter, error) {
	method, ok := mollieMethods[config.ProviderID]
	if !ok {
		return nil, ErrEffectInput
	}
	transport, err := NewMollieDeleteAdapter(config.ProviderID, config.APIKey)
	if err != nil {
		return nil, err
	}
	redirect, err := mollieHTTPURL(config.RedirectURL)
	if err != nil || redirect.EscapedPath() != "/api/mollie-return" {
		return nil, ErrEffectInput
	}
	backend, err := mollieHTTPURL(config.BackendURL)
	if err != nil {
		return nil, ErrEffectInput
	}
	webhook := backend.ResolveReference(&url.URL{Path: "/hooks/payment/" + strings.TrimPrefix(config.ProviderID, "pp_")})
	description := config.Description
	if description == "" {
		description = "iPhone retail order"
	}
	if !utf8.ValidString(description) || utf8.RuneCountInString(description) > 255 {
		return nil, ErrEffectInput
	}
	captureMode := "automatic"
	if config.ManualCapture {
		captureMode = "manual"
	}
	return &MollieCreateAdapter{transport, redirect, webhook.String(), description, method, captureMode}, nil
}

// The original integration always sends two decimals. Preserve that settlement
// format using exact half-away rounding, correcting Number/toFixed precision
// loss per payment-contract.md. This never quantizes SQL or cart raw amounts.
func mollieAmount(raw string) (string, error) {
	normalized, err := totals.Resolve(totals.Number{Decimal: raw})
	if err != nil || strings.HasPrefix(normalized, "-") {
		return "", ErrEffectInput
	}
	n, ok := new(big.Rat).SetString(normalized)
	if !ok {
		return "", ErrEffectInput
	}
	n.Mul(n, big.NewRat(100, 1))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(n.Num(), n.Denom(), rem)
	if new(big.Int).Lsh(rem, 1).Cmp(n.Denom()) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	s := q.String()
	if len(s) < 3 {
		s = strings.Repeat("0", 3-len(s)) + s
	}
	return s[:len(s)-2] + "." + s[len(s)-2:], nil
}

func (a *MollieCreateAdapter) createBody(request SessionRequest) ([]byte, string, string, error) {
	if a == nil || a.transport == nil || a.redirect == nil || request.ProviderID != a.transport.providerID ||
		!strings.HasPrefix(request.SessionID, "payses_") || !effectID.MatchString(request.SessionID) ||
		!effectID.MatchString(request.CollectionID) || !requestCurrency.MatchString(request.Currency) {
		return nil, "", "", ErrEffectInput
	}
	raw, err := requestObject(request.Data, false)
	var data struct {
		CartID        string  `json:"cart_id"`
		SessionID     *string `json:"session_id"`
		MollieAttempt *string `json:"mollie_attempt"`
		RedirectURL   *string `json:"redirect_url"`
	}
	if err != nil || json.Unmarshal(raw, &data) != nil || !mollieCartID.MatchString(data.CartID) ||
		(data.SessionID != nil && *data.SessionID != request.SessionID) ||
		(data.MollieAttempt != nil && *data.MollieAttempt != request.SessionID) {
		return nil, "", "", ErrEffectInput
	}
	contextRaw, err := requestObject(request.Context, true)
	var savedContext struct {
		IdempotencyKey *string `json:"idempotency_key"`
	}
	if err != nil || json.Unmarshal(contextRaw, &savedContext) != nil ||
		(savedContext.IdempotencyKey != nil && *savedContext.IdempotencyKey != request.SessionID) {
		return nil, "", "", ErrEffectInput
	}
	redirect := a.redirect
	if data.RedirectURL != nil {
		redirect, err = mollieHTTPURL(*data.RedirectURL)
		if err != nil {
			return nil, "", "", ErrEffectInput
		}
	}
	if mollieOrigin(redirect) != mollieOrigin(a.redirect) || redirect.EscapedPath() != "/api/mollie-return" ||
		redirect.Query().Get("cart_id") != data.CartID {
		return nil, "", "", ErrEffectInput
	}
	amount, err := mollieAmount(request.Amount)
	if err != nil {
		return nil, "", "", err
	}
	body := map[string]any{
		"amount":      map[string]string{"value": amount, "currency": strings.ToUpper(request.Currency)},
		"description": a.description, "webhookUrl": a.webhook, "redirectUrl": redirect.String(),
		"metadata":    map[string]string{"session_id": request.SessionID, "cart_id": data.CartID},
		"captureMode": a.captureMode,
	}
	if a.method != "" {
		body["method"] = a.method
	}
	encoded, err := json.Marshal(body)
	return encoded, redirect.String(), data.CartID, err
}

func (a *MollieCreateAdapter) CreateSession(ctx context.Context, request SessionRequest) (CreateOutcome, CreatedSession, error) {
	body, redirect, cartID, err := a.createBody(request)
	if err != nil {
		// No HTTP operation was attempted; this is a definitive local refusal.
		return CreateFailed, CreatedSession{}, nil
	}
	defer clear(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mollie.com/v2/payments", bytes.NewReader(body))
	if err != nil {
		return CreateFailed, CreatedSession{}, nil
	}
	// bytes.Reader supplies GetBody by default. With Idempotency-Key that lets
	// net/http replay POST on a reused broken connection. Recovery belongs to
	// the durable journal, so do not grant the transport a body replay function.
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+a.transport.apiKey)
	req.Header.Set("Idempotency-Key", request.SessionID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/hal+json")
	resp, err := a.transport.client.Do(req)
	if err != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	defer resp.Body.Close()
	// Even a validation/rate error is not automatically retryable. The journal's
	// uncertain path requires reconciliation, never a new key or blind POST.
	if resp.StatusCode != http.StatusCreated {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(protected.MaxPlaintext)+1))
	if err != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	defer clear(raw)
	validated, err := requestObject(raw, false)
	if err != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	result, _, err := a.verifiedSession(validated, request, "", redirect, cartID)
	if err != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	return CreateSucceeded, result, nil
}

// verifiedSession is shared by initial creation and authoritative GET recovery.
// expectedID is empty only on a POST whose new remote ID was not known yet.
func (a *MollieCreateAdapter) verifiedSession(validated json.RawMessage, request SessionRequest, expectedID, redirect, cartID string) (CreatedSession, string, error) {
	var payment molliePayment
	var properties struct {
		Resource    string `json:"resource"`
		Mode        string `json:"mode"`
		RedirectURL string `json:"redirectUrl"`
		Links       struct {
			Checkout struct {
				Href string `json:"href"`
			} `json:"checkout"`
		} `json:"_links"`
	}
	if json.Unmarshal(validated, &payment) != nil || json.Unmarshal(validated, &properties) != nil ||
		!molliePaymentID.MatchString(payment.ID) || (expectedID != "" && payment.ID != expectedID) || !verifyMolliePayment(payment, request, payment.ID, cartID) ||
		properties.Resource != "payment" || properties.Mode != strings.SplitN(a.transport.apiKey, "_", 2)[0] || properties.RedirectURL != redirect {
		return CreatedSession{}, "", ErrCreateExecution
	}
	status := map[string]string{"open": "pending", "pending": "pending", "paid": "authorized", "authorized": "authorized", "canceled": "canceled", "expired": "error", "failed": "error"}[payment.Status]
	if status == "" {
		return CreatedSession{}, "", ErrCreateExecution
	}
	if status == "pending" {
		checkout, err := mollieHTTPURL(properties.Links.Checkout.Href)
		if err != nil || checkout.Scheme != "https" || (checkout.Hostname() != "mollie.com" && !strings.HasSuffix(checkout.Hostname(), ".mollie.com")) || (checkout.Port() != "" && checkout.Port() != "443") {
			return CreatedSession{}, "", ErrCreateExecution
		}
	}
	var saved map[string]json.RawMessage
	if json.Unmarshal(validated, &saved) != nil {
		return CreatedSession{}, "", ErrCreateExecution
	}
	for key, value := range map[string]string{"cart_id": cartID, "redirect_url": redirect, "session_id": request.SessionID} {
		saved[key], _ = json.Marshal(value)
	}
	encoded, err := json.Marshal(saved)
	if err != nil {
		return CreatedSession{}, "", ErrCreateExecution
	}
	return CreatedSession{Status: status, Data: encoded}, payment.Status, nil
}

// MollieSettlementAmount exposes the existing exact settlement rounding to the
// trusted cart coordinator. It performs no provider call or currency conversion.
func MollieSettlementAmount(amount string) (string, error) { return mollieAmount(amount) }
