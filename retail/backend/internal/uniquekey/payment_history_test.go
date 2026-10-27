package uniquekey

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
)

func testPaymentHistorySQL(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	ring, _ := protected.New("fixture-payment-row", map[string][]byte{"fixture-payment-row": bytes.Repeat([]byte{47}, 32)})
	converter, _ := rowimport.NewPaymentHistory(ring)
	text := func(s string) rowimport.Cell { raw, _ := json.Marshal(s); return rowimport.Cell{Raw: raw} }
	amount := rowimport.Cell{Raw: json.RawMessage(`"1.2300"`)}
	rawAmount := rowimport.Cell{Raw: json.RawMessage(`{"value":"1.2300000000000000000","precision":20}`)}
	timestamp := text("2026-10-27T00:00:00.123456Z")
	collection := rowimport.SourceRow{"id": text("fixture-history-collection"), "currency_code": text("eur"), "amount": amount,
		"raw_amount": rawAmount, "authorized_amount": {SQLNull: true}, "raw_authorized_amount": {SQLNull: true},
		"captured_amount": {SQLNull: true}, "raw_captured_amount": {SQLNull: true}, "refunded_amount": {SQLNull: true},
		"raw_refunded_amount": {SQLNull: true}, "created_at": timestamp, "updated_at": timestamp,
		"deleted_at": {SQLNull: true}, "completed_at": {SQLNull: true}, "status": text("not_paid"), "metadata": {SQLNull: true}}
	parent, err := rowimport.Convert("payment_collection", collection)
	if err != nil {
		t.Fatal("artificial payment parent conversion failed")
	}
	data := rowimport.Cell{Raw: json.RawMessage(`{"id":"provider-fixture","mode":"test","resource":"payment","status":"open","amount":{"currency":"EUR","value":"1.23"},"session_id":"fixture-history-session","cart_id":"fixture-cart","metadata":{"session_id":"fixture-history-session","cart_id":"fixture-cart"}}`)}
	source := rowimport.SourceRow{"id": text("fixture-history-session"), "currency_code": text("eur"), "amount": amount,
		"raw_amount": rawAmount, "provider_id": text("pp_mollie-hosted-checkout_mollie"), "data": data,
		"context": {Raw: json.RawMessage(`{"customer":{"addresses":[{"token":"excluded-old-login"}],"billing_address":{"address_1":"历史交易地址"}}}`)},
		"status":  text("pending"), "authorized_at": {SQLNull: true}, "payment_collection_id": text("fixture-history-collection"),
		"metadata": {Raw: json.RawMessage(`null`)}, "created_at": timestamp, "updated_at": timestamp, "deleted_at": {SQLNull: true}}
	session, err := converter.Convert("payment_session", source)
	if err != nil {
		t.Fatal("artificial complete payment session conversion failed")
	}
	paymentSource := rowimport.SourceRow{"id": text("fixture-history-payment"), "amount": amount, "raw_amount": rawAmount,
		"currency_code": text("eur"), "provider_id": text("pp_mollie-hosted-checkout_mollie"), "data": data,
		"created_at": timestamp, "updated_at": timestamp, "deleted_at": {SQLNull: true}, "captured_at": {SQLNull: true},
		"canceled_at": {SQLNull: true}, "payment_collection_id": text("fixture-history-collection"),
		"payment_session_id": text("fixture-history-session"), "metadata": {SQLNull: true}}
	payment, err := converter.Convert("payment", paymentSource)
	if err != nil || session.Operations.Disposition != "hold" || payment.Operations.Disposition != "hold" {
		t.Fatal("artificial complete payment conversion or hold failed")
	}
	insert := func(tx *sql.Tx, bound rowimport.BoundRow) error {
		var columns, markers []string
		for _, column := range bound.Columns {
			columns = append(columns, "`"+column+"`")
			markers = append(markers, "?")
		}
		returnErr := error(nil)
		_, returnErr = tx.ExecContext(ctx, "INSERT INTO relational_fixture.`"+bound.Table+"` ("+strings.Join(columns, ",")+") VALUES ("+strings.Join(markers, ",")+")", bound.Values...)
		return returnErr
	}
	// This tests converted complete rows in the full reviewed schema. It is
	// not the future importer: durable operation-hold persistence is still due.
	for _, commit := range []bool{false, true} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("payment fixture transaction unavailable")
		}
		defer tx.Rollback()
		for _, row := range []rowimport.BoundRow{parent, session.Row, payment.Row} {
			if err := insert(tx, row); err != nil {
				t.Fatal("complete payment row insert failed")
			}
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal("complete payment transaction failed")
		}
		if !commit {
			for _, table := range []string{"payment_collections", "payment_sessions", "payments"} {
				var count int
				if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relational_fixture.`"+table+"` WHERE id LIKE 'fixture-history-%'").Scan(&count); err != nil || count != 0 {
					t.Fatal("payment rollback left business rows")
				}
			}
		}
	}
	var storedData, storedContext, storedMetadata, storedAmount, storedRaw, storedStatus, storedTime string
	if err := db.QueryRowContext(ctx, `SELECT data,context,metadata,amount,raw_amount,status,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s.%f')
	 FROM relational_fixture.payment_sessions WHERE id='fixture-history-session'`).Scan(&storedData, &storedContext, &storedMetadata, &storedAmount, &storedRaw, &storedStatus, &storedTime); err != nil {
		t.Fatal("complete session roundtrip unavailable")
	}
	identity := rowimport.PaymentDataIdentity{SourceTable: "payment_session", SourceID: "fixture-history-session", SessionID: "fixture-history-session", ProviderID: "pp_mollie-hosted-checkout_mollie", CurrencyCode: "eur", Amount: amount.Raw}
	if plain, err := rowimport.OpenPaymentData(ring, identity, storedData); err != nil || !bytes.Equal(plain, data.Raw) {
		t.Fatal("stored provider evidence differs")
	}
	if plain, err := rowimport.OpenPaymentContext(ring, identity.SourceID, storedContext); err != nil || bytes.Contains(plain, []byte("excluded-old-login")) || !bytes.Contains(plain, []byte("历史交易地址")) {
		t.Fatal("stored transaction context projection differs")
	}
	if plain, err := rowimport.OpenPaymentMetadata(ring, identity, storedMetadata); err != nil || string(plain) != "null" {
		t.Fatal("JSON null payment metadata lost provenance")
	}
	if storedAmount != "1.230000000000000000" || storedRaw != string(rawAmount.Raw) || storedStatus != "pending" || storedTime != "2026-10-27 00:00:00.123456" {
		t.Fatal("session scalar facts changed")
	}
	var metadata sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT data,metadata FROM relational_fixture.payments WHERE id='fixture-history-payment'").Scan(&storedData, &metadata); err != nil || metadata.Valid {
		t.Fatal("SQL NULL payment metadata became a JSON document")
	}
	identity.SourceTable, identity.SourceID = "payment", "fixture-history-payment"
	if plain, err := rowimport.OpenPaymentData(ring, identity, storedData); err != nil || !bytes.Equal(plain, data.Raw) {
		t.Fatal("stored payment evidence differs")
	}
}
