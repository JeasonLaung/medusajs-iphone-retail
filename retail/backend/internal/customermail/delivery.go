// Package customermail dispatches committed new-account business events.
package customermail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"regexp"

	"strings"
	"time"
)

var ErrUnavailable = errors.New("customer event mail unavailable")
var fingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Sender interface {
	Fingerprint() string
	PrepareEvent(maildelivery.CustomerEvent) ([]byte, error)
	Send(context.Context, string, []byte) maildelivery.Result
}
type Store struct {
	DB   *gorm.DB
	Ring *protected.Ring
}
type Result struct {
	ID, Status string
	Attempts   uint
}
type job struct {
	ID, CustomerID, Kind, ResourceID, Reference, Action, Status string

	Attempts uint
}
type delivery struct {
	NotificationID                                                             string
	RequestEnvelope                                                            []byte
	SenderFingerprint, RecipientFingerprint, LeaseToken, ProviderID, ErrorCode *string
	FirstAttemptAt, NextAttemptAt, LeaseUntil, SentAt, FailedAt                *time.Time
}

func requestContext(id string) protected.Context {
	return protected.Context{Purpose: "customer-event-mail-request-v1", Table: "customer_event_mail_deliveries", Column: "request_envelope", Row: id}
}
func (s Store) Dispatch(ctx context.Context, sender Sender) (Result, error) {
	if s.DB == nil || s.Ring == nil || sender == nil || !fingerprint.MatchString(sender.Fingerprint()) {
		return Result{}, ErrUnavailable
	}
	var claimed job
	var body []byte
	var token string
	var result Result
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var gate struct{ NextClaimAt time.Time }
		if tx.Table("customer_event_mail_gate").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&gate).Error != nil {
			return ErrUnavailable
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if gate.NextClaimAt.After(now) {
			return nil
		}
		var jobs []job
		if tx.Table("customer_event_mail n").Select("n.*").Joins("LEFT JOIN customer_event_mail_deliveries d ON d.notification_id=n.id").Where("n.status IN ('pending','sending') AND (d.next_attempt_at IS NULL OR d.next_attempt_at<=?) AND (d.lease_until IS NULL OR d.lease_until<=?)", now, now).Order("n.created_at,n.id").Limit(1).Scan(&jobs).Error != nil {
			return ErrUnavailable
		}
		if len(jobs) == 0 {
			return nil
		}
		j := jobs[0]
		if tx.Table("customer_event_mail").Where("id=?", j.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&j).Error != nil {
			return ErrUnavailable
		}
		if j.Status != "pending" && j.Status != "sending" {
			return nil
		}
		parsed, e := uuid.Parse(j.ID)
		if e != nil || parsed == uuid.Nil || parsed.String() != j.ID {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO customer_event_mail_deliveries(notification_id) VALUES(?) ON DUPLICATE KEY UPDATE notification_id=notification_id", j.ID).Error != nil {
			return ErrUnavailable
		}
		var d delivery
		if tx.Table("customer_event_mail_deliveries").Where("notification_id=?", j.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error != nil {
			return ErrUnavailable
		}
		result = Result{ID: j.ID, Status: j.Status, Attempts: j.Attempts}
		fail := func(code string) error {
			if tx.Table("customer_event_mail").Where("id=?", j.ID).Update("status", "failed").Error != nil {
				return ErrUnavailable
			}
			result.Status = "failed"
			return tx.Table("customer_event_mail_deliveries").Where("notification_id=?", j.ID).Updates(map[string]any{"failed_at": now, "error_code": code, "lease_until": nil, "lease_token": nil}).Error
		}
		if j.Attempts >= 3 || d.FirstAttemptAt != nil && !d.FirstAttemptAt.Add(23*time.Hour).After(now) {
			return fail("retry_exhausted")
		}
		if d.LeaseUntil != nil && d.LeaseUntil.After(now) {
			result = Result{}
			return nil
		}
		var account struct{ Email string }
		if tx.Table("customer_accounts").Select("email").Where("id=? AND enabled=TRUE AND verified_at IS NOT NULL", j.CustomerID).Take(&account).Error != nil {
			return fail("recipient_unavailable")
		}
		digest := sha256.Sum256([]byte(account.Email))
		recipient := hex.EncodeToString(digest[:])
		if d.FirstAttemptAt != nil && (d.RecipientFingerprint == nil || *d.RecipientFingerprint != recipient) {
			return fail("recipient_changed")
		}
		locale := "en"
		if j.Kind == "buyback.updated" {
			var count int64
			if tx.Table("buyback_account_access a").Joins("JOIN buyback_requests r ON r.id=a.request_id AND r.deleted_at IS NULL").Where("a.request_id=? AND a.customer_id=?", j.ResourceID, j.CustomerID).Count(&count).Error != nil || count != 1 {
				return fail("recipient_changed")
			}
		} else {
			var orders []struct{ Locale *string }
			if tx.Table("customer_order_ownership own").Select("o.locale").Joins("JOIN orders o ON o.id=own.order_id AND o.deleted_at IS NULL AND o.is_draft_order=FALSE").Where("own.order_id=? AND own.customer_id=?", j.ResourceID, j.CustomerID).Limit(2).Scan(&orders).Error != nil || len(orders) != 1 {
				return fail("recipient_changed")
			}
			if orders[0].Locale != nil && strings.HasPrefix(strings.ToLower(*orders[0].Locale), "zh") {
				locale = "zh"
			}
		}
		changes := map[string]any{}
		if d.FirstAttemptAt == nil {
			var e error
			body, e = sender.PrepareEvent(maildelivery.CustomerEvent{To: account.Email, Reference: j.Reference, Kind: j.Kind, Action: j.Action, Locale: locale})
			if e != nil || len(body) == 0 || len(body) > 16384 {
				return ErrUnavailable
			}
			sealed, e := s.Ring.Seal(requestContext(j.ID), body)
			if e != nil {
				return ErrUnavailable
			}
			changes["request_envelope"], changes["sender_fingerprint"], changes["first_attempt_at"], changes["recipient_fingerprint"] = sealed, sender.Fingerprint(), now, recipient
		} else {
			if d.SenderFingerprint == nil || *d.SenderFingerprint != sender.Fingerprint() {
				return fail("sender_changed")
			}
			var e error
			body, e = s.Ring.Open(requestContext(j.ID), d.RequestEnvelope)
			if e != nil || len(body) == 0 || len(body) > 16384 {
				return ErrUnavailable
			}
		}
		token = uuid.NewString()
		changes["lease_token"], changes["lease_until"], changes["error_code"] = token, now.Add(30*time.Second), nil
		if tx.Table("customer_event_mail_deliveries").Where("notification_id=?", j.ID).Updates(changes).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("customer_event_mail").Where("id=?", j.ID).Updates(map[string]any{"status": "sending", "attempts": j.Attempts + 1}).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("customer_event_mail_gate").Where("id=1").Update("next_claim_at", now.Add(time.Second)).Error != nil {
			return ErrUnavailable
		}
		j.Attempts++
		claimed = j
		result.Attempts = j.Attempts
		return nil
	})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	if token == "" {
		return result, nil
	}
	defer clear(body)
	sendCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	sent := invoke(sender, sendCtx, "mail/ce_"+claimed.ID, body)
	cancel()
	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	err = s.DB.WithContext(finish).Transaction(func(tx *gorm.DB) error {
		var current job
		if tx.Table("customer_event_mail").Where("id=?", claimed.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error != nil {
			return ErrUnavailable
		}
		var d delivery
		if tx.Table("customer_event_mail_deliveries").Where("notification_id=?", claimed.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error != nil {
			return ErrUnavailable
		}
		if current.Status != "sending" || d.LeaseToken == nil || *d.LeaseToken != token {
			result.Status = "superseded"
			return nil
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		changes := map[string]any{"lease_token": nil, "lease_until": nil}
		if sent.ID != "" {
			id, e := uuid.Parse(sent.ID)
			if e != nil || id == uuid.Nil || id.String() != sent.ID {
				sent = maildelivery.Result{Code: "provider_response", Retry: true}
			}
		}
		result.Status = "pending"
		if sent.ID != "" {
			result.Status = "sent"
			changes["provider_id"], changes["sent_at"], changes["error_code"] = sent.ID, now, nil
		} else {
			code := sent.Code
			switch code {
			case "network_error", "provider_temporary", "provider_rejected", "provider_response", "invalid_request":
			default:
				code = "provider_error"
			}
			changes["error_code"] = code
			if sent.Retry && claimed.Attempts < 3 {
				changes["next_attempt_at"] = now.Add(time.Minute)
			} else {
				result.Status = "failed"
				changes["failed_at"] = now
			}
		}
		if tx.Table("customer_event_mail_deliveries").Where("notification_id=? AND lease_token=?", claimed.ID, token).Updates(changes).Error != nil {
			return ErrUnavailable
		}
		return tx.Table("customer_event_mail").Where("id=? AND status='sending'", claimed.ID).Update("status", result.Status).Error
	})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	return result, nil
}
func invoke(s Sender, ctx context.Context, key string, body []byte) (out maildelivery.Result) {
	defer func() {
		if recover() != nil {
			out = maildelivery.Result{Code: "network_error", Retry: true}
		}
	}()
	return s.Send(ctx, key, body)
}
