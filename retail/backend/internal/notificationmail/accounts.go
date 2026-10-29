// Package notificationmail manages explicitly approved notification senders.
// Historical imported mail accounts remain held and are never selected here.
package notificationmail

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/uniquekey"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInput = errors.New("invalid notification mailbox command")
var ErrConflict = errors.New("notification mailbox revision or operation conflict")
var ErrHeld = errors.New("historical mailbox requires reconciliation")
var ErrUnavailable = errors.New("notification mailbox unavailable")

type Store struct {
	DB   *gorm.DB
	Ring *protected.Ring
}
type Account struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	ReplyTo          string `json:"reply_to"`
	Enabled          bool   `json:"enabled"`
	HasAPIKey        bool   `json:"has_api_key"`
	HasWebhookSecret bool   `json:"has_webhook_secret"`
	Registration     bool   `json:"registration"`
}
type Directory struct {
	Revision              string    `json:"revision"`
	RegistrationAccountID *string   `json:"registration_account_id"`
	Accounts              []Account `json:"accounts"`
}

// Blank secrets preserve existing values. They are never returned or included
// in audit JSON. Operation receipts encrypt the exact normalized request.
type Command struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	ID               string `json:"id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	ReplyTo          string `json:"reply_to"`
	APIKey           string `json:"api_key"`
	WebhookSecret    string `json:"webhook_secret"`
	Enabled          bool   `json:"enabled"`
	Registration     bool   `json:"registration"`
}
type Result struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}
type settings struct {
	ID                    uint
	Revision              uint64
	RegistrationAccountID *string
}
type storedAccount struct {
	ID, Name, Email, ReplyTo string
	Enabled                  bool
	APIKey, WebhookSecret    []byte
	CreatedAt, UpdatedAt     time.Time
	DeletedAt                *time.Time
}

func secretContext(id, column string) protected.Context {
	return protected.Context{Purpose: "merchant-config", Table: "mail_accounts", Column: column, Row: id}
}
func receiptContext(id string) protected.Context {
	return protected.Context{Purpose: "notification-mail-command", Table: "notification_mail_commands", Column: "request_payload", Row: id}
}
func validAddress(v string, optional bool) bool {
	if v == "" {
		return optional
	}
	a, e := mail.ParseAddress(v)
	return e == nil && a.Name == "" && a.Address == v && len(v) <= 254 && !strings.ContainsAny(v, "\r\n\x00")
}
func normalize(in Command) (Command, error) {
	id, e := uuid.Parse(in.OperationID)
	revision, rerr := strconv.ParseUint(in.ExpectedRevision, 10, 64)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || rerr != nil || revision == 0 || strconv.FormatUint(revision, 10) != in.ExpectedRevision {
		return Command{}, ErrInput
	}
	if in.ID != "" {
		parsed, e := uuid.Parse(strings.TrimPrefix(in.ID, "mail_"))
		if e != nil || parsed.Version() != 4 || in.ID != "mail_"+parsed.String() {
			return Command{}, ErrInput
		}
	}
	if !utf8.ValidString(in.Name) || strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > 160 || strings.ContainsAny(in.Name, "\r\n\x00") || !validAddress(in.Email, false) || !validAddress(in.ReplyTo, true) {
		return Command{}, ErrInput
	}
	if in.APIKey != "" {
		if _, e := maildelivery.NewResend(in.APIKey, (&mail.Address{Name: in.Name, Address: in.Email}).String(), in.ReplyTo); e != nil {
			return Command{}, ErrInput
		}
	}
	if !utf8.ValidString(in.WebhookSecret) || len(in.WebhookSecret) > 512 || strings.ContainsAny(in.WebhookSecret, " \t\r\n\x00") || in.Registration && !in.Enabled {
		return Command{}, ErrInput
	}
	return in, nil
}
func (s Store) Directory(ctx context.Context, actor identity.User) (Directory, error) {
	out := Directory{Accounts: []Account{}}
	if s.DB == nil || s.Ring == nil {
		return out, ErrUnavailable
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.MailRead); e != nil {
			return e
		}
		var state settings
		if e := tx.Table("notification_mail_settings").Where("id=1").Take(&state).Error; e != nil || state.Revision == 0 {
			return ErrUnavailable
		}
		out.Revision = strconv.FormatUint(state.Revision, 10)
		out.RegistrationAccountID = state.RegistrationAccountID
		var rows []struct {
			ID, Name, Email, ReplyTo  string
			Enabled, HasWebhookSecret bool
			APIKey                    []byte
		}
		if e := tx.Table("notification_mail_accounts n").Select("m.*,n.has_webhook_secret").Joins("JOIN mail_accounts m ON m.id=n.account_id").Where("m.deleted_at IS NULL").Order("m.created_at,m.id").Limit(101).Scan(&rows).Error; e != nil {
			return ErrUnavailable
		}
		if len(rows) > 100 {
			return ErrUnavailable
		}
		for _, row := range rows {
			out.Accounts = append(out.Accounts, Account{ID: row.ID, Name: row.Name, Email: row.Email, ReplyTo: row.ReplyTo, Enabled: row.Enabled, HasAPIKey: len(row.APIKey) > 0, HasWebhookSecret: row.HasWebhookSecret, Registration: state.RegistrationAccountID != nil && *state.RegistrationAccountID == row.ID})
		}
		return nil
	})
	return out, err
}
func (s Store) Save(ctx context.Context, actor identity.User, input Command) (Result, error) {
	in, e := normalize(input)
	if e != nil {
		return Result{}, e
	}
	if s.DB == nil || s.Ring == nil {
		return Result{}, ErrUnavailable
	}
	request, e := json.Marshal(in)
	if e != nil {
		return Result{}, ErrInput
	}
	defer clear(request)
	var out Result
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.MailWrite); e != nil {
			return e
		}
		var state settings
		if e := tx.Table("notification_mail_settings").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&state).Error; e != nil || state.Revision == 0 {
			return ErrUnavailable
		}
		var receipt struct {
			ActorID        string
			RequestPayload []byte
			ResultRevision uint64
			AccountID      string
		}
		err := tx.Table("notification_mail_commands").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if err == nil {
			plain, e := s.Ring.Open(receiptContext(in.OperationID), receipt.RequestPayload)
			if e != nil {
				return ErrUnavailable
			}
			defer clear(plain)
			if receipt.ActorID != actor.ID || !bytes.Equal(plain, request) {
				return ErrConflict
			}
			out = Result{ID: receipt.AccountID, Revision: strconv.FormatUint(receipt.ResultRevision, 10), Replayed: true}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if strconv.FormatUint(state.Revision, 10) != in.ExpectedRevision || state.Revision == ^uint64(0) {
			return ErrConflict
		}
		id := in.ID
		before := Account{}
		var current storedAccount
		webhookConfigured := false
		if id == "" {
			var count int64
			if tx.Table("notification_mail_accounts").Count(&count).Error != nil || count >= 100 {
				return ErrConflict
			}
			id = "mail_" + uuid.NewString()
		} else {
			var approval struct{ HasWebhookSecret bool }
			if tx.Table("notification_mail_accounts").Where("account_id=?", id).Take(&approval).Error != nil {
				return ErrHeld
			}
			webhookConfigured = approval.HasWebhookSecret
			if tx.Table("mail_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", id).Take(&current).Error != nil {
				return ErrConflict
			}
			before = Account{ID: current.ID, Name: current.Name, Email: current.Email, ReplyTo: current.ReplyTo, Enabled: current.Enabled, HasAPIKey: len(current.APIKey) > 0, HasWebhookSecret: webhookConfigured, Registration: state.RegistrationAccountID != nil && *state.RegistrationAccountID == id}
		}
		// Never silently release a historical merchant disposition, even if an
		// approval row has been accidentally inserted by an offline operator.
		var holds int64
		if tx.Table("migration_merchant_dispositions").Where("source_table='mail_account' AND source_id=?", id).Count(&holds).Error != nil {
			return ErrUnavailable
		}
		if holds != 0 {
			return ErrHeld
		}
		key := []byte(in.APIKey)
		if len(key) == 0 && in.ID != "" {
			key, e = s.Ring.Open(secretContext(id, "api_key"), current.APIKey)
			if e != nil {
				return ErrUnavailable
			}
		}
		defer clear(key)
		if _, e := maildelivery.NewResend(string(key), (&mail.Address{Name: in.Name, Address: in.Email}).String(), in.ReplyTo); e != nil {
			return ErrInput
		}
		sealedKey, e := s.Ring.Seal(secretContext(id, "api_key"), key)
		if e != nil {
			return ErrUnavailable
		}
		webhook := current.WebhookSecret
		if in.ID == "" || in.WebhookSecret != "" {
			webhook, e = s.Ring.Seal(secretContext(id, "webhook_secret"), []byte(in.WebhookSecret))
			if e != nil {
				return ErrUnavailable
			}
			webhookConfigured = in.WebhookSecret != ""
		}
		sqlTx, ok := tx.Statement.ConnPool.(*sql.Tx)
		if !ok {
			return ErrUnavailable
		}
		if e := uniquekey.Apply(ctx, sqlTx, []uniquekey.Change{{Scope: "source_uk_0dd39c15e819059c91c55ba4", Owner: id, Active: true, Components: []*string{&in.Email}}}); e != nil {
			if errors.Is(e, uniquekey.ErrConflict) {
				return ErrConflict
			}
			return ErrUnavailable
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if in.ID == "" {
			current = storedAccount{ID: id, Name: in.Name, Email: in.Email, ReplyTo: in.ReplyTo, Enabled: in.Enabled, APIKey: sealedKey, WebhookSecret: webhook, CreatedAt: now, UpdatedAt: now}
			if tx.Table("mail_accounts").Create(&current).Error != nil {
				return ErrUnavailable
			}
			if tx.Exec("INSERT INTO notification_mail_accounts(account_id,has_webhook_secret,created_at) VALUES(?,?,?)", id, webhookConfigured, now).Error != nil {
				return ErrUnavailable
			}
		} else {
			if tx.Table("mail_accounts").Where("id=?", id).Updates(map[string]any{"name": in.Name, "email": in.Email, "reply_to": in.ReplyTo, "enabled": in.Enabled, "api_key": sealedKey, "webhook_secret": webhook, "updated_at": now}).Error != nil {
				return ErrUnavailable
			}
			if tx.Table("notification_mail_accounts").Where("account_id=?", id).Update("has_webhook_secret", webhookConfigured).Error != nil {
				return ErrUnavailable
			}
		}
		selected := state.RegistrationAccountID
		if in.Registration {
			selected = &id
		} else if selected != nil && *selected == id {
			selected = nil
		}
		revision := state.Revision + 1
		if tx.Table("notification_mail_settings").Where("id=1").Updates(map[string]any{"revision": revision, "registration_account_id": selected}).Error != nil {
			return ErrUnavailable
		}
		encrypted, e := s.Ring.Seal(receiptContext(in.OperationID), request)
		if e != nil {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO notification_mail_commands(operation_id,actor_id,request_payload,result_revision,account_id,created_at) VALUES(?,?,?,?,?,?)", in.OperationID, actor.ID, encrypted, revision, id, now).Error != nil {
			return ErrUnavailable
		}
		after := Account{ID: id, Name: in.Name, Email: in.Email, ReplyTo: in.ReplyTo, Enabled: in.Enabled, HasAPIKey: true, HasWebhookSecret: webhookConfigured, Registration: in.Registration}
		audit, _ := json.Marshal(map[string]any{"before": before, "after": after, "revision": strconv.FormatUint(revision, 10), "previous_registration_account_id": state.RegistrationAccountID, "registration_account_id": selected})
		if tx.Exec("INSERT INTO audit_events(id,actor_id,action,resource_id,data,created_at) VALUES(?,?,'notification_mail.save',?,?,?)", "audit_"+uuid.NewString(), actor.ID, id, string(audit), now).Error != nil {
			return ErrUnavailable
		}
		out = Result{ID: id, Revision: strconv.FormatUint(revision, 10)}
		return nil
	})
	return out, e
}

// RegistrationSender preserves the original oldest-enabled fallback among
// approved accounts. Explicit selection never falls back if unavailable.
// The worker reloads each cycle; historical imported accounts remain excluded.
func (s Store) RegistrationSender(ctx context.Context) (*maildelivery.Resend, error) {
	return s.sender(ctx, true)
}

// OrderSender uses the oldest enabled approved notification mailbox. A selected
// registration mailbox is not implicitly selected for other business events.
func (s Store) OrderSender(ctx context.Context) (*maildelivery.Resend, error) {
	return s.sender(ctx, false)
}
func (s Store) sender(ctx context.Context, registration bool) (*maildelivery.Resend, error) {
	if s.DB == nil || s.Ring == nil {
		return nil, ErrUnavailable
	}
	var sender *maildelivery.Resend
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state settings
		if tx.Table("notification_mail_settings").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&state).Error != nil {
			return ErrUnavailable
		}
		if !registration {
			state.RegistrationAccountID = nil
		}
		if state.RegistrationAccountID == nil {
			var first struct{ ID string }
			if tx.Table("notification_mail_accounts n").Select("m.id").Joins("JOIN mail_accounts m ON m.id=n.account_id").Where("m.deleted_at IS NULL AND m.enabled=TRUE").Order("m.created_at,m.id").Take(&first).Error != nil {
				return ErrUnavailable
			}
			state.RegistrationAccountID = &first.ID
		}
		var current storedAccount
		var approval int64
		if tx.Table("notification_mail_accounts").Where("account_id=?", *state.RegistrationAccountID).Count(&approval).Error != nil || approval != 1 {
			return ErrUnavailable
		}
		if tx.Table("mail_accounts").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=? AND enabled=TRUE AND deleted_at IS NULL", *state.RegistrationAccountID).Take(&current).Error != nil {
			return ErrUnavailable
		}
		var held int64
		if tx.Table("migration_merchant_dispositions").Where("source_table='mail_account' AND source_id=?", current.ID).Count(&held).Error != nil || held != 0 {
			return ErrHeld
		}
		key, e := s.Ring.Open(secretContext(current.ID, "api_key"), current.APIKey)
		if e != nil {
			return ErrUnavailable
		}
		defer clear(key)
		sender, e = maildelivery.NewResend(string(key), (&mail.Address{Name: current.Name, Address: current.Email}).String(), current.ReplyTo)
		if e != nil {
			return ErrUnavailable
		}
		return nil
	})
	return sender, e
}
