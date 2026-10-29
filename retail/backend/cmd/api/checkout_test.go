package main

import (
	"strings"
	"testing"
)

func checkoutFixture() map[string]string {
	return map[string]string{"CHECKOUT_PAYMENT_ENABLED": "true", "STOREFRONT_ORIGIN": "http://127.0.0.1:9300", "MOLLIE_BACKEND_ORIGIN": "http://127.0.0.1:9100", "STOREFRONT_STORE_ID": "shop", "MOLLIE_API_KEY": "test_artificial", "PAYMENT_KEY_ACTIVE": "payment-1", "PAYMENT_KEYRING_JSON": `{"payment-1":"` + strings.Repeat("12", 32) + `"}`}
}
func TestCheckoutConfiguration(t *testing.T) {
	for _, enabled := range []string{"", "false"} {
		o, err := checkoutOptions(func(k string) string {
			if k == "CHECKOUT_PAYMENT_ENABLED" {
				return enabled
			}
			t.Fatal("disabled checkout read credentials")
			return ""
		})
		if err != nil || o != nil {
			t.Fatal("disabled checkout registered provider")
		}
	}
	for _, production := range []bool{false, true} {
		e := checkoutFixture()
		if production {
			e["APP_ENV"] = "production"
			e["STOREFRONT_ORIGIN"] = "https://shop.example"
			e["MOLLIE_BACKEND_ORIGIN"] = "https://api.example"
		}
		o, err := checkoutOptions(func(k string) string { return e[k] })
		if err != nil || o == nil || o.Ring == nil || o.ReturnOrigin != e["STOREFRONT_ORIGIN"] {
			t.Fatal("valid explicit checkout unavailable")
		}
		if _, err = o.Adapters.Lookup("pp_mollie-hosted-checkout_mollie"); err != nil {
			t.Fatal("hosted adapter missing")
		}
		if _, err = o.Adapters.Lookup("pp_paypal_paypal"); err == nil {
			t.Fatal("unconfigured provider registered")
		}
	}
}
func TestCheckoutConfigurationRejectsUnsafeOriginsAndMissingSecrets(t *testing.T) {
	for _, key := range []string{"STOREFRONT_STORE_ID", "MOLLIE_API_KEY", "PAYMENT_KEY_ACTIVE", "PAYMENT_KEYRING_JSON", "MOLLIE_BACKEND_ORIGIN", "STOREFRONT_ORIGIN"} {
		e := checkoutFixture()
		delete(e, key)
		o, err := checkoutOptions(func(k string) string { return e[k] })
		if err != errCheckoutConfiguration || o != nil {
			t.Fatalf("missing %s accepted", key)
		}
	}
	for _, key := range []string{"STOREFRONT_ORIGIN", "MOLLIE_BACKEND_ORIGIN"} {
		for _, raw := range []string{"http://public.example", "https://user:secret@shop.example", "https://shop.example/path", "https://shop.example?token=secret", "https://shop.example#fragment", "https://shop.example/", "file:///private", "https://shop.example?"} {
			e := checkoutFixture()
			e[key] = raw
			o, err := checkoutOptions(func(k string) string { return e[k] })
			if err != errCheckoutConfiguration || o != nil {
				t.Fatal("unsafe checkout origin accepted")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("credentials escaped error")
			}
		}
	}
	e := checkoutFixture()
	e["APP_ENV"] = "production"
	if _, err := checkoutOptions(func(k string) string { return e[k] }); err != errCheckoutConfiguration {
		t.Fatal("production cleartext checkout accepted")
	}
	for _, raw := range []string{"TRUE", "1", " true"} {
		e := checkoutFixture()
		e["CHECKOUT_PAYMENT_ENABLED"] = raw
		if _, err := checkoutOptions(func(k string) string { return e[k] }); err != errCheckoutConfiguration {
			t.Fatal("invalid switch accepted")
		}
	}
}
