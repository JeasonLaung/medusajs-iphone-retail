package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Private notification storage API. Every function needs an explicit caller
// transaction; rollback on any error. No HTTP, implicit commit or order writes.
type MollieNotification struct {
	ProviderID, RemoteID                               string
	Generation, ProcessedGeneration, ClaimedGeneration uint64
	State                                              string
	AttemptID                                          *string
	LeaseUntil                                         *time.Time
	AvailableAt                                        time.Time
	Dispatch                                           bool           `gorm:"-"`
	Request                                            SessionRequest `gorm:"-"`
}

type mollieReference struct {
	ProviderID, RemoteID, ScopeID, OperationID, EffectKind, ResourceID string
	RequestSHA256                                                      []byte
}

type MollieNotificationLocator struct{ ProviderID, RemoteID string }

// DueMollieNotifications returns bounded server work locators. This snapshot
// grants no dispatch; each locator still needs a committed fenced claim.
func DueMollieNotifications(ctx context.Context, tx *gorm.DB, limit int) ([]MollieNotificationLocator, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrEffectInput
	}
	q, cancel, err := notificationTx(ctx, tx, true)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var rows []MollieNotificationLocator
	if q.Table("mollie_notification_inbox").Select("provider_id,remote_id").Where("(state='pending' AND available_at<=UTC_TIMESTAMP(6)) OR (state='inflight' AND lease_until<=UTC_TIMESTAMP(6))").Order("available_at,provider_id,remote_id").Limit(limit).Find(&rows).Error != nil {
		return nil, ErrEffectStorage
	}
	return rows, nil
}

// ReadMollieNotificationObservation authenticates immutable private evidence for
// the future durable business consumer. Reading does not mark it consumed or
// grant order/payment authority. Missing keys/corruption never fall back to JSON.
func ReadMollieNotificationObservation(ctx context.Context, tx *gorm.DB, attempt string, ring *protected.Ring) (SessionObservation, error) {
	id, err := uuid.Parse(attempt)
	if err != nil || id.Version() != 4 || id.String() != attempt {
		return SessionObservation{}, ErrEffectInput
	}
	q, cancel, err := notificationTx(ctx, tx, true)
	if err != nil {
		return SessionObservation{}, err
	}
	defer cancel()
	var row struct {
		ProviderID, RemoteID string
		Generation           uint64
		ResultEnvelope       []byte
	}
	if q.Table("mollie_notification_observations").Where("attempt_id=?", attempt).Take(&row).Error != nil {
		return SessionObservation{}, ErrEffectStorage
	}
	raw, err := ring.Open(notificationContext(attempt), row.ResultEnvelope)
	if err != nil {
		return SessionObservation{}, ErrEffectProtection
	}
	defer clear(raw)
	var observed SessionObservation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&observed) != nil {
		return SessionObservation{}, ErrEffectProtection
	}
	canonical, err := json.Marshal(observed)
	if err != nil || !bytes.Equal(raw, canonical) || observed.PaymentID != row.RemoteID || !validMollieLocator(row.ProviderID, row.RemoteID) || row.Generation == 0 {
		return SessionObservation{}, ErrEffectProtection
	}
	return observed, nil
}

func (r mollieReference) key() EffectKey {
	key := EffectKey{ScopeID: r.ScopeID, OperationID: r.OperationID, Kind: r.EffectKind, ResourceID: r.ResourceID}
	copy(key.Digest[:], r.RequestSHA256)
	return key
}

func notificationTx(ctx context.Context, tx *gorm.DB, enabled bool) (*gorm.DB, context.CancelFunc, error) {
	noop := func() {}
	if tx == nil || tx.Statement == nil {
		return nil, noop, ErrEffectInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, noop, ErrEffectInput
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	q := tx.WithContext(bounded)
	var runtime struct {
		Version int
		Enabled bool
	}
	if Ready(bounded, q) != nil || q.Table("mollie_notification_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&runtime).Error != nil || runtime.Version != 1 || enabled && !runtime.Enabled {
		cancel()
		return nil, noop, ErrSchema
	}
	return q, cancel, nil
}

func validMollieLocator(provider, remote string) bool {
	_, ok := mollieMethods[provider]
	return ok && molliePaymentID.MatchString(remote)
}
func referenceWhere(q *gorm.DB, provider, remote string) *gorm.DB {
	return q.Table("mollie_payment_references").Where("provider_id=? AND remote_id=?", provider, remote)
}
func notificationWhere(q *gorm.DB, provider, remote string) *gorm.DB {
	return q.Table("mollie_notification_inbox").Where("provider_id=? AND remote_id=?", provider, remote)
}

// Called atomically by RecordSessionCreation for known Mollie providers. Keep
// references even when receiving is disabled; a later webhook must find the old
// remote payment after local session replacement. Never backfill by email/ID.
func registerMollieReference(ctx context.Context, tx *gorm.DB, key EffectKey, ring *protected.Ring, result CreatedSession) error {
	request, err := ReadSessionRequest(ctx, tx, key, ring)
	if err != nil {
		return err
	}
	if _, ok := mollieMethods[request.ProviderID]; !ok {
		return nil
	}
	var payment molliePayment
	var cart struct {
		CartID string `json:"cart_id"`
	}
	if json.Unmarshal(request.Data, &cart) != nil || !mollieCartID.MatchString(cart.CartID) || json.Unmarshal(result.Data, &payment) != nil || !validMollieLocator(request.ProviderID, payment.ID) || !verifyMolliePayment(payment, request, payment.ID, cart.CartID) {
		return ErrEffectInput
	}
	q, cancel, err := notificationTx(ctx, tx, false)
	if err != nil {
		return err
	}
	defer cancel()
	var existing mollieReference
	lookupErr := referenceWhere(q, request.ProviderID, payment.ID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&existing).Error
	if lookupErr == nil {
		if existing.key() != key || len(existing.RequestSHA256) != 32 {
			return ErrEffectConflict
		}
		return nil
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return ErrEffectStorage
	}
	if q.Exec(`INSERT INTO mollie_payment_references(provider_id,remote_id,scope_id,operation_id,effect_kind,resource_id,request_sha256,created_at) VALUES(?,?,?,?,?,?,?,UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE remote_id=mollie_payment_references.remote_id`, request.ProviderID, payment.ID, key.ScopeID, key.OperationID, key.Kind, key.ResourceID, key.Digest[:]).Error != nil {
		return ErrEffectStorage
	}
	var saved mollieReference
	if referenceWhere(q, request.ProviderID, payment.ID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&saved).Error != nil {
		return ErrEffectConflict
	}
	if saved.key() != key || saved.ProviderID != request.ProviderID || len(saved.RequestSHA256) != 32 {
		return ErrEffectConflict
	}
	return nil
}

// EnqueueMollieNotification stores only KNOWN local references. False,nil is an
// unknown ID, deliberately suitable for the same HTTP ACK as a known ID. No raw
// body/status/amount/metadata becomes a trusted fact, and unknown IDs grow no rows.
func EnqueueMollieNotification(ctx context.Context, tx *gorm.DB, provider, remote string) (bool, error) {
	if !validMollieLocator(provider, remote) {
		return false, ErrEffectInput
	}
	q, cancel, err := notificationTx(ctx, tx, true)
	if err != nil {
		return false, err
	}
	defer cancel()
	var ref mollieReference
	err = referenceWhere(q, provider, remote).Clauses(clause.Locking{Strength: "SHARE"}).Take(&ref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, ErrEffectStorage
	}
	if q.Exec(`INSERT INTO mollie_notification_inbox(provider_id,remote_id,generation,state,available_at,created_at,updated_at) VALUES(?,?,1,'pending',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE generation=generation+1,available_at=IF(state='idle',UTC_TIMESTAMP(6),available_at),state=IF(state='idle','pending',state),updated_at=UTC_TIMESTAMP(6)`, provider, remote).Error != nil {
		return false, ErrEffectStorage
	}
	return true, nil
}

func lockNotification(q *gorm.DB, provider, remote string) (MollieNotification, error) {
	var row MollieNotification
	if notificationWhere(q, provider, remote).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
		return row, ErrEffectStorage
	}
	return row, nil
}

// ClaimMollieNotification is a lease for a SAFE authoritative GET, not a payment
// creation/cancellation dispatch. Expired leases may be re-queried under a new
// fenced attempt. Call the observer only AFTER committing this transaction.
func ClaimMollieNotification(ctx context.Context, tx *gorm.DB, provider, remote string, ring *protected.Ring, lease time.Duration) (MollieNotification, error) {
	if !validMollieLocator(provider, remote) || lease < 10*time.Second || lease > 5*time.Minute {
		return MollieNotification{}, ErrEffectInput
	}
	q, cancel, err := notificationTx(ctx, tx, true)
	if err != nil {
		return MollieNotification{}, err
	}
	defer cancel()
	row, err := lockNotification(q, provider, remote)
	if err != nil {
		return row, err
	}
	var now time.Time
	if q.Raw("SELECT UTC_TIMESTAMP(6)").Scan(&now).Error != nil {
		return MollieNotification{}, ErrEffectStorage
	}
	if row.State == "idle" || row.State == "inflight" && row.LeaseUntil != nil && now.Before(*row.LeaseUntil) || row.State == "pending" && now.Before(row.AvailableAt) {
		return row, nil
	}
	var ref mollieReference
	if referenceWhere(q, provider, remote).Take(&ref).Error != nil || len(ref.RequestSHA256) != 32 {
		return MollieNotification{}, ErrEffectStorage
	}
	request, err := ReadSessionRequest(ctx, tx, ref.key(), ring)
	if err != nil {
		return MollieNotification{}, err
	}
	created, err := ReadSessionCreation(ctx, tx, ref.key(), ring)
	if err != nil {
		return MollieNotification{}, err
	}
	var saved struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(created.Data, &saved) != nil || saved.ID != remote || request.ProviderID != provider || request.SessionID != ref.ResourceID {
		return MollieNotification{}, ErrEffectConflict
	}
	attempt := uuid.NewString()
	until := now.Add(lease).Truncate(time.Microsecond)
	if notificationWhere(q, provider, remote).Updates(map[string]any{"state": "inflight", "attempt_id": attempt, "lease_until": until, "claimed_generation": row.Generation, "updated_at": now}).Error != nil {
		return MollieNotification{}, ErrEffectStorage
	}
	row.State = "inflight"
	row.AttemptID = &attempt
	row.LeaseUntil = &until
	row.ClaimedGeneration = row.Generation
	row.Dispatch = true
	row.Request = request
	return row, nil
}

func notificationContext(attempt string) protected.Context {
	return protected.Context{Purpose: "mollie-observation-v1", Table: "mollie_notification_observations", Column: "result_envelope", Row: attempt}
}

// CompleteMollieNotification saves VERIFIED provider evidence and acknowledges
// only the generation this worker claimed in one transaction. Business payment
// processing is a later durable consumer; this does NOT mark any order paid.
func CompleteMollieNotification(ctx context.Context, tx *gorm.DB, claim MollieNotification, observed SessionObservation, ring *protected.Ring) error {
	if !validMollieLocator(claim.ProviderID, claim.RemoteID) || claim.AttemptID == nil {
		return ErrEffectInput
	}
	id, err := uuid.Parse(*claim.AttemptID)
	if err != nil || id.Version() != 4 || id.String() != *claim.AttemptID {
		return ErrEffectInput
	}
	q, cancel, err := notificationTx(ctx, tx, true)
	if err != nil {
		return err
	}
	defer cancel()
	row, err := lockNotification(q, claim.ProviderID, claim.RemoteID)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(observed)
	if err != nil || len(raw) > protected.MaxPlaintext {
		return ErrEffectInput
	}
	defer clear(raw)
	var previous struct {
		ProviderID, RemoteID string
		Generation           uint64
		ResultEnvelope       []byte
	}
	err = q.Table("mollie_notification_observations").Where("attempt_id=?", *claim.AttemptID).Take(&previous).Error
	if err == nil {
		decoded, err := ring.Open(notificationContext(*claim.AttemptID), previous.ResultEnvelope)
		if err != nil {
			return ErrEffectProtection
		}
		defer clear(decoded)
		if previous.ProviderID != claim.ProviderID || previous.RemoteID != claim.RemoteID || previous.Generation != claim.ClaimedGeneration || !bytes.Equal(decoded, raw) {
			return ErrEffectConflict
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrEffectStorage
	}
	if row.State != "inflight" || row.AttemptID == nil || *row.AttemptID != *claim.AttemptID || row.ClaimedGeneration != claim.ClaimedGeneration {
		return ErrEffectConflict
	}
	var ref mollieReference
	if referenceWhere(q, claim.ProviderID, claim.RemoteID).Take(&ref).Error != nil || len(ref.RequestSHA256) != 32 {
		return ErrEffectStorage
	}
	request, err := ReadSessionRequest(ctx, tx, ref.key(), ring)
	if err != nil {
		return err
	}
	data, err := requestObject(observed.Session.Data, false)
	if err != nil {
		return err
	}
	var payment molliePayment
	var cart struct {
		CartID string `json:"cart_id"`
	}
	if json.Unmarshal(request.Data, &cart) != nil || json.Unmarshal(data, &payment) != nil || observed.PaymentID != claim.RemoteID || observed.ProviderStatus != payment.Status || !verifyMolliePayment(payment, request, claim.RemoteID, cart.CartID) {
		return ErrEffectConflict
	}
	actions := map[string]string{"open": "not_supported", "pending": "pending", "paid": "successful", "authorized": "authorized", "canceled": "canceled", "expired": "failed", "failed": "failed"}
	statuses := map[string]string{"open": "pending", "pending": "pending", "paid": "authorized", "authorized": "authorized", "canceled": "canceled", "expired": "error", "failed": "error"}
	expectedAmount, err := mollieAmount(request.Amount)
	if err != nil {
		return err
	}
	observedAmount, err := totals.Resolve(totals.Number{Decimal: observed.Amount})
	expectedAmount, canonicalErr := totals.Resolve(totals.Number{Decimal: expectedAmount})
	if err != nil || canonicalErr != nil || observedAmount != expectedAmount || observed.Currency != request.Currency || actions[payment.Status] == "" || observed.Action != actions[payment.Status] || observed.Session.Status != statuses[payment.Status] {
		return ErrEffectConflict
	}
	envelope, err := ring.Seal(notificationContext(*claim.AttemptID), raw)
	if err != nil {
		return ErrEffectProtection
	}
	defer clear(envelope)
	if q.Exec(`INSERT INTO mollie_notification_observations(attempt_id,provider_id,remote_id,generation,result_envelope,created_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(6))`, *claim.AttemptID, claim.ProviderID, claim.RemoteID, claim.ClaimedGeneration, envelope).Error != nil {
		return ErrEffectStorage
	}
	state := "idle"
	if row.Generation > row.ClaimedGeneration {
		state = "pending"
	}
	if notificationWhere(q, claim.ProviderID, claim.RemoteID).Updates(map[string]any{"state": state, "processed_generation": row.ClaimedGeneration, "lease_until": nil, "available_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
		return ErrEffectStorage
	}
	return nil
}

// Retry is only a delayed GET retry. The old attempt loses its right to ACK.
func RetryMollieNotification(ctx context.Context, tx *gorm.DB, claim MollieNotification, delay time.Duration) error {
	if !validMollieLocator(claim.ProviderID, claim.RemoteID) || claim.AttemptID == nil || delay < time.Second || delay > time.Hour {
		return ErrEffectInput
	}
	q, cancel, err := notificationTx(ctx, tx, true)
	if err != nil {
		return err
	}
	defer cancel()
	row, err := lockNotification(q, claim.ProviderID, claim.RemoteID)
	if err != nil {
		return err
	}
	if row.State != "inflight" || row.AttemptID == nil || *row.AttemptID != *claim.AttemptID || row.ClaimedGeneration != claim.ClaimedGeneration {
		return ErrEffectConflict
	}
	var now time.Time
	if q.Raw("SELECT UTC_TIMESTAMP(6)").Scan(&now).Error != nil {
		return ErrEffectStorage
	}
	if notificationWhere(q, claim.ProviderID, claim.RemoteID).Updates(map[string]any{"state": "pending", "lease_until": nil, "available_at": now.Add(delay), "updated_at": now}).Error != nil {
		return ErrEffectStorage
	}
	return nil
}
