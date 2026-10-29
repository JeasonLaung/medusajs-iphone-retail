package payments

import (
	"context"
	"crypto/subtle"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrEffectInput    = errors.New("invalid payment effect input")
	ErrEffectConflict = errors.New("payment effect conflict")
	ErrEffectStorage  = errors.New("payment effect storage unavailable")
	effectID          = regexp.MustCompile(`^[A-Za-z0-9_-]{1,191}$`)
)

// EffectKey is private and grants no cart/customer/provider authority. Authorize
// the enclosing operation before every entry point, including receipt replay.
// Digest binds the full immutable outbound request, including provider identity,
// source snapshot and create-session idempotency key. Never hash only the ID.
type EffectKey struct {
	ScopeID, OperationID, Kind, ResourceID string
	Digest                                 [32]byte
}

type EffectReceipt struct {
	State, AttemptID string
	Attempts         int
	LeaseUntil       *time.Time
	// Dispatch is valid ONLY after the caller successfully commits ClaimEffect.
	Dispatch bool
}

type effectRow struct {
	RequestSHA256 []byte
	State         string
	AttemptID     *string
	Attempts      int
	LeaseUntil    *time.Time
}

func effectTx(ctx context.Context, tx *gorm.DB, key EffectKey) (*gorm.DB, context.CancelFunc, error) {
	noop := func() {}
	id, err := uuid.Parse(key.OperationID)
	if tx == nil || tx.Statement == nil || !effectID.MatchString(key.ScopeID) || !effectID.MatchString(key.ResourceID) ||
		(key.Kind != "delete_session" && key.Kind != "create_session") || key.Digest == ([32]byte{}) || err != nil || id.Version() != 4 || id.String() != key.OperationID {
		return nil, noop, ErrEffectInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, noop, ErrEffectInput
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	q := tx.WithContext(bounded)
	if Ready(bounded, q.Clauses(clause.Locking{Strength: "SHARE"})) != nil {
		cancel()
		return nil, noop, ErrSchema
	}
	return q, cancel, nil
}

func effectWhere(tx *gorm.DB, key EffectKey) *gorm.DB {
	return tx.Table("payment_effect_journal").Where("scope_id=? AND operation_id=? AND effect_kind=? AND resource_id=?", key.ScopeID, key.OperationID, key.Kind, key.ResourceID)
}

func lockEffect(tx *gorm.DB, key EffectKey) (effectRow, error) {
	var row effectRow
	if effectWhere(tx, key).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
		return row, ErrEffectStorage
	}
	if subtle.ConstantTimeCompare(row.RequestSHA256, key.Digest[:]) != 1 {
		return effectRow{}, ErrEffectConflict
	}
	switch row.State {
	case "pending":
		if row.AttemptID != nil || row.Attempts != 0 || row.LeaseUntil != nil {
			return effectRow{}, ErrEffectStorage
		}
	case "inflight", "succeeded", "failed", "uncertain":
		if row.AttemptID == nil || row.LeaseUntil == nil || row.Attempts < 1 || row.Attempts > 100 {
			return effectRow{}, ErrEffectStorage
		}
		id, err := uuid.Parse(*row.AttemptID)
		if err != nil || id.Version() != 4 || id.String() != *row.AttemptID {
			return effectRow{}, ErrEffectStorage
		}
	default:
		return effectRow{}, ErrEffectStorage
	}
	return row, nil
}

func effectReceipt(row effectRow) EffectReceipt {
	r := EffectReceipt{State: row.State, Attempts: row.Attempts, LeaseUntil: row.LeaseUntil}
	if row.AttemptID != nil {
		r.AttemptID = *row.AttemptID
	}
	return r
}

// StageEffect binds one effect to an immutable request. The caller supplies a
// real transaction and must roll back on ANY error. Payload secrets are not
// stored here. The coordinator must persist its protected recovery snapshot in
// the SAME transaction before committing; this receipt alone is not a job.
func StageEffect(ctx context.Context, tx *gorm.DB, key EffectKey) (EffectReceipt, error) {
	q, cancel, err := effectTx(ctx, tx, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	defer cancel()
	if q.Exec(`INSERT INTO payment_effect_journal(scope_id,operation_id,effect_kind,resource_id,request_sha256,state,created_at,updated_at) VALUES(?,?,?,?,?,'pending',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE operation_id=operation_id`, key.ScopeID, key.OperationID, key.Kind, key.ResourceID, key.Digest[:]).Error != nil {
		return EffectReceipt{}, ErrEffectStorage
	}
	row, err := lockEffect(q, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	return effectReceipt(row), nil
}

// ClaimEffect serializes dispatch. Call the provider only AFTER committing the
// returned Dispatch=true claim, outside the DB transaction. Expiry transitions
// inflight to uncertain: it NEVER authorizes a second dispatch. Commit that
// transition and reconcile the original attempt with provider evidence.
func ClaimEffect(ctx context.Context, tx *gorm.DB, key EffectKey, lease time.Duration) (EffectReceipt, error) {
	if lease < 10*time.Second || lease > 5*time.Minute {
		return EffectReceipt{}, ErrEffectInput
	}
	q, cancel, err := effectTx(ctx, tx, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	defer cancel()
	row, err := lockEffect(q, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	var now time.Time
	if q.Raw("SELECT UTC_TIMESTAMP(6)").Scan(&now).Error != nil {
		return EffectReceipt{}, ErrEffectStorage
	}
	switch row.State {
	case "inflight":
		if !now.Before(*row.LeaseUntil) {
			if effectWhere(q, key).Updates(map[string]any{"state": "uncertain", "updated_at": now}).Error != nil {
				return EffectReceipt{}, ErrEffectStorage
			}
			row.State = "uncertain"
		}
		return effectReceipt(row), nil
	case "succeeded", "uncertain":
		return effectReceipt(row), nil
	case "pending", "failed":
		if row.Attempts >= 100 {
			return EffectReceipt{}, ErrEffectConflict
		}
	}
	attempt := uuid.NewString()
	until := now.Add(lease).Truncate(time.Microsecond)
	if effectWhere(q, key).Updates(map[string]any{"state": "inflight", "attempt_id": attempt, "attempts": row.Attempts + 1, "lease_until": until, "updated_at": now}).Error != nil {
		return EffectReceipt{}, ErrEffectStorage
	}
	return EffectReceipt{State: "inflight", AttemptID: attempt, Attempts: row.Attempts + 1, LeaseUntil: &until, Dispatch: true}, nil
}

// RecordEffect records ONLY a verified outcome of this attempt. "failed" means
// the adapter knows the effect did not happen (safe to retry); timeout, EOF,
// cancellation and ambiguous provider errors MUST be "uncertain". An uncertain
// attempt may later be resolved by its late response or provider reconciliation.
// Stale attempts cannot overwrite a newer attempt. No external call occurs here.
func RecordEffect(ctx context.Context, tx *gorm.DB, key EffectKey, attempt, outcome string) (EffectReceipt, error) {
	if outcome != "succeeded" && outcome != "failed" && outcome != "uncertain" {
		return EffectReceipt{}, ErrEffectInput
	}
	q, cancel, err := effectTx(ctx, tx, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	defer cancel()
	row, err := lockEffect(q, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	if row.AttemptID == nil || *row.AttemptID != attempt || (row.State != "inflight" && row.State != "uncertain" && row.State != outcome) {
		return EffectReceipt{}, ErrEffectConflict
	}
	if row.State != outcome {
		if effectWhere(q, key).Updates(map[string]any{"state": outcome, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return EffectReceipt{}, ErrEffectStorage
		}
		row.State = outcome
	}
	return effectReceipt(row), nil
}
