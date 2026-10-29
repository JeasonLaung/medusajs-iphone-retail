package devices

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"
)

func decodeWarehouseDeviceCommand(r io.Reader) (WarehouseDeviceCommand, error) {
	var in WarehouseDeviceCommand
	raw, e := io.ReadAll(io.LimitReader(r, 4097))
	if e != nil || len(raw) > 4096 || !utf8.Valid(raw) {
		return in, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if uniqueJSON(d) != nil || d.Decode(new(any)) != io.EOF {
		return in, ErrInput
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		return in, ErrInput
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 {
		return in, ErrInput
	}
	for _, v := range fields {
		if len(v) == 0 || v[0] != '"' {
			return in, ErrInput
		}
	}
	if !validWarehouseCommand(in) {
		return in, ErrInput
	}
	return in, nil
}
func (h Handler) WarehouseDevicesHTTP(c *gin.Context)       { h.warehouseDevicesHTTP(c, false) }
func (h Handler) WarehouseDeviceCommandHTTP(c *gin.Context) { h.warehouseDevicesHTTP(c, true) }
func (h Handler) warehouseDevicesHTTP(c *gin.Context, write bool) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	locationID, category := c.Param("location_id"), c.Param("category")
	if !write {
		query, e := url.ParseQuery(c.Request.URL.RawQuery)
		offset := 0
		if e != nil || len(query) > 2 {
			trackingFail(c, ErrInput)
			return
		}
		for k, v := range query {
			if (k != "offset" && k != "q") || len(v) != 1 {
				trackingFail(c, ErrInput)
				return
			}
		}
		if v, ok := query["offset"]; ok {
			offset, e = strconv.Atoi(v[0])
			if e != nil || strconv.Itoa(offset) != v[0] {
				trackingFail(c, ErrInput)
				return
			}
		}
		out, e := h.ReadWarehouseDevices(ctx, actor, locationID, category, query.Get("q"), offset)
		if e != nil {
			trackingFail(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	if c.Request.URL.RawQuery != "" {
		trackingFail(c, ErrInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "device.content_type", "JSON required")
		return
	}
	in, e := decodeWarehouseDeviceCommand(c.Request.Body)
	if e != nil {
		trackingFail(c, e)
		return
	}
	out, e := h.CommandWarehouseDevice(ctx, actor, locationID, category, c.Param("device_id"), in)
	if e != nil {
		trackingFail(c, e)
		return
	}
	c.JSON(200, out)
}
