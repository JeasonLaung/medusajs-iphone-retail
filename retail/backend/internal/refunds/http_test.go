package refunds

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
)

func TestRefundJSONRejectsAmbiguousOrPrivateFields(t *testing.T) {
	required := []string{"operation_id", "payment_id", "amount"}
	optional := []string{"previous_operation_id"}
	for _, raw := range []string{`{"operation_id":"a","payment_id":"p","amount":"1","amount":"2"}`, `{"operation_id":"a","payment_id":"p","amount":1}`, `{"operation_id":"a","payment_id":"p","amount":null}`, `{"operation_id":"a","payment_id":"p","amount":{"value":"1"}}`, `{"operation_id":"a","payment_id":"p","amount":"1","remote_id":"tr_foreign"}`, `[]`, `{} {}`, `{"operation_id":"a","payment_id":"p"}`, strings.Repeat(" ", 4097)} {
		if _, e := decodeStrings(strings.NewReader(raw), required, optional); e != ErrInput {
			t.Fatal("unsafe refund JSON accepted")
		}
	}
	fields, e := decodeStrings(strings.NewReader(`{"operation_id":"a","payment_id":"p","amount":"9007199254740993.01","previous_operation_id":"prior"}`), required, optional)
	if e != nil || fields["amount"] != "9007199254740993.01" {
		t.Fatal("money coerced")
	}
}
func TestOriginalVersion83RefundDigestIsPreserved(t *testing.T) {
	in := Input{OperationID: "operation", OrderID: "order", RequestID: "request", PaymentID: "payment", Amount: "100.01"}
	original := struct {
		OperationID string `json:"operation_id"`
		OrderID     string `json:"order_id"`
		RequestID   string `json:"request_id"`
		PaymentID   string `json:"payment_id"`
		Amount      string `json:"amount"`
	}{in.OperationID, in.OrderID, in.RequestID, in.PaymentID, in.Amount}
	raw, _ := json.Marshal(original)
	if digest(in) != sha256.Sum256(raw) {
		t.Fatal("upgrade invalidated original immutable instruction")
	}
}
