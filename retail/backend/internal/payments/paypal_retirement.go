package payments

import (
	"context"
	"encoding/json"
)

// PayPal Orders v2 has no DELETE order operation. The original provider's
// deletePayment only discards the local session. This adapter first verifies
// the saved merchant/order and refuses any capture or authorization history.
// The cart coordinator must freeze settlement and exclude existing dispatches
// before calling it. Success means local retirement, never remote cancellation.
type PayPalRetirementAdapter struct{ creator *PayPalCreateAdapter }

func (a *PayPalCreateAdapter) RetirementAdapter() (*PayPalRetirementAdapter, error) {
	if a == nil || a.client == nil {
		return nil, ErrEffectInput
	}
	return &PayPalRetirementAdapter{creator: a}, nil
}

func PayPalRetirementOutcome(request SessionRequest, id string, observed SessionObservation) (DeleteOutcome, error) {
	if err := ValidatePayPalObservation(request, id, observed); err != nil {
		return DeleteUncertain, ErrDeleteExecution
	}
	var data struct {
		PayPal struct {
			Order paypalObservedOrder `json:"order"`
		} `json:"paypal"`
	}
	if json.Unmarshal(observed.Session.Data, &data) != nil || len(data.PayPal.Order.Units) != 1 {
		return DeleteUncertain, ErrDeleteExecution
	}
	unit := data.PayPal.Order.Units[0]
	if len(unit.Payments.Captures) != 0 || len(unit.Payments.Authorizations) != 0 {
		return DeleteFailed, nil
	}
	switch data.PayPal.Order.Status {
	case "CREATED", "SAVED", "PAYER_ACTION_REQUIRED", "APPROVED", "VOIDED":
		return DeleteSucceeded, nil
	default:
		return DeleteUncertain, ErrDeleteExecution
	}
}

func (a *PayPalRetirementAdapter) DeleteSession(ctx context.Context, request SessionRequest) (DeleteOutcome, error) {
	if a == nil || a.creator == nil {
		return DeleteUncertain, ErrDeleteExecution
	}
	id, err := PayPalCreatedOrderID(request, CreatedSession{Status: "pending", Data: request.Data})
	if err != nil {
		return DeleteUncertain, ErrDeleteExecution
	}
	observed, err := a.creator.ObserveSession(ctx, request, id)
	if err != nil {
		return DeleteUncertain, ErrDeleteExecution
	}
	return PayPalRetirementOutcome(request, id, observed)
}
