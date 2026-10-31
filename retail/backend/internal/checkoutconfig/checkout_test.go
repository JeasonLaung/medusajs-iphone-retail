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

func TestRefundsRequireIndependentExplicitEnablement(t *testing.T) {
	e := fixture()
	env := func(k string) string { return e[k] }
	payment, err := Load(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"", "false"} {
		e["ORDER_REFUNDS_ENABLED"] = flag
		out, err := Refunds(env, payment)
		if err != nil || out.Enabled || out.Ring != nil || out.Adapters != nil {
			t.Fatal("checkout enabled refunds implicitly")
		}
	}
	for _, flag := range []string{"TRUE", "1", " true", "false "} {
		e["ORDER_REFUNDS_ENABLED"] = flag
		if _, err := Refunds(env, payment); err != ErrInvalid {
			t.Fatal("ambiguous refund flag")
		}
	}
	e["ORDER_REFUNDS_ENABLED"] = "true"
	if _, err := Refunds(env, nil); err != ErrInvalid {
		t.Fatal("refunds without protected configured provider")
	}
	out, err := Refunds(env, payment)
	if err != nil || !out.Enabled || out.StoreID != "shop" || out.Ring != payment.Ring || out.Adapters != payment.Adapters {
		t.Fatal("refunds not bound to original configuration")
	}
	e["STOREFRONT_STORE_ID"] = "../other"
	if _, err := Refunds(env, payment); err != ErrInvalid {
		t.Fatal("invalid refund store accepted")
	}
}
