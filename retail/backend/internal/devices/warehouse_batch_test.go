package devices

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDecodeWarehouseBatch(t *testing.T) {
	valid := WarehouseBatchInput{OperationID: uuid.NewString(), Devices: []WarehouseBatchItem{{ID: "dev_owned", ExpectedRevision: strings.Repeat("a", 64)}}}
	raw, _ := json.Marshal(valid)
	if _, e := decodeWarehouseBatch(strings.NewReader(string(raw))); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{
		`{}`, `null`, strings.Replace(string(raw), `"devices":[`, `"devices":null,"other":[`, 1),
		strings.Replace(string(raw), `"id":"dev_owned"`, `"id":"dev_owned","id":"dev_other"`, 1),
		strings.Replace(string(raw), `"id":"dev_owned"`, `"id":"dev_owned","private":true`, 1),
		strings.Replace(string(raw), `"devices":`, `"devices":[],"devices":`, 1), string(raw) + `{}`,
	} {
		if _, e := decodeWarehouseBatch(strings.NewReader(bad)); e == nil {
			t.Fatalf("accepted invalid command: %s", bad)
		}
	}
	valid.Devices = append(valid.Devices, valid.Devices[0])
	if validWarehouseBatch(valid) {
		t.Fatal("duplicate device accepted")
	}
	valid.Devices = nil
	for i := 0; i < 200; i++ {
		valid.Devices = append(valid.Devices, WarehouseBatchItem{ID: fmt.Sprintf("dev_owned_%d", i), ExpectedRevision: strings.Repeat("b", 64)})
	}
	raw, _ = json.Marshal(valid)
	if _, e := decodeWarehouseBatch(strings.NewReader(string(raw))); e != nil {
		t.Fatal("original 200-device limit rejected", e)
	}
	valid.Devices = append(valid.Devices, WarehouseBatchItem{ID: "dev_overflow", ExpectedRevision: strings.Repeat("b", 64)})
	if validWarehouseBatch(valid) {
		t.Fatal("over 200 accepted")
	}
}
