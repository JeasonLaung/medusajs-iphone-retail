package devices

import (
	"reflect"
	"testing"
)

func TestWarehouseVariantSearchTerms(t *testing.T) {
	for _, v := range []struct {
		q    string
		want []string
	}{{" iPhone 14 Pro Max 128GB 蓝色 B 85% ", []string{"i14promax", "128gb", "蓝色", "b", "85%"}}, {"ｉ１４ｐｍ １ＴＢ", []string{"i14pm", "1tb"}}, {"i13p", []string{"i13p"}}, {"", []string{}}} {
		if got := warehouseSearchTerms(v.q); !reflect.DeepEqual(got, v.want) {
			t.Fatalf("search terms %q: %#v", v.q, got)
		}
	}
}
