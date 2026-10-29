package devices

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestAllocationInputAndCapacityUnits(t *testing.T) {
	in := AllocationInput{uuid.NewString(), strings.Repeat("a", 64), "reserve", []string{"device-a"}}
	b, _ := json.Marshal(in)
	if _, e := decodeAllocation(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{string(b) + `{}`, strings.Replace(string(b), `"reserve"`, `"ship"`, 1), strings.Replace(string(b), `["device-a"]`, `["device-a","device-a"]`, 1), strings.Replace(string(b), `"action":"reserve"`, `"action":"reserve","action":"release"`, 1), strings.Replace(string(b), `["device-a"]`, `null`, 1), string(b) + string([]byte{255})} {
		if _, e := decodeAllocation(strings.NewReader(s)); e == nil {
			t.Fatal("invalid allocation")
		}
	}
	for _, s := range []string{"-1", "1.1", "1000000001", "bad"} {
		if _, e := allocationUnits(s); e == nil {
			t.Fatal("invalid device count")
		}
	}
	if n, e := allocationUnits("2.000000000000000000"); e != nil || n != 2 {
		t.Fatal("whole device quantity")
	}
}
