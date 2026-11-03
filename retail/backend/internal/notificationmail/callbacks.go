package notificationmail

// Signature contracts ported from iphone0902 relayme_callback.go and
// resend_webhook.go. External callbacks do not use browser cookie auth.
import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func validCallbackTime(raw string, now time.Time) bool {
	v, e := strconv.ParseInt(raw, 10, 64)
	if e != nil {
		return false
	}
	d := now.Sub(time.Unix(v, 0))
	return d >= -5*time.Minute && d <= 5*time.Minute
}
func verifyRelayme(method, path string, body []byte, timestamp, nonce, signature, secret string, now time.Time) bool {
	if !validCallbackTime(timestamp, now) || nonce == "" || len(nonce) > 191 || len(signature) > 256 || secret == "" {
		return false
	}
	digest := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = io.WriteString(mac, method+"\n"+path+"\n"+timestamp+"\n"+nonce+"\n"+hex.EncodeToString(digest[:]))
	got, e := base64.RawURLEncoding.DecodeString(signature)
	return e == nil && hmac.Equal(mac.Sum(nil), got)
}
func verifyResend(body []byte, id, timestamp, signatures, secret string, now time.Time) bool {
	if id == "" || len(id) > 191 || !validCallbackTime(timestamp, now) || !strings.HasPrefix(secret, "whsec_") || len(signatures) > 2048 {
		return false
	}
	key, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if e != nil {
		return false
	}
	defer clear(key)
	mac := hmac.New(sha256.New, key)
	_, _ = io.WriteString(mac, id+"."+timestamp+".")
	_, _ = mac.Write(body)
	for _, candidate := range strings.Fields(signatures) {
		parts := strings.SplitN(candidate, ",", 2)
		if len(parts) == 2 && parts[0] == "v1" {
			got, e := base64.StdEncoding.DecodeString(parts[1])
			if e == nil && hmac.Equal(mac.Sum(nil), got) {
				return true
			}
		}
	}
	return false
}

type callbackMailbox struct {
	Account storedAccount
	Config  TransportConfig
	Webhook string
}

func (s Store) callbackMailbox(tx *gorm.DB, id string) (callbackMailbox, error) {
	var out callbackMailbox
	if s.Ring == nil {
		return out, ErrUnavailable
	}
	var approved int64
	if tx.Table("notification_mail_accounts").Where("account_id=?", id).Count(&approved).Error != nil || approved != 1 {
		return out, ErrInput
	}
	if tx.Table("mail_accounts").Where("id=? AND enabled=TRUE AND deleted_at IS NULL", id).Take(&out.Account).Error != nil {
		return out, ErrInput
	}
	var held int64
	if tx.Table("migration_merchant_dispositions").Where("source_table='mail_account' AND source_id=?", id).Count(&held).Error != nil || held != 0 {
		return out, ErrHeld
	}
	c, e := s.transport(tx, id)
	if e != nil {
		return out, e
	}
	out.Config = c
	if c.Provider != "resend" || len(out.Account.WebhookSecret) == 0 {
		return out, nil
	}
	raw, e := s.Ring.Open(secretContext(id, "webhook_secret"), out.Account.WebhookSecret)
	if e != nil {
		return out, ErrUnavailable
	}
	out.Webhook = string(raw)
	clear(raw)
	return out, nil
}
func (s Store) RegisterWebhooks(router *gin.Engine) {
	router.POST("/api/webhooks/mail/:account/:provider", s.mailCallback)
}

type callbackEvent struct {
	FromAddress  string    `json:"from_address"`
	Subject      string    `json:"subject"`
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"`
	OccurredAt   time.Time `json:"occurred_at"`
	Mailbox      string    `json:"mailbox"`
	ToAddress    string    `json:"to_address"`
	SubmissionID string    `json:"submission_id"`
	MailFrom     string    `json:"mail_from"`
	Recipient    string    `json:"recipient"`
	Status       string    `json:"status"`
}

func (s Store) mailCallback(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		c.Status(400)
		return
	}
	provider, id := c.Param("provider"), c.Param("account")
	if provider != "relayme" && provider != "resend" {
		c.Status(404)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if e != nil || len(raw) > 1<<20 {
		c.Status(413)
		return
	}
	defer clear(raw)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	var box callbackMailbox
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var e error; box, e = s.callbackMailbox(tx, id); return e })
	if e != nil {
		c.Status(401)
		return
	}
	authenticated := func(current callbackMailbox) bool {
		if provider == "relayme" {
			return current.Config.Provider == "quguo" && current.Config.Relayme != nil && current.Config.Relayme.KeyID == c.GetHeader("X-Relayme-Key-ID") && verifyRelayme(c.Request.Method, c.Request.URL.EscapedPath(), raw, c.GetHeader("X-Relayme-Timestamp"), c.GetHeader("X-Relayme-Nonce"), c.GetHeader("X-Relayme-Signature"), current.Config.Relayme.Secret, time.Now())
		}
		return current.Config.Provider == "resend" && verifyResend(raw, c.GetHeader("svix-id"), c.GetHeader("svix-timestamp"), c.GetHeader("svix-signature"), current.Webhook, time.Now())
	}
	if !authenticated(box) {
		c.Status(401)
		return
	}
	event := callbackEvent{}
	if provider == "relayme" {
		if json.Unmarshal(raw, &event) != nil {
			c.Status(400)
			return
		}
	} else {
		var in struct {
			Type      string    `json:"type"`
			CreatedAt time.Time `json:"created_at"`
			Data      struct {
				EmailID string   `json:"email_id"`
				To      []string `json:"to"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &in) != nil {
			c.Status(400)
			return
		}
		event.EventID = c.GetHeader("svix-id")
		event.EventType = in.Type
		event.OccurredAt = in.CreatedAt
		event.SubmissionID = in.Data.EmailID
		if len(in.Data.To) == 1 {
			event.Recipient = in.Data.To[0]
		}
		switch in.Type {
		case "email.delivered":
			event.Status = "success"
		case "email.delivery_delayed":
			event.Status = "delayed"
		case "email.bounced", "email.failed", "email.complained", "email.suppressed":
			event.Status = "failed"
		default:
			event.Status = "observed"
		}
	}
	if event.EventID == "" || len(event.EventID) > 191 || len(event.EventType) > 64 || len(event.SubmissionID) > 191 || len(event.Recipient) > 254 || event.OccurredAt.IsZero() {
		c.Status(400)
		return
	}
	inbound := provider == "relayme" && event.EventType == ""
	if inbound {
		if event.Mailbox != "INBOX" || !strings.EqualFold(event.ToAddress, box.Account.Email) || !validAddress(event.FromAddress, false) || box.Config.IMAP == nil {
			c.Status(400)
			return
		}
	} else if provider == "relayme" {
		if event.EventType != "delivery_status" && !strings.HasPrefix(event.EventType, "remote_dsn_") || strings.HasPrefix(event.EventType, "remote_dsn_") && strings.TrimPrefix(event.EventType, "remote_dsn_") != event.Status || !strings.EqualFold(event.MailFrom, box.Account.Email) || event.SubmissionID == "" || !validAddress(event.Recipient, false) || event.Status != "success" && event.Status != "delayed" && event.Status != "failed" {
			c.Status(400)
			return
		}
	}
	digest := sha256.Sum256(raw)
	var prior struct{ BodySHA256 []byte }
	check := s.DB.WithContext(ctx).Table("notification_mail_callbacks").Where("account_id=? AND provider=? AND event_id=?", id, provider, event.EventID).Take(&prior)
	if check.Error == nil {
		if !hmac.Equal(prior.BodySHA256, digest[:]) {
			c.Status(409)
		} else {
			c.Status(204)
		}
		return
	}
	if check.Error != gorm.ErrRecordNotFound {
		c.Status(503)
		return
	}
	if inbound {
		// Import through the same durable UID cursor. The event itself is never
		// treated as authoritative message content. Retry until the existing INBOX
		// backlog has been archived, then acknowledge the callback.
		result, e := s.syncInboxAuthorized(ctx, id, func(tx *gorm.DB) (TransportConfig, error) {
			current, e := s.callbackMailbox(tx, id)
			if e != nil || !authenticated(current) {
				return TransportConfig{}, ErrConflict
			}
			return current.Config, nil
		})
		if e != nil || result.More {
			c.Status(503)
			return
		}
		var candidates []InboxMessage
		if s.DB.WithContext(ctx).Table("notification_mail_inbox m").Select("m.*").Joins("JOIN notification_mail_sync s ON s.account_id=m.account_id AND s.source_sha256=m.source_sha256 AND s.uid_validity=m.uid_validity").Where("m.account_id=? AND m.subject=? AND m.received_at BETWEEN ? AND ?", id, event.Subject, event.OccurredAt.Add(-30*time.Minute), event.OccurredAt.Add(30*time.Minute)).Limit(200).Find(&candidates).Error != nil {
			c.Status(503)
			return
		}
		found := false
		for _, candidate := range candidates {
			address, e := mail.ParseAddress(candidate.FromAddress)
			if e == nil && strings.EqualFold(address.Address, event.FromAddress) {
				var part struct{ RawEnvelope []byte }
				if s.DB.WithContext(ctx).Table("notification_mail_inbox_parts").Where("message_id=? AND part_number=0", candidate.ID).Take(&part).Error != nil {
					continue
				}
				plain, e := s.Ring.Open(inboxPartContext(candidate.ID, 0), part.RawEnvelope)
				if e != nil {
					continue
				}
				parsed, parseErr := mail.ReadMessage(strings.NewReader(string(plain)))
				clear(plain)
				if parseErr != nil {
					continue
				}
				recipients, e := mail.ParseAddressList(parsed.Header.Get("To"))
				if e != nil {
					continue
				}
				for _, recipient := range recipients {
					if strings.EqualFold(recipient.Address, box.Account.Email) {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
		if !found {
			c.Status(503)
			return
		}
	}
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, e := s.callbackMailbox(tx, id)
		if e != nil || !authenticated(current) {
			return ErrConflict
		}
		row := map[string]any{"account_id": id, "provider": provider, "event_id": event.EventID, "body_sha256": digest[:], "event_type": event.EventType, "provider_id": event.SubmissionID, "recipient": event.Recipient, "delivery_status": event.Status, "occurred_at": event.OccurredAt.UTC(), "created_at": time.Now().UTC()}
		inserted := tx.Table("notification_mail_callbacks").Clauses(clause.OnConflict{DoUpdates: clause.Assignments(map[string]any{"event_id": gorm.Expr("event_id")})}).Create(row)
		if inserted.Error != nil {
			return inserted.Error
		}
		{
			var existing struct{ BodySHA256 []byte }
			if tx.Table("notification_mail_callbacks").Where("account_id=? AND provider=? AND event_id=?", id, provider, event.EventID).Take(&existing).Error != nil || !hmac.Equal(existing.BodySHA256, digest[:]) {
				return ErrConflict
			}
			if inserted.RowsAffected == 0 {
				return nil
			}
		}
		if inbound || event.SubmissionID == "" || event.Status == "observed" {
			return nil
		}
		channel := provider
		if channel == "relayme" {
			channel = "quguo"
		}
		return tx.Table("notification_mail_compositions").Where("account_id=? AND submitted_provider=? AND provider_id=? AND (delivery_at IS NULL OR delivery_at<?)", id, channel, event.SubmissionID, event.OccurredAt.UTC()).Updates(map[string]any{"delivery_status": event.Status, "delivery_at": event.OccurredAt.UTC()}).Error
	})
	if e != nil {
		c.Status(503)
		return
	}
	c.Status(204)
}
