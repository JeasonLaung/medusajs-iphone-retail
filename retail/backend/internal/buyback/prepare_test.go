package buyback

import (
	"encoding/json"
	"testing"
)

func TestReferenceSumPreservesOriginalFinalRounding(t *testing.T) {
	for _, c := range []struct {
		values []string
		want   string
	}{{[]string{"1.005"}, "1"}, {[]string{"1.005", "1.005"}, "2.01"}, {[]string{"0.1", "0.2"}, "0.3"}, {[]string{"294", "20"}, "314"}, {[]string{"10.1234567890123456789"}, "10.12"}} {
		items := []RequestItem{}
		for _, s := range c.values {
			n := json.Number(s)
			items = append(items, RequestItem{ReferenceAmount: &n})
		}
		got, e := sumReference(items)
		if e != nil || got == nil || string(*got) != c.want {
			t.Fatal("reference sum changed original final rounding")
		}
	}
	for _, s := range []string{"NaN", "Infinity", "-1", "bad", "1e999"} {
		n := json.Number(s)
		if _, e := sumReference([]RequestItem{{ReferenceAmount: &n}}); e == nil {
			t.Fatal("invalid reference accepted")
		}
	}
	n := json.Number("1")
	if got, e := sumReference([]RequestItem{{ReferenceAmount: &n}, {}}); e != nil || got != nil {
		t.Fatal("missing amount became zero")
	}
}
func TestOriginalIPhoneTitleEligibility(t *testing.T) {
	for _, s := range []string{"iPhone", "IPHONE 13", "iPhone-SE", "iPhone中文"} {
		if !iphoneTitle(s) {
			t.Fatal("eligible title rejected")
		}
	}
	for _, s := range []string{" iPhone 13", "iPhones", "iPhone13", "iPhone_13", "Phone", ""} {
		if iphoneTitle(s) {
			t.Fatal("ineligible title accepted")
		}
	}
}
