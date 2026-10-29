package devices

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestWarehouseDeviceCommandWire(t *testing.T) {
	in := WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), Action: "retire"}
	raw, _ := json.Marshal(in)
	if _, e := decodeWarehouseDeviceCommand(strings.NewReader(string(raw))); e != nil {
		t.Fatal("valid command rejected")
	}
	for _, body := range []string{`{}`, string(raw) + `{}`, strings.Replace(string(raw), `"action":"retire"`, `"action":null`, 1), strings.Replace(string(raw), `"action":"retire"`, `"action":"ship"`, 1), strings.Replace(string(raw), `"action":"retire"`, `"action":"retire","action":"delete"`, 1), strings.Replace(string(raw), `"action":"retire"`, `"action":"retire","location_id":"other"`, 1)} {
		if _, e := decodeWarehouseDeviceCommand(strings.NewReader(body)); e == nil {
			t.Fatal("invalid warehouse command accepted")
		}
	}
	for _, category := range []string{"recycled", "SALE", "", "supplier"} {
		if _, e := warehouseDeviceActions(category, false); e == nil {
			t.Fatal("category accepted")
		}
	}
	if q, e := normalizeWarehouseSearch("  sn-ab  "); e != nil || q != "SN-AB" {
		t.Fatal("normalized search failed")
	}
	if _, e := normalizeWarehouseSearch(strings.Repeat("x", 101)); e == nil {
		t.Fatal("unbounded search")
	}
}

func TestWarehouseTransferReturnWire(t *testing.T) {
	in := WarehouseDeviceCommand{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), Action: "transfer_return", Notes: strings.Repeat("退", 2000)}
	raw, _ := json.Marshal(in)
	if _, e := decodeWarehouseDeviceCommand(strings.NewReader(string(raw))); e != nil {
		t.Fatal("valid unicode reason rejected", e)
	}
	for _, notes := range []string{"", " ", " padded ", strings.Repeat("退", 2001)} {
		in.Notes = notes
		b, _ := json.Marshal(in)
		if _, e := decodeWarehouseDeviceCommand(strings.NewReader(string(b))); e == nil {
			t.Fatal("invalid reason accepted")
		}
	}
	in.Notes = "returned after check"
	raw, _ = json.Marshal(in)
	for _, body := range []string{
		strings.Replace(string(raw), `"notes":"returned after check"`, `"notes":null`, 1),
		strings.Replace(string(raw), `,"notes":"returned after check"`, "", 1),
		strings.Replace(string(raw), `"notes":"returned after check"`, `"notes":"a","notes":"b"`, 1),
		strings.Replace(string(raw), `"action":"transfer_return"`, `"action":"retire"`, 1),
	} {
		if _, e := decodeWarehouseDeviceCommand(strings.NewReader(body)); e == nil {
			t.Fatal("invalid transfer wire accepted")
		}
	}
}
