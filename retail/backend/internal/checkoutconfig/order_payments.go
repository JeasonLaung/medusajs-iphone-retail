package checkoutconfig

import (
	"regexp"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/payments"
)

// OrderPayments remains independently opt-in. Parsing never changes SQL gates,
// starts a worker or contacts a provider. The caller attaches the dedicated DB.
func OrderPayments(env func(string) string, payment *cart.InitialPaymentOptions) (*orderpayments.Service, error) {
	switch env("ORDER_PAYMENTS_ENABLED") {
	case "", "false":
		return nil, nil
	case "true":
	default:
		return nil, ErrInvalid
	}
	store := env("STOREFRONT_STORE_ID")
	if payment == nil || payment.Ring == nil || payment.Adapters == nil || !checkoutOrigin(payment.ReturnOrigin, env("APP_ENV") == "production") || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`).MatchString(store) {
		return nil, ErrInvalid
	}
	webhooks, e := Webhooks(env, payment)
	if e != nil || len(webhooks.ProviderIDs) != 1 {
		return nil, ErrInvalid
	}
	const provider = "pp_mollie-hosted-checkout_mollie"
	if _, e := payment.Adapters.Observer(provider); e != nil {
		return nil, ErrInvalid
	}
	a, e := payment.Adapters.Lookup(provider)
	if e != nil {
		return nil, ErrInvalid
	}
	if _, ok := a.(payments.SessionDiscoverer); !ok {
		return nil, ErrInvalid
	}
	return &orderpayments.Service{StoreID: store, ReturnOrigin: payment.ReturnOrigin, Ring: payment.Ring, Adapters: payment.Adapters}, nil
}
