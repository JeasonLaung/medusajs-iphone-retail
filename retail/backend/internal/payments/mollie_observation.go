package payments

import (
	"context"
	"net/http"

	"github.com/eurorefurb/retail/internal/totals"
)

// SessionObservation is private server evidence, never a webhook/body DTO or a
// grant of order/cart authority. Session holds protected provider data. Successful
// observation alone does not record a receipt, release a freeze or create an
// order. Paid is deliberately separate from authorized: allocation needs cash.
type SessionObservation struct {
	PaymentID, ProviderStatus, Action string
	Amount, Currency                  string
	Session                           CreatedSession
}

// ObserveSession fetches the CURRENT payment using the configured merchant key.
// Candidate ID can come from an untrusted notification; every identity, amount,
// mode and return URL must match the trusted persisted request. No browser status
// or amount is accepted. Unknown IDs/errors are not evidence of no payment.
// Calls must occur outside SQL transactions; authorization and persistence are
// the coordinator's responsibility. There is no automatic POST/DELETE fallback.
func (a *MollieCreateAdapter) ObserveSession(ctx context.Context, request SessionRequest, candidateID string) (SessionObservation, error) {
	if !molliePaymentID.MatchString(candidateID) {
		return SessionObservation{}, ErrCreateExecution
	}
	body, redirect, cartID, err := a.createBody(request)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	clear(body)
	raw, err := a.transport.paymentJSON(ctx, http.MethodGet, candidateID)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	defer clear(raw)
	session, status, err := a.verifiedSession(raw, request, candidateID, redirect, cartID)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	settled, err := mollieAmount(request.Amount)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	amount, err := totals.Resolve(totals.Number{Decimal: settled})
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	// Same actions as the existing getWebhookActionAndData; retain the
	// provider status as well because paid and authorized share session status.
	action := map[string]string{
		"open": "not_supported", "pending": "pending", "paid": "successful",
		"authorized": "authorized", "canceled": "canceled", "expired": "failed", "failed": "failed",
	}[status]
	if action == "" {
		return SessionObservation{}, ErrCreateExecution
	}
	return SessionObservation{candidateID, status, action, amount, request.Currency, session}, nil
}
