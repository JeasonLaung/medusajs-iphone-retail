package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PayPalSettler interface {
	SessionObserver
	SettlementIntent() string
	SettleOrder(context.Context, SessionRequest, string) (SessionObservation, error)
}

func (a *PayPalCreateAdapter) SettlementIntent() string {
	if a == nil {
		return ""
	}
	return a.config.Intent
}

func InvokePayPalSettlement(ctx context.Context, adapter PayPalSettler, request SessionRequest, id string) (result SessionObservation, err error) {
	defer func() {
		if recover() != nil {
			result = SessionObservation{}
			err = ErrCreateExecution
		}
	}()
	if adapter == nil {
		return SessionObservation{}, ErrCreateExecution
	}
	result, err = adapter.SettleOrder(ctx, request, id)
	if err != nil || ValidatePayPalObservation(request, id, result) != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	return result, nil
}

// PayPalSettlementRequestID binds the settlement to the immutable local
// session, remote order and action. It is distinct from create's request key
// and remains within PayPal's 38-byte request-id limit. Caller retry UUIDs are
// deliberately absent. Invalid input never reaches a merchant endpoint.
func PayPalSettlementRequestID(request SessionRequest, id, intent string) (string, error) {
	if (request.ProviderID != "pp_paypal_paypal" && request.ProviderID != "pp_paypal_card_paypal_card") || !effectID.MatchString(request.SessionID) || !effectID.MatchString(request.CollectionID) || !paypalOrderID.MatchString(id) || (intent != "CAPTURE" && intent != "AUTHORIZE") {
		return "", ErrEffectInput
	}
	identity, _ := json.Marshal([]string{"retail-paypal-settlement-v1", request.ProviderID, request.CollectionID, request.SessionID, id, intent})
	return uuid.NewSHA1(uuid.NameSpaceOID, identity).String(), nil
}

// SettleOrder is one trusted, already-claimed provider effect, not an HTTP
// customer command. A durable coordinator must commit its claim before calling
// this method and persist its result even if the client disconnects. Unknown
// outcomes require ObserveSession, never automatic POST retries or new keys.
// The configured intent controls capture versus authorization; the browser
// cannot choose an action, amount, merchant or endpoint.
func (a *PayPalCreateAdapter) SettleOrder(ctx context.Context, request SessionRequest, id string) (SessionObservation, error) {
	if a == nil {
		return SessionObservation{}, ErrCreateExecution
	}
	key, err := PayPalSettlementRequestID(request, id, a.config.Intent)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	body, _, _, err := a.createBody(request)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	clear(body)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	// Authenticate the current order and exact money before any mutation. A
	// completed result is recovery evidence and never dispatches another POST.
	before, err := a.ObserveSession(ctx, request, id)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	if before.ProviderStatus == "paid" || before.ProviderStatus == "authorized" {
		return before, nil
	}
	var data struct {
		PayPal struct {
			Order paypalObservedOrder `json:"order"`
		} `json:"paypal"`
	}
	if json.Unmarshal(before.Session.Data, &data) != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	order := data.PayPal.Order
	if before.ProviderStatus != "pending" || order.Status != "APPROVED" || len(order.Units) != 1 || len(order.Units[0].Payments.Captures) != 0 || len(order.Units[0].Payments.Authorizations) != 0 {
		return SessionObservation{}, ErrCreateExecution
	}
	token, err := a.accessToken(ctx)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	action := map[string]string{"CAPTURE": "capture", "AUTHORIZE": "authorize"}[a.config.Intent]
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/v2/checkout/orders/"+id+"/"+action, strings.NewReader("{}"))
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("PayPal-Request-Id", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")
	resp, err := a.client.Do(req)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		return SessionObservation{}, ErrCreateExecution
	}
	raw, err := paypalJSON(resp)
	if err != nil {
		return SessionObservation{}, err
	}
	defer clear(raw)
	observed, err := a.verifiedObservation(request, id, raw)
	if err != nil || ValidatePayPalObservation(request, id, observed) != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	var settled paypalObservedOrder
	if json.Unmarshal(raw, &settled) != nil || len(settled.Units) != 1 || len(settled.Units[0].Payments.Captures)+len(settled.Units[0].Payments.Authorizations) == 0 {
		return SessionObservation{}, ErrCreateExecution
	}
	return observed, nil
}
