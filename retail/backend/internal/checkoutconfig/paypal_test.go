package checkoutconfig

import (
	"github.com/eurorefurb/retail/internal/payments"
	"strings"
	"testing"
)

func TestPayPalWalletExplicitConfiguration(t *testing.T) {
	for _, flag := range []string{"", "false"} {
		env := fixture()
		env["PAYPAL_WALLET_ENABLED"] = flag
		o, err := Load(func(k string) string {
			if strings.HasPrefix(k, "PAYPAL_") && k != "PAYPAL_WALLET_ENABLED" {
				t.Fatal("disabled wallet read credentials")
			}
			return env[k]
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, e := o.Adapters.Lookup("pp_paypal_paypal"); e == nil {
			t.Fatal("implicitly enabled")
		}
	}
	env := fixture()
	for k, v := range map[string]string{"PAYPAL_WALLET_ENABLED": "true", "PAYPAL_CLIENT_ID": "artificial_client", "PAYPAL_CLIENT_SECRET": "artificial_secret", "PAYPAL_MERCHANT_ID": "ABC1234567890", "PAYPAL_ENVIRONMENT": "sandbox", "PAYPAL_INTENT": "CAPTURE"} {
		env[k] = v
	}
	o, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	a, err := o.Adapters.Lookup("pp_paypal_paypal")
	if err != nil {
		t.Fatal(err)
	}
	sdk, err := a.(*payments.PayPalCreateAdapter).SDKConfiguration("eur")
	if err != nil || sdk.ClientID != "artificial_client" || sdk.Currency != "EUR" || sdk.Intent != "capture" || sdk.Environment != "sandbox" {
		t.Fatal("public SDK binding", err)
	}
	if _, err = a.(*payments.PayPalCreateAdapter).SDKConfiguration("invalid"); err == nil {
		t.Fatal("invalid currency")
	}
	for _, k := range []string{"PAYPAL_CLIENT_ID", "PAYPAL_CLIENT_SECRET", "PAYPAL_MERCHANT_ID", "PAYPAL_ENVIRONMENT", "PAYPAL_INTENT"} {
		saved := env[k]
		env[k] = ""
		if _, err := Load(func(k string) string { return env[k] }); err != ErrInvalid {
			t.Fatal("missing explicit wallet setting", k)
		}
		env[k] = saved
	}
	for _, flag := range []string{"1", "TRUE", " true"} {
		env["PAYPAL_WALLET_ENABLED"] = flag
		if _, err := Load(func(k string) string { return env[k] }); err != ErrInvalid {
			t.Fatal("ambiguous wallet enablement")
		}
	}
}
