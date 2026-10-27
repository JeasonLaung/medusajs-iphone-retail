package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEditRejectsAmbiguousOrLossyInput(t *testing.T) {
	valid := `{"operation_id":"12345678-1234-4234-8234-123456789abc","expected_revision":"` + strings.Repeat("a", 64) + `","title":"Phone","subtitle":null,"description":null,"status":"draft"}`
	if _, err := decodeEdit(strings.NewReader(valid)); err != nil {
		t.Fatal("valid edit rejected")
	}
	for _, raw := range []string{
		strings.Replace(valid, `"title":"Phone"`, `"title":"Phone","title":"Other"`, 1),
		strings.Replace(valid, `"title":"Phone"`, `"title":null`, 1),
		strings.Replace(valid, `"title":"Phone"`, `"title":"\ud800"`, 1),
		strings.Replace(valid, `"title":"Phone"`, `"title":"`+string([]byte{0xff})+`"`, 1),
		strings.Replace(valid, `"subtitle":null`, `"subtitle":123`, 1),
		strings.Replace(valid, `"title":"Phone"`, `"title":"\u0000"`, 1),
		strings.Replace(valid, `"title":"Phone"`, `"title":"   "`, 1),
		strings.Replace(valid, `"status":"draft"`, `"status":"deleted"`, 1),
		strings.Replace(valid, `"status":"draft"`, `"status":"draft","metadata":{}`, 1),
		strings.Replace(valid, `"subtitle":null,`, "", 1), valid + ` {}`,
	} {
		if _, err := decodeEdit(strings.NewReader(raw)); err == nil {
			t.Fatal("ambiguous, lossy or invalid edit accepted")
		}
	}
	var in EditInput
	if json.Unmarshal([]byte(valid), &in) != nil {
		t.Fatal("fixture invalid")
	}
	in.Title = string([]byte{0xff})
	if validateEdit(in) == nil {
		t.Fatal("library accepted invalid UTF-8")
	}
}
