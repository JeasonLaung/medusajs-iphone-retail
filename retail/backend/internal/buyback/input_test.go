package buyback

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictPricingInputs(t *testing.T) {
	rules, _ := json.Marshal(testRules())
	if _, e := ParseRules(rules); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{
		strings.Replace(string(rules), `"enabled":true`, `"enabled":true,"enabled":false`, 1),
		strings.Replace(string(rules), `"enabled":true`, `"enabled":null`, 1),
		strings.Replace(string(rules), `"version":1,`, "", 1),
		strings.Replace(string(rules), `"damage_fee":13.98`, `"damage_fee":"13.98"`, 1),
		strings.Replace(string(rules), `"LIKE_NEW":294`, `"LIKE_NEW":294,"LIKE_NEW":295`, 1),
		strings.TrimSuffix(string(rules), "}") + `,"source":{}}`,
		strings.TrimSuffix(string(rules), "}") + `,"source":{"name":"x","country":"dk","fetched_at":"x","secret":"x"}}`,
		strings.TrimSuffix(string(rules), "}") + `,"Enabled":false}`,
		strings.Replace(string(rules), `"LIKE_NEW":294`, `"LIKE_NEW":"294"`, 1),
		string(rules) + ` {}`, `null`, `[]`, string([]byte{0xff}),
	} {
		if _, e := ParseRules([]byte(raw)); e == nil {
			t.Fatalf("accepted rules %s", raw)
		}
	}
	good := `{"product_id":"phone","capacity":"1024GB","assessment":{"battery":"","checks":[null,null,null,null,null],"faults":[],"glass":[],"screen":"","frame":"","back":""}}`
	in, e := ParseEstimate([]byte(good))
	if e != nil || in.Capacity != "1TB" {
		t.Fatal(in, e)
	}
	for _, raw := range []string{
		strings.Replace(good, `"battery":""`, `"battery":"101"`, 1),
		strings.Replace(good, `"battery":""`, `"battery":"","Battery":"90"`, 1),
		strings.Replace(good, `"battery":""`, `"battery":null`, 1),
		strings.Replace(good, `"battery":"",`, "", 1),
		strings.Replace(good, `"checks":[null,null,null,null,null]`, `"checks":[true,false]`, 1),
		strings.Replace(good, `"glass":[]`, `"glass":[null]`, 1),
		strings.Replace(good, `"glass":[]`, `"glass":null`, 1),
		strings.Replace(good, `"screen":""`, `"screen":"damaged"`, 1),
		strings.Replace(good, `"product_id":"phone"`, `"product_id":"phone","amount":999`, 1),
		strings.Replace(good, `"product_id":"phone"`, `"product_id":"phone","product_id":"other"`, 1),
		strings.Replace(good, `"capacity":"1024GB"`, `"capacity":"1tb"`, 1),
		strings.Replace(good, `"battery":""`, `"battery":"","internal":true`, 1),
	} {
		if _, e := ParseEstimate([]byte(raw)); e == nil {
			t.Fatalf("accepted estimate %s", raw)
		}
	}
}
