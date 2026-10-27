package rowimport

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func paymentHistoryFixture(t *testing.T, table string) SourceRow {
	t.Helper()
	plans, _ := compiledPlans()
	plan := plans[table]
	row := SourceRow{}
	for _, f := range plan.Fields {
		if f.Nullable {
			row[f.Source] = Cell{SQLNull: true}
		} else {
			row[f.Source] = Cell{Raw: artificial(f)}
		}
	}
	satisfyFixtureChecks(plan, row)
	relationText(row, "id", "session-1")
	relationText(row, "provider_id", "pp_mollie-hosted-checkout_mollie")
	relationText(row, "currency_code", "eur")
	if table == "payment" {
		relationText(row, "id", "payment-1")
		relationText(row, "payment_session_id", "session-1")
	}
	row["amount"] = Cell{Raw: json.RawMessage(`"9007199254740993.123456789012345678"`)}
	row["raw_amount"] = Cell{Raw: json.RawMessage(`{"value":"9007199254740993.123456789012345678","precision":38}`)}
	_, raw := paymentDataFixture()
	row["data"] = Cell{Raw: raw}
	row["metadata"] = Cell{Raw: json.RawMessage(` { } `)}
	if table == "payment_session" {
		row["context"] = Cell{Raw: json.RawMessage(`{"customer":{"id":"legacy-customer","addresses":[{"token":"old-login"}],"account_holders":[],"billing_address":{"address_1":"历史地址","is_default_billing":true}}}`)}
	}
	return row
}

func TestPaymentHistoryCompleteRowsAndMandatoryHold(t *testing.T) {
	ring, _ := protected.New("payment-test", map[string][]byte{"payment-test": bytes.Repeat([]byte{31}, 32)})
	converter, _ := NewPaymentHistory(ring)
	for _, table := range []string{"payment_session", "payment"} {
		source := paymentHistoryFixture(t, table)
		original, _ := json.Marshal(source)
		result, err := converter.Convert(table, source)
		if err != nil || len(result.Row.Columns) != 14 || len(result.Row.Values) != 14 {
			t.Fatal("complete payment conversion failed")
		}
		identity, _ := paymentRowIdentity(table, source)
		if result.Operations != (EffectDisposition{SourceTable: table, SourceID: identity.SourceID, Effect: "payment-operations", Disposition: "hold", Reason: "legacy-payment-reconciliation"}) {
			t.Fatal("mandatory payment operations hold absent")
		}
		for index, column := range result.Row.Columns {
			value := result.Row.Values[index]
			switch column {
			case "data":
				opened, err := OpenPaymentData(ring, identity, value.(string))
				if err != nil || !bytes.Equal(opened, source["data"].Raw) {
					t.Fatal("transaction evidence differs")
				}
			case "context":
				opened, err := OpenPaymentContext(ring, identity.SourceID, value.(string))
				if err != nil || bytes.Contains(opened, []byte("old-login")) || !bytes.Contains(opened, []byte("历史地址")) || len(result.ContextRemoved) != 3 {
					t.Fatal("old profile exclusions missing")
				}
			case "metadata":
				opened, err := OpenPaymentMetadata(ring, identity, value.(string))
				if err != nil || !bytes.Equal(opened, source["metadata"].Raw) {
					t.Fatal("metadata exactness differs")
				}
			case "amount":
				if value != "9007199254740993.123456789012345678" {
					t.Fatal("exact amount changed")
				}
			}
		}
		after, _ := json.Marshal(source)
		if !bytes.Equal(original, after) {
			t.Fatal("source row mutated")
		}
		if got, err := Convert(table, source); err != ErrHistoryPolicy || len(got.Values) != 0 {
			t.Fatal("ordinary entry bypassed hold")
		}
		config, _ := NewConfigSecrets(ring, nil)
		if got, err := config.Convert(table, source); err != ErrHistoryPolicy || len(got.Values) != 0 {
			t.Fatal("config entry bypassed hold")
		}
	}
}

func TestPaymentHistoryNullProvenanceAndNoPartialRow(t *testing.T) {
	ring, _ := protected.New("payment-test", map[string][]byte{"payment-test": bytes.Repeat([]byte{31}, 32)})
	converter, _ := NewPaymentHistory(ring)
	for _, metadata := range []Cell{{SQLNull: true}, {Raw: json.RawMessage(`null`)}, {Raw: json.RawMessage(`{}`)}} {
		source := paymentHistoryFixture(t, "payment")
		source["data"] = Cell{SQLNull: true}
		source["metadata"] = metadata
		result, err := converter.Convert("payment", source)
		if err != nil {
			t.Fatal("valid nullable history rejected")
		}
		for index, column := range result.Row.Columns {
			value := result.Row.Values[index]
			if column == "data" && value != nil {
				t.Fatal("SQL NULL data became an object")
			}
			if column == "metadata" {
				if metadata.SQLNull {
					if value != nil {
						t.Fatal("SQL NULL metadata became JSON null")
					}
				} else if plain, err := OpenPaymentMetadata(ring, PaymentDataIdentity{SourceTable: "payment", SourceID: "payment-1", SessionID: "session-1", ProviderID: "pp_mollie-hosted-checkout_mollie"}, value.(string)); err != nil || !bytes.Equal(plain, metadata.Raw) {
					t.Fatal("JSON metadata became SQL NULL")
				}
			}
		}
		// Even rows with ALL protected values NULL require disposition.
		source["metadata"] = Cell{SQLNull: true}
		if _, err := Convert("payment", source); err != ErrHistoryPolicy {
			t.Fatal("NULL values bypassed historical policy")
		}
	}
	for _, mutate := range []func(SourceRow){
		func(r SourceRow) { delete(r, "updated_at") },
		func(r SourceRow) { r["unknown"] = Cell{Raw: json.RawMessage(`true`)} },
		func(r SourceRow) { r["metadata"] = Cell{Raw: json.RawMessage(`{"token":"old-login"}`)} },
		func(r SourceRow) { r["metadata"] = Cell{SQLNull: true, Raw: json.RawMessage(`null`)} },
		func(r SourceRow) { r["metadata"] = Cell{Raw: json.RawMessage(`{"id":1,"id":2}`)} },
		func(r SourceRow) { r["raw_amount"] = Cell{Raw: json.RawMessage(`{"value":"1","precision":20}`)} },
		func(r SourceRow) { relationText(r, "status", "future") },
		func(r SourceRow) { relationText(r, "provider_id", "unknown-provider") },
		func(r SourceRow) { r["context"] = Cell{Raw: json.RawMessage(`{"token":"old-login"}`)} },
		func(r SourceRow) { r["data"] = Cell{SQLNull: true} },
		func(r SourceRow) { relationText(r, "updated_at", "invalid") },
	} {
		source := paymentHistoryFixture(t, "payment_session")
		mutate(source)
		got, err := converter.Convert("payment_session", source)
		if err == nil || !reflect.DeepEqual(got, HistoricalPayment{}) {
			t.Fatal("invalid input returned partial payment or hold")
		}
	}
	if _, err := NewPaymentHistory(nil); err != ErrProtection {
		t.Fatal("missing key accepted")
	}
	var missing *PaymentHistory
	if got, err := missing.Convert("payment", nil); err != ErrProtection || !reflect.DeepEqual(got, HistoricalPayment{}) {
		t.Fatal("nil converter accepted")
	}
}
