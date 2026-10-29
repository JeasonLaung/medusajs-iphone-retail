package customer

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"time"

	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeliverySender implementations prepare deterministic bodies and do not
// automatically retry. A production sender pins the Resend endpoint and timeout.
type DeliverySender interface {
	Fingerprint() string
	Prepare(string, string, string) ([]byte, error)
	Send(context.Context, string, []byte) maildelivery.Result
}
type deliveryJob struct {
	ID, CustomerID                                              string
	Payload                                                     []byte
	Status                                                      string
	CreatedAt, ExpiresAt                                        time.Time
	Attempts                                                    uint
	FirstAttemptAt, NextAttemptAt, LeaseUntil, FailedAt, SentAt *time.Time
	LeaseToken, SenderFingerprint, ErrorCode, ProviderID        *string
	RequestPayload                                              []byte
}

func (deliveryJob) TableName() string { return "customer_verification_mail" }

type Delivery struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Attempts uint   `json:"attempts"`
}
type deliveryClaim struct {
	job                deliveryJob
	body               []byte
	fingerprint, lease string
}

var mailFingerprint = regexp.MustCompile(`^[0-9a-f]{64}$`)
var verificationPath = regexp.MustCompile(`^/[a-z]{2}-(en|zh)/verify-account$`)

func deliveryContext(id string) protected.Context {
	return protected.Context{Purpose: "customer-verification-request", Table: "customer_verification_mail", Column: "request_payload", Row: id}
}

// DispatchVerification processes at most one durable intent. The application
// scheduler or explicit CLI supplies pacing; the SQL gate enforces one claim
// per second across workers, including workers in another process.
func (s *Store) DispatchVerification(ctx context.Context, sender DeliverySender) (Delivery, error) {
	if s == nil || s.db == nil || s.ring == nil || s.origin == "" || sender == nil || !mailFingerprint.MatchString(sender.Fingerprint()) {
		return Delivery{}, ErrUnavailable
	}
	var candidates []deliveryJob
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Select("id,customer_id").Where("status='pending' AND failed_at IS NULL AND (next_attempt_at IS NULL OR next_attempt_at<=?) AND (lease_until IS NULL OR lease_until<=?)", now, now).Order("created_at,id").Limit(1).Find(&candidates).Error; err != nil {
		return Delivery{}, ErrUnavailable
	}
	if len(candidates) == 0 {
		return Delivery{}, nil
	}
	var claim deliveryClaim
	var result Delivery
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var account record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", candidates[0].CustomerID).Take(&account).Error; err != nil {
			return err
		}
		var job deliveryJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND customer_id=?", candidates[0].ID, account.ID).Take(&job).Error; err != nil {
			return err
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if job.Status != "pending" || job.FailedAt != nil || job.NextAttemptAt != nil && job.NextAttemptAt.After(now) || job.LeaseUntil != nil && job.LeaseUntil.After(now) {
			return nil
		}
		result = Delivery{ID: job.ID, Status: "pending", Attempts: job.Attempts}
		fail := func(code string, cancel bool) error {
			changes := map[string]any{"failed_at": now, "error_code": code, "lease_until": nil, "lease_token": nil}
			result.Status = "failed"
			if cancel {
				changes["status"] = "cancelled"
				result.Status = "cancelled"
			}
			return tx.Model(&deliveryJob{}).Where("id=?", job.ID).Updates(changes).Error
		}
		var challenge verification
		err := tx.Where("customer_id=?", account.ID).Take(&challenge).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err != nil || challenge.ID != job.ID || challenge.ConsumedAt != nil || !challenge.ExpiresAt.After(now) || !job.ExpiresAt.After(now) || !account.Enabled || account.VerifiedAt != nil || account.AuthVersion == 0 {
			return fail("verification_inactive", true)
		}
		if job.Attempts >= 3 || job.FirstAttemptAt != nil && !job.FirstAttemptAt.Add(23*time.Hour).After(now) {
			return fail("retry_exhausted", false)
		}
		plain, err := s.ring.Open(mailContext(job.ID), job.Payload)
		if err != nil {
			return ErrUnavailable
		}
		var intent struct{ Email, URL, Locale string }
		if json.Unmarshal(plain, &intent) != nil {
			return fail("invalid_intent", false)
		}
		parsed, err := url.Parse(intent.URL)
		if err != nil || parsed == nil || parsed.User != nil || parsed.Fragment != "" || parsed.Scheme+"://"+parsed.Host != s.origin || !verificationPath.MatchString(parsed.Path) || intent.Locale != "en" && intent.Locale != "zh" || parsed.Path[4:6] != intent.Locale || intent.Email != account.Email {
			return fail("invalid_intent", false)
		}
		query, err := url.ParseQuery(parsed.RawQuery)
		if err != nil || len(query) != 1 || len(query["token"]) != 1 || !tokenOK(query.Get("token")) {
			return fail("invalid_intent", false)
		}
		digest := sha256.Sum256([]byte(query.Get("token")))
		if subtle.ConstantTimeCompare(challenge.TokenHash, digest[:]) != 1 {
			return fail("verification_replaced", true)
		}
		fingerprint := sender.Fingerprint()
		body := []byte(nil)
		changes := map[string]any{}
		if job.FirstAttemptAt == nil {
			body, err = sender.Prepare(intent.Email, intent.URL, intent.Locale)
			if err != nil || len(body) == 0 || len(body) > 16384 {
				return ErrUnavailable
			}
			sealed, err := s.ring.Seal(deliveryContext(job.ID), body)
			if err != nil {
				return ErrUnavailable
			}
			changes["request_payload"], changes["sender_fingerprint"], changes["first_attempt_at"] = sealed, fingerprint, now
		} else {
			if job.SenderFingerprint == nil || *job.SenderFingerprint != fingerprint {
				return fail("sender_changed", false)
			}
			body, err = s.ring.Open(deliveryContext(job.ID), job.RequestPayload)
			if err != nil || len(body) == 0 || len(body) > 16384 {
				return ErrUnavailable
			}
		}
		var gate struct {
			ID          int
			NextClaimAt time.Time
		}
		if err := tx.Table("customer_mail_dispatch_gate").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&gate).Error; err != nil {
			return err
		}
		if gate.NextClaimAt.After(now) {
			result = Delivery{}
			return nil
		}
		if err := tx.Table("customer_mail_dispatch_gate").Where("id=1").Update("next_claim_at", now.Add(time.Second)).Error; err != nil {
			return err
		}
		lease := "cvl_" + uuid.NewString()
		changes["attempts"], changes["lease_until"], changes["lease_token"], changes["error_code"] = job.Attempts+1, now.Add(30*time.Second), lease, nil
		if err := tx.Model(&deliveryJob{}).Where("id=?", job.ID).Updates(changes).Error; err != nil {
			return err
		}
		job.Attempts++
		claim = deliveryClaim{job: job, body: body, fingerprint: fingerprint, lease: lease}
		result.Attempts = job.Attempts
		return nil
	})
	if err != nil {
		return Delivery{}, ErrUnavailable
	}
	if claim.lease == "" {
		return result, nil
	}
	// Never call a provider while holding the business transaction. A crash here
	// leaves a recoverable lease and exactly the same encrypted request/key.
	sendCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	sent := sender.Send(sendCtx, "mail/"+claim.job.ID, claim.body)
	cancel()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer finishCancel()
	updates := map[string]any{"lease_until": nil, "lease_token": nil}
	finished := time.Now().UTC().Truncate(time.Microsecond)
	if sent.ID != "" {
		providerID, parseErr := uuid.Parse(sent.ID)
		if parseErr != nil || providerID == uuid.Nil || sent.ID != providerID.String() {
			sent = maildelivery.Result{Code: "provider_response", Retry: true}
		}
	}
	if sent.ID != "" {
		updates["status"], updates["provider_id"], updates["sent_at"], updates["error_code"] = "sent", sent.ID, finished, nil
		result.Status = "sent"
	} else {
		code := sent.Code
		switch code {
		case "network_error", "provider_temporary", "provider_rejected", "provider_response", "invalid_request":
		default:
			code = "provider_error"
		}
		updates["error_code"] = code
		if sent.Retry && claim.job.Attempts < 3 && claim.job.ExpiresAt.After(finished.Add(time.Minute)) {
			updates["next_attempt_at"] = finished.Add(time.Minute)
			result.Status = "pending"
		} else {
			updates["failed_at"] = finished
			result.Status = "failed"
		}
	}
	// Verification/resend can cancel a claim while it is in flight. Do not turn
	// that cancelled intent back into sent/pending. Its old link is invalid.
	saved := s.db.WithContext(finishCtx).Model(&deliveryJob{}).Where("id=? AND status='pending' AND lease_token=? AND sender_fingerprint=?", claim.job.ID, claim.lease, claim.fingerprint).Updates(updates)
	if saved.Error != nil {
		return Delivery{}, ErrUnavailable
	}
	if saved.RowsAffected != 1 {
		return Delivery{ID: claim.job.ID, Status: "superseded", Attempts: claim.job.Attempts}, nil
	}
	return result, nil
}
