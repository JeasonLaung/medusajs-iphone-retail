package cart

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestContextInputBoundary(t *testing.T) {
	valid := ContextCommand{OperationID: uuid.NewString(), Version: "1", RegionID: "region-eu", Locale: "zh-CN"}
	if _, e := validateContext(valid); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*ContextCommand){
		func(c *ContextCommand) { c.OperationID = "chosen" }, func(c *ContextCommand) { c.OperationID = strings.ToUpper(c.OperationID) },
		func(c *ContextCommand) { c.Version = "01" }, func(c *ContextCommand) { c.Version = "18446744073709551615" }, func(c *ContextCommand) { c.Version = "0" },
		func(c *ContextCommand) { c.RegionID = "foreign/region" }, func(c *ContextCommand) { c.RegionID = "" }, func(c *ContextCommand) { c.Locale = "" }, func(c *ContextCommand) { c.Locale = "zh_CN" }, func(c *ContextCommand) { c.Locale = "en\n" },
	} {
		in := valid
		change(&in)
		if _, e := validateContext(in); e != ErrInput {
			t.Fatal("invalid context accepted", in)
		}
	}
	body := `{"version":"1","region_id":"region-eu","locale":"zh-CN"}`
	in, e := decodeContext(strings.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	in.OperationID = valid.OperationID
	if in != valid {
		t.Fatal("context decoder changed intent")
	}
	for _, bad := range []string{
		`null`, `[]`, body + ` {}`, strings.Replace(body, `"version":"1"`, `"version":"1","version":"2"`, 1),
		strings.Replace(body, `"locale":"zh-CN"`, `"locale":null`, 1), strings.Replace(body, `"version":"1"`, `"version":1`, 1),
		strings.Replace(body, `"region_id":"region-eu"`, `"region_id":{"id":"region-eu"}`, 1),
		strings.Replace(body, `"locale":"zh-CN"`, `"currency_code":"usd","locale":"zh-CN"`, 1),
	} {
		if _, e := decodeContext(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid JSON accepted", bad)
		}
	}
}
