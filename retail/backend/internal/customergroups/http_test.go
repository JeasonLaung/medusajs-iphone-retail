package customergroups

import (
	"strings"
	"testing"
)

func TestDecodeCommandBoundaries(t *testing.T) {
	valid := `{"operation_id":"11111111-1111-4111-8111-111111111111","expected_revision":"1","kind":"create","group_id":"","name":"Verified customers 客户","customer_ids":[]}`
	if _, err := decode(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"duplicate":             strings.Replace(valid, `"kind":"create"`, `"kind":"delete","kind":"create"`, 1),
		"unknown":               strings.Replace(valid, `"kind":"create"`, `"kind":"create","actor_id":"admin"`, 1),
		"null":                  strings.Replace(valid, `"customer_ids":[]`, `"customer_ids":null`, 1),
		"missing":               strings.Replace(valid, `,"customer_ids":[]`, ``, 1),
		"wrong type":            strings.Replace(valid, `"expected_revision":"1"`, `"expected_revision":1`, 1),
		"revision leading zero": strings.Replace(valid, `"expected_revision":"1"`, `"expected_revision":"01"`, 1),
		"revision overflow":     strings.Replace(valid, `"expected_revision":"1"`, `"expected_revision":"18446744073709551615"`, 1),
		"create with members":   strings.Replace(valid, `"customer_ids":[]`, `"customer_ids":["new_account"]`, 1),
		"padded name":           strings.Replace(valid, `Verified customers 客户`, ` Verified customers 客户`, 1),
		"trailing":              valid + `{}`,
		"oversized":             strings.Repeat(" ", 65537) + valid,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decode(strings.NewReader(body)); err == nil {
				t.Fatal("invalid command accepted")
			}
		})
	}
}
