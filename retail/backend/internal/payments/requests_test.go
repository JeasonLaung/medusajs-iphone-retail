package payments

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
)

func TestSessionRequestEncoding(t *testing.T) {
	key := EffectKey{ScopeID: "cart", OperationID: uuid.NewString(), Kind: "create_session", ResourceID: "session"}
	in := SessionRequest{ProviderID: "pp_paypal_paypal", CollectionID: "collection", SessionID: "session", Amount: "2.35e2", Currency: "eur", Data: json.RawMessage(` { "id": "provider-session", "large":9007199254740993,"precise":1.000000000000000000001 } `)}
	raw, err := encodeSessionRequest(key, in)
	if err != nil || !bytes.Contains(raw, []byte(`"amount":"235"`)) || !bytes.Contains(raw, []byte(`9007199254740993`)) || !bytes.Contains(raw, []byte(`1.000000000000000000001`)) || !bytes.Contains(raw, []byte(`"context":null`)) {
		t.Fatal("request normalized away precise provider facts")
	}
	for _, bad := range []string{``, `null`, `[]`, `true`, `{"a":1,"a":2}`, `{"nested":{"same":1,"same":2}}`, `{"array":[{"same":1,"same":2}]}`, `{"a":1} {"b":2}`, `{"bad":NaN}`, `{"broken":`, string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}), `{"deep":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`} {
		changed := in
		changed.Data = json.RawMessage(bad)
		if got, err := encodeSessionRequest(key, changed); err != ErrEffectInput || got != nil {
			t.Fatal("invalid provider JSON accepted")
		}
	}
	for _, mutate := range []func(*SessionRequest){
		func(v *SessionRequest) { v.ProviderID = "provider with space" }, func(v *SessionRequest) { v.CollectionID = "" }, func(v *SessionRequest) { v.SessionID = "other" },
		func(v *SessionRequest) { v.Currency = "EUR" }, func(v *SessionRequest) { v.Amount = "-0.01" }, func(v *SessionRequest) { v.Amount = "NaN" },
		func(v *SessionRequest) { v.Context = json.RawMessage(`{"duplicate":1,"duplicate":2}`) },
		func(v *SessionRequest) { v.Context = json.RawMessage(strings.Repeat(" ", protected.MaxPlaintext) + "null") },
		func(v *SessionRequest) {
			v.Data = json.RawMessage(`{"large":"` + strings.Repeat("x", protected.MaxPlaintext) + `"}`)
		},
	} {
		changed := in
		mutate(&changed)
		if got, err := encodeSessionRequest(key, changed); err != ErrEffectInput || got != nil {
			t.Fatal("invalid session snapshot accepted")
		}
	}
}
