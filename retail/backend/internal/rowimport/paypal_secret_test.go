package rowimport

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/eurorefurb/retail/internal/legacysecret"
	"github.com/eurorefurb/retail/internal/protected"
)

func TestPayPalActualNodeRowConversion(t *testing.T) {
	path := os.Getenv("PAYPAL_NODE_FIXTURE")
	if path == "" {
		t.Skip("explicit artificial Node fixture required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("Node fixture unavailable")
	}
	var fixtures []struct{ Key, Plaintext, Stored string }
	if json.Unmarshal(data, &fixtures) != nil || len(fixtures) != 9 {
		t.Fatal("Node fixture invalid")
	}
	plans, _ := compiledPlans()
	ring, _ := protected.New("paypal-target", map[string][]byte{"paypal-target": bytes.Repeat([]byte{43}, 32)})
	for _, fixture := range fixtures {
		source := SourceRow{}
		for _, field := range plans["paypal_connection"].Fields {
			if field.Nullable {
				source[field.Source] = Cell{SQLNull: true}
			} else {
				source[field.Source] = Cell{Raw: artificial(field)}
			}
		}
		relationText(source, "id", "paypal-merchant")
		relationText(source, "environment", "live")
		relationText(source, "seller_client_id", "live-client")
		nested, _ := json.Marshal(map[string]any{"active_environment": "live", "credentials": map[string]any{
			"live":    map[string]any{"client_id": "live-client", "client_secret": fixture.Stored},
			"sandbox": map[string]any{"clientId": "sandbox-client", "clientSecret": fixture.Stored},
		}})
		source["metadata"] = Cell{Raw: nested}
		for _, field := range []string{"seller_client_secret", "app_access_token"} {
			relationText(source, field, fixture.Stored)
		}
		reader, _ := legacysecret.NewPayPal(fixture.Key)
		converter, _ := NewConfigSecretsWithPayPal(ring, nil, reader)
		bound, err := converter.Convert("paypal_connection", source)
		if err != nil {
			t.Fatal("actual PayPal row conversion failed")
		}
		for i, column := range bound.Columns {
			if column != "seller_client_secret" && column != "app_access_token" && column != "metadata" {
				continue
			}
			plain, err := ring.Open(protected.Context{Purpose: "merchant-config", Table: "paypal_connections", Column: column, Row: "paypal-merchant"}, bound.Values[i].([]byte))
			if err != nil {
				t.Fatal("target PayPal field could not be authenticated")
			}
			if column == "metadata" {
				var projected struct {
					Credentials map[string]map[string]string `json:"credentials"`
				}
				if json.Unmarshal(plain, &projected) != nil || projected.Credentials["live"]["client_secret"] != fixture.Plaintext || projected.Credentials["sandbox"]["clientSecret"] != fixture.Plaintext {
					t.Fatal("nested merchant ciphertext was rewrapped instead of decrypted")
				}
				clear(plain)
				continue
			}
			if string(plain) != fixture.Plaintext {
				t.Fatal("target contains old PayPal ciphertext instead of usable secret")
			}
		}
		if len(fixture.Stored) > 4 && fixture.Stored[:4] == "enc:" {
			missing, _ := NewConfigSecrets(ring, nil)
			if got, err := missing.Convert("paypal_connection", source); err != ErrProtection || len(got.Values) != 0 {
				t.Fatal("missing PayPal key rewrapped old ciphertext")
			}
		}
	}
}
