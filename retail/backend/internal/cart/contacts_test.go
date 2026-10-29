package cart

import (
	"testing"

	"github.com/google/uuid"
)

func TestContactInput(t *testing.T) {
	address := ContactAddress{FirstName: "Name", LastName: "Customer", Address1: "Street", City: "City", PostalCode: "0012", Country: "dk"}
	valid := ContactCommand{OperationID: uuid.NewString(), Version: "1", Email: "new@example.invalid", Shipping: address, Billing: address}
	if _, err := validateContact(valid); err != nil {
		t.Fatal("valid contact rejected")
	}
	for _, change := range []func(*ContactCommand){
		func(c *ContactCommand) { c.Version = "01" }, func(c *ContactCommand) { c.Version = "18446744073709551615" }, func(c *ContactCommand) { c.OperationID = "chosen" },
		func(c *ContactCommand) { c.Email = "Name <new@example.invalid>" }, func(c *ContactCommand) { c.Email = " new@example.invalid" }, func(c *ContactCommand) { c.Email = "new\n@example.invalid" }, func(c *ContactCommand) { c.Email = "new\xff@example.invalid" },
		func(c *ContactCommand) { c.Shipping.Country = "DK" }, func(c *ContactCommand) { c.Billing.Country = "" }, func(c *ContactCommand) { c.Shipping.FirstName = " " }, func(c *ContactCommand) { c.Shipping.Address1 = "bad\x00street" },
	} {
		in := valid
		change(&in)
		if _, err := validateContact(in); err == nil {
			t.Fatal("invalid contact accepted")
		}
	}
}
