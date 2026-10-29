package notificationmail

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"strconv"
	"time"
)

func (s Store) orderJobs(c *gin.Context) {
	q := c.Request.URL.Query()
	offset := 0
	if len(q) > 1 || len(q) > 0 && (len(q["offset"]) != 1) {
		fail(c, ErrInput)
		return
	}
	if raw := q.Get("offset"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || strconv.Itoa(n) != raw {
			fail(c, ErrInput)
			return
		}
		offset = n
	} else if len(q) > 0 {
		fail(c, ErrInput)
		return
	}
	user, ok := actor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := s.OrderJobs(ctx, user, offset)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
func (s Store) retryOrderJob(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "unsupported_media_type", "JSON body required")
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 1025))
	if e != nil || len(raw) > 1024 {
		platform.Fail(c, 413, "request_too_large", "Retry request too large")
		return
	}
	// Token decoding rejects duplicate fields as well as unknown/null values.
	d := json.NewDecoder(bytes.NewReader(raw))
	start, e := d.Token()
	if e != nil || start != json.Delim('{') {
		fail(c, ErrInput)
		return
	}
	seen := map[string]bool{}
	var in RetryOrderJob
	for d.More() {
		k, e := d.Token()
		name, ok := k.(string)
		if e != nil || !ok || seen[name] {
			fail(c, ErrInput)
			return
		}
		seen[name] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || string(value) == "null" {
			fail(c, ErrInput)
			return
		}
		switch name {
		case "operation_id":
			e = json.Unmarshal(value, &in.OperationID)
		case "expected_attempts":
			e = json.Unmarshal(value, &in.ExpectedAttempts)
		default:
			fail(c, ErrInput)
			return
		}
		if e != nil {
			fail(c, ErrInput)
			return
		}
	}
	if _, e = d.Token(); e != nil || len(seen) != 2 || d.Decode(new(any)) != io.EOF {
		fail(c, ErrInput)
		return
	}
	user, ok := actor(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := s.RetryOrderJob(ctx, user, c.Param("id"), in)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
