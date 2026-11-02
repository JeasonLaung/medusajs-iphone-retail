package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPolicyCommandSourceValidation(t *testing.T) {
	const wrapper = `{"operation_id":"123e4567-e89b-42d3-a456-426614174000","expected_revision":"` + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + `","policies":POLICIES}`
	valid := `{"warranty_months":24,"return_days":14,"warranty_note":{"en":"  note  ","zh":"  中文  ","ignored":true},"return_note":{"en":"","zh":""}}`
	parse := func(s string) (PolicyCommand, error) {
		return decodePolicyCommand(strings.NewReader(strings.Replace(wrapper, "POLICIES", s, 1)))
	}
	got, e := parse(valid)
	if e != nil || got.Policies.WarrantyNote.EN != "note" || got.Policies.WarrantyNote.ZH != "中文" {
		t.Fatal("source trim and nested stripping")
	}
	for _, s := range []string{strings.Replace(valid, ":24", ":0", 1), strings.Replace(valid, ":24", ":120", 1), strings.Replace(valid, ":24", ":2.4e1", 1), strings.Replace(valid, "\"return_days\":14", "\"return_days\":365", 1)} {
		if _, e := parse(s); e != nil {
			t.Fatal("valid source integer rejected")
		}
	}
	for _, s := range []string{
		strings.Replace(valid, ":24", ":121", 1), strings.Replace(valid, ":24", ":-1", 1), strings.Replace(valid, ":24", ":1.5", 1), strings.Replace(valid, ":24", `:"24"`, 1), strings.Replace(valid, ":24", ":null", 1),
		strings.Replace(valid, "\"return_days\":14", "\"return_days\":366", 1),
		strings.Replace(valid, `"en":"  note  "`, `"en":null`, 1), strings.Replace(valid, `"zh":"  中文  "`, `"zh":42`, 1),
		strings.Replace(valid, `"en":"  note  "`, `"en":"`+strings.Repeat("😀", 101)+`"`, 1),
		strings.Replace(valid, `"warranty_months":24`, `"warranty_months":24,"warranty_months":24`, 1),
		strings.Replace(valid, `"en":"  note  "`, `"en":"a","en":"a"`, 1),
		strings.Replace(valid, `"return_days":14`, `"extra":true,"return_days":14`, 1),
	} {
		if _, e := parse(s); e == nil {
			t.Fatal("invalid command accepted")
		}
	}
	got.Policies.WarrantyNote.EN = strings.Repeat("😀", 100)
	raw, _ := json.Marshal(got)
	if _, e := decodePolicyCommand(strings.NewReader(string(raw))); e != nil {
		t.Fatal("exact UTF16 boundary rejected")
	}
	if _, e := decodePolicyCommand(strings.NewReader(strings.Replace(string(raw), `"operation_id":`, `"operation_id":null,"operation_id":`, 1))); e == nil {
		t.Fatal("duplicate operation accepted")
	}
}
