package main

import (
	"errors"
	"net/url"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
)

// Parsing configuration never contacts the provider or changes SQL gates.
// All errors are fixed strings: invalid credentials must not enter logs.
var errCheckoutConfiguration = errors.New("Explicit checkout configuration invalid")

func checkoutOrigin(raw string, production bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.String() != raw || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	return u.Scheme == "https" || !production && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
func checkoutOptions(env func(string) string) (*cart.InitialPaymentOptions, error) {
	switch env("CHECKOUT_PAYMENT_ENABLED") {
	case "", "false":
		return nil, nil
	case "true":
	default:
		return nil, errCheckoutConfiguration
	}
	origin, backend := env("STOREFRONT_ORIGIN"), env("MOLLIE_BACKEND_ORIGIN")
	production := env("APP_ENV") == "production"
	if !checkoutOrigin(origin, production) || !checkoutOrigin(backend, production) || env("STOREFRONT_STORE_ID") == "" {
		return nil, errCheckoutConfiguration
	}
	ring, err := protected.ParseKeyring(env("PAYMENT_KEY_ACTIVE"), env("PAYMENT_KEYRING_JSON"))
	if err != nil {
		return nil, errCheckoutConfiguration
	}
	const provider = "pp_mollie-hosted-checkout_mollie"
	adapter, err := payments.NewMollieCreateAdapter(payments.MollieCreateConfig{ProviderID: provider, APIKey: env("MOLLIE_API_KEY"), RedirectURL: origin + "/api/mollie-return", BackendURL: backend})
	if err != nil {
		return nil, errCheckoutConfiguration
	}
	adapters, err := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{provider: adapter})
	if err != nil {
		return nil, errCheckoutConfiguration
	}
	return &cart.InitialPaymentOptions{Ring: ring, Adapters: adapters, ReturnOrigin: origin}, nil
}
