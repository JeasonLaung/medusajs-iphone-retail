package payments

import (
	"encoding/json"
	"github.com/eurorefurb/retail/internal/totals"
)

// ValidateMollieObservation checks a trusted adapter result against its saved
// request before a coordinator can persist/apply it. No browser status is input.
func ValidateMollieObservation(request SessionRequest, candidate string, observed SessionObservation) error {
	raw, err := requestObject(observed.Session.Data, false)
	if err != nil {
		return err
	}
	var payment molliePayment
	var cart struct {
		CartID string `json:"cart_id"`
	}
	if !molliePaymentID.MatchString(candidate) || json.Unmarshal(raw, &payment) != nil || json.Unmarshal(request.Data, &cart) != nil || !mollieCartID.MatchString(cart.CartID) || observed.PaymentID != candidate || payment.Status != observed.ProviderStatus || !verifyMolliePayment(payment, request, candidate, cart.CartID) {
		return ErrEffectConflict
	}
	actions := map[string]string{"open": "not_supported", "pending": "pending", "paid": "successful", "authorized": "authorized", "canceled": "canceled", "expired": "failed", "failed": "failed"}
	statuses := map[string]string{"open": "pending", "pending": "pending", "paid": "authorized", "authorized": "authorized", "canceled": "canceled", "expired": "error", "failed": "error"}
	expected, err := mollieAmount(request.Amount)
	if err != nil {
		return err
	}
	expected, err = totals.Resolve(totals.Number{Decimal: expected})
	if err != nil {
		return err
	}
	amount, err := totals.Resolve(totals.Number{Decimal: observed.Amount})
	if err != nil || amount != expected || observed.Currency != request.Currency || actions[payment.Status] == "" || observed.Action != actions[payment.Status] || observed.Session.Status != statuses[payment.Status] {
		return ErrEffectConflict
	}
	return nil
}
