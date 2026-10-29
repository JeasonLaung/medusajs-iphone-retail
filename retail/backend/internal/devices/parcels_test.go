package devices

import (
	"strings"
	"testing"
)

func TestParcelWire(t *testing.T) {
	const base = `{"operation_id":"d40f2fb0-39f3-4ac9-a8a0-58de6295c06a","expected_revision":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","action":"ship","tracking_number":" TRACK-001 ","tracking_url":"https://tracking.example.invalid/item?id=1"}`
	in, e := decodeParcel(strings.NewReader(base))
	if e != nil || in.TrackingNumber != "TRACK-001" {
		t.Fatal("normalized shipment input", e)
	}
	for _, body := range []string{
		base + `{}`, strings.Replace(base, `"action":"ship"`, `"action":"ship","action":"cancel"`, 1), strings.Replace(base, `"action":"ship"`, `"action":null`, 1), strings.Replace(base, `"action":"ship"`, `"action":"ship","unknown":1`, 1), strings.Replace(base, `"action":"ship"`, `"action":"cancel"`, 1), strings.Replace(base, ` TRACK-001 `, strings.Repeat("X", 201), 1), strings.Replace(base, ` TRACK-001 `, `bad\ntracking`, 1), strings.Replace(base, `https://tracking.example.invalid/item?id=1`, `javascript:alert(1)`, 1), strings.Replace(base, `https://tracking.example.invalid/item?id=1`, `https://user:password@example.invalid/`, 1), strings.Replace(base, `"tracking_number":" TRACK-001 "`, `"tracking_number":12`, 1), strings.Repeat(" ", 16385) + base,
	} {
		if _, e := decodeParcel(strings.NewReader(body)); e == nil {
			t.Fatal("unsafe parcel input accepted")
		}
	}
	cancel := strings.Replace(base, `"action":"ship"`, `"action":"cancel"`, 1)
	cancel = strings.Replace(cancel, `"tracking_number":" TRACK-001 "`, `"tracking_number":""`, 1)
	cancel = strings.Replace(cancel, `"tracking_url":"https://tracking.example.invalid/item?id=1"`, `"tracking_url":""`, 1)
	if _, e := decodeParcel(strings.NewReader(cancel)); e != nil {
		t.Fatal("valid cancellation", e)
	}
}
