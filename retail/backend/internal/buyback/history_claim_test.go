package buyback

import "testing"

func TestHistoryClaimInput(t *testing.T) {
	for _, raw := range []string{`{}`, `{"customer_id":"new","evidence_note":"short"}`, `{"customer_id":"new","customer_id":"other","evidence_note":"Verified original documents"}`, `{"customer_id":"new","evidence_note":"Verified original documents","email":"same@example.invalid"}`, `{"customer_id":"new","evidence_note":null}`} {
		if _, e := ParseHistoryClaim([]byte(raw)); e == nil {
			t.Fatal("invalid human claim accepted")
		}
	}
	in, e := ParseHistoryClaim([]byte(`{"customer_id":"new","evidence_note":"  Verified original documents  "}`))
	if e != nil || in.EvidenceNote != "Verified original documents" {
		t.Fatal("claim normalization")
	}
}
