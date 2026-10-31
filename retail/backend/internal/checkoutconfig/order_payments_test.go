package checkoutconfig

import "testing"

func TestOrderPaymentWorkerRequiresIndependentConsentAndIntake(t *testing.T) {
	for _, flag := range []string{"", "false"} {
		opts, e := OrderPayments(func(k string) string {
			if k != "ORDER_PAYMENTS_ENABLED" {
				t.Fatal("disabled worker read unrelated config")
			}
			return flag
		}, nil)
		if e != nil || opts != nil {
			t.Fatal("checkout implicitly enabled supplemental payments")
		}
	}
	settings := fixture()
	env := func(k string) string { return settings[k] }
	payment, e := Load(env)
	if e != nil {
		t.Fatal(e)
	}
	for _, flag := range []string{"TRUE", "1", " true", "false "} {
		settings["ORDER_PAYMENTS_ENABLED"] = flag
		if _, e := OrderPayments(env, payment); e != ErrInvalid {
			t.Fatal("ambiguous worker consent")
		}
	}
	settings["ORDER_PAYMENTS_ENABLED"] = "true"
	if _, e := OrderPayments(env, nil); e != ErrInvalid {
		t.Fatal("worker without protected request/merchant config")
	}
	if _, e := OrderPayments(env, payment); e != ErrInvalid {
		t.Fatal("worker without notification configuration")
	}
	settings["MOLLIE_NOTIFICATIONS_ENABLED"] = "true"
	opts, e := OrderPayments(env, payment)
	if e != nil || opts == nil || opts.DB != nil || opts.StoreID != "shop" || opts.Ring != payment.Ring || opts.Adapters != payment.Adapters || opts.ReturnOrigin != payment.ReturnOrigin {
		t.Fatal("worker changed trusted checkout config", e)
	}
	settings["STOREFRONT_STORE_ID"] = "../other"
	if _, e := OrderPayments(env, payment); e != ErrInvalid {
		t.Fatal("invalid worker store")
	}
}
