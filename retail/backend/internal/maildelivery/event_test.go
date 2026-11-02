package maildelivery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomerEventTemplates(t *testing.T) {
	sender, e := NewResend("re_artificial_event_key", "notify@example.invalid", "")
	if e != nil {
		t.Fatal(e)
	}
	events := []CustomerEvent{}
	for _, kind := range []string{"payment.received", "refund.created", "shipment.created", "delivery.created", "order.canceled", "after_sales.processing", "after_sales.completed", "after_sales.rejected"} {
		events = append(events, CustomerEvent{Kind: kind})
	}
	for _, action := range []string{"review_photo", "approve_shipping", "mark_received", "final_offer", "request_return", "mark_returning", "mark_returned", "mark_paid", "cancel"} {
		events = append(events, CustomerEvent{Kind: "buyback.updated", Action: action})
	}
	for _, event := range events {
		for _, locale := range []string{"en", "zh"} {
			event.To = "customer@example.invalid"
			event.Reference = "REF-001"
			event.Locale = locale
			body, e := sender.PrepareEvent(event)
			if e != nil {
				t.Fatalf("template %s %s %s", event.Kind, event.Action, locale)
			}
			var out request
			if json.Unmarshal(body, &out) != nil || out.To[0] != event.To || !strings.Contains(out.Text, event.Reference) || out.HTML == "" || out.Subject == "" {
				t.Fatal("message projection")
			}
		}
	}
	invalid := []CustomerEvent{
		{Kind: "buyback.updated", Action: "link_device"}, {Kind: "buyback.updated", Action: "approved"},
		{Kind: "after_sales.processing", Action: "final_offer"}, {Kind: "unknown"},
	}
	for _, event := range invalid {
		event.To = "customer@example.invalid"
		event.Reference = "REF-001"
		event.Locale = "en"
		if _, e := sender.PrepareEvent(event); e == nil {
			t.Fatal("unsupported event accepted")
		}
	}
	for _, recipient := range []string{"Other <customer@example.invalid>", "customer@example.invalid\r\nBcc:x@example.invalid"} {
		if _, e := sender.PrepareEvent(CustomerEvent{To: recipient, Reference: "REF-001", Locale: "en", Kind: "shipment.created"}); e == nil {
			t.Fatal("recipient injection accepted")
		}
	}
}
