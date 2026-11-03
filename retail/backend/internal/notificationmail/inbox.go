package notificationmail

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"mime"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InboxMessage struct {
	ID          string     `json:"id"`
	AccountID   string     `json:"account_id"`
	Subject     string     `json:"subject"`
	FromAddress string     `json:"from_address"`
	ReceivedAt  time.Time  `json:"received_at"`
	ReadAt      *time.Time `json:"read_at"`
	Archived    bool       `json:"archived"`
	Deleted     bool       `json:"deleted"`
}
type syncCursor struct {
	AccountID               string
	UIDValidity, ScannedUID uint32
	SourceSHA256            []byte
}
type SyncResult struct {
	More        bool   `json:"more"`
	Imported    int    `json:"imported"`
	ScannedUID  uint32 `json:"scanned_uid"`
	UIDValidity uint32 `json:"uid_validity"`
}

func inboxPartContext(id string, part int) protected.Context {
	return protected.Context{Purpose: "notification-inbox-rfc822", Table: "notification_mail_inbox_parts", Column: "raw_envelope", Row: id + ":" + strconv.Itoa(part)}
}
func (s Store) activeMailbox(ctx context.Context, tx *gorm.DB, actor identity.User, id string, action authorization.Action) (TransportConfig, error) {
	if e := authorization.CheckTx(ctx, tx, actor, action); e != nil {
		return TransportConfig{}, e
	}
	var count int64
	if tx.Table("notification_mail_accounts n").Joins("JOIN mail_accounts m ON m.id=n.account_id").Where("m.id=? AND m.enabled=TRUE AND m.deleted_at IS NULL", id).Count(&count).Error != nil || count != 1 {
		return TransportConfig{}, ErrUnavailable
	}
	var held int64
	if tx.Table("migration_merchant_dispositions").Where("source_table='mail_account' AND source_id=?", id).Count(&held).Error != nil || held != 0 {
		return TransportConfig{}, ErrHeld
	}
	return s.transport(tx, id)
}

// SyncInbox reads only INBOX. No remote flags, folder moves or deletions occur.
// Network work stays outside SQL; the second transaction rechecks grants,
// settings and cursor before atomic archive insertion plus cursor advancement.
func (s Store) SyncInbox(ctx context.Context, actor identity.User, id string) (SyncResult, error) {
	return s.syncInboxAuthorized(ctx, id, func(tx *gorm.DB) (TransportConfig, error) {
		return s.activeMailbox(ctx, tx, actor, id, authorization.MailWrite)
	})
}
func (s Store) syncInboxAuthorized(ctx context.Context, id string, authorize func(*gorm.DB) (TransportConfig, error)) (SyncResult, error) {
	out := SyncResult{}
	if s.DB == nil || s.Ring == nil {
		return out, ErrUnavailable
	}
	var config TransportConfig
	var cursor syncCursor
	var source [32]byte
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		config, e = authorize(tx)
		if e != nil {
			return e
		}
		if config.IMAP == nil {
			return ErrInput
		}
		sourceRaw, _ := json.Marshal([]any{config.IMAP.Host, config.IMAP.Port, config.IMAP.Username, "INBOX"})
		source = sha256.Sum256(sourceRaw)
		if tx.Exec("INSERT INTO notification_mail_sync(account_id,source_sha256) VALUES(?,?) ON DUPLICATE KEY UPDATE account_id=account_id", id, source[:]).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("notification_mail_sync").Where("account_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&cursor).Error != nil {
			return ErrUnavailable
		}
		if subtle.ConstantTimeCompare(cursor.SourceSHA256, source[:]) != 1 {
			if tx.Table("notification_mail_sync").Where("account_id=?", id).Updates(map[string]any{"source_sha256": source[:], "uid_validity": 0, "scanned_uid": 0}).Error != nil {
				return ErrUnavailable
			}
			cursor.UIDValidity = 0
			cursor.ScannedUID = 0
			cursor.SourceSHA256 = source[:]
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	batch, e := maildelivery.FetchIMAP(ctx, *config.IMAP, "INBOX", cursor.UIDValidity, cursor.ScannedUID, 50)
	if e != nil {
		return out, ErrUnavailable
	}
	defer func() {
		for _, message := range batch.Messages {
			clear(message.Raw)
		}
	}()
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, e := authorize(tx)
		if e != nil {
			return e
		}
		before, _ := jsonConfig(config)
		after, _ := jsonConfig(current)
		defer clear(before)
		defer clear(after)
		if !bytes.Equal(before, after) {
			return ErrConflict
		}
		var locked syncCursor
		if tx.Table("notification_mail_sync").Where("account_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&locked).Error != nil {
			return ErrUnavailable
		}
		if locked.UIDValidity != cursor.UIDValidity || locked.ScannedUID != cursor.ScannedUID || subtle.ConstantTimeCompare(locked.SourceSHA256, cursor.SourceSHA256) != 1 {
			return ErrConflict
		}
		if batch.UIDValidity == 0 {
			return ErrUnavailable
		}
		if cursor.UIDValidity != 0 && cursor.UIDValidity != batch.UIDValidity {
			out.More = true
			out.UIDValidity = batch.UIDValidity
			// Preserve archives from the old epoch, reset only this mailbox's cursor.
			return tx.Table("notification_mail_sync").Where("account_id=?", id).Updates(map[string]any{"uid_validity": batch.UIDValidity, "scanned_uid": 0}).Error
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		for _, message := range batch.Messages {
			digest := sha256.Sum256(message.Raw)
			var existing struct{ RawSHA256 []byte }
			e := tx.Table("notification_mail_inbox").Where("account_id=? AND source_sha256=? AND uid_validity=? AND uid=?", id, source[:], batch.UIDValidity, message.UID).Take(&existing).Error
			if e == nil {
				if subtle.ConstantTimeCompare(digest[:], existing.RawSHA256) != 1 {
					return ErrConflict
				}
				continue
			}
			if !errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrUnavailable
			}
			subject, from := "(No subject)", ""
			if parsed, e := mail.ReadMessage(bytes.NewReader(message.Raw)); e == nil {
				subject = parsed.Header.Get("Subject")
				if decoded, e := new(mime.WordDecoder).DecodeHeader(subject); e == nil {
					subject = decoded
				}
				from = parsed.Header.Get("From")
			}
			subject = strings.ToValidUTF8(subject, "�")
			from = strings.ToValidUTF8(from, "�")
			if len(subject) > 8192 {
				subject = string([]rune(subject)[:min(2048, utf8.RuneCountInString(subject))])
			}
			if len(from) > 8192 {
				from = "(See original message)"
			}
			messageID := uuid.NewString()
			received := message.ReceivedAt.UTC()
			if received.IsZero() {
				received = now
			}
			row := map[string]any{"id": messageID, "account_id": id, "source_sha256": source[:], "uid_validity": batch.UIDValidity, "uid": message.UID, "subject": subject, "from_address": from, "received_at": received, "raw_sha256": digest[:], "raw_size": len(message.Raw), "created_at": now}
			if tx.Table("notification_mail_inbox").Create(row).Error != nil {
				return ErrUnavailable
			}
			for start, part := 0, 0; start < len(message.Raw); start, part = start+protected.MaxPlaintext, part+1 {
				sealed, e := s.Ring.Seal(inboxPartContext(messageID, part), message.Raw[start:min(start+protected.MaxPlaintext, len(message.Raw))])
				if e != nil {
					return ErrUnavailable
				}
				e = tx.Exec("INSERT INTO notification_mail_inbox_parts(message_id,part_number,raw_envelope) VALUES(?,?,?)", messageID, part, sealed).Error
				clear(sealed)
				if e != nil {
					return ErrUnavailable
				}
			}
			out.Imported++
		}
		scanned := max(cursor.ScannedUID, batch.ScannedThrough)
		out.UIDValidity = batch.UIDValidity
		out.ScannedUID = scanned
		out.More = scanned < batch.LastUID
		if tx.Table("notification_mail_sync").Where("account_id=?", id).Updates(map[string]any{"uid_validity": batch.UIDValidity, "scanned_uid": scanned}).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	return out, e
}
func jsonConfig(c TransportConfig) ([]byte, error) { return json.Marshal(c) }

func (s Store) Inbox(ctx context.Context, actor identity.User, account, folder string, offset int) ([]InboxMessage, error) {
	out := []InboxMessage{}
	if offset < 0 || offset > 100000 || folder != "inbox" && folder != "archive" && folder != "trash" {
		return out, ErrInput
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := s.activeMailbox(ctx, tx, actor, account, authorization.MailRead); e != nil {
			return e
		}
		query := tx.Table("notification_mail_inbox").Where("account_id=?", account)
		if folder == "trash" {
			query = query.Where("deleted=TRUE")
		} else {
			query = query.Where("deleted=FALSE AND archived=?", folder == "archive")
		}
		return query.Order("received_at DESC,id DESC").Offset(offset).Limit(50).Scan(&out).Error
	})
	return out, e
}
func (s Store) MessageRaw(ctx context.Context, actor identity.User, id string) (InboxMessage, []byte, error) {
	var message InboxMessage
	var raw []byte
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.MailRead); e != nil {
			return e
		}
		var row struct {
			InboxMessage
			RawSHA256 []byte
			RawSize   int
		}
		if tx.Table("notification_mail_inbox").Where("id=?", id).Take(&row).Error != nil {
			return ErrInput
		}
		message = row.InboxMessage
		if _, e := s.activeMailbox(ctx, tx, actor, message.AccountID, authorization.MailRead); e != nil {
			return e
		}
		if row.RawSize < 0 || row.RawSize > 26<<20 {
			return ErrUnavailable
		}
		var parts []struct {
			PartNumber  int
			RawEnvelope []byte
		}
		if tx.Table("notification_mail_inbox_parts").Where("message_id=?", id).Order("part_number").Find(&parts).Error != nil {
			return ErrUnavailable
		}
		for index, part := range parts {
			if part.PartNumber != index {
				return ErrUnavailable
			}
			plain, e := s.Ring.Open(inboxPartContext(id, index), part.RawEnvelope)
			if e != nil {
				return ErrUnavailable
			}
			raw = append(raw, plain...)
			clear(plain)
		}
		digest := sha256.Sum256(raw)
		if len(raw) != row.RawSize || subtle.ConstantTimeCompare(digest[:], row.RawSHA256) != 1 {
			return ErrUnavailable
		}
		return nil
	})
	if e != nil {
		clear(raw)
		return message, nil, e
	}
	return message, raw, nil
}
