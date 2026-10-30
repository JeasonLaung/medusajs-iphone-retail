package customer

import (
	"strings"
	"testing"
)

func TestAfterSalesStrictWire(t *testing.T) {
	good := `{"operation_id":"f9adc90d-1b76-46b3-8f1c-36c83c69d627","type":"return","reason":"Please review this item.","items":[{"item_id":"line","quantity":1}]}`
	if _, e := decodeAfterSales([]byte(good)); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{
		good + `{}`, strings.Replace(good, `"type":"return"`, `"type":"return","type":"return"`, 1),
		strings.Replace(good, `"quantity":1`, `"quantity":1,"quantity":1`, 1),
		strings.Replace(good, `"quantity":1`, `"quantity":1,"private":"leak"`, 1),
		strings.Replace(good, `"quantity":1`, `"quantity":1,"title":""`, 1),
		strings.Replace(good, `"quantity":1`, `"quantity":null`, 1), strings.Replace(good, `"quantity":1`, `"quantity":1.0`, 1),
		strings.Replace(good, `"quantity":1`, `"quantity":1e0`, 1), strings.Replace(good, `"quantity":1`, `"quantity":10001`, 1),
		strings.Replace(good, `"quantity":1`, `"quantity":1,"device_ids":[]`, 1),
		strings.Replace(good, `"quantity":1`, `"quantity":1,"device_ids":["d","d"]`, 1),
		strings.Replace(good, `"items":[`, `"items":null,"ignored":[`, 1), strings.Replace(good, `"reason":"Please review this item."`, `"reason":"short"`, 1),
		strings.Replace(good, `"type":"return"`, `"type":"refund"`, 1),
		strings.Replace(good, `"reason":"Please review this item."`, `"reason":"Please\u0000review this item."`, 1),
	} {
		if _, e := decodeAfterSales([]byte(bad)); e == nil {
			t.Fatal("invalid after-sales input accepted", bad)
		}
	}
}
