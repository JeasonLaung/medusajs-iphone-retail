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
	adapters, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{provider: adapter})
	if err != nil {
		return nil, ErrInvalid
	}
	return adapters, nil
}
