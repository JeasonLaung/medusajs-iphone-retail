package catalog

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestVariantCreationInput(t *testing.T) {
	in := VariantCreateInput{uuid.NewString(), strings.Repeat("a", 64), "128 GB / Black", "", []string{"storage-128", "black"}}
	b, _ := json.Marshal(in)
	if _, e := decodeVariantCreate(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{
		strings.Replace(string(b), `"sku":""`, `"sku":null`, 1),
		strings.Replace(string(b), `"sku":""`, `"sku":"","sku":"x"`, 1),
		strings.Replace(string(b), `"sku":""`, `"sku":"","unknown":1`, 1),
		strings.Replace(string(b), `["storage-128","black"]`, `["black","black"]`, 1),
		strings.Replace(string(b), `["storage-128","black"]`, `null`, 1),
		string(b) + `{}`, string(b) + string([]byte{255}), strings.Repeat(" ", 262145),
	} {
		if _, e := decodeVariantCreate(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid variant accepted")
		}
	}
}
func TestERPReservationAndInventoryTitle(t *testing.T) {
	for s, want := range map[string]int64{"ERP10001": 1, "ERP10000": 0, "ERP99999": 89999, "ERP00010001": 1, "erp10002": 0, "ERP1": 0, "ERP9999999999999999999": 0} {
		if reservedSKUNumber(s) != want {
			t.Fatal("ERP reserve", s)
		}
	}
	for _, pair := range [][3]string{{"iPhone", "128 GB", "iPhone / 128 GB"}, {" iPhone ", "iPhone", "iPhone"}, {"iPhone", "iPhone / 128 GB", "iPhone / 128 GB"}} {
		if inventoryCreateTitle(pair[0], pair[1]) != pair[2] {
			t.Fatal("inventory title")
		}
	}
}
