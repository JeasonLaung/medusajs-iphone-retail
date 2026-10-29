package inventory

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestCreateLevelStrictInput(t *testing.T) {
	in := CreateLevelInput{uuid.NewString(), strings.Repeat("a", 64), "warehouse", "1.000000000000000001", "0.000000000000000000"}
	b, _ := json.Marshal(in)
	if _, e := decodeCreateLevel(strings.NewReader(string(b))); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{string(b) + `{}`, strings.Replace(string(b), `"location_id":"warehouse"`, `"location_id":null`, 1), strings.Replace(string(b), `"location_id":"warehouse"`, `"location_id":"warehouse","location_id":"other"`, 1), strings.Replace(string(b), `"location_id":"warehouse"`, `"location_id":"warehouse","unknown":"x"`, 1), strings.Replace(string(b), `"1.000000000000000001"`, `1`, 1), strings.Replace(string(b), `1.000000000000000001`, `-1.000000000000000000`, 1), string(b) + string([]byte{255}), strings.Repeat(" ", 4097)} {
		if _, e := decodeCreateLevel(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid create level accepted")
		}
	}
}
