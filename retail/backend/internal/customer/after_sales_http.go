package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

// Preserve nested request types while rejecting duplicates at every depth.
func decodeAfterSales(body []byte) (AfterSalesInput, error) {
	var out AfterSalesInput
	if len(body) > 16<<10 || !utf8.Valid(body) {
		return out, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		if depth > 6 {
			return nil, ErrInvalid
		}
		t, e := d.Token()
		if e != nil || t == nil {
			return nil, ErrInvalid
		}
		if t == json.Delim('{') {
			m := map[string]any{}
			for d.More() {
				k, e := d.Token()
				key, ok := k.(string)
				if e != nil || !ok {
					return nil, ErrInvalid
				}
				if _, ok = m[key]; ok {
					return nil, ErrInvalid
				}
				v, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				m[key] = v
			}
			if t, e = d.Token(); e != nil || t != json.Delim('}') {
				return nil, ErrInvalid
			}
			return m, nil
		}
		if t == json.Delim('[') {
			a := []any{}
			for d.More() {
				if len(a) >= 100 {
					return nil, ErrInvalid
				}
				v, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			if t, e = d.Token(); e != nil || t != json.Delim(']') {
				return nil, ErrInvalid
			}
			return a, nil
		}
		switch t.(type) {
		case string, json.Number, bool:
			return t, nil
		}
		return nil, ErrInvalid
	}
	v, e := read(0)
	if e != nil {
		return out, e
	}
	if _, e = d.Token(); e != io.EOF {
		return out, ErrInvalid
	}
	root, ok := v.(map[string]any)
	if !ok || len(root) != 4 {
		return out, ErrInvalid
	}
	lines, ok := root["items"].([]any)
	if !ok {
		return out, ErrInvalid
	}
	for _, raw := range lines {
		l, ok := raw.(map[string]any)
		if !ok || l["item_id"] == nil || l["quantity"] == nil {
			return out, ErrInvalid
		}
		if raw, present := l["device_ids"]; present {
			a, ok := raw.([]any)
			if !ok || len(a) == 0 {
				return out, ErrInvalid
			}
		}
		if _, present := l["title"]; present {
			return out, ErrInvalid
		}
	}
	encoded, _ := json.Marshal(v)
	typed := json.NewDecoder(bytes.NewReader(encoded))
	typed.DisallowUnknownFields()
	if typed.Decode(&out) != nil || !validAfterSales(out) {
		return AfterSalesInput{}, ErrInvalid
	}
	return out, nil
}
func (h *HTTPHandler) AfterSales(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.store.AfterSales(ctx, token, c.Param("id"))
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, gin.H{"requests": out})
}
func (h *HTTPHandler) OrderDevices(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := h.store.OrderDevices(ctx, token, c.Param("id"))
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(200, gin.H{"devices": out})
}
func (h *HTTPHandler) CreateAfterSales(c *gin.Context) {
	if !h.originOK(c, true) {
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "customer.content_type", "JSON required")
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	if e != nil {
		platform.InvalidJSON(c, e, "customer.invalid_request", "Valid customer JSON required")
		return
	}
	in, e := decodeAfterSales(body)
	if e != nil {
		h.fail(c, e)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	out, e := h.store.CreateAfterSales(ctx, token, c.Param("id"), in)
	if e != nil {
		h.fail(c, e)
		return
	}
	c.JSON(201, gin.H{"request": out})
}
