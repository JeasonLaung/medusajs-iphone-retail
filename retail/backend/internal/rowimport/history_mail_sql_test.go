package rowimport

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHistoryMailRowFingerprintIncludesLexemesAndNull(t *testing.T) {
	a := SourceRow{"a": {Raw: json.RawMessage(`1.0`)}, "b": {SQLNull: true}}
	b := SourceRow{"b": {SQLNull: true}, "a": {Raw: json.RawMessage(`1.0`)}}
	if sourceRowDigest(a) != sourceRowDigest(b) {
		t.Fatal("map iteration changes fingerprint")
	}
	for _, different := range []SourceRow{
		{"a": {Raw: json.RawMessage(`1`)}, "b": {SQLNull: true}},
		{"a": {Raw: json.RawMessage(`1.0`)}, "b": {Raw: json.RawMessage(`null`)}},
		{"a": {Raw: json.RawMessage(`1.0`)}, "b": {SQLNull: true}, "c": {}},
	} {
		if sourceRowDigest(a) == sourceRowDigest(different) {
			t.Fatal("source drift hidden")
		}
	}
	if err := ImportHistoricalMail(context.Background(), nil, "", nil); err != ErrHistoryStorage {
		t.Fatal("nil transaction accepted")
	}
	if _, err := HistoricalMailDeliveryDisposition(context.Background(), nil, "id"); err != ErrHistoryStorage {
		t.Fatal("nil delivery transaction accepted")
	}
}
