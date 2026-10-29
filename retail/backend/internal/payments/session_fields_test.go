package payments

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/eurorefurb/retail/internal/protected"
)

func TestRuntimeSessionProtection(t *testing.T) {
	ring, err := protected.New("test", map[string][]byte{"test": bytes.Repeat([]byte{0x47}, 32)})
	if err != nil {
		t.Fatal("artificial ring unavailable")
	}
	in := SessionRequest{ProviderID: "pp_mollie-hosted-checkout_mollie", CollectionID: "collection", SessionID: "session", Amount: "1e1", Currency: "eur", Data: json.RawMessage(`{"id":"private-id","amount":9007199254740993.000000000000000000001}`), Context: json.RawMessage(`{"idempotency_key":"stable"}`)}
	data, context, err := SealSessionFields(ring, in)
	if err != nil || context == nil || bytes.Contains([]byte(data), []byte("private-id")) {
		t.Fatal("runtime session fields not protected")
	}
	restored, err := OpenSessionFields(ring, in, data, context)
	if err != nil || restored.Amount != "10" || !bytes.Equal(restored.Data, in.Data) || !bytes.Equal(restored.Context, in.Context) {
		t.Fatal("runtime session protection lost exact facts")
	}
	for _, mutate := range []func(*SessionRequest){func(v *SessionRequest) { v.ProviderID = "other" }, func(v *SessionRequest) { v.CollectionID = "other" }, func(v *SessionRequest) { v.SessionID = "other" }, func(v *SessionRequest) { v.Amount = "11" }, func(v *SessionRequest) { v.Currency = "usd" }} {
		changed := in
		mutate(&changed)
		if result, err := OpenSessionFields(ring, changed, data, context); err != ErrEffectProtection || result.SessionID != "" {
			t.Fatal("cross-session field substitution accepted")
		}
	}
	if _, err := OpenSessionFields(ring, in, *context, &data); err != ErrEffectProtection {
		t.Fatal("data/context substitution accepted")
	}
	if _, err := OpenSessionFields(nil, in, data, context); err != ErrEffectProtection {
		t.Fatal("missing session key accepted")
	}
	if _, err := OpenSessionFields(ring, in, string(in.Data), context); err != ErrEffectProtection {
		t.Fatal("plaintext session fallback accepted")
	}
	in.Context = nil
	data, context, err = SealSessionFields(ring, in)
	if err != nil || context != nil {
		t.Fatal("SQL NULL context changed")
	}
	if result, err := OpenSessionFields(ring, in, data, nil); err != nil || string(result.Context) != "null" {
		t.Fatal("NULL context recovery failed")
	}
}
