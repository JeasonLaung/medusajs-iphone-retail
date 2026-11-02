package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/totals"
)

type paypalMoney struct {
	Value    string `json:"value"`
	Currency string `json:"currency_code"`
}
type paypalTransaction struct {
	ID           string      `json:"id"`
	Status       string      `json:"status"`
	Amount       paypalMoney `json:"amount"`
	FinalCapture *bool       `json:"final_capture"`
}
type paypalObservedOrder struct {
	ID     string `json:"id"`
	Intent string `json:"intent"`
	Status string `json:"status"`
	Units  []struct {
		Reference string      `json:"reference_id"`
		Cart      string      `json:"custom_id"`
		Invoice   string      `json:"invoice_id"`
		Amount    paypalMoney `json:"amount"`
		Payee     struct {
			Merchant string `json:"merchant_id"`
		} `json:"payee"`
		Payments struct {
			Captures       []paypalTransaction `json:"captures"`
			Authorizations []paypalTransaction `json:"authorizations"`
		} `json:"payments"`
	} `json:"purchase_units"`
}

func paypalExactMoney(money paypalMoney, amount, currency string) bool {
	actual, e := totals.Resolve(totals.Number{Decimal: money.Value})
	expected, ee := totals.Resolve(totals.Number{Decimal: amount})
	return e == nil && ee == nil && actual == expected && money.Currency == strings.ToUpper(currency)
}

// Observation never infers captured cash from order COMPLETED or approval.
// Only a single completed full final capture is eligible for paid. Partial,
// refunded, ambiguous and unrecognized histories require explicit recovery.
func paypalObservedState(order paypalObservedOrder) (string, string, string, error) {
	if len(order.Units) != 1 {
		return "", "", "", ErrCreateExecution
	}
	unit := order.Units[0]
	captures, auths := unit.Payments.Captures, unit.Payments.Authorizations
	if len(captures) > 1 || len(auths) > 1 {
		return "", "", "", ErrCreateExecution
	}
	for _, tx := range append(append([]paypalTransaction{}, captures...), auths...) {
		if !paypalOrderID.MatchString(tx.ID) || !paypalExactMoney(tx.Amount, unit.Amount.Value, strings.ToLower(unit.Amount.Currency)) {
			return "", "", "", ErrCreateExecution
		}
	}
	if len(captures) == 1 {
		capture := captures[0]
		if order.Status != "COMPLETED" && order.Status != "APPROVED" {
			return "", "", "", ErrCreateExecution
		}
		if len(auths) == 1 && (order.Intent != "AUTHORIZE" || (auths[0].Status != "CAPTURED" && auths[0].Status != "PARTIALLY_CAPTURED" && auths[0].Status != "CREATED")) {
			return "", "", "", ErrCreateExecution
		}
		switch capture.Status {
		case "COMPLETED":
			if capture.FinalCapture == nil || !*capture.FinalCapture {
				return "", "", "", ErrCreateExecution
			}
			return "paid", "successful", "authorized", nil
		case "PENDING":
			return "pending", "pending", "pending", nil
		case "DECLINED", "DENIED", "FAILED":
			return "failed", "failed", "error", nil
		default:
			return "", "", "", ErrCreateExecution
		}
	}
	if len(auths) == 1 {
		if order.Intent != "AUTHORIZE" || (order.Status != "COMPLETED" && order.Status != "APPROVED") {
			return "", "", "", ErrCreateExecution
		}
		switch auths[0].Status {
		case "CREATED":
			return "authorized", "authorized", "authorized", nil
		case "PENDING":
			return "pending", "pending", "pending", nil
		case "VOIDED", "EXPIRED":
			return "canceled", "canceled", "canceled", nil
		case "DENIED", "DECLINED", "FAILED":
			return "failed", "failed", "error", nil
		default:
			return "", "", "", ErrCreateExecution
		}
	}
	switch order.Status {
	case "CREATED", "SAVED", "PAYER_ACTION_REQUIRED", "APPROVED":
		return "pending", "pending", "pending", nil
	case "VOIDED":
		return "canceled", "canceled", "canceled", nil
	default:
		return "", "", "", ErrCreateExecution
	}
}

func verifyPayPalObservedOrder(raw json.RawMessage, request SessionRequest, id, cart, merchant, intent string) (paypalObservedOrder, error) {
	var order paypalObservedOrder
	if _, err := requestObject(raw, false); err != nil {
		return order, ErrCreateExecution
	}
	if json.Unmarshal(raw, &order) != nil || !paypalOrderID.MatchString(id) || order.ID != id || order.Intent != intent || len(order.Units) != 1 {
		return order, ErrCreateExecution
	}
	unit := order.Units[0]
	amount, err := paypalAmount(request.Amount, request.Currency)
	if err != nil || unit.Reference != "default" || unit.Cart != cart || unit.Invoice != request.SessionID || unit.Payee.Merchant != merchant || !paypalMerchant.MatchString(merchant) || !paypalExactMoney(unit.Amount, amount, request.Currency) {
		return order, ErrCreateExecution
	}
	return order, nil
}

// ObserveSession authenticates to the fixed merchant API and only GETs the
// saved order locator. It never captures/authorizes, retries creation, follows
// provider links, writes SQL or accepts a browser's payment success claims.
func (a *PayPalCreateAdapter) ObserveSession(ctx context.Context, request SessionRequest, id string) (SessionObservation, error) {
	if !paypalOrderID.MatchString(id) {
		return SessionObservation{}, ErrCreateExecution
	}
	body, _, _, err := a.createBody(request)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	clear(body)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	token, err := a.accessToken(ctx)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/v2/checkout/orders/"+id, nil)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return SessionObservation{}, ErrCreateExecution
	}
	raw, err := paypalJSON(resp)
	if err != nil {
		return SessionObservation{}, err
	}
	defer clear(raw)
	return a.verifiedObservation(request, id, raw)
}

func (a *PayPalCreateAdapter) verifiedObservation(request SessionRequest, id string, raw json.RawMessage) (SessionObservation, error) {
	body, cart, amount, err := a.createBody(request)
	if err != nil {
		return SessionObservation{}, err
	}
	clear(body)
	order, err := verifyPayPalObservedOrder(raw, request, id, cart, a.config.MerchantID, a.config.Intent)
	if err != nil {
		return SessionObservation{}, err
	}
	status, action, local, err := paypalObservedState(order)
	if err != nil {
		return SessionObservation{}, err
	}
	data, _ := json.Marshal(map[string]any{"session_id": request.SessionID, "cart_id": cart, "merchant_id": a.config.MerchantID, "intent": a.config.Intent, "paypal": map[string]any{"order_id": id, "order": raw}})
	amount, err = totals.Resolve(totals.Number{Decimal: amount})
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	return SessionObservation{PaymentID: id, ProviderStatus: status, Action: action, Amount: amount, Currency: request.Currency, Session: CreatedSession{Status: local, Data: data}}, nil
}

// ValidatePayPalObservation rechecks the private adapter projection before a
// coordinator persists it. Merchant credentials remain the adapter's authority;
// these serialized fields are never customer inputs or proof on their own.
func ValidatePayPalObservation(request SessionRequest, id string, observed SessionObservation) error {
	if request.ProviderID != "pp_paypal_paypal" && request.ProviderID != "pp_paypal_card_paypal_card" {
		return ErrEffectConflict
	}
	raw, err := requestObject(observed.Session.Data, false)
	if err != nil {
		return ErrEffectConflict
	}
	var data struct {
		Session  string `json:"session_id"`
		Cart     string `json:"cart_id"`
		Merchant string `json:"merchant_id"`
		Intent   string `json:"intent"`
		PayPal   struct {
			ID    string          `json:"order_id"`
			Order json.RawMessage `json:"order"`
		} `json:"paypal"`
	}
	var input struct {
		Cart string `json:"cart_id"`
	}
	if json.Unmarshal(raw, &data) != nil || json.Unmarshal(request.Data, &input) != nil || !mollieCartID.MatchString(input.Cart) || data.Cart != input.Cart || data.Session != request.SessionID || data.PayPal.ID != id || (data.Intent != "CAPTURE" && data.Intent != "AUTHORIZE") {
		return ErrEffectConflict
	}
	order, err := verifyPayPalObservedOrder(data.PayPal.Order, request, id, input.Cart, data.Merchant, data.Intent)
	if err != nil {
		return ErrEffectConflict
	}
	status, action, local, err := paypalObservedState(order)
	if err != nil || observed.PaymentID != id || observed.ProviderStatus != status || observed.Action != action || observed.Session.Status != local || observed.Currency != request.Currency {
		return ErrEffectConflict
	}
	amount, err := paypalAmount(request.Amount, request.Currency)
	if err != nil {
		return ErrEffectConflict
	}
	expected, _ := totals.Resolve(totals.Number{Decimal: amount})
	actual, err := totals.Resolve(totals.Number{Decimal: observed.Amount})
	if err != nil || actual != expected {
		return ErrEffectConflict
	}
	return nil
}
