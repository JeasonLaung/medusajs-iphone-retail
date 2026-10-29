package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
)

var molliePaymentID = regexp.MustCompile(`^tr_[A-Za-z0-9_]{1,120}$`)
var mollieAPIKey = regexp.MustCompile(`^(test|live)_[A-Za-z0-9]{1,200}$`)

// MollieDeleteAdapter is opt-in server configuration. It is not registered by
// startup and never reads credentials from the browser, request data or env.
// The API origin is fixed and redirects are forbidden, including on GET.
type MollieDeleteAdapter struct {
	providerID string
	apiKey     string
	client     *http.Client
}

func NewMollieDeleteAdapter(providerID, apiKey string) (*MollieDeleteAdapter, error) {
	if !effectID.MatchString(providerID) || !mollieAPIKey.MatchString(apiKey) {
		return nil, ErrEffectInput
	}
	return &MollieDeleteAdapter{
		providerID: providerID, apiKey: apiKey,
		client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

type molliePayment struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	IsCancelable *bool  `json:"isCancelable"`
	Amount       struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`
	Metadata struct {
		SessionID string `json:"session_id"`
		CartID    string `json:"cart_id"`
	} `json:"metadata"`
}

func (a *MollieDeleteAdapter) readPayment(ctx context.Context, method, id string) (molliePayment, error) {
	var payment molliePayment
	req, err := http.NewRequestWithContext(ctx, method, "https://api.mollie.com/v2/payments/"+id, nil)
	if err != nil {
		return payment, ErrDeleteExecution
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Accept", "application/hal+json")
	resp, err := a.client.Do(req)
	if err != nil {
		return payment, ErrDeleteExecution
	}
	defer resp.Body.Close()
	// 404 is not evidence of a prior successful cancellation. 422 may be a
	// concurrent payment; 429/5xx/transport failures must not enable retries.
	if resp.StatusCode != http.StatusOK {
		return payment, ErrDeleteExecution
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(protected.MaxPlaintext)+1))
	if err != nil {
		return payment, ErrDeleteExecution
	}
	defer clear(raw)
	validated, err := requestObject(raw, false)
	if err != nil || json.Unmarshal(validated, &payment) != nil {
		return molliePayment{}, ErrDeleteExecution
	}
	return payment, nil
}

func verifyMolliePayment(payment molliePayment, request SessionRequest, id, cartID string) bool {
	amount, err := totals.Resolve(totals.Number{Decimal: payment.Amount.Value})
	settled, expectedErr := mollieAmount(request.Amount)
	expected, canonicalErr := totals.Resolve(totals.Number{Decimal: settled})
	return err == nil && expectedErr == nil && canonicalErr == nil && !strings.HasPrefix(expected, "-") &&
		amount == expected && payment.Amount.Currency == strings.ToUpper(request.Currency) &&
		payment.ID == id && payment.Metadata.SessionID == request.SessionID &&
		(cartID == "" || payment.Metadata.CartID == cartID)
}

// DeleteSession retains the original Mollie cancelPayment decisions: paid or
// authorized sessions are protected; terminal unpaid sessions are already done;
// an explicitly non-cancelable open hosted checkout may be discarded locally.
// That last case does NOT claim the remote payment was canceled. Webhook/order
// reconciliation must retain the old provider reference before this is enabled.
func (a *MollieDeleteAdapter) DeleteSession(ctx context.Context, request SessionRequest) (DeleteOutcome, error) {
	if a == nil || a.client == nil || request.ProviderID != a.providerID ||
		!effectID.MatchString(request.SessionID) || !requestCurrency.MatchString(request.Currency) {
		return DeleteUncertain, ErrDeleteExecution
	}
	raw, err := requestObject(request.Data, false)
	var data struct {
		ID        string `json:"id"`
		SessionID string `json:"session_id"`
		CartID    string `json:"cart_id"`
	}
	if err != nil || json.Unmarshal(raw, &data) != nil || !molliePaymentID.MatchString(data.ID) ||
		(data.SessionID != "" && data.SessionID != request.SessionID) {
		return DeleteUncertain, ErrDeleteExecution
	}
	// Validate monetary input before any network call.
	amount, err := totals.Resolve(totals.Number{Decimal: request.Amount})
	if err != nil || strings.HasPrefix(amount, "-") {
		return DeleteUncertain, ErrDeleteExecution
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	payment, err := a.readPayment(callCtx, http.MethodGet, data.ID)
	if err != nil || !verifyMolliePayment(payment, request, data.ID, data.CartID) {
		return DeleteUncertain, ErrDeleteExecution
	}
	switch payment.Status {
	case "paid", "authorized":
		return DeleteFailed, nil // definitive refusal, no DELETE issued
	case "canceled", "expired", "failed":
		return DeleteSucceeded, nil
	case "open":
		if payment.IsCancelable != nil && !*payment.IsCancelable {
			return DeleteSucceeded, nil // legacy local discard, not remote cancel
		}
	case "pending":
		// A processing payment must never be hidden as a discarded open checkout.
	default:
		return DeleteUncertain, ErrDeleteExecution
	}
	if payment.IsCancelable == nil || !*payment.IsCancelable {
		return DeleteFailed, nil // no cancellation attempted; preserve local session
	}
	payment, err = a.readPayment(callCtx, http.MethodDelete, data.ID)
	if err != nil || !verifyMolliePayment(payment, request, data.ID, data.CartID) {
		return DeleteUncertain, ErrDeleteExecution
	}
	switch payment.Status {
	case "canceled", "expired", "failed":
		return DeleteSucceeded, nil
	default:
		return DeleteUncertain, ErrDeleteExecution
	}
}
