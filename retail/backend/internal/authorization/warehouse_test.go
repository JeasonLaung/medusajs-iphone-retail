package authorization

import (
	"strings"
	"testing"
)

func TestWarehouseLocationIdentifier(t *testing.T) {
	for _, id := range []string{"a", "sloc_123", "仓库-一", strings.Repeat("仓", 191)} {
		if !warehouseLocationID(id) {
			t.Fatal("source ID rejected")
		}
	}
	for _, id := range []string{"", " a", "a ", "a/b", `a\b`, "a?x", "a#x", "a\n", string([]byte{0xff}), strings.Repeat("仓", 192)} {
		if warehouseLocationID(id) {
			t.Fatal("unsafe location ID accepted")
		}
	}
}
