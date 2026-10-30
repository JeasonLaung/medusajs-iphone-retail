package buyback

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrConflict = errors.New("buyback submission conflicts with existing request")
var ErrUnavailable = errors.New("buyback requests unavailable")

type History struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Actor  string    `json:"actor"`
	Status string    `json:"status"`
}
type CreatedRequest struct {
	ID                   string            `json:"id"`
	Status               string            `json:"status"`
	ContactName          string            `json:"contact_name"`
	Email                string            `json:"email"`
	Phone                string            `json:"phone"`
	ReturnAddress        ReturnAddress     `json:"return_address"`
	Items                []RequestItem     `json:"items"`
	Photos               []json.RawMessage `json:"photos"`
	ReferenceAmount      *json.Number      `json:"reference_amount"`
	FinalAmount          *json.Number      `json:"final_amount"`
	CurrencyCode         string            `json:"currency_code"`
	ShippingInstructions string            `json:"shipping_instructions"`
	TrackingNumber       string            `json:"tracking_number"`
	ReturnTracking       string            `json:"return_tracking"`
	PaymentReference     string            `json:"payment_reference"`
	ReviewNote           string            `json:"review_note"`
	History              []History         `json:"history"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	RetentionDays        int               `json:"retention_days"`
}

// StageCreate requires an authenticated caller and an existing transaction.
// The account lock serializes submissions with revocation/account changes;
// customer.Store rechecks the Redis session after acquiring the same lock.
// No commit or provider/notification side effects occur here.
func StageCreate(ctx context.Context, tx *gorm.DB, storeID, accountID string, in Submission) (CreatedRequest, error) {
	var out CreatedRequest
	if tx == nil || !validID(storeID, 191) || !validID(accountID, 80) {
		return out, ErrInput
	}
	raw, e := json.Marshal(in)
	if e != nil {
		return out, ErrInput
	}
	in, e = ParseSubmission(raw)
	if e != nil {
		return out, e
	}
	raw, _ = json.Marshal(in)
	tx = tx.WithContext(ctx)
	var account struct {
		ID, Email  string
		Enabled    bool
		VerifiedAt *time.Time
	}
	if tx.Table("customer_accounts").Select("id,email,enabled,verified_at").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", accountID).Take(&account).Error != nil || !account.Enabled || account.VerifiedAt == nil {
		return out, ErrInput
	}
	var store struct{ ID string }
	if tx.Table("commerce_stores").Select("id").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=? AND deleted_at IS NULL", storeID).Take(&store).Error != nil {
		return out, ErrUnavailable
	}
	var receipt struct{ StoreID, RequestID, RequestJSON, ResultJSON string }
	e = tx.Table("customer_buyback_submissions").Where("customer_id=? AND submission_key=?", accountID, in.SubmissionKey).Take(&receipt).Error
	if e == nil {
		if receipt.StoreID != storeID || receipt.RequestJSON != string(raw) {
			return out, ErrConflict
		}
		if json.Unmarshal([]byte(receipt.ResultJSON), &out) != nil || out.ID != receipt.RequestID || out.Status != "awaiting_photos" || len(out.Items) < 1 || len(out.Items) > 10 {
			return CreatedRequest{}, ErrUnavailable
		}
		return out, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrUnavailable
	}
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("buyback_request_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || !gate.Enabled {
		return out, ErrUnavailable
	}
	prepared, e := PrepareSubmission(ctx, tx, in)
	if e != nil {
		return out, e
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	out = CreatedRequest{ID: "bbr_" + uuid.NewString(), Status: "awaiting_photos", ContactName: in.ContactName, Email: account.Email, Phone: in.Phone, ReturnAddress: in.ReturnAddress, Items: prepared.Items, Photos: []json.RawMessage{}, ReferenceAmount: prepared.ReferenceAmount, CurrencyCode: "eur", History: []History{{At: now, Action: "submitted", Actor: "customer", Status: "awaiting_photos"}}, CreatedAt: now, UpdatedAt: now, RetentionDays: 180}
	encode := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	var amount, rawAmount any
	if out.ReferenceAmount != nil {
		amount = string(*out.ReferenceAmount)
		rawAmount = encode(map[string]any{"value": string(*out.ReferenceAmount), "precision": 100})
	}
	row := map[string]any{"id": out.ID, "customer_id": accountID, "submission_key": in.SubmissionKey, "contact_name": out.ContactName, "email": out.Email, "phone": out.Phone, "return_address": encode(out.ReturnAddress), "items": encode(out.Items), "status": out.Status, "reference_amount": amount, "raw_reference_amount": rawAmount, "final_amount": nil, "raw_final_amount": nil, "currency_code": "eur", "shipping_instructions": "", "tracking_number": "", "return_tracking": "", "payment_reference": "", "review_note": "", "history": encode(out.History), "created_at": now, "updated_at": now}
	if tx.Table("buyback_requests").Create(row).Error != nil {
		return CreatedRequest{}, ErrUnavailable
	}
	receiptRow := map[string]any{"customer_id": accountID, "submission_key": in.SubmissionKey, "store_id": storeID, "request_id": out.ID, "request_json": string(raw), "result_json": encode(out), "created_at": now}
	if tx.Table("customer_buyback_submissions").Create(receiptRow).Error != nil {
		return CreatedRequest{}, ErrUnavailable
	}
	return out, nil
}
