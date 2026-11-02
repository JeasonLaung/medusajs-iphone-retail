package devices

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestWarehouseEditWire(t *testing.T) {
	in := WarehouseEditInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), Fields: WarehouseEditFields{VariantID: "variant", SourceID: "source", LocationID: "location", Category: "sale", State: "available", CurrencyCode: "eur", Photos: []Photo{}}}
	raw, _ := json.Marshal(in)
	if _, e := decodeWarehouseEdit(strings.NewReader(string(raw))); e != nil {
		t.Fatal("valid nullable edit", e)
	}
	for _, body := range []string{string(raw) + `{}`, strings.Replace(string(raw), `"cost_cents":0`, `"cost_cents":null`, 1), strings.Replace(string(raw), `"imei":null`, `"imei":""`, 1), strings.Replace(string(raw), `"state":"available"`, `"state":"available","serial_number":"changed"`, 1), strings.Replace(string(raw), `"state":"available"`, `"state":"available","state":"shipped"`, 1), strings.Replace(string(raw), `"category":"sale"`, `"category":"recycled"`, 1), strings.Replace(string(raw), `"photos":[]`, `"photos":null`, 1)} {
		if _, e := decodeWarehouseEdit(strings.NewReader(body)); e == nil {
			t.Fatal("invalid device edit accepted", body)
		}
	}
}
