package devices

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestServiceStateAndWire(t *testing.T) {
	for _, v := range []struct {
		result          string
		repair          bool
		state, category string
	}{{"pass", true, "repair_ready", "returned"}, {"pass", false, "available", "sale"}, {"repair", true, "repair", ""}, {"retire", false, "retired", ""}, {"supplier_return", false, "supplier_returned", ""}} {
		state, category := serviceState(v.result, v.repair)
		if state != v.state || category != v.category {
			t.Fatal("inspection state rule")
		}
	}
	in := ServiceInput{OperationID: "12345678-1234-4234-8234-123456789012", ExpectedRevision: strings.Repeat("a", 64), Action: "inspect", Result: "pass", VariantID: "sku", Grade: "B", BatteryHealth: 85, Notes: "Actual inspection", Photos: []Photo{}}
	b, _ := json.Marshal(in)
	if _, e := decodeService(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{`null`, `{}`, string(b) + ` {}`, strings.Replace(string(b), `"grade":"B"`, `"grade":null`, 1), strings.Replace(string(b), `"grade":"B"`, `"grade":"B","grade":"C"`, 1), strings.Replace(string(b), `"photos":[]`, `"photos":null`, 1), strings.Replace(string(b), `"battery_health":85`, `"battery_health":85.5`, 1), strings.Replace(string(b), `"grade":"B"`, `"grade":"Z"`, 1)} {
		if _, e := decodeService(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid inspection wire")
		}
	}
	in = ServiceInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, Action: "repair_ship", TrackingNumber: "TRACK-1", Photos: []Photo{}}
	b, _ = json.Marshal(in)
	if _, e := decodeService(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	in.Notes = "ambiguous"
	if validService(in) {
		t.Fatal("mixed inspection/shipment accepted")
	}
}
