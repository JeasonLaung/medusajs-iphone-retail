package cart

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
)

// Walk both object levels so duplicate keys cannot bypass validation. No
// historical address IDs, ownership, prices or metadata are browser inputs.
func contactObject(d *json.Decoder, consume func(string) error) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return ErrInput
	}
	seen := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return ErrInput
		}
		seen[name] = true
		if err := consume(name); err != nil {
			return err
		}
	}
	token, err = d.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('}') {
		return ErrInput
	}
	return nil
}
func contactString(d *json.Decoder, out *string) error {
	var value *string
	if err := d.Decode(&value); err != nil {
		return err
	}
	if value == nil {
		return ErrInput
	}
	*out = *value
	return nil
}
func decodeContactAddress(d *json.Decoder, a *ContactAddress) error {
	return contactObject(d, func(name string) error {
		fields := map[string]*string{"first_name": &a.FirstName, "last_name": &a.LastName, "company": &a.Company, "address_1": &a.Address1, "address_2": &a.Address2, "city": &a.City, "country_code": &a.Country, "province": &a.Province, "postal_code": &a.PostalCode, "phone": &a.Phone}
		out, ok := fields[name]
		if !ok {
			return ErrInput
		}
		return contactString(d, out)
	})
}
func decodeContact(reader io.Reader) (ContactCommand, error) {
	d := json.NewDecoder(reader)
	var in ContactCommand
	err := contactObject(d, func(name string) error {
		switch name {
		case "version":
			return contactString(d, &in.Version)
		case "email":
			return contactString(d, &in.Email)
		case "shipping_address":
			return decodeContactAddress(d, &in.Shipping)
		case "billing_address":
			return decodeContactAddress(d, &in.Billing)
		default:
			return ErrInput
		}
	})
	if err != nil {
		return ContactCommand{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		if err != nil {
			return ContactCommand{}, err
		}
		return ContactCommand{}, ErrInput
	}
	return in, nil
}
func (h *HTTPHandler) Contacts(c *gin.Context) {
	if c.Request.URL.RawQuery != "" || !idPattern.MatchString(c.Param("id")) {
		cartHTTPError(c, ErrInput)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "cart.content_type", "JSON required")
		return
	}
	command, err := decodeContact(c.Request.Body)
	if err != nil {
		platform.InvalidJSON(c, err, "cart.invalid_input", "Invalid cart request")
		return
	}
	keys := c.Request.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		cartHTTPError(c, ErrInput)
		return
	}
	command.OperationID = keys[0]
	if _, err := validateContact(command); err != nil {
		cartHTTPError(c, err)
		return
	}
	if !h.ready(c) || !h.originOK(c, true) {
		return
	}
	token, valid, _ := h.cookie(c, "possession")
	if !valid {
		cartHTTPError(c, sessionstore.ErrMissing)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	ref, err := h.sessions.AuthorizeCartWrite(ctx, token, csrf(c))
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	if ref.StoreID != h.store || ref.CartID != c.Param("id") {
		cartHTTPError(c, ErrDenied)
		return
	}
	if !h.allow(c, ctx, "contact-possession", token) {
		return
	}
	result, err := SaveContacts(ctx, h.db, h.store, ref, command, h.manualProviders)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	// Receipt only: never echo addresses/email or issue/extend cookies.
	c.JSON(200, result)
}
