package notificationmail

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (s Store) listInbox(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	query := c.Request.URL.Query()
	if len(query) != 3 || len(query["account_id"]) != 1 || len(query["folder"]) != 1 || len(query["offset"]) != 1 {
		fail(c, ErrInput)
		return
	}
	offset, e := strconv.Atoi(query.Get("offset"))
	if e != nil {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	messages, e := s.Inbox(ctx, user, query.Get("account_id"), query.Get("folder"), offset)
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, gin.H{"messages": messages, "limit": 50, "offset": offset})
}
func (s Store) syncInbox(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 4))
	if e != nil || string(bytes.TrimSpace(raw)) != "{}" {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	out, e := s.SyncInbox(ctx, user, c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, out)
}

type MessageDetail struct {
	Message     InboxMessage       `json:"message"`
	Text        string             `json:"text"`
	HTML        string             `json:"html"`
	Attachments []AttachmentDetail `json:"attachments"`
}
type AttachmentDetail struct {
	Index    int    `json:"index"`
	Filename string `json:"filename"`
}

func parsedMessage(raw []byte) (string, string, []AttachmentDetail) {
	text, html := "", ""
	attachments := []AttachmentDetail{}
	reader, e := mail.CreateReader(bytes.NewReader(raw))
	if e != nil {
		return "(Original message available for download)", "", attachments
	}
	defer reader.Close()
	for index := 0; index < 100; index++ {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			break
		}
		switch header := part.Header.(type) {
		case *mail.InlineHeader:
			kind, _, _ := header.ContentType()
			body, e := io.ReadAll(io.LimitReader(part.Body, 1<<20))
			if e != nil {
				continue
			}
			if kind == "text/plain" && len(text) < 1<<20 {
				text += strings.ToValidUTF8(string(body), "�")
			}
			if kind == "text/html" && len(html) < 1<<20 {
				html += strings.ToValidUTF8(string(body), "�")
			}
		case *mail.AttachmentHeader:
			filename, _ := header.Filename()
			attachments = append(attachments, AttachmentDetail{Index: index, Filename: strings.ToValidUTF8(filename, "�")})
		}
	}
	return text, html, attachments
}
func (s Store) readInbox(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	message, raw, e := s.MessageRaw(ctx, user, c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	defer clear(raw)
	text, html, attachments := parsedMessage(raw)
	c.JSON(200, MessageDetail{Message: message, Text: text, HTML: html, Attachments: attachments})
}
func (s Store) downloadInbox(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	_, raw, e := s.MessageRaw(ctx, user, c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	defer clear(raw)
	c.Header("Content-Disposition", `attachment; filename="message.eml"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/octet-stream", raw)
}

func (s Store) downloadInboxAttachment(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	index, e := strconv.Atoi(c.Param("index"))
	if e != nil || index < 0 || index >= 100 || c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	_, raw, e := s.MessageRaw(ctx, user, c.Param("id"))
	if e != nil {
		fail(c, e)
		return
	}
	defer clear(raw)
	reader, e := mail.CreateReader(bytes.NewReader(raw))
	if e != nil {
		fail(c, ErrInput)
		return
	}
	defer reader.Close()
	for partIndex := 0; partIndex <= index; partIndex++ {
		part, e := reader.NextPart()
		if e != nil {
			fail(c, ErrInput)
			return
		}
		if partIndex != index {
			continue
		}
		header, ok := part.Header.(*mail.AttachmentHeader)
		if !ok {
			fail(c, ErrInput)
			return
		}
		filename, _ := header.Filename()
		filename = strings.ReplaceAll(strings.ReplaceAll(filename, "\\", "_"), "/", "_")
		if filename == "" {
			filename = "attachment"
		}
		body, e := io.ReadAll(io.LimitReader(part.Body, 26<<20+1))
		if e != nil || len(body) > 26<<20 {
			fail(c, ErrUnavailable)
			return
		}
		defer clear(body)
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(200, "application/octet-stream", body)
		return
	}
}
func (s Store) updateInbox(c *gin.Context) {
	user, ok := actor(c)
	if !ok {
		return
	}
	if c.Request.URL.RawQuery != "" {
		fail(c, ErrInput)
		return
	}
	var command struct{ Read, Archived, Deleted bool }
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 1025))
	if e != nil || len(raw) > 1024 || !uniqueTransportFields(raw) {
		fail(c, ErrInput)
		return
	}
	// Set absolute state so a lost success response can be replayed safely.
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 {
		fail(c, ErrInput)
		return
	}
	for _, key := range []string{"read", "archived", "deleted"} {
		if !bytes.Equal(fields[key], []byte("true")) && !bytes.Equal(fields[key], []byte("false")) {
			fail(c, ErrInput)
			return
		}
	}
	if json.Unmarshal(raw, &command) != nil {
		fail(c, ErrInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, user, authorization.MailWrite); e != nil {
			return e
		}
		var message InboxMessage
		if tx.Table("notification_mail_inbox").Where("id=?", c.Param("id")).Take(&message).Error != nil {
			return ErrInput
		}
		if _, e := s.activeMailbox(ctx, tx, user, message.AccountID, authorization.MailWrite); e != nil {
			return e
		}
		var readAt any
		if command.Read {
			if message.ReadAt != nil {
				readAt = message.ReadAt
			} else {
				readAt = time.Now().UTC().Truncate(time.Microsecond)
			}
		}
		return tx.Table("notification_mail_inbox").Where("id=?", message.ID).Updates(map[string]any{"read_at": readAt, "archived": command.Archived, "deleted": command.Deleted}).Error
	})
	if e != nil {
		fail(c, e)
		return
	}
	c.JSON(200, gin.H{"saved": true})
}
