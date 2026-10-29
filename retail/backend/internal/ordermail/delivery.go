// Package ordermail dispatches only locally committed paid-order notifications.
package ordermail

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("order mail unavailable")
var fingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Sender interface {
	Fingerprint() string
	PrepareOrder(maildelivery.OrderPlaced) ([]byte, error)
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
	ID, OrderID, Event, Status string
	PayloadEnvelope            []byte
	Attempts                   uint
}
type delivery struct {
	NotificationID                                              string
	RequestEnvelope                                             []byte
	SenderFingerprint, LeaseToken, ProviderID, ErrorCode        *string
	FirstAttemptAt, NextAttemptAt, LeaseUntil, SentAt, FailedAt *time.Time
}

func intentContext(id string) protected.Context {
	return protected.Context{Purpose: "order-notification-v1", Table: "order_notification_outbox", Column: "payload_envelope", Row: id}
}
func requestContext(id string) protected.Context {
	return protected.Context{Purpose: "order-mail-request-v1", Table: "order_mail_deliveries", Column: "request_envelope", Row: id}
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
		if tx.Table("order_mail_dispatch_gate").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&gate).Error != nil {
			return ErrUnavailable
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if gate.NextClaimAt.After(now) {
			return nil
		}
		var jobs []job
		if tx.Table("order_notification_outbox n").Select("n.*").Joins("LEFT JOIN order_mail_deliveries d ON d.notification_id=n.id").Where("n.event='order.placed' AND n.status IN ('pending','sending') AND (d.next_attempt_at IS NULL OR d.next_attempt_at<=?) AND (d.lease_until IS NULL OR d.lease_until<=?)", now, now).Order("n.created_at,n.id").Limit(1).Scan(&jobs).Error != nil {
			return ErrUnavailable
		}
		if len(jobs) == 0 {
			return nil
		}
		j := jobs[0]
		if tx.Table("order_notification_outbox").Where("id=?", j.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&j).Error != nil {
			return ErrUnavailable
		}
		if j.Status != "pending" && j.Status != "sending" {
			return nil
		}
		parsed, e := uuid.Parse(j.ID)
		if e != nil || parsed == uuid.Nil || parsed.String() != j.ID {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO order_mail_deliveries(notification_id) VALUES(?) ON DUPLICATE KEY UPDATE notification_id=notification_id", j.ID).Error != nil {
			return ErrUnavailable
		}
		var d delivery
		if tx.Table("order_mail_deliveries").Where("notification_id=?", j.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error != nil {
			return ErrUnavailable
		}
		result = Result{ID: j.ID, Status: j.Status, Attempts: j.Attempts}
		fail := func(code string) error {
			if tx.Table("order_notification_outbox").Where("id=?", j.ID).Update("status", "failed").Error != nil {
				return ErrUnavailable
			}
			result.Status = "failed"
			return tx.Table("order_mail_deliveries").Where("notification_id=?", j.ID).Updates(map[string]any{"failed_at": now, "error_code": code, "lease_until": nil, "lease_token": nil}).Error
		}
		if j.Attempts >= 3 || d.FirstAttemptAt != nil && !d.FirstAttemptAt.Add(23*time.Hour).After(now) {
			return fail("retry_exhausted")
		}
		if d.LeaseUntil != nil && d.LeaseUntil.After(now) {
			result = Result{}
			return nil
		}
		plain, e := s.Ring.Open(intentContext(j.ID), j.PayloadEnvelope)
		if e != nil {
			return ErrUnavailable
		}
		defer clear(plain)
		var intent struct {
			OrderID, Email string
			Locale         *string
		}
		if json.Unmarshal(plain, &intent) != nil || intent.OrderID != j.OrderID {
			return fail("invalid_intent")
		}
		var order struct {
			Email                   string
			Locale, CustomDisplayID *string
			DisplayID               uint64
			CurrencyCode            string
		}
		if tx.Table("orders").Select("email,locale,custom_display_id,display_id,currency_code").Where("id=? AND deleted_at IS NULL AND is_draft_order=FALSE AND status<>'canceled'", j.OrderID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&order).Error != nil {
			return fail("order_unavailable")
		}
		if order.Email != intent.Email || !sameLocale(order.Locale, intent.Locale) {
			return fail("recipient_changed")
		}
		// The receipt/capture binds this event to the completed local paid order.
		var paid struct{ Amount, RawAmount, CurrencyCode string }
		if tx.Table("cart_completion_receipts r").Select("p.amount,p.raw_amount,p.currency_code").Joins("JOIN payments p ON p.id=r.payment_id AND p.deleted_at IS NULL AND p.canceled_at IS NULL AND p.captured_at IS NOT NULL").Joins("JOIN payment_captures c ON c.id=r.capture_id AND c.payment_id=p.id AND c.deleted_at IS NULL").Where("r.order_id=?", j.OrderID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&paid).Error != nil || paid.CurrencyCode != order.CurrencyCode {
			return fail("payment_unavailable")
		}
		var raw totals.RawValue
		if json.Unmarshal([]byte(paid.RawAmount), &raw) != nil {
			return fail("payment_unavailable")
		}
		paid.Amount, e = totals.Resolve(totals.Number{Decimal: paid.Amount, Raw: &raw})
		if e != nil {
			return fail("payment_unavailable")
		}
		locale := "en"
		if intent.Locale != nil && strings.HasPrefix(strings.ToLower(*intent.Locale), "zh") {
			locale = "zh"
		}
		reference := strconv.FormatUint(order.DisplayID, 10)
		if order.CustomDisplayID != nil && *order.CustomDisplayID != "" {
			reference = *order.CustomDisplayID
		}
		changes := map[string]any{}
		if d.FirstAttemptAt == nil {
			body, e = sender.PrepareOrder(maildelivery.OrderPlaced{To: intent.Email, Reference: reference, Amount: paid.Amount, Currency: paid.CurrencyCode, Locale: locale})
			if e != nil || len(body) == 0 || len(body) > 16384 {
				return ErrUnavailable
			}
			sealed, e := s.Ring.Seal(requestContext(j.ID), body)
			if e != nil {
				return ErrUnavailable
			}
			changes["request_envelope"], changes["sender_fingerprint"], changes["first_attempt_at"] = sealed, sender.Fingerprint(), now
		} else {
			if d.SenderFingerprint == nil || *d.SenderFingerprint != sender.Fingerprint() {
				return fail("sender_changed")
			}
			body, e = s.Ring.Open(requestContext(j.ID), d.RequestEnvelope)
			if e != nil || len(body) == 0 || len(body) > 16384 {
				return ErrUnavailable
			}
		}
		token = uuid.NewString()
		changes["lease_token"], changes["lease_until"], changes["error_code"] = token, now.Add(30*time.Second), nil
		if tx.Table("order_mail_deliveries").Where("notification_id=?", j.ID).Updates(changes).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("order_notification_outbox").Where("id=?", j.ID).Updates(map[string]any{"status": "sending", "attempts": j.Attempts + 1}).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("order_mail_dispatch_gate").Where("id=1").Update("next_claim_at", now.Add(time.Second)).Error != nil {
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
	sent := invoke(sender, sendCtx, "mail/op_"+claimed.ID, body)
	cancel()
	finish, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	err = s.DB.WithContext(finish).Transaction(func(tx *gorm.DB) error {
		var current job
		if tx.Table("order_notification_outbox").Where("id=?", claimed.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error != nil {
			return ErrUnavailable
		}
		var d delivery
		if tx.Table("order_mail_deliveries").Where("notification_id=?", claimed.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&d).Error != nil {
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
		if tx.Table("order_mail_deliveries").Where("notification_id=? AND lease_token=?", claimed.ID, token).Updates(changes).Error != nil {
			return ErrUnavailable
		}
		return tx.Table("order_notification_outbox").Where("id=? AND status='sending'", claimed.ID).Update("status", result.Status).Error
	})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	return result, nil
}
func sameLocale(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func invoke(s Sender, ctx context.Context, key string, body []byte) (out maildelivery.Result) {
	defer func() {
		if recover() != nil {
			out = maildelivery.Result{Code: "network_error", Retry: true}
		}
	}()
	return s.Send(ctx, key, body)
}
