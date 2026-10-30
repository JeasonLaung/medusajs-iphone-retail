package buyback

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HistoryClaim struct {
	CustomerID   string `json:"customer_id"`
	EvidenceNote string `json:"evidence_note"`
}

func ParseHistoryClaim(raw []byte) (HistoryClaim, error) {
	var in HistoryClaim
	if decodeBounded(raw, &in, 8192, "customer_id", "evidence_note") != nil || !validID(in.CustomerID, 80) {
		return in, ErrInput
	}
	note, e := trimSubmission(in.EvidenceNote, 10, 1000)
	if e != nil {
		return HistoryClaim{}, ErrInput
	}
	in.EvidenceNote = note
	return in, nil
}

// Human verification is explicit, recorded once and never inferred from email.
// Lock new customer before request, matching customer command lock order.
func ClaimHistoricalTx(ctx context.Context, tx *gorm.DB, actor identity.User, store, id string, in HistoryClaim) (AdminDetail, error) {
	var out AdminDetail
	if tx == nil || !validID(store, 191) || !validID(id, 191) {
		return out, ErrInput
	}
	if _, e := adminGate(ctx, tx, actor, true); e != nil {
		return out, e
	}
	raw, _ := json.Marshal(in)
	in, e := ParseHistoryClaim(raw)
	if e != nil {
		return out, e
	}
	var account struct {
		ID         string
		Enabled    bool
		VerifiedAt *time.Time
	}
	if tx.Table("customer_accounts").Select("id,enabled,verified_at").Where("id=?", in.CustomerID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&account).Error != nil || !account.Enabled || account.VerifiedAt == nil {
		return out, ErrInput
	}
	var row struct {
		ID, Status string
		ClosedAt   *time.Time
	}
	if historicalQuery(tx, store, id).Select("r.id,r.status,r.closed_at").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
		return out, ErrMissing
	}
	var prior struct{ CustomerID, EvidenceNote string }
	e = tx.Table("buyback_history_claims").Where("request_id=?", id).Take(&prior).Error
	if e == nil {
		if prior.CustomerID != in.CustomerID || prior.EvidenceNote != in.EvidenceNote {
			return out, ErrConflict
		}
		return AdminRequestTx(ctx, tx, actor, store, id, nil)
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrUnavailable
	}
	if row.ClosedAt != nil || row.Status == "paid" || row.Status == "returned" || row.Status == "canceled" {
		return out, ErrCommandState
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if tx.Table("buyback_history_claims").Create(map[string]any{"request_id": id, "customer_id": in.CustomerID, "actor_id": actor.ID, "evidence_note": in.EvidenceNote, "created_at": now}).Error != nil {
		return out, ErrUnavailable
	}
	audit, _ := json.Marshal(map[string]any{"customer_id": in.CustomerID, "claim_request_id": id})
	if tx.Table("audit_events").Create(map[string]any{"id": "audit_" + uuid.NewString(), "actor_id": actor.ID, "action": "buyback.history_claim", "resource_id": id, "data": string(audit), "created_at": now}).Error != nil {
		return out, ErrUnavailable
	}
	return AdminRequestTx(ctx, tx, actor, store, id, nil)
}
