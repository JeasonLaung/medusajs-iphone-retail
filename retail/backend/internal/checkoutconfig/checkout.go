package checkoutconfig

import (
	"crypto/x509"
	"errors"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/refunds"
)

// Parsing configuration never contacts the provider or changes SQL gates.
// All errors are fixed strings: invalid credentials must not enter logs.
var ErrInvalid = errors.New("Explicit checkout configuration invalid")

func checkoutOrigin(raw string, production bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.String() != raw || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	return u.Scheme == "https" || !production && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
func Load(env func(string) string) (*cart.InitialPaymentOptions, error) {
	switch env("CHECKOUT_PAYMENT_ENABLED") {
	case "", "false":
		return nil, nil
	case "true":
	default:
		return nil, ErrInvalid
	}
	origin, backend := env("STOREFRONT_ORIGIN"), env("MOLLIE_BACKEND_ORIGIN")
	production := env("APP_ENV") == "production"
	if !checkoutOrigin(origin, production) || !checkoutOrigin(backend, production) || env("STOREFRONT_STORE_ID") == "" {
		return nil, ErrInvalid
	}
	ring, err := protected.ParseKeyring(env("PAYMENT_KEY_ACTIVE"), env("PAYMENT_KEYRING_JSON"))
	if err != nil {
		return nil, ErrInvalid
	}
	const provider = "pp_mollie-hosted-checkout_mollie"
	var roots *x509.CertPool
	if path := env("MOLLIE_CA_FILE"); path != "" {
		file, e := os.Open(path)
		if e != nil {
			return nil, ErrInvalid
		}
		pem, e := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		_ = file.Close()
		if e != nil || len(pem) > 1<<20 {
			return nil, ErrInvalid
		}
		roots, e = x509.SystemCertPool()
		if e != nil {
			return nil, ErrInvalid
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, ErrInvalid
		}
	}
	localTest := false
	switch env("MOLLIE_LOCAL_TEST_WITHOUT_WEBHOOK") {
	case "", "false":
	case "true":
		if env("APP_ENV") != "development" {
			return nil, ErrInvalid
		}
		localTest = true
	default:
		return nil, ErrInvalid
	}
	adapter, err := payments.NewMollieCreateAdapter(payments.MollieCreateConfig{ProviderID: provider, APIKey: env("MOLLIE_API_KEY"), RedirectURL: origin + "/api/mollie-return", BackendURL: backend, RootCAs: roots, LocalTestWithoutWebhook: localTest})
	if err != nil {
		return nil, ErrInvalid
	}
	entries := map[string]payments.CreateSessionAdapter{provider: adapter}
	switch env("PAYPAL_WALLET_ENABLED") {
	case "", "false":
	case "true":
		paypal, e := payments.NewPayPalCreateAdapter(payments.PayPalCreateConfig{ProviderID: "pp_paypal_paypal", ClientID: env("PAYPAL_CLIENT_ID"), ClientSecret: env("PAYPAL_CLIENT_SECRET"), MerchantID: env("PAYPAL_MERCHANT_ID"), Environment: env("PAYPAL_ENVIRONMENT"), Intent: env("PAYPAL_INTENT"), ReturnURL: origin + "/api/paypal-return", CancelURL: origin + "/api/paypal-return"})
		if e != nil {
			return nil, ErrInvalid
		}
		entries["pp_paypal_paypal"] = paypal
	default:
		return nil, ErrInvalid
	}
	adapters, err := payments.NewCreateAdapters(entries)
	if err != nil {
		return nil, ErrInvalid
	}
	providers := []string{}
	if raw := env("MANUAL_SHIPPING_PROVIDER_IDS"); raw != "" {
		providers = strings.Split(raw, ",")
		seen := map[string]bool{}
		for _, id := range providers {
			if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`).MatchString(id) || seen[id] || len(providers) > 100 {
				return nil, ErrInvalid
			}
			seen[id] = true
		}
	}
	return &cart.InitialPaymentOptions{Ring: ring, Adapters: adapters, ReturnOrigin: origin, ManualProviders: providers}, nil
}

// Webhooks remain independently disabled by default. Only the hosted provider
// registered by Load is enabled; database rows never register extra adapters.
func Webhooks(env func(string) string, payment *cart.InitialPaymentOptions) (payments.MollieWebhookOptions, error) {
	switch env("MOLLIE_NOTIFICATIONS_ENABLED") {
	case "", "false":
		return payments.MollieWebhookOptions{}, nil
	case "true":
		if payment == nil || payment.Adapters == nil || payment.Ring == nil {
			return payments.MollieWebhookOptions{}, ErrInvalid
		}
		const provider = "pp_mollie-hosted-checkout_mollie"
		if _, err := payment.Adapters.Observer(provider); err != nil {
			return payments.MollieWebhookOptions{}, ErrInvalid
		}
		return payments.MollieWebhookOptions{ProviderIDs: []string{provider}}, nil
	default:
		return payments.MollieWebhookOptions{}, ErrInvalid
	}
}

// Refunds is separately opt-in; checkout enablement never authorizes refunds.
func Refunds(env func(string) string, payment *cart.InitialPaymentOptions) (refunds.Options, error) {
	out := refunds.Options{StoreID: env("STOREFRONT_STORE_ID")}
	switch env("ORDER_REFUNDS_ENABLED") {
	case "", "false":
		return out, nil
	case "true":
		if payment == nil || payment.Ring == nil || payment.Adapters == nil || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`).MatchString(out.StoreID) {
			return refunds.Options{}, ErrInvalid
		}
		if _, e := payment.Adapters.Refunder("pp_mollie-hosted-checkout_mollie"); e != nil {
			return refunds.Options{}, ErrInvalid
		}
		out.Enabled = true
		out.Ring = payment.Ring
		out.Adapters = payment.Adapters
		return out, nil
	default:
		return refunds.Options{}, ErrInvalid
	}
}
