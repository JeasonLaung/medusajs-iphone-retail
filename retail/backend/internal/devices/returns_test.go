package devices

import (
	"strings"
	"testing"
)

func TestReturnBoundary(t *testing.T) {
	good := `{"operation_id":"12345678-1234-4234-8234-123456789012","expected_revision":"` + strings.Repeat("a", 64) + `","assignment_id":"assigned","source_id":"source","scan":"SN-1"}`
	if _, e := decodeReturn(strings.NewReader(good)); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{`null`, `[]`, `{}`, strings.Replace(good, `"scan":"SN-1"`, `"scan":null`, 1), strings.Replace(good, `"scan":"SN-1"`, `"scan":"SN-1","scan":"SN-2"`, 1), strings.Replace(good, `"scan":"SN-1"`, `"scan":"SN-1","unknown":"x"`, 1), good + ` {}`, strings.Replace(good, `"source"`, `"source/foreign"`, 1), strings.Replace(good, `"SN-1"`, `"   "`, 1)} {
		if _, e := decodeReturn(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid receipt wire accepted")
		}
	}
	radio := "000000000000007"
	d := ReturnDevice{SerialNumber: "SN-AbC", IMEI: &radio}
	if !returnScan("sn-abc", d) || !returnScan(radio, d) || returnScan("different", d) {
		t.Fatal("physical device scan mismatch")
	}
}
