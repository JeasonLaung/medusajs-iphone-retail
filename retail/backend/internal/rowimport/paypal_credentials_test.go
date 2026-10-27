package rowimport

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func paypalNestedFixture(t *testing.T) (*ConfigSecrets, *protected.Ring, SourceRow) {
	t.Helper()
	ring, _ := protected.New("nested-paypal", map[string][]byte{"nested-paypal": bytes.Repeat([]byte{67}, 32)})
	converter, _ := NewConfigSecrets(ring, nil)
	plans, _ := compiledPlans()
	row := SourceRow{}
	for _, field := range plans["paypal_connection"].Fields {
		if field.Nullable {
			row[field.Source] = Cell{SQLNull: true}
		} else {
			row[field.Source] = Cell{Raw: artificial(field)}
		}
	}
	for name, value := range map[string]string{"id": "merchant-nested", "environment": "live", "seller_client_id": "client-live", "seller_client_secret": "secret-live", "seller_merchant_id": "merchant-live", "seller_email": "merchant@example.invalid"} {
		relationText(row, name, value)
	}
	row["metadata"] = Cell{Raw: json.RawMessage(` {"active_environment":"live","credentials":{"live":{"client_id":"client-live","clientId":"client-live","client_secret":"secret-live","clientSecret":"secret-live","seller_merchant_id":"merchant-live","sellerMerchantId":"merchant-live","payer_id":"merchant-live","merchant_id":"merchant-live","merchantId":"merchant-live","seller_email":"merchant@example.invalid","sellerEmail":"merchant@example.invalid"},"sandbox":{"client_id":"client-sandbox","client_secret":"secret-sandbox","seller_email":null}}} `)}
	return converter, ring, row
}

func TestNestedPayPalAliasesAndPrivateProjection(t *testing.T) {
	converter, ring, row := paypalNestedFixture(t)
	original, _ := json.Marshal(row)
	bound, err := converter.Convert("paypal_connection", row)
	if err != nil {
		t.Fatal("known nested merchant aliases rejected")
	}
	var stored []byte
	for i, column := range bound.Columns {
		if column == "metadata" {
			stored = bound.Values[i].([]byte)
		}
	}
	context := protected.Context{Purpose: "merchant-config", Table: "paypal_connections", Column: "metadata", Row: "merchant-nested"}
	plain, err := ring.Open(context, stored)
	if err != nil || !bytes.Contains(plain, []byte("secret-live")) || !bytes.Contains(plain, []byte("secret-sandbox")) || bytes.Contains(stored, []byte("secret-live")) {
		t.Fatal("nested merchant facts were not privately projected")
	}
	var projected map[string]json.RawMessage
	if json.Unmarshal(plain, &projected) != nil {
		t.Fatal("nested projection not JSON")
	}
	context.Column = "seller_client_secret"
	if _, err := ring.Open(context, stored); err == nil {
		t.Fatal("metadata ciphertext usable as string secret")
	}
	after, _ := json.Marshal(row)
	if !bytes.Equal(original, after) {
		t.Fatal("source credential document changed")
	}
	// Inactive-only credentials must remain preserved without granting an
	// active connection or manufacturing active denormalized values.
	row["metadata"] = Cell{Raw: json.RawMessage(`{"active_environment":"live","credentials":{"sandbox":{"client_id":"sandbox","client_secret":"sandbox-secret"}}}`)}
	for _, column := range []string{"seller_client_id", "seller_client_secret", "seller_merchant_id", "seller_email"} {
		row[column] = Cell{SQLNull: true}
	}
	if _, err := converter.Convert("paypal_connection", row); err != nil {
		t.Fatal("inactive-only merchant credentials rejected")
	}
	relationText(row, "seller_client_id", "unexpected-active")
	if got, err := converter.Convert("paypal_connection", row); err != ErrProtection || !reflect.DeepEqual(got, BoundRow{}) {
		t.Fatal("inactive merchant silently became active")
	}
}

func TestNestedPayPalRejectsConflictsAndUnknownCapabilities(t *testing.T) {
	for _, raw := range []string{
		`{"active_environment":"live","credentials":{"live":{"client_id":"other"}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_id":"client-live","clientId":"different"}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_secret":"secret-live","clientSecret":"different"}}}`,
		`{"active_environment":"live","credentials":{"live":{"merchant_id":"one","seller_merchant_id":"two"}}}`,
		`{"active_environment":"live","credentials":{"live":{"seller_email":"a","sellerEmail":"b"}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_id":"client-live","password_hash":"old"}}}`,
		`{"active_environment":"live","credentials":{"sandbox":{"login_token":"old"}}}`,
		`{"active_environment":"live","credentials":{"future":{}}}`,
		`{"active_environment":"live","credentials":{"live":null}}`,
		`{"active_environment":"live","credentials":{"live":[]}}`,
		`{"active_environment":"live","credentials":{"live":{"client_secret":"enc:v3:unknown"}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_secret":"enc:v1:AA==:AA==:AA=="}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_id":1}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_secret":true}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_secret":"\ud800"}}}`,
		`{"active_environment":"live","credentials":{"live":{},"live":{}}}`,
		`{"active_environment":"live","credentials":{"live":{"client_id":"one","client_id":"two"}}}`,
		`{"credentials":{"live":{"client_id":"one"}}}`,
		`{"active_environment":"sandbox","credentials":{"live":{}}}`,
		`{"active_environment":"live","credentials":{"live":{}},"old_login_grant":true}`,
	} {
		converter, _, row := paypalNestedFixture(t)
		row["metadata"] = Cell{Raw: json.RawMessage(raw)}
		if got, err := converter.Convert("paypal_connection", row); err != ErrProtection || !reflect.DeepEqual(got, BoundRow{}) {
			t.Fatal("unknown/conflicting credentials returned partial row")
		}
	}
	for _, column := range []string{"seller_client_id", "seller_client_secret", "seller_merchant_id", "seller_email"} {
		converter, _, row := paypalNestedFixture(t)
		relationText(row, column, "different-mirror")
		if got, err := converter.Convert("paypal_connection", row); err != ErrProtection || !reflect.DeepEqual(got, BoundRow{}) {
			t.Fatal("active mirror mismatch silently repaired")
		}
	}
}
