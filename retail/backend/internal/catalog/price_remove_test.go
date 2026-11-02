package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestPriceRemoveWire(t *testing.T) {
	in := PriceRemoveInput{uuid.NewString(), strings.Repeat("a", 64)}
	raw, _ := json.Marshal(in)
	if _, e := decodePriceRemove(strings.NewReader(string(raw))); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{`null`, string(raw) + `{}`, `{"operation_id":null,"expected_revision":"` + in.ExpectedRevision + `"}`, `{"operation_id":1,"expected_revision":"` + in.ExpectedRevision + `"}`, `{"operation_id":"` + in.OperationID + `","operation_id":"` + in.OperationID + `","expected_revision":"` + in.ExpectedRevision + `"}`, strings.TrimSuffix(string(raw), "}") + `,"amount":"1"}`, `{"operation_id":"` + in.OperationID + `"}`, strings.Replace(string(raw), in.ExpectedRevision, "wrong", 1)} {
		if _, e := decodePriceRemove(strings.NewReader(body)); e == nil {
			t.Fatal("invalid removal wire accepted", body)
		}
	}
}
