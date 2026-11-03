package notificationmail

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/gin-gonic/gin"
)

func (s Store) listCompositions(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	q := c.Request.URL.Query()
	if len(q) != 1 || len(q["account_id"]) != 1 {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := s.Compositions(ctx, user, q.Get("account_id"))
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, gin.H{"messages": out})
}
func (s Store) readComposition(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	row, content, e := s.Composition(ctx, user, c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, gin.H{"message": row, "content": content})
}
func (s Store) saveComposition(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 900001))
	if e != nil || len(raw) > 900000 {
		fail(c, ErrInput)
		return
	}
	defer clear(raw)
	var in struct {
		AccountID string                     `json:"account_id"`
		Revision  uint64                     `json:"revision"`
		Content   maildelivery.ManualMessage `json:"content"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	out, e := s.SaveComposition(ctx, user, c.Param("id"), in.AccountID, in.Revision, in.Content)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
func (s Store) sendComposition(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 129))
	if e != nil || len(raw) > 128 || !uniqueTransportFields(raw) {
		fail(c, ErrInput)
		return
	}
	var in struct {
		Revision uint64 `json:"revision"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || in.Revision == 0 || decoder.Decode(new(any)) != io.EOF {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	out, e := s.SendComposition(ctx, user, c.Param("id"), in.Revision)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}
