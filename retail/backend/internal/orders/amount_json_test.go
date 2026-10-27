package orders

import (
	"github.com/eurorefurb/retail/internal/totals"
	"testing"
)

func TestFinancialJSONProjection(t *testing.T) {
	for _, raw := range []string{`{"value":"1","value":"2","precision":20}`, `{"value":"1","precision":20,"extra":1}`, `{"value":null,"precision":20}`, `{"value":"NaN","precision":20}`, `{"value":"1","precision":0}`, `{"value":"1","precision":"20"}`, `[]`, `null`, `{"value":"1","precision":20} {}`} {
		if _, err := amountNumber("12", &raw); err == nil {
			t.Fatal("invalid raw JSON fell back to scalar")
		}
	}
	for _, raw := range []string{`{"value":"9007199254740993.000000000000000001","precision":38}`, `{"value":9007199254740993.000000000000000001,"precision":38}`} {
		n, err := amountNumber("1", &raw)
		if err != nil {
			t.Fatal(err)
		}
		effective, err := totals.Resolve(n)
		if err != nil || effective != "9007199254740993.000000000000000001" {
			t.Fatal("raw financial precision lost")
		}
	}
	for _, text := range []string{`{"transaction_total":1,"transaction_total":2}`, `{"raw_transaction_total":{"value":"1","value":"2","precision":20}}`, `{"transaction_total":true}`, `{"transaction_total":"NaN"}`} {
		if _, _, err := transactionSummary(&text); err == nil {
			t.Fatal("ambiguous summary accepted")
		}
	}
	for _, text := range []string{`{}`, `{"transaction_total":null}`, `{"transaction_total":0,"private":"must-not-escape"}`} {
		s, amount, err := transactionSummary(&text)
		if err != nil || s == nil || amount == nil || *amount != "0" {
			t.Fatal("missing/null summary zero differs")
		}
	}
	text := `{"transaction_total":1,"raw_transaction_total":{"value":"2.5","precision":20},"private":"must-not-escape"}`
	s, amount, err := transactionSummary(&text)
	if err != nil || s == nil || amount == nil || *amount != "2.5" {
		t.Fatal("raw summary precedence differs")
	}
	for _, text := range []string{`null`, ` null `} {
		s, a, err := transactionSummary(&text)
		if err != nil || s != nil || a != nil {
			t.Fatal("null summary is not absent")
		}
	}
}
