package cart

import (
	"strings"
	"testing"
)

func TestCartHTTPInput(t *testing.T) {
	for _, body := range []string{
		`{"region_id":"region","locale":"zh-CN"}`,
		`{"region_id":"region"}`,
		`{"locale":"","region_id":"region"}`,
	} {
		if _, _, err := decodeHTTP(strings.NewReader(body), true); err != nil {
			t.Fatal("valid cart creation JSON rejected")
		}
	}
	for _, body := range []string{
		``, `{}`, `null`, `[]`, `{"region_id":null}`, `{"region_id":"region","locale":null}`,
		`{"region_id":17}`, `{"region_id":"region","region_id":"other"}`,
		`{"region_id":"region","locale":"en","locale":"zh-CN"}`,
		`{"region_id":"region","price":"0"}`, `{"region_id":"region","customer_id":"old"}`,
		`{"region_id":"region","scope_hash":"chosen"}`, `{"region_id":"region","store_id":"chosen"}`,
		`{"region_id":"region"} {}`, `{"region_id":"region/id"}`, `{"region_id":"region","locale":"en_US"}`,
	} {
		if _, _, err := decodeHTTP(strings.NewReader(body), true); err == nil {
			t.Fatal("invalid cart creation JSON accepted")
		}
	}
	if _, _, err := decodeHTTP(strings.NewReader(`{}`), false); err != nil {
		t.Fatal("empty scope request rejected")
	}
	for _, body := range []string{`{"region_id":"region"}`, `null`, `[]`, `{} {}`} {
		if _, _, err := decodeHTTP(strings.NewReader(body), false); err == nil {
			t.Fatal("scope request accepted creation or trailing data")
		}
	}
}
