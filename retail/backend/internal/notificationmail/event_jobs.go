package notificationmail

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type EventJobView struct {
	Kind           string     `json:"kind"`
	Action         string     `json:"action"`
	ID             string     `json:"id"`
	ResourceID     string     `json:"resource_id"`
	Status         string     `json:"status"`
	Attempts       uint       `json:"attempts"`
	CreatedAt      time.Time  `json:"created_at"`
	FirstAttemptAt *time.Time `json:"first_attempt_at"`
	NextAttemptAt  *time.Time `json:"next_attempt_at"`
	SentAt         *time.Time `json:"sent_at"`
	FailedAt       *time.Time `json:"failed_at"`
	ErrorCode      *string    `json:"error_code"`
	Retryable      bool       `json:"retryable"`
}
type EventJobsResult struct {
	Jobs   []EventJobView `json:"jobs"`
	Count  int64          `json:"count"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func (s Store) EventJobs(ctx context.Context, actor identity.User, offset int) (EventJobsResult, error) {
	out := EventJobsResult{Jobs: []EventJobView{}, Limit: 50, Offset: offset}
	if s.DB == nil || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.MailRead); e != nil {
			return e
		}
		if tx.Table("customer_event_mail").Count(&out.Count).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("customer_event_mail n").Select("n.id,n.resource_id,n.kind,n.action,n.status,n.attempts,n.created_at,d.first_attempt_at,d.next_attempt_at,d.sent_at,d.failed_at,d.error_code").Joins("LEFT JOIN customer_event_mail_deliveries d ON d.notification_id=n.id").Order("n.created_at DESC,n.id DESC").Limit(50).Offset(offset).Scan(&out.Jobs).Error != nil {
			return ErrUnavailable
		}
		now := time.Now().UTC()
		for i := range out.Jobs {
			out.Jobs[i].Retryable = canRetry(out.Jobs[i].Status, out.Jobs[i].FirstAttemptAt, now)
		}
		return nil
	})
	return out, err
}

type RetryEventJob struct {
	OperationID      string `json:"operation_id"`
	ExpectedAttempts uint   `json:"expected_attempts"`
}
type RetryEventJobResult struct {
	ID       string `json:"id"`
	Replayed bool   `json:"replayed"`
}

func (s Store) RetryEventJob(ctx context.Context, actor identity.User, id string, in RetryEventJob) (RetryEventJobResult, error) {
	out := RetryEventJobResult{ID: id}
	op, e := uuid.Parse(in.OperationID)
	jobID, je := uuid.Parse(id)
	if s.DB == nil || e != nil || op.Version() != 4 || op.String() != in.OperationID || je != nil || jobID == uuid.Nil || jobID.String() != id {
		return out, ErrInput
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.MailWrite); e != nil {
			return e
		}
		// Stable audit identity is also the command receipt. Replays reauthorize,
		// never reset a job that has already been claimed or sent after the retry.
		auditID := "customer-event-mail-retry_" + in.OperationID
		var audit struct{ ActorID, Action, ResourceID, Data string }
		e := tx.Table("audit_events").Where("id=?", auditID).Take(&audit).Error
		if e == nil {
			var saved RetryEventJob
			if audit.ActorID != actor.ID || audit.Action != "customer_event_mail.retry" || audit.ResourceID != id || json.Unmarshal([]byte(audit.Data), &saved) != nil || saved != in {
				return ErrConflict
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		var job struct {
			Status, ResourceID string
			Attempts           uint
		}
		if tx.Table("customer_event_mail").Select("status,resource_id,attempts").Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&job).Error != nil {
			return ErrConflict
		}
		// Recheck receipt under the job lock for concurrent same-operation retries.
		if tx.Table("audit_events").Where("id=?", auditID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&audit).Error == nil {
			var saved RetryEventJob
			if audit.ActorID != actor.ID || audit.ResourceID != id || audit.Action != "customer_event_mail.retry" || json.Unmarshal([]byte(audit.Data), &saved) != nil || saved != in {
				return ErrConflict
			}
			out.Replayed = true
			return nil
		}
		var delivery struct{ FirstAttemptAt, LeaseUntil *time.Time }
		if tx.Table("customer_event_mail_deliveries").Select("first_attempt_at,lease_until").Where("notification_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&delivery).Error != nil {
			return ErrConflict
		}
		now := time.Now().UTC()
		if job.Attempts != in.ExpectedAttempts || !canRetry(job.Status, delivery.FirstAttemptAt, now) || delivery.LeaseUntil != nil && delivery.LeaseUntil.After(now) {
			return ErrConflict
		}
		if tx.Table("customer_event_mail").Where("id=?", id).Updates(map[string]any{"status": "pending", "attempts": 0}).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("customer_event_mail_deliveries").Where("notification_id=?", id).Updates(map[string]any{"next_attempt_at": now, "failed_at": nil, "lease_token": nil, "lease_until": nil}).Error != nil {
			return ErrUnavailable
		}
		raw, _ := json.Marshal(in)
		if tx.Exec("INSERT INTO audit_events(id,actor_id,action,resource_id,data,created_at) VALUES(?,?,'customer_event_mail.retry',?,?,?)", auditID, actor.ID, id, string(raw), now).Error != nil {
			return ErrConflict
		}
		return nil
	})
	return out, err
}
