package checkoutconfig

import (
	"strings"
	"testing"
)

func fixture() map[string]string {
	return map[string]string{"CHECKOUT_PAYMENT_ENABLED": "true", "STOREFRONT_ORIGIN": "http://127.0.0.1:9300", "MOLLIE_BACKEND_ORIGIN": "http://127.0.0.1:9100", "STOREFRONT_STORE_ID": "shop", "MOLLIE_API_KEY": "test_artificial", "PAYMENT_KEY_ACTIVE": "payment-1", "PAYMENT_KEYRING_JSON": `{"payment-1":"` + strings.Repeat("12", 32) + `"}`, "MANUAL_SHIPPING_PROVIDER_IDS": "manual_one,manual_two"}
}

func TestExplicitIndependentNotificationConfiguration(t *testing.T) {
	for _, flag := range []string{"", "false"} {
		options, err := Webhooks(func(k string) string {
			if k != "MOLLIE_NOTIFICATIONS_ENABLED" {
				t.Fatal("disabled receiver read unrelated settings")
			}
			return flag
		}, nil)
		if err != nil || len(options.ProviderIDs) != 0 {
			t.Fatal("disabled receiver enabled")
		}
	}
	e := fixture()
	payment, err := Load(func(k string) string { return e[k] })
	if err != nil || len(payment.ManualProviders) != 2 || payment.ManualProviders[0] != "manual_one" {
		t.Fatal("shared checkout/worker shipping config", err)
	}
	for _, flag := range []string{"true", "TRUE", "1", " true"} {
		e["MOLLIE_NOTIFICATIONS_ENABLED"] = flag
		opts, err := Webhooks(func(k string) string { return e[k] }, payment)
		if flag == "true" {
			if err != nil || len(opts.ProviderIDs) != 1 || opts.ProviderIDs[0] != "pp_mollie-hosted-checkout_mollie" {
				t.Fatal("hosted receiver not explicitly bound")
			}
		} else if err != ErrInvalid {
			t.Fatal("ambiguous enable flag accepted")
		}
	}
	e["MOLLIE_NOTIFICATIONS_ENABLED"] = "true"
	if _, err := Webhooks(func(k string) string { return e[k] }, nil); err != ErrInvalid {
		t.Fatal("unregistered checkout receiver accepted")
	}
	for _, raw := range []string{"manual_one,manual_one", " manual_one", "manual_one,", "../../other"} {
		e["MANUAL_SHIPPING_PROVIDER_IDS"] = raw
		if _, err := Load(func(k string) string { return e[k] }); err != ErrInvalid {
			t.Fatal("invalid manual provider allowlist accepted")
		}
	}
}
