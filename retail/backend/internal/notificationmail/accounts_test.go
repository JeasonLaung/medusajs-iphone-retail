package notificationmail

import (
	"encoding/json"
	"github.com/google/uuid"
	"testing"
)

func TestStrictMailboxInput(t *testing.T) {
	in := Command{OperationID: uuid.NewString(), ExpectedRevision: "1", Name: "EuroRefurb 中文", Email: "sender@example.invalid", APIKey: "re_artificial_key", Enabled: true, Registration: true}
	raw, _ := json.Marshal(in)
	if _, e := decodeCommand(raw); e != nil {
		t.Fatal("valid command rejected")
	}
	for _, bad := range []string{`{}`, `null`, `{"enabled":true,"enabled":false}`, string(raw) + ` {}`, `{"api_key":null}`, `{"enabled":"true"}`, `{"other":"secret"}`} {
		if _, e := decodeCommand([]byte(bad)); e == nil {
			t.Fatal("malformed command accepted")
		}
	}
	for _, change := range []func(*Command){func(c *Command) { c.Email = "Named <sender@example.invalid>" }, func(c *Command) { c.ExpectedRevision = "01" }, func(c *Command) { c.ID = "legacy" }, func(c *Command) { c.Enabled = false }, func(c *Command) { c.APIKey = "invalid" }, func(c *Command) { c.WebhookSecret = "bad\nsecret" }} {
		v := in
		change(&v)
		if _, e := normalize(v); e == nil {
			t.Fatal("invalid mailbox accepted")
		}
	}
}
