package cart

import (
	"strings"
	"testing"
)

func TestPromotionCommandBoundary(t *testing.T) {
	const operation = "00000000-0000-4000-8000-000000000001"
	for _, body := range []string{`{"version":"1","codes":[]}`, `{"version":"1","codes":["CODE","中文"]}`} {
		in, err := decodePromotion(strings.NewReader(body))
		in.OperationID = operation
		if err != nil {
			t.Fatal(err)
		}
		if _, err = validatePromotion(in); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{`null`, `[]`, `{"version":"1"}`, `{"version":1,"codes":[]}`, `{"version":"1","codes":null}`, `{"version":"1","codes":[1]}`, `{"version":"1","codes":{},"amount":"0"}`, `{"version":"1","codes":[],"codes":[]}`, `{"version":"1","version":"1","codes":[]}`, `{"version":"1","codes":[],"customer_id":"legacy"}`, `{"version":"1","codes":[]} {}`, `{"version":"1","codes":[""]}`, `{"version":"1","codes":["line\ncode"]}`, `{"version":"01","codes":[]}`, `{"version":"18446744073709551615","codes":[]}`} {
		in, err := decodePromotion(strings.NewReader(body))
		in.OperationID = operation
		if err == nil {
			_, err = validatePromotion(in)
		}
		if err == nil {
			t.Fatalf("invalid promotion payload accepted: %s", body)
		}
	}
	for _, code := range []string{strings.Repeat("a", 192), string([]byte{0xff}), "a\x00b", "a\x7fb"} {
		if _, err := validatePromotion(PromotionCommand{OperationID: operation, Version: "1", Codes: []string{code}}); err == nil {
			t.Fatal("invalid code accepted")
		}
	}
	if _, err := validatePromotion(PromotionCommand{OperationID: operation, Version: "1", Codes: make([]string, 101)}); err == nil {
		t.Fatal("unbounded codes accepted")
	}
}
