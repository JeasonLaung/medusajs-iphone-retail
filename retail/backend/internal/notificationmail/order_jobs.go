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

type JobView struct {
	ID             string     `json:"id"`
	OrderID        string     `json:"order_id"`
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
type Jobs struct {
	Jobs   []JobView `json:"jobs"`
	Count  int64     `json:"count"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

func canRetry(status string, first *time.Time, now time.Time) bool {
	return status == "failed" && first != nil && first.Add(23*time.Hour).After(now)
}
func (s Store) OrderJobs(ctx context.Context, actor identity.User, offset int) (Jobs, error) {
	out := Jobs{Jobs: []JobView{}, Limit: 50, Offset: offset}
	if s.DB == nil || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.MailRead); e != nil {
			return e
		}
		if tx.Table("order_notification_outbox").Where("event='order.placed'").Count(&out.Count).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("order_notification_outbox n").Select("n.id,n.order_id,n.status,n.attempts,n.created_at,d.first_attempt_at,d.next_attempt_at,d.sent_at,d.failed_at,d.error_code").Joins("LEFT JOIN order_mail_deliveries d ON d.notification_id=n.id").Where("n.event='order.placed'").Order("n.created_at DESC,n.id DESC").Limit(50).Offset(offset).Scan(&out.Jobs).Error != nil {
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

type RetryOrderJob struct {
	OperationID      string `json:"operation_id"`
	ExpectedAttempts uint   `json:"expected_attempts"`
}
type RetryJobResult struct {
	ID       string `json:"id"`
	Replayed bool   `json:"replayed"`
}

func (s Store) RetryOrderJob(ctx context.Context, actor identity.User, id string, in RetryOrderJob) (RetryJobResult, error) {
	out := RetryJobResult{ID: id}
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
		auditID := "order-mail-retry_" + in.OperationID
		var audit struct{ ActorID, Action, ResourceID, Data string }
		e := tx.Table("audit_events").Where("id=?", auditID).Take(&audit).Error
		if e == nil {
			var saved RetryOrderJob
			if audit.ActorID != actor.ID || audit.Action != "order_mail.retry" || audit.ResourceID != id || json.Unmarshal([]byte(audit.Data), &saved) != nil || saved != in {
				return ErrConflict
			}
			out.Replayed = true
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		var job struct {
			Status, OrderID string
			Attempts        uint
		}
		if tx.Table("order_notification_outbox").Select("status,order_id,attempts").Where("id=? AND event='order.placed'", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&job).Error != nil {
			return ErrConflict
		}
		// Recheck receipt under the job lock for concurrent same-operation retries.
		if tx.Table("audit_events").Where("id=?", auditID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&audit).Error == nil {
			var saved RetryOrderJob
			if audit.ActorID != actor.ID || audit.ResourceID != id || audit.Action != "order_mail.retry" || json.Unmarshal([]byte(audit.Data), &saved) != nil || saved != in {
				return ErrConflict
			}
			out.Replayed = true
			return nil
		}
		var delivery struct{ FirstAttemptAt, LeaseUntil *time.Time }
		if tx.Table("order_mail_deliveries").Select("first_attempt_at,lease_until").Where("notification_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&delivery).Error != nil {
			return ErrConflict
		}
		now := time.Now().UTC()
		if job.Attempts != in.ExpectedAttempts || !canRetry(job.Status, delivery.FirstAttemptAt, now) || delivery.LeaseUntil != nil && delivery.LeaseUntil.After(now) {
			return ErrConflict
		}
		if tx.Table("order_notification_outbox").Where("id=?", id).Updates(map[string]any{"status": "pending", "attempts": 0}).Error != nil {
			return ErrUnavailable
		}
		if tx.Table("order_mail_deliveries").Where("notification_id=?", id).Updates(map[string]any{"next_attempt_at": now, "failed_at": nil, "lease_token": nil, "lease_until": nil}).Error != nil {
			return ErrUnavailable
		}
		raw, _ := json.Marshal(in)
		if tx.Exec("INSERT INTO audit_events(id,actor_id,action,resource_id,data,created_at) VALUES(?,?,'order_mail.retry',?,?,?)", auditID, actor.ID, id, string(raw), now).Error != nil {
			return ErrConflict
		}
		return nil
	})
	return out, err
}
