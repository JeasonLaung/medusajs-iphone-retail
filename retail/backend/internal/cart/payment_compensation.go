package cart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentCompensation struct {
	OperationID, State, Version string
	Replacements                []PaymentSessionReplacement
	Replayed                    bool
}
type PaymentSessionReplacement struct {
	OriginalID string
	Create     payments.EffectKey
}
type compensationManifest struct {
	Format       int
	Replacements []PaymentSessionReplacement
}
type compensationRow struct {
	State, ManifestJSON string
	ManifestSHA256      []byte
	ResultVersion       *uint64
}

func compensationWhere(tx *gorm.DB, ref sessionstore.CartReference, in PaymentRefreshCommand) *gorm.DB {
	return tx.Table("cart_payment_refresh_compensations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID)
}
func replacementRequest(original payments.SessionRequest, id string) (payments.SessionRequest, error) {
	var data map[string]json.RawMessage
	if json.Unmarshal(original.Data, &data) != nil || data == nil {
		return payments.SessionRequest{}, ErrSchema
	}
	encoded, _ := json.Marshal(id)
	data["session_id"] = encoded
	if _, exists := data["mollie_attempt"]; exists {
		data["mollie_attempt"] = encoded
	}
	var context map[string]json.RawMessage
	if len(original.Context) != 0 && string(original.Context) != "null" && json.Unmarshal(original.Context, &context) != nil {
		return payments.SessionRequest{}, ErrSchema
	}
	if context == nil {
		context = map[string]json.RawMessage{}
	}
	context["idempotency_key"] = encoded
	original.SessionID = id
	original.Data, _ = json.Marshal(data)
	original.Context, _ = json.Marshal(context)
	return original, nil
}

// StagePaymentCompensation irreversibly chooses compensation for this refresh,
// preventing further delete dispatch or normal completion. First reconcile ALL
// inflight/uncertain deletes. Only succeeded deletes receive stable new local
// session IDs and encrypted create requests; failed/pending originals remain.
// It rechecks the same current cart authority/version/source/freeze as refresh.
// Caller must roll back on any error. No external call or local session write.
func StagePaymentCompensation(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, ring *protected.Ring) (PaymentCompensation, error) {
	failed := PaymentCompensation{}
	if err := paymentOperationReady(ctx, tx, in); err != nil {
		return failed, err
	}
	if !refOK(store, ref) {
		return failed, ErrDenied
	}
	channel, err := storeChannel(ctx, tx, store)
	if err != nil {
		return failed, err
	}
	current, err := lockCurrentPaymentOperation(ctx, tx, store, channel, ref, "UPDATE", in.OperationID)
	if err != nil {
		return failed, err
	}
	original, source, found, err := readPaymentOperation(tx, ref, in)
	if err != nil {
		return failed, err
	}
	if !found {
		return failed, ErrConflict
	}
	var row compensationRow
	r := compensationWhere(tx, ref, in).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row)
	exists := r.Error == nil
	if r.Error != nil && !errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return failed, ErrStorage
	}
	manifest := compensationManifest{Format: 1, Replacements: []PaymentSessionReplacement{}}
	if exists {
		raw := []byte(row.ManifestJSON)
		digest := sha256.Sum256(raw)
		if len(raw) > 1<<20 || subtle.ConstantTimeCompare(row.ManifestSHA256, digest[:]) != 1 {
			return failed, ErrStorage
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if d.Decode(&manifest) != nil || d.Decode(new(any)) != io.EOF || manifest.Format != 1 || len(manifest.Replacements) > len(source.Effects) {
			return failed, ErrStorage
		}
		seen := map[string]bool{}
		originalIDs := map[string]bool{}
		for _, key := range source.Effects {
			originalIDs[key.ResourceID] = true
		}
		for _, replacement := range manifest.Replacements {
			key := replacement.Create
			if !originalIDs[replacement.OriginalID] || seen[replacement.OriginalID] || key.ScopeID != ref.CartID || key.OperationID != in.OperationID || key.Kind != "create_session" || !idPattern.MatchString(key.ResourceID) || key.ResourceID == replacement.OriginalID || key.Digest == ([32]byte{}) {
				return failed, ErrStorage
			}
			seen[replacement.OriginalID] = true
		}
		if row.State == "completed" {
			v, _ := strconv.ParseUint(current.Version, 10, 64)
			if original.State != "completed" || row.ResultVersion == nil || *row.ResultVersion != original.CartVersion+1 || v < *row.ResultVersion {
				return failed, ErrConflict
			}
			return PaymentCompensation{in.OperationID, row.State, strconv.FormatUint(*row.ResultVersion, 10), manifest.Replacements, true}, nil
		}
		if row.State != "staged" || row.ResultVersion != nil {
			return failed, ErrStorage
		}
	}
	if original.State != "staged" || current.Version != in.Version {
		return failed, ErrConflict
	}
	var owner struct{ PaymentRefreshOperationID *string }
	if tx.Table("cart_ownership").Select("payment_refresh_operation_id").Where("cart_id=?", ref.CartID).Take(&owner).Error != nil || owner.PaymentRefreshOperationID == nil || *owner.PaymentRefreshOperationID != in.OperationID {
		return failed, ErrConflict
	}
	plan, err := preparePaymentRefreshOperation(ctx, tx, store, ref, in.Version, in.OperationID)
	if err != nil {
		return failed, err
	}
	_, _, fingerprint, err := paymentSources(ctx, tx, ref.CartID, plan)
	if err != nil {
		return failed, err
	}
	if !reflect.DeepEqual(plan, source.Plan) || fingerprint != source.SourceSHA256 {
		return failed, ErrConflict
	}
	succeeded := []payments.EffectKey{}
	for _, key := range source.Effects {
		receipt, err := payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return failed, err
		}
		switch receipt.State {
		case "succeeded":
			succeeded = append(succeeded, key)
		case "pending", "failed":
		default:
			return failed, ErrConflict
		}
	}
	if exists {
		if len(succeeded) != len(manifest.Replacements) {
			return failed, ErrConflict
		}
		for i, key := range succeeded {
			if manifest.Replacements[i].OriginalID != key.ResourceID {
				return failed, ErrConflict
			}
			if _, err := payments.ReadSessionRequest(ctx, tx, manifest.Replacements[i].Create, ring); err != nil {
				return failed, err
			}
		}
		return PaymentCompensation{in.OperationID, row.State, in.Version, manifest.Replacements, true}, nil
	}
	for _, key := range succeeded {
		original, err := payments.ReadSessionRequest(ctx, tx, key, ring)
		if err != nil {
			return failed, err
		}
		id := "payses_" + uuid.NewString()
		request, err := replacementRequest(original, id)
		if err != nil {
			return failed, err
		}
		replacement, _, err := payments.StageSessionRequest(ctx, tx, payments.EffectKey{ScopeID: ref.CartID, OperationID: in.OperationID, Kind: "create_session", ResourceID: id}, ring, request)
		if err != nil {
			return failed, err
		}
		manifest.Replacements = append(manifest.Replacements, PaymentSessionReplacement{key.ResourceID, replacement})
	}
	raw, err := json.Marshal(manifest)
	if err != nil || len(raw) > 1<<20 {
		return failed, ErrStorage
	}
	digest := sha256.Sum256(raw)
	if tx.Table("cart_payment_refresh_compensations").Create(map[string]any{"cart_id": ref.CartID, "operation_id": in.OperationID, "state": "staged", "manifest_json": string(raw), "manifest_sha256": digest[:], "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
		return failed, ErrStorage
	}
	return PaymentCompensation{in.OperationID, "staged", in.Version, manifest.Replacements, false}, nil
}

func ClaimPaymentCompensationSession(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, originalID string, ring *protected.Ring) (payments.EffectReceipt, payments.SessionRequest, error) {
	operation, err := StagePaymentCompensation(ctx, tx, store, ref, in, ring)
	if err != nil {
		return payments.EffectReceipt{}, payments.SessionRequest{}, err
	}
	if operation.State != "staged" {
		return payments.EffectReceipt{}, payments.SessionRequest{}, ErrConflict
	}
	for _, replacement := range operation.Replacements {
		if replacement.OriginalID == originalID {
			return payments.ClaimSessionRequest(ctx, tx, replacement.Create, ring, 30*time.Second)
		}
	}
	return payments.EffectReceipt{}, payments.SessionRequest{}, ErrInput
}

// CompletePaymentCompensation activates only verified, protected, unpaid create
// results. Atomically insert new sessions, soft-delete only replaced originals,
// advance cart version and release freeze. Preserve old collection amount and
// currency. A success receipt without its authenticated result is insufficient.
func CompletePaymentCompensation(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, ring *protected.Ring) (PaymentCompensation, error) {
	failed := PaymentCompensation{}
	operation, err := StagePaymentCompensation(ctx, tx, store, ref, in, ring)
	if err != nil {
		return failed, err
	}
	if operation.State == "completed" {
		return operation, nil
	}
	for _, replacement := range operation.Replacements {
		request, err := payments.ReadSessionRequest(ctx, tx, replacement.Create, ring)
		if err != nil {
			return failed, err
		}
		result, err := payments.ReadSessionCreation(ctx, tx, replacement.Create, ring)
		if err != nil {
			return failed, err
		}
		switch result.Status {
		case "pending", "requires_more", "error", "canceled":
		default:
			return failed, ErrConflict
		}
		var data, returned map[string]json.RawMessage
		if json.Unmarshal(request.Data, &data) != nil || json.Unmarshal(result.Data, &returned) != nil {
			return failed, ErrSchema
		}
		for key, value := range returned {
			data[key] = value
		}
		data["session_id"], _ = json.Marshal(request.SessionID)
		request.Data, _ = json.Marshal(data)
		protectedData, protectedContext, err := payments.SealSessionFields(ring, request)
		if err != nil {
			return failed, err
		}
		amount, raw, err := sqlAmount(request.Amount)
		if err != nil {
			return failed, err
		}
		if tx.Table("payment_sessions").Create(map[string]any{"id": request.SessionID, "provider_id": request.ProviderID, "payment_collection_id": request.CollectionID, "currency_code": request.Currency, "amount": amount, "raw_amount": raw, "data": protectedData, "context": protectedContext, "status": result.Status, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return failed, ErrStorage
		}
		r := tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", replacement.OriginalID, request.CollectionID).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != 1 {
			return failed, ErrStorage
		}
	}
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	version++
	r := tx.Table("cart_ownership").Where("cart_id=? AND version=? AND payment_refresh_operation_id=?", ref.CartID, in.Version, in.OperationID).Updates(map[string]any{"version": version, "payment_refresh_operation_id": nil, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return failed, ErrConflict
	}
	r = compensationWhere(tx, ref, in).Where("state='staged'").Updates(map[string]any{"state": "completed", "result_version": version, "completed_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return failed, ErrStorage
	}
	r = tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=? AND state='staged'", ref.CartID, in.OperationID).Updates(map[string]any{"state": "completed", "result_version": version, "completed_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return failed, ErrStorage
	}
	operation.State = "completed"
	operation.Version = strconv.FormatUint(version, 10)
	operation.Replayed = false
	return operation, nil
}
