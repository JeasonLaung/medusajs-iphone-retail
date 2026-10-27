package rowimport

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func TestPayPalConfigJSONReviewBoundaries(t *testing.T) {
	for _, entry := range []struct{ table, column, raw string }{
		{"paypal_connection", "metadata", ` {"credentials":{},"active_environment":"sandbox"} `},
		{"paypal_settings", "data", ` {"api_details":{"currency_code":"EUR","storefront_url":"https://example.invalid"},"onboarding_config":{"seller_nonce":" merchant nonce 中文 ","backend_url":"http://localhost:9001","bn_code":"","partner_merchant_id_live":"merchant","partner_merchant_id_sandbox":"","partner_js_url":"https://example.invalid","partner_service_url":"https://example.invalid"}} `},
		{"paypal_settings", "data", `null`}, {"paypal_connection", "metadata", `{}`},
	} {
		got, err := ReviewPayPalConfigJSON(entry.table, entry.column, json.RawMessage(entry.raw))
		if err != nil || string(got) != entry.raw {
			t.Fatal("reviewed merchant source JSON changed")
		}
	}
	for _, entry := range []struct{ table, column, raw string }{
		{"paypal_connection", "metadata", `{"credentials":{"live":{"client_secret":"not-yet-reviewed"}}}`},
		{"paypal_connection", "metadata", `{"credentials":null}`},
		{"paypal_connection", "metadata", `{"active_environment":"test"}`},
		{"paypal_connection", "metadata", `{"active_environment":"live","active_environment":"sandbox"}`},
		{"paypal_connection", "metadata", `{"token":"old-login"}`},
		{"paypal_settings", "data", `{"api_details":{"currency_code":"eur"}}`},
		{"paypal_settings", "data", `{"api_details":{"password":"old"}}`},
		{"paypal_settings", "data", `{"api_details":{"storefront_url":"javascript:alert(1)"}}`},
		{"paypal_settings", "data", `{"onboarding_config":{"seller_nonce":1}}`},
		{"paypal_settings", "data", `{"onboarding_config":{"seller_nonce":"\ud800"}}`},
		{"paypal_settings", "data", `{"onboarding_config":{"seller_nonce":"a","seller_nonce":"b"}}`},
		{"paypal_settings", "data", `{"onboarding_config":{"login_token":"old"}}`},
		{"paypal_settings", "data", `{"future_settings":{}}`},
		{"paypal_settings", "data", `[]`}, {"customer", "metadata", `{}`},
	} {
		if got, err := ReviewPayPalConfigJSON(entry.table, entry.column, json.RawMessage(entry.raw)); err != ErrProtection || got != nil {
			t.Fatal("unreviewed merchant extension accepted")
		}
	}
}

func TestPayPalJSONCompleteRowAndEnvironmentBinding(t *testing.T) {
	plans, _ := compiledPlans()
	ring, _ := protected.New("paypal-json", map[string][]byte{"paypal-json": bytes.Repeat([]byte{61}, 32)})
	converter, _ := NewConfigSecrets(ring, nil)
	for _, table := range []string{"paypal_connection", "paypal_settings"} {
		row := SourceRow{}
		for _, f := range plans[table].Fields {
			if f.Nullable {
				row[f.Source] = Cell{SQLNull: true}
			} else {
				row[f.Source] = Cell{Raw: artificial(f)}
			}
		}
		relationText(row, "id", "paypal-json-row")
		column := "data"
		if table == "paypal_connection" {
			column = "metadata"
			relationText(row, "environment", "live")
			row[column] = Cell{Raw: json.RawMessage(` {"credentials":{},"active_environment":"live"} `)}
		} else {
			row[column] = Cell{Raw: json.RawMessage(` {"api_details":{"currency_code":"EUR"},"onboarding_config":{"seller_nonce":"private merchant setup capability"}} `)}
		}
		bound, err := converter.Convert(table, row)
		if err != nil || len(bound.Values) != len(plans[table].Fields) {
			t.Fatal("complete PayPal config row rejected")
		}
		for i, name := range bound.Columns {
			if name != column {
				continue
			}
			sealed, ok := bound.Values[i].([]byte)
			if !ok || bytes.Contains(sealed, []byte("merchant setup")) {
				t.Fatal("private merchant JSON not sealed as binary")
			}
			context := protected.Context{Purpose: "merchant-config", Table: bound.Table, Column: column, Row: "paypal-json-row"}
			plain, err := ring.Open(context, sealed)
			if err != nil || !bytes.Equal(plain, row[column].Raw) {
				t.Fatal("merchant JSON changed under new envelope")
			}
			context.Row = "other-row"
			if _, err := ring.Open(context, sealed); err == nil {
				t.Fatal("merchant JSON wrong row accepted")
			}
		}
		if table == "paypal_connection" {
			relationText(row, "environment", "sandbox")
			if got, err := converter.Convert(table, row); err != ErrProtection || !reflect.DeepEqual(got, BoundRow{}) {
				t.Fatal("mismatched active merchant environment accepted")
			}
		}
	}
}

func TestPinnedPayPalConfigV2Offline(t *testing.T) {
	path := os.Getenv("ROWIMPORT_PAYPAL_CONFIG_V2")
	if path == "" {
		t.Skip("explicit ignored v2 snapshot required")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("private PayPal snapshot missing")
	}
	defer file.Close()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("temporary merchant key unavailable")
	}
	defer clear(key)
	ring, _ := protected.New("offline-paypal", map[string][]byte{"offline-paypal": key})
	converter, _ := NewConfigSecrets(ring, nil)
	plans, _ := compiledPlans()
	counts := map[string]int{}
	snapshot, err := VerifyStream(file, "b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985", func(table string, row SourceRow) error {
		if table != "paypal_connection" && table != "paypal_settings" {
			return nil
		}
		bound, err := converter.Convert(table, row)
		if err != nil {
			return ErrRow
		}
		identity, ok := paymentString(row["id"].Raw)
		if !ok {
			return ErrRow
		}
		for i, f := range plans[table].Fields {
			if f.Transform != "protected_string" && f.Transform != "protected_exact_json" {
				continue
			}
			if row[f.Source].SQLNull {
				if bound.Values[i] != nil {
					return ErrRow
				}
				counts["explicit_sql_null_secret_fields"]++
				continue
			}
			// Pinned source has no actual encrypted/plaintext credential values.
			if f.Transform != "protected_exact_json" {
				return ErrRow
			}
			sealed, ok := bound.Values[i].([]byte)
			if !ok {
				return ErrRow
			}
			context := protected.Context{Purpose: "merchant-config", Table: bound.Table, Column: f.Target, Row: identity}
			plain, err := ring.Open(context, sealed)
			if err != nil || !bytes.Equal(plain, row[f.Source].Raw) {
				return ErrRow
			}
			clear(plain)
			context.Row += "-wrong"
			if _, err := ring.Open(context, sealed); err == nil {
				return ErrRow
			}
			counts["protected_json_roundtrips"]++
			counts["wrong_row_rejections"]++
		}
		counts[table+".complete_rows"]++
		return nil
	})
	if err != nil || len(snapshot.Tables) != 159 || counts["paypal_connection.complete_rows"] != 1 || counts["paypal_settings.complete_rows"] != 1 || counts["protected_json_roundtrips"] != 2 || counts["explicit_sql_null_secret_fields"] != 3 {
		t.Fatal("PayPal private v2 conversion incomplete")
	}
	report, _ := json.Marshal(counts)
	t.Log("PAYPAL_CONFIG_V2_AUDIT " + string(report))
}
