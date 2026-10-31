package refunds

import (
	"github.com/eurorefurb/retail/internal/payments"
	"strings"
	"testing"
	"time"
)

func TestRefundCommandRejectsRoundedAndUnsafeAmounts(t *testing.T) {
	in := Input{OperationID: "12345678-1234-4234-8234-123456789abc", OrderID: "order", RequestID: "return", PaymentID: "payment", Amount: "9007199254740993.01"}
	if !valid(in) {
		t.Fatal("exact above safe float rejected")
	}
	for _, amount := range []string{"0", "0.00", "-1", "1e2", "1/2", "01", "1.001", "NaN", " 1", "1 ", "9999999999999999999.01"} {
		v := in
		v.Amount = amount
		if valid(v) {
			t.Fatalf("unsafe amount %s", amount)
		}
	}
	for _, id := range []string{"", "../order", strings.Repeat("a", 192), "order?x"} {
		v := in
		v.OrderID = id
		if valid(v) {
			t.Fatal("unsafe order")
		}
	}
}
func TestRefundLedgerExactValueDoesNotUseFloatOrForgiveBadRaw(t *testing.T) {
	v, e := exact("9007199254740993.010000000000000000", `{"value":"9007199254740993.01","precision":100}`)
	if e != nil || v != "9007199254740993.01" {
		t.Fatal(v, e)
	}
	for _, raw := range []string{`{"value":"1","value":"1000","precision":100}`, `{"value":"1"}`, `null`, `{"value":"1","precision":100,"unknown":true}`} {
		if _, e := exact("1", raw); e == nil {
			t.Fatal("corrupt refund raw accepted")
		}
	}
}
func TestRefundObservationCannotChangePaymentAmountOrTerminalVocabulary(t *testing.T) {
	remote := "re_original"
	r := operation{RemotePaymentID: "tr_original", Amount: "100.01", CurrencyCode: "eur", RemoteRefundID: &remote}
	good := payments.RefundObservation{RefundID: remote, PaymentID: "tr_original", Amount: "100.010", Currency: "eur", CreatedAt: time.Now(), Status: "refunded"}
	if !matches(r, good) {
		t.Fatal("same exact amount rejected")
	}
	for _, mutate := range []func(*payments.RefundObservation){
		func(o *payments.RefundObservation) { o.Amount = "100.02" }, func(o *payments.RefundObservation) { o.RefundID = "re_other" }, func(o *payments.RefundObservation) { o.PaymentID = "tr_other" }, func(o *payments.RefundObservation) { o.Currency = "usd" }, func(o *payments.RefundObservation) { o.Status = "paid" }, func(o *payments.RefundObservation) { o.CreatedAt = time.Time{} },
	} {
		bad := good
		mutate(&bad)
		if matches(r, bad) {
			t.Fatal("foreign observation accepted")
		}
	}
}
