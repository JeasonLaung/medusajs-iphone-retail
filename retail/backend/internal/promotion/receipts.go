package promotion

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrUsageConflict = errors.New("promotion usage operation conflict")

// UsageOperation is an internal, caller-authorized checkout/order operation.
// A retry MUST reuse both keys. Possessing these identifiers grants no authority.
// Authorize scope ownership before either entry point, including receipt replay.
type UsageOperation struct{ ScopeID, OperationID string }
type UsageReceiptResult struct {
	Usage              UsageResult
	Replayed, Reverted bool
}
type usageDelta struct{ Kind, ID, CampaignID, BudgetID, Delta string }
type usageReceipt struct {
	RequestSHA256                 []byte
	State, DeltasJSON, ResultJSON string
}

func receiptTx(ctx context.Context, tx *gorm.DB, key UsageOperation) (*gorm.DB, context.CancelFunc, error) {
	noop := func() {}
	id, err := uuid.Parse(key.OperationID)
	if tx == nil || tx.Statement == nil || !usageID.MatchString(key.ScopeID) || err != nil || id.Version() != 4 || id.String() != key.OperationID {
		return nil, noop, ErrUsageInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, noop, ErrUsageInput
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	q := tx.WithContext(bounded)
	if Ready(bounded, q.Clauses(clause.Locking{Strength: "SHARE"})) != nil {
		cancel()
		return nil, noop, ErrSchema
	}
	return q, cancel, nil
}

func receiptDigest(actions []UsageAction, in UsageRegistration) ([32]byte, error) {
	empty := [32]byte{}
	if len(actions) > 1000 || len(in.Attributes) > 100 {
		return empty, ErrUsageInput
	}
	sums := map[string]*big.Rat{}
	for _, a := range actions {
		if !usageID.MatchString(a.PromotionID) {
			return empty, ErrUsageInput
		}
		amount, err := usageValue(a.Amount, nil)
		if err != nil || amount.Sign() <= 0 {
			return empty, ErrUsageInput
		}
		if old, ok := sums[a.PromotionID]; ok {
			old.Add(old, amount)
		} else {
			sums[a.PromotionID] = amount
		}
	}
	ids := []string{}
	for id := range sums {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	normalized := []UsageAction{}
	for _, id := range ids {
		_, raw, err := usageSQL(sums[id])
		if err != nil {
			return empty, err
		}
		var amount totals.RawValue
		if json.Unmarshal([]byte(raw), &amount) != nil {
			return empty, ErrUsageInput
		}
		normalized = append(normalized, UsageAction{PromotionID: id, Amount: amount.Value})
	}
	// Trusted time is intentionally excluded: retrying later must not change
	// a successfully committed registration. JSON map keys sort canonically.
	attributes := map[string]string{}
	for k, v := range in.Attributes {
		attributes[k] = v
	}
	raw, err := json.Marshal(struct {
		Actions    []UsageAction
		Currency   string
		Attributes map[string]string
	}{normalized, in.Currency, attributes})
	if err != nil || len(raw) > 1<<20 {
		return empty, ErrUsageInput
	}
	return sha256.Sum256(raw), nil
}

// RegisterUsageOnce records exact applied deltas and counters atomically in the
// caller's transaction. It supplies receipt idempotency, not scope authorization
// or transaction ownership. Any error requires the caller to roll back.
func RegisterUsageOnce(ctx context.Context, tx *gorm.DB, key UsageOperation, actions []UsageAction, in UsageRegistration) (UsageReceiptResult, error) {
	empty := UsageReceiptResult{}
	digest, err := receiptDigest(actions, in)
	if err != nil {
		return empty, err
	}
	tx, cancel, err := receiptTx(ctx, tx, key)
	if err != nil {
		return empty, err
	}
	defer cancel()
	if tx.Exec(`INSERT INTO promotion_usage_operations(scope_id,operation_id,request_sha256,state,deltas_json,result_json,created_at) VALUES(?,?,?,'pending','[]','{}',UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE operation_id=operation_id`, key.ScopeID, key.OperationID, digest[:]).Error != nil {
		return empty, ErrUsageStorage
	}
	var saved usageReceipt
	if tx.Table("promotion_usage_operations").Clauses(clause.Locking{Strength: "UPDATE"}).Where("scope_id=? AND operation_id=?", key.ScopeID, key.OperationID).Take(&saved).Error != nil {
		return empty, ErrUsageStorage
	}
	if subtle.ConstantTimeCompare(saved.RequestSHA256, digest[:]) != 1 || saved.State == "reverted" {
		return empty, ErrUsageConflict
	}
	if saved.State == "registered" {
		var result UsageResult
		if json.Unmarshal([]byte(saved.ResultJSON), &result) != nil {
			return empty, ErrUsageStorage
		}
		return UsageReceiptResult{Usage: result, Replayed: true}, nil
	}
	if saved.State != "pending" {
		return empty, ErrUsageConflict
	}
	deltas := []usageDelta{}
	result, err := changeUsage(tx.Statement.Context, tx, actions, in, false, &deltas)
	if err != nil {
		return empty, err
	}
	raw, err := json.Marshal(deltas)
	if err != nil {
		return empty, ErrUsageStorage
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return empty, ErrUsageStorage
	}
	r := tx.Table("promotion_usage_operations").Where("scope_id=? AND operation_id=? AND state='pending'", key.ScopeID, key.OperationID).Updates(map[string]any{"state": "registered", "deltas_json": string(raw), "result_json": string(encoded), "registered_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return empty, ErrUsageStorage
	}
	return UsageReceiptResult{Usage: result}, nil
}

// RevertUsageOnce reverses saved counters exactly once, without requalifying
// an expired/deactivated promotion or following its newly edited campaign.
// Missing rows, underflow or inconsistent evidence fail closed; no clamp can
// release amounts belonging to another operation. Caller must roll back errors.
func RevertUsageOnce(ctx context.Context, tx *gorm.DB, key UsageOperation) (UsageReceiptResult, error) {
	empty := UsageReceiptResult{}
	tx, cancel, err := receiptTx(ctx, tx, key)
	if err != nil {
		return empty, err
	}
	defer cancel()
	var saved usageReceipt
	if tx.Table("promotion_usage_operations").Clauses(clause.Locking{Strength: "UPDATE"}).Where("scope_id=? AND operation_id=?", key.ScopeID, key.OperationID).Take(&saved).Error != nil {
		return empty, ErrUsageConflict
	}
	var result UsageResult
	if json.Unmarshal([]byte(saved.ResultJSON), &result) != nil {
		return empty, ErrUsageStorage
	}
	if saved.State == "reverted" {
		return UsageReceiptResult{Usage: result, Replayed: true, Reverted: true}, nil
	}
	if saved.State != "registered" {
		return empty, ErrUsageConflict
	}
	var deltas []usageDelta
	if len(saved.DeltasJSON) > 1<<20 || json.Unmarshal([]byte(saved.DeltasJSON), &deltas) != nil || deltas == nil || len(deltas) > 3000 {
		return empty, ErrUsageStorage
	}
	order := map[string]int{"promotion": 0, "budget": 1, "attribute": 2}
	seen := map[string]bool{}
	campaigns := map[string]bool{}
	for _, d := range deltas {
		if _, ok := order[d.Kind]; !ok || !usageID.MatchString(d.ID) || seen[d.Kind+":"+d.ID] {
			return empty, ErrUsageStorage
		}
		seen[d.Kind+":"+d.ID] = true
		delta, e := usageValue(d.Delta, nil)
		if e != nil || delta.Sign() <= 0 {
			return empty, ErrUsageStorage
		}
		if d.Kind == "promotion" && d.Delta != "1" {
			return empty, ErrUsageStorage
		}
		if d.Kind == "budget" {
			if !usageID.MatchString(d.CampaignID) {
				return empty, ErrUsageStorage
			}
			campaigns[d.CampaignID] = true
		}
		if d.Kind == "attribute" && !usageID.MatchString(d.BudgetID) {
			return empty, ErrUsageStorage
		}
	}
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].Kind != deltas[j].Kind {
			return order[deltas[i].Kind] < order[deltas[j].Kind]
		}
		return deltas[i].ID < deltas[j].ID
	})
	// Match registration's parent lock order; campaign dates/status do not
	// override the historical registration, and soft-deleted rows stay usable.
	for _, d := range deltas {
		if d.Kind == "promotion" {
			var p struct{ ID string }
			if tx.Table("promotions").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id=?", d.ID).Take(&p).Error != nil {
				return empty, ErrUsageUnavailable
			}
		}
	}
	ids := []string{}
	for id := range campaigns {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var row struct{ ID string }
		if tx.Table("promotion_campaigns").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id=?", id).Take(&row).Error != nil {
			return empty, ErrUsageUnavailable
		}
	}
	type write struct {
		table, id, scalar, raw string
		remove                 bool
	}
	writes := []write{}
	for _, d := range deltas {
		if d.Kind == "attribute" && !seen["budget:"+d.BudgetID] {
			return empty, ErrUsageStorage
		}
		table := map[string]string{"promotion": "promotions", "budget": "promotion_campaign_budgets", "attribute": "promotion_budget_usages"}[d.Kind]
		var row struct {
			Used      string
			RawUsed   *string
			BudgetID  string
			DeletedAt *time.Time
		}
		selects := "CAST(used AS CHAR) AS used"
		if d.Kind != "promotion" {
			selects += ",raw_used,deleted_at"
		}
		if d.Kind == "attribute" {
			selects += ",budget_id"
		}
		if tx.Table(table).Clauses(clause.Locking{Strength: "UPDATE"}).Select(selects).Where("id=?", d.ID).Take(&row).Error != nil {
			return empty, ErrUsageUnavailable
		}
		if d.Kind == "attribute" && (row.BudgetID != d.BudgetID || row.DeletedAt != nil) {
			return empty, ErrUsageUnavailable
		}
		current, e := usageValue(row.Used, row.RawUsed)
		if e != nil {
			return empty, ErrUsageUnavailable
		}
		delta, e := usageValue(d.Delta, nil)
		if e != nil || current.Cmp(delta) < 0 {
			return empty, ErrUsageConflict
		}
		next := new(big.Rat).Sub(current, delta)
		scalar, raw, e := usageSQL(next)
		if e != nil {
			return empty, e
		}
		if d.Kind == "promotion" {
			if !next.IsInt() || !next.Num().IsInt64() {
				return empty, ErrUsageStorage
			}
			scalar = strconv.FormatInt(next.Num().Int64(), 10)
		}
		writes = append(writes, write{table, d.ID, scalar, raw, d.Kind == "attribute" && next.Sign() == 0})
	}
	for _, w := range writes {
		values := map[string]any{"used": w.scalar, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}
		if w.table != "promotions" {
			values["raw_used"] = w.raw
		}
		if w.remove {
			values["deleted_at"] = gorm.Expr("UTC_TIMESTAMP(6)")
		}
		r := tx.Table(w.table).Where("id=?", w.id).Updates(values)
		if r.Error != nil || r.RowsAffected != 1 {
			return empty, ErrUsageStorage
		}
	}
	r := tx.Table("promotion_usage_operations").Where("scope_id=? AND operation_id=? AND state='registered'", key.ScopeID, key.OperationID).Updates(map[string]any{"state": "reverted", "reverted_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return empty, ErrUsageStorage
	}
	return UsageReceiptResult{Usage: result, Reverted: true}, nil
}
