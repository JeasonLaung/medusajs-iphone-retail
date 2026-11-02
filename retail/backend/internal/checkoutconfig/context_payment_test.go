package checkoutconfig

import "testing"

func TestContextCancellationRequiresExplicitConfiguration(t *testing.T) {
	for _, flag := range []string{"", "false"} {
		out, err := ContextPayments(func(key string) string {
			if key != "CHECKOUT_CONTEXT_PAYMENT_ENABLED" {
				t.Fatal("disabled context read credentials")
			}
			return flag
		}, nil)
		if err != nil || out != nil {
			t.Fatal("implicit cancellation")
		}
	}
	env := fixture()
	get := func(key string) string { return env[key] }
	initial, err := Load(get)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"TRUE", "1", " true", "false "} {
		env["CHECKOUT_CONTEXT_PAYMENT_ENABLED"] = flag
		if _, err := ContextPayments(get, initial); err != ErrInvalid {
			t.Fatal("ambiguous cancellation enablement")
		}
	}
	env["CHECKOUT_CONTEXT_PAYMENT_ENABLED"] = "true"
	if _, err := ContextPayments(get, nil); err != ErrInvalid {
		t.Fatal("cancellation without checkout")
	}
	// Credentials are already authenticated in the configured creator. Do not
	// re-read a second merchant or discard its configured TLS roots.
	delete(env, "MOLLIE_API_KEY")
	out, err := ContextPayments(get, initial)
	if err != nil || out == nil {
		t.Fatal("configured cancellation unavailable", err)
	}
	if _, err := out.Lookup("pp_mollie-hosted-checkout_mollie"); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Lookup("foreign"); err == nil {
		t.Fatal("foreign provider registered")
	}
}

func TestContextRetirementUsesOnlyExplicitPayPalCreators(t *testing.T) {
	for _, enabled := range []string{"PAYPAL_WALLET_ENABLED", "PAYPAL_CARD_ENABLED"} {
		env := fixture()
		for k, v := range map[string]string{enabled: "true", "CHECKOUT_CONTEXT_PAYMENT_ENABLED": "true", "PAYPAL_CLIENT_ID": "artificial_client", "PAYPAL_CLIENT_SECRET": "artificial_secret", "PAYPAL_MERCHANT_ID": "ABC1234567890", "PAYPAL_ENVIRONMENT": "sandbox", "PAYPAL_INTENT": "CAPTURE"} {
			env[k] = v
		}
		get := func(k string) string { return env[k] }
		initial, err := Load(get)
		if err != nil {
			t.Fatal(err)
		}
		// Configuration reuses the creator and does not read another credential.
		out, err := ContextPayments(func(k string) string {
			if k != "CHECKOUT_CONTEXT_PAYMENT_ENABLED" {
				t.Fatal("retirement reread credentials")
			}
			return env[k]
		}, initial)
		if err != nil {
			t.Fatal(err)
		}
		for flag, id := range map[string]string{"PAYPAL_WALLET_ENABLED": "pp_paypal_paypal", "PAYPAL_CARD_ENABLED": "pp_paypal_card_paypal_card"} {
			_, err := out.Lookup(id)
			if (err == nil) != (flag == enabled) {
				t.Fatal("implicit/missing retirement", flag, err)
			}
		}
	}
}
