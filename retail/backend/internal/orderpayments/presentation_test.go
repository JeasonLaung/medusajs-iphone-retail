package orderpayments

import "testing"

func TestHostedCheckoutLinkBoundary(t *testing.T) {
	for _, url := range []string{"https://www.mollie.com/checkout/owned", "https://mollie.com:443/checkout/owned?lang=en"} {
		if _, e := checkoutLink(url); e != nil {
			t.Fatal(e)
		}
	}
	for _, url := range []string{"", "http://mollie.com/checkout", "https://mollie.com.evil.test/checkout", "https://evil-mollie.com/checkout", "https://user@mollie.com/checkout", "https://mollie.com:444/checkout", "https://mollie.com/checkout#private", "//mollie.com/checkout", "javascript:alert(1)", "https://mollie.com/\ncheckout"} {
		if _, e := checkoutLink(url); e == nil {
			t.Fatal("unsafe checkout URL accepted", url)
		}
	}
}
