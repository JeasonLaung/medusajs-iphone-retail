package warehouseusers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
)

func TestStrictWarehouseUserCommand(t *testing.T) {
	c := Command{OperationID: "3b763111-96f8-4e68-b267-c1424f68ee58", ExpectedRevision: "1", Name: "仓库用户", Email: "A@example.invalid", Password: "artificial-only-password", Enabled: false, Mode: "selected", Locations: []string{}, Actions: []authorization.Action{authorization.InventoryRead}}
	raw, _ := json.Marshal(c)
	out, e := decode(raw)
	if e != nil || out.Email != "a@example.invalid" || out.Enabled {
		t.Fatal("valid command rejected")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"enabled":false`, `"enabled":false,"enabled":true`, 1),
		strings.Replace(string(raw), `"enabled":false`, `"enabled":null`, 1),
		strings.Replace(string(raw), `"locations":[]`, `"locations":[null]`, 1),
		strings.Replace(string(raw), `"locations":[]`, `"locations":null`, 1),
		strings.Replace(string(raw), `"mode":"selected"`, `"mode":"other"`, 1),
		strings.Replace(string(raw), `"mode":"selected"`, `"mode":"all","unknown":true`, 1),
		strings.Replace(string(raw), `"inventory.read"`, `"order.read"`, 1),
		string(raw) + ` {}`,
		strings.Replace(string(raw), `"expected_revision":"1"`, `"expected_revision":"01"`, 1),
		strings.Replace(string(raw), `"locations":[]`, `"locations":["a","a"]`, 1),
		strings.Replace(string(raw), `"actions":["inventory.read"]`, `"actions":["inventory.read","inventory.read"]`, 1),
		"",
	} {
		if _, e := decode([]byte(bad)); e == nil {
			t.Fatal("invalid command accepted")
		}
	}
	c.Mode = "all"
	c.Locations = []string{"a"}
	if _, e := normalize(c); e == nil {
		t.Fatal("ambiguous all scope accepted")
	}
	c.Mode = "selected"
	c.Locations = []string{}
	c.Password = strings.Repeat("仓", 25)
	if _, e := normalize(c); e == nil {
		t.Fatal("password byte limit bypassed")
	}
}
