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

// Item fields are strict canonical strings. Never decode price, flags,
// metadata, customer/store, line IDs or origin from a browser-selected object.
func decodeItem(reader io.Reader) (AddItemCommand, error) {
	d := json.NewDecoder(reader)
	var command AddItemCommand
	token, err := d.Token()
	if err != nil {
		return command, err
	}
	if token != json.Delim('{') {
		return command, ErrInput
	}
	fields := map[string]string{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return command, err
		}
		name, ok := key.(string)
		if !ok || (name != "version" && name != "variant_id" && name != "quantity") {
			return command, ErrInput
		}
		if _, exists := fields[name]; exists {
			return command, ErrInput
		}
		var value *string
		if err := d.Decode(&value); err != nil {
			return command, err
		}
		if value == nil {
			return command, ErrInput
		}
		fields[name] = *value
	}
	token, err = d.Token()
	if err != nil {
		return command, err
	}
	if token != json.Delim('}') || len(fields) != 3 {
		return command, ErrInput
	}
	if err := d.Decode(new(any)); err != io.EOF {
		if err != nil {
			return command, err
		}
		return command, ErrInput
	}
	command.Version, command.VariantID, command.Quantity = fields["version"], fields["variant_id"], fields["quantity"]
	return command, nil
}

func (h *HTTPHandler) AddItem(c *gin.Context) {
	if c.Request.URL.RawQuery != "" || !idPattern.MatchString(c.Param("id")) {
		cartHTTPError(c, ErrInput)
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 415, "cart.content_type", "JSON required")
		return
	}
	command, err := decodeItem(c.Request.Body)
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
	if !h.allow(c, ctx, "item-possession", token) {
		return
	}
	result, err := AddItem(ctx, h.db, h.store, ref, command)
	if err != nil {
		cartHTTPError(c, err)
		return
	}
	// A stable operation receipt, not a newly authorized checkout total. No
	// new cookie is issued and a replay does not silently refresh credentials.
	c.JSON(200, result)
}
