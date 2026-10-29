package devices

import (
	"strings"
	"testing"
)

func TestIdentityCanonicalAndProductPrecedence(t *testing.T) {
	in, e := NormalizeIdentity(Identity{" sn-001 ", " 000000000000001 ", "000000000000002", "00000000000000000000000000000001"}, true)
	if e != nil || in.SerialNumber != "SN-001" || in.IMEI != "000000000000001" || in.EID != "00000000000000000000000000000001" {
		t.Fatal("canonical identity lost")
	}
	for _, bad := range []Identity{{}, {SerialNumber: "a\x00"}, {SerialNumber: strings.Repeat("a", 101)}, {SerialNumber: string([]byte{0xff})}, {SerialNumber: "a", IMEI: "123"}, {SerialNumber: "a", IMEI: "000000000000001", IMEI2: "000000000000001"}, {SerialNumber: "a", EID: "123"}} {
		if _, e := NormalizeIdentity(bad, false); e != ErrIdentity {
			t.Fatal("invalid identity accepted")
		}
	}
	if _, e := NormalizeIdentity(Identity{SerialNumber: "SN"}, true); e != ErrIdentity {
		t.Fatal("radio requirement ignored")
	}
	if _, e := NormalizeIdentity(Identity{SerialNumber: strings.Repeat("仓", 100)}, false); e != nil {
		t.Fatal("source Unicode serial rejected")
	}
	if normalized, e := NormalizeIdentity(Identity{SerialNumber: "straße"}, false); e != nil || normalized.SerialNumber != "STRASSE" {
		t.Fatal("full Unicode uppercase differs from source")
	}
	for _, c := range []struct {
		metadata            map[string]any
		title, handle, kind string
		want                bool
	}{
		{nil, "Apple iPhone 14", "", "", true}, {nil, "case", "case-iphone", "", true}, {nil, "Headphone", "", "", false},
		{map[string]any{"requires_imei": false}, "iPhone", "", "", false},
		{map[string]any{"requires_imei": true, "device_type": "tablet"}, "", "", "", true},
		{map[string]any{"device_type": "IPHONE"}, "", "", "", true},
		{map[string]any{"device_type": "tablet"}, "iPhone", "", "", false},
	} {
		if RequiresRadio(c.metadata, c.title, c.handle, c.kind) != c.want {
			t.Fatal("source product precedence changed")
		}
	}
}
