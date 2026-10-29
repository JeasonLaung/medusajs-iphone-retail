package devices

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestIntakeWireRejectsAmbiguousNestedFields(t *testing.T) {
	in := IntakeInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), Devices: []IntakeDevice{{Identity: Identity{SerialNumber: "wire-sn"}, SourceID: "wire-source", Category: "sale", CurrencyCode: "eur", Photos: []Photo{}}}}
	b, _ := json.Marshal(in)
	valid := string(b)
	if _, e := decodeIntake(strings.NewReader(valid)); e != nil {
		t.Fatal("explicit canonical wire rejected", e)
	}
	for _, bad := range []string{
		strings.Replace(valid, `"source_id":"wire-source"`, `"source_id":"wire-source","source_id":"other"`, 1),
		strings.Replace(valid, `"photos":[]`, `"photos":null`, 1),
		strings.Replace(valid, `"cost_cents":0`, `"cost_cents":0.1`, 1),
		strings.Replace(valid, `"grade":null,`, "", 1),
		strings.Replace(valid, `"source_id":"wire-source"`, `"source_id":"wire-source","location_id":"untrusted"`, 1),
		valid + `{}`, `null`,
	} {
		if _, e := decodeIntake(strings.NewReader(bad)); e != ErrInput {
			t.Fatal("ambiguous intake wire accepted", e)
		}
	}
}
