package checkoutconfig

import (
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
)

// ContextPayments is separately opt-in. Enabling initial checkout alone never
// registers a provider cancellation adapter. Runtime SQL gates still apply.
func ContextPayments(env func(string) string, payment *cart.InitialPaymentOptions) (*payments.DeleteAdapters, error) {
	switch env("CHECKOUT_CONTEXT_PAYMENT_ENABLED") {
	case "", "false":
		return nil, nil
	case "true":
	default:
		return nil, ErrInvalid
	}
	if payment == nil || payment.Ring == nil || payment.Adapters == nil {
		return nil, ErrInvalid
	}
	const provider = "pp_mollie-hosted-checkout_mollie"
	if _, err := payment.Adapters.Observer(provider); err != nil {
		return nil, ErrInvalid
	}
	creator, err := payment.Adapters.Lookup(provider)
	if err != nil {
		return nil, ErrInvalid
	}
	mollie, ok := creator.(*payments.MollieCreateAdapter)
	if !ok {
		return nil, ErrInvalid
	}
	adapter, err := mollie.CancellationAdapter()
	if err != nil {
		return nil, ErrInvalid
	}
	entries := map[string]payments.DeleteSessionAdapter{provider: adapter}
	for _, id := range []string{"pp_paypal_paypal", "pp_paypal_card_paypal_card"} {
		creator, err := payment.Adapters.Lookup(id)
		if err != nil {
			continue // only explicitly configured PayPal methods participate
		}
		paypal, ok := creator.(*payments.PayPalCreateAdapter)
		if !ok {
			return nil, ErrInvalid
		}
		retirement, err := paypal.RetirementAdapter()
		if err != nil {
			return nil, ErrInvalid
		}
		entries[id] = retirement
	}
	adapters, err := payments.NewDeleteAdapters(entries)
	if err != nil {
		return nil, ErrInvalid
	}
	return adapters, nil
}
