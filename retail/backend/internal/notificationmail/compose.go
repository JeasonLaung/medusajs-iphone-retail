package notificationmail

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Composition struct {
	DeliveryStatus  *string    `json:"delivery_status"`
	DeliveryAt      *time.Time `json:"delivery_at"`
	ID              string     `json:"id"`
	AccountID       string     `json:"account_id"`
	Revision        uint64     `json:"revision"`
	Subject         string     `json:"subject"`
	Status          string     `json:"status"`
	ProviderID      *string    `json:"provider_id"`
	ErrorCode       *string    `json:"error_code"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	SentAt          *time.Time `json:"sent_at"`
	ContentEnvelope []byte     `json:"-"`
}

func composeContext(id string) protected.Context {
	return protected.Context{Purpose: "notification-compose", Table: "notification_mail_compositions", Column: "content_envelope", Row: id}
}
func (s Store) Compositions(ctx context.Context, actor identity.User, account string) ([]Composition, error) {
	out := []Composition{}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := s.activeMailbox(ctx, tx, actor, account, authorization.MailRead); e != nil {
			return e
		}
		return tx.Table("notification_mail_compositions").Select("id,account_id,revision,subject,status,provider_id,error_code,created_at,updated_at,sent_at,delivery_status,delivery_at").Where("account_id=?", account).Order("updated_at DESC,id DESC").Limit(100).Find(&out).Error
	})
	return out, e
}
func (s Store) Composition(ctx context.Context, actor identity.User, id string) (Composition, maildelivery.ManualMessage, error) {
	var out Composition
	var content maildelivery.ManualMessage
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if authorization.CheckTx(ctx, tx, actor, authorization.MailRead) != nil {
			return authorization.ErrDenied
		}
		if tx.Table("notification_mail_compositions").Where("id=?", id).Take(&out).Error != nil {
			return ErrInput
		}
		if _, e := s.activeMailbox(ctx, tx, actor, out.AccountID, authorization.MailRead); e != nil {
			return e
		}
		raw, e := s.Ring.Open(composeContext(id), out.ContentEnvelope)
		if e != nil {
			return ErrUnavailable
		}
		defer clear(raw)
		if json.Unmarshal(raw, &content) != nil {
			return ErrUnavailable
		}
		return nil
	})
	clear(out.ContentEnvelope)
	out.ContentEnvelope = nil
	return out, content, e
}

// The client chooses a UUID before saving. A lost create response is resolved by
// reading that same ID; edits require the current revision and remain drafts.
func (s Store) SaveComposition(ctx context.Context, actor identity.User, id, account string, revision uint64, content maildelivery.ManualMessage) (Composition, error) {
	var out Composition
	parsed, e := uuid.Parse(id)
	if e != nil || parsed.Version() != 4 || parsed.String() != id || maildelivery.ValidateManual(content) != nil {
		return out, ErrInput
	}
	raw, e := json.Marshal(content)
	if e != nil || len(raw) > 900000 {
		return out, ErrInput
	}
	defer clear(raw)
	sealed, e := s.Ring.Seal(composeContext(id), raw)
	if e != nil {
		return out, ErrUnavailable
	}
	defer clear(sealed)
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := s.activeMailbox(ctx, tx, actor, account, authorization.MailWrite); e != nil {
			return e
		}
		result := tx.Table("notification_mail_compositions").Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&out)
		now := time.Now().UTC().Truncate(time.Microsecond)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			if revision != 0 {
				return ErrConflict
			}
			out = Composition{ID: id, AccountID: account, Revision: 1, Subject: content.Subject, Status: "draft", ContentEnvelope: sealed, CreatedAt: now, UpdatedAt: now}
			return tx.Table("notification_mail_compositions").Create(&out).Error
		}
		if result.Error != nil {
			return ErrUnavailable
		}
		if out.AccountID != account || out.Status != "draft" || out.Revision != revision {
			return ErrConflict
		}
		out.Revision++
		out.Subject = content.Subject
		out.UpdatedAt = now
		return tx.Table("notification_mail_compositions").Where("id=?", id).Updates(map[string]any{"revision": out.Revision, "subject": out.Subject, "content_envelope": sealed, "updated_at": now}).Error
	})
	out.ContentEnvelope = nil
	return out, e
}
func (s Store) SendComposition(ctx context.Context, actor identity.User, id string, revision uint64) (Composition, error) {
	var out Composition
	var sender *maildelivery.Resend
	var payload []byte
	claimed := false
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if authorization.CheckTx(ctx, tx, actor, authorization.MailWrite) != nil {
			return authorization.ErrDenied
		}
		if tx.Table("notification_mail_compositions").Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&out).Error != nil {
			return ErrInput
		}
		config, e := s.activeMailbox(ctx, tx, actor, out.AccountID, authorization.MailWrite)
		if e != nil {
			return e
		}
		// A prior claimed attempt is returned without another network submission,
		// including after process death or a lost response. Never duplicate SMTP.
		if out.Status != "draft" {
			return nil
		}
		if out.Revision != revision {
			return ErrConflict
		}
		var account storedAccount
		if tx.Table("mail_accounts").Where("id=?", out.AccountID).Take(&account).Error != nil {
			return ErrUnavailable
		}
		var key []byte
		if config.Provider == "resend" {
			key, e = s.Ring.Open(secretContext(account.ID, "api_key"), account.APIKey)
			if e != nil {
				return ErrUnavailable
			}
			defer clear(key)
		}
		sender, e = makeSender(config, string(key), account.Name, account.Email, account.ReplyTo)
		if e != nil {
			return ErrUnavailable
		}
		raw, e := s.Ring.Open(composeContext(id), out.ContentEnvelope)
		if e != nil {
			return ErrUnavailable
		}
		defer clear(raw)
		var content maildelivery.ManualMessage
		if json.Unmarshal(raw, &content) != nil {
			return ErrUnavailable
		}
		payload, e = sender.PrepareManual(content)
		if e != nil {
			return ErrInput
		}
		out.Status = "sending"
		claimed = true
		return tx.Table("notification_mail_compositions").Where("id=?", id).Updates(map[string]any{"status": "sending", "submitted_provider": config.Provider, "updated_at": time.Now().UTC()}).Error
	})
	defer clear(payload)
	out.ContentEnvelope = nil
	if e != nil || !claimed {
		return out, e
	}
	result := sender.Send(ctx, "mail/mc_"+id, payload)
	// Persist final observation even if the browser disconnected after submission.
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	values := map[string]any{"updated_at": time.Now().UTC()}
	if result.ID != "" && maildelivery.ValidSubmissionID(sender, result.ID) {
		out.Status = "sent"
		out.ProviderID = &result.ID
		now := time.Now().UTC()
		out.SentAt = &now
		values["status"] = "sent"
		values["provider_id"] = result.ID
		values["sent_at"] = now
	} else {
		out.Status = "review_required"
		code := result.Code
		if code == "" {
			code = "provider_response"
		}
		out.ErrorCode = &code
		values["status"] = out.Status
		values["error_code"] = code
	}
	e = s.DB.WithContext(finish).Transaction(func(tx *gorm.DB) error {
		if tx.Table("notification_mail_compositions").Where("id=? AND status='sending'", id).Updates(values).Error != nil {
			return ErrUnavailable
		}
		if out.Status != "sent" {
			return nil
		}
		var channel struct{ SubmittedProvider string }
		if tx.Table("notification_mail_compositions").Where("id=?", id).Take(&channel).Error != nil {
			return ErrUnavailable
		}
		provider := channel.SubmittedProvider
		if provider == "quguo" {
			provider = "relayme"
		}
		var events []struct {
			DeliveryStatus string
			OccurredAt     time.Time
		}
		if tx.Table("notification_mail_callbacks").Where("account_id=? AND provider=? AND provider_id=? AND delivery_status IN ?", out.AccountID, provider, result.ID, []string{"success", "delayed", "failed"}).Order("occurred_at DESC,event_id DESC").Limit(1).Find(&events).Error != nil {
			return ErrUnavailable
		}
		if len(events) == 1 {
			out.DeliveryStatus = &events[0].DeliveryStatus
			out.DeliveryAt = &events[0].OccurredAt
			return tx.Table("notification_mail_compositions").Where("id=? AND (delivery_at IS NULL OR delivery_at<?)", id, events[0].OccurredAt).Updates(map[string]any{"delivery_status": events[0].DeliveryStatus, "delivery_at": events[0].OccurredAt}).Error
		}
		return nil
	})
	return out, e
}
