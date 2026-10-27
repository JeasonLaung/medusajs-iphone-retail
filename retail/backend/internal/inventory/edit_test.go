package inventory

import (
	"strings"
	"testing"
)

func TestStockBoundary(t *testing.T) {
	const valid = `{"operation_id":"8f8cbbd0-77d0-4c5a-9a3a-0b99111c0446","expected_revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","stocked_quantity":"9007199254740993.000000000000000001","incoming_quantity":"0.000000000000000000"}`
	if _, err := decodeStock(strings.NewReader(valid)); err != nil {
		t.Fatal("exact quantity rejected")
	}
	for _, bad := range []string{
		strings.Replace(valid, `"incoming_quantity":"0.000000000000000000"`, `"incoming_quantity":"0.000000000000000000","incoming_quantity":"1.000000000000000000"`, 1),
		strings.Replace(valid, `"incoming_quantity":"0.000000000000000000"`, `"reserved_quantity":"0.000000000000000000"`, 1),
		strings.Replace(valid, `"0.000000000000000000"`, `0`, 1),
		strings.Replace(valid, `"0.000000000000000000"`, `null`, 1),
		strings.Replace(valid, `"0.000000000000000000"`, `"0"`, 1),
		strings.Replace(valid, `"0.000000000000000000"`, `"-1.000000000000000000"`, 1),
		strings.Replace(valid, `"0.000000000000000000"`, `"0.0000000000000000001"`, 1),
		strings.Replace(valid, `"0.000000000000000000"`, `"100000000000000000000.000000000000000000"`, 1),
		strings.Replace(valid, `"0.000000000000000000"`, `"1e3"`, 1),
		valid + `{}`, strings.Replace(valid, "8f8cbbd0", "8F8CBBD0", 1),
	} {
		if _, err := decodeStock(strings.NewReader(bad)); err == nil {
			t.Fatal("unsafe stock input accepted")
		}
	}
}
