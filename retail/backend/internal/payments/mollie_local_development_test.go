package payments

import (
	"encoding/json"
	"testing"
)

func TestMollieLocalTestWithoutWebhook(t *testing.T) {
	config := mollieCreateConfig()
	config.LocalTestWithoutWebhook = true
	config.BackendURL = "http://127.0.0.1:9100"
	config.RedirectURL = "http://127.0.0.1:9300/api/mollie-return?cart_id=cart_owned"
	adapter, err := NewMollieCreateAdapter(config)
	if err != nil {
		t.Fatal(err)
	}
	body, _, _, err := adapter.createBody(mollieCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if json.Unmarshal(body, &fields) != nil {
		t.Fatal("invalid body")
	}
	if _, ok := fields["webhookUrl"]; ok {
		t.Fatal("loopback webhook sent to remote provider")
	}
	for _, bad := range []MollieCreateConfig{
		{ProviderID: config.ProviderID, APIKey: "live_artificial", RedirectURL: config.RedirectURL, BackendURL: config.BackendURL, LocalTestWithoutWebhook: true},
		{ProviderID: config.ProviderID, APIKey: config.APIKey, RedirectURL: "https://shop.example/api/mollie-return", BackendURL: config.BackendURL, LocalTestWithoutWebhook: true},
		{ProviderID: config.ProviderID, APIKey: config.APIKey, RedirectURL: config.RedirectURL, BackendURL: "https://api.example", LocalTestWithoutWebhook: true},
	} {
		if _, err := NewMollieCreateAdapter(bad); err == nil {
			t.Fatal("nonlocal/live webhook omission allowed")
		}
	}
}
