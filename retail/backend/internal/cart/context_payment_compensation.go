package cart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"reflect"
	"strconv"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Context compensation uses Format2 and the original CREATE request, never
// a provider response repurposed as creation data. Legacy decoders reject it.
type contextCompensationManifest struct {
	Format       int
	Replacements []PaymentSessionReplacement
}

// contextPaymentOriginal locks immutable intent and ORIGINAL source without
// repricing its target. Catalogue drift must not strand compensation. Current
// cart/checkout/generation, exact request and authenticated payment still bind.
func contextPaymentOriginal(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (contextPaymentManifest, payments.SessionRequest, error) {
	fail := func(err error) (contextPaymentManifest, payments.SessionRequest, error) {
		return contextPaymentManifest{}, payments.SessionRequest{}, err
	}
	if err := paymentOperationReady(ctx, tx, PaymentRefreshCommand{OperationID: in.OperationID, Version: in.Version}); err != nil {
		return fail(err)
	}
	var gate struct {
		Version int
		Enabled bool
	}
	r := tx.Raw("SELECT version,enabled FROM cart_context_runtime WHERE id=1 FOR SHARE").Scan(&gate)
	if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
		return fail(ErrSchema)
	}
	channel, err := storeChannel(ctx, tx, store)
	if err != nil {
		return fail(err)
	}
	var owner struct{ CheckoutAttemptID, PaymentRefreshOperationID *string }
	if tx.Table("cart_ownership").Select("checkout_attempt_id,payment_refresh_operation_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
		return fail(ErrDenied)
	}
	if owner.CheckoutAttemptID == nil || owner.PaymentRefreshOperationID == nil || *owner.PaymentRefreshOperationID != in.OperationID {
		return fail(ErrConflict)
	}
	current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", in.OperationID, *owner.CheckoutAttemptID)
	if err != nil {
		return fail(err)
	}
	if current.Version != in.Version {
		return fail(ErrConflict)
	}
	if err := validateContextPaymentCheckout(ctx, tx, ref, current, *owner.CheckoutAttemptID, options); err != nil {
		return fail(err)
	}
	where := func() *gorm.DB {
		return tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var sizes []struct{ Bytes int64 }
	if where().Select("OCTET_LENGTH(manifest_json) AS bytes").Scan(&sizes).Error != nil {
		return fail(ErrStorage)
	}
	if len(sizes) != 1 {
		return fail(ErrConflict)
	}
	if sizes[0].Bytes > 1<<20 {
		return fail(ErrStorage)
	}
	var row paymentOperationRow
	if where().Take(&row).Error != nil {
		return fail(ErrStorage)
	}
	raw := []byte(row.ManifestJSON)
	digest := sha256.Sum256(raw)
	var manifest contextPaymentManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if subtle.ConstantTimeCompare(digest[:], row.ManifestSHA256) != 1 || d.Decode(&manifest) != nil || d.Decode(new(any)) != io.EOF || manifest.Format != 2 || len(manifest.Effects) > 100 {
		return fail(ErrStorage)
	}
	request, _ := json.Marshal(in)
	requestSHA := sha256.Sum256(request)
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	if row.State != "staged" || row.ResultVersion != nil || row.Generation != ref.Generation || row.CartVersion != version || manifest.RequestSHA256 != requestSHA || manifest.Target.CartID != ref.CartID || manifest.Target.Version != in.Version || manifest.Target.CheckoutOperationID != *owner.CheckoutAttemptID {
		return fail(ErrConflict)
	}
	plan := manifest.Target.Payment
	plan.UpdateCollection = true
	if tx.Table("payment_sessions").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", plan.CollectionID).Order("id").Limit(101).Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&plan.DeleteSessionIDs).Error != nil || len(plan.DeleteSessionIDs) > 100 {
		return fail(ErrSchema)
	}
	_, _, sourceSHA, err := paymentSources(ctx, tx, ref.CartID, plan)
	if err != nil {
		return fail(err)
	}
	if sourceSHA != manifest.Target.SourceSHA256 || len(manifest.Effects) != len(manifest.Target.Payment.DeleteSessionIDs) {
		return fail(ErrConflict)
	}
	for i, key := range manifest.Effects {
		if key.ScopeID != ref.CartID || key.OperationID != in.OperationID || key.Kind != "delete_session" || key.ResourceID != manifest.Target.Payment.DeleteSessionIDs[i] {
			return fail(ErrStorage)
		}
		if _, err := payments.ReadSessionRequest(ctx, tx, key, options.Ring); err != nil {
			return fail(err)
		}
	}
	var original initialAttempt
	command := InitialPaymentCommand{OperationID: *owner.CheckoutAttemptID}
	if initialWhere(tx, ref, command).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&original).Error != nil {
		return fail(ErrStorage)
	}
	command.Version = strconv.FormatUint(original.InputVersion, 10)
	key, err := validateInitialBinding(ctx, tx, ref, command, original, options)
	if err != nil {
		return fail(err)
	}
	creation, err := payments.ReadSessionRequest(ctx, tx, key, options.Ring)
	if err != nil {
		return fail(err)
	}
	return manifest, creation, nil
}

// StageContextPaymentCompensation irreversibly chooses restoration of the
// original context. ALL ambiguous deletes must be reconciled first. Only a
// confirmed succeeded deletion receives a stable new ID and protected original
// create request. No provider call, session activation or version change occurs.
func StageContextPaymentCompensation(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (PaymentCompensation, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return PaymentCompensation{}, err
	}
	var result PaymentCompensation
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = stageContextPaymentCompensation(ctx, tx, store, ref, in, options)
		return err
	})
	if err != nil {
		return PaymentCompensation{}, err
	}
	return result, nil
}

func stageContextPaymentCompensation(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (PaymentCompensation, error) {
	manifest, original, err := contextPaymentOriginal(ctx, tx, store, ref, in, options)
	if err != nil {
		return PaymentCompensation{}, err
	}
	var succeeded []string
	for _, key := range manifest.Effects {
		receipt, err := payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return PaymentCompensation{}, err
		}
		switch receipt.State {
		case "pending", "failed":
		case "succeeded":
			if key.ResourceID != original.SessionID {
				return PaymentCompensation{}, ErrConflict
			}
			succeeded = append(succeeded, key.ResourceID)
		default:
			return PaymentCompensation{}, ErrConflict
		}
	}
	where := func() *gorm.DB {
		return compensationWhere(tx, ref, PaymentRefreshCommand{OperationID: in.OperationID}).Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var sizes []struct{ Bytes int64 }
	if where().Select("OCTET_LENGTH(manifest_json) AS bytes").Scan(&sizes).Error != nil {
		return PaymentCompensation{}, ErrStorage
	}
	var row compensationRow
	exists := len(sizes) == 1
	if len(sizes) > 1 || exists && sizes[0].Bytes > 1<<20 {
		return PaymentCompensation{}, ErrStorage
	}
	saved := contextCompensationManifest{Format: 2, Replacements: []PaymentSessionReplacement{}}
	if exists {
		if where().Take(&row).Error != nil {
			return PaymentCompensation{}, ErrStorage
		}
		raw := []byte(row.ManifestJSON)
		digest := sha256.Sum256(raw)
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if subtle.ConstantTimeCompare(digest[:], row.ManifestSHA256) != 1 || d.Decode(&saved) != nil || d.Decode(new(any)) != io.EOF || saved.Format != 2 || len(saved.Replacements) != len(succeeded) || row.State != "staged" || row.ResultVersion != nil {
			return PaymentCompensation{}, ErrStorage
		}
		for i, replacement := range saved.Replacements {
			key := replacement.Create
			if replacement.OriginalID != succeeded[i] || key.ScopeID != ref.CartID || key.OperationID != in.OperationID || key.Kind != "create_session" || !idPattern.MatchString(key.ResourceID) || key.ResourceID == replacement.OriginalID {
				return PaymentCompensation{}, ErrStorage
			}
			expected, err := replacementRequest(original, key.ResourceID)
			if err != nil {
				return PaymentCompensation{}, err
			}
			actual, err := payments.ReadSessionRequest(ctx, tx, key, options.Ring)
			if err != nil {
				return PaymentCompensation{}, err
			}
			if !reflect.DeepEqual(expected, actual) {
				return PaymentCompensation{}, ErrConflict
			}
		}
	} else {
		for _, id := range succeeded {
			replacement, err := replacementRequest(original, "payses_"+uuid.NewString())
			if err != nil {
				return PaymentCompensation{}, err
			}
			key, _, err := payments.StageSessionRequest(ctx, tx, payments.EffectKey{ScopeID: ref.CartID, OperationID: in.OperationID, Kind: "create_session", ResourceID: replacement.SessionID}, options.Ring, replacement)
			if err != nil {
				return PaymentCompensation{}, err
			}
			saved.Replacements = append(saved.Replacements, PaymentSessionReplacement{id, key})
		}
		raw, err := json.Marshal(saved)
		if err != nil || len(raw) > 1<<20 {
			return PaymentCompensation{}, ErrStorage
		}
		digest := sha256.Sum256(raw)
		if tx.Table("cart_payment_refresh_compensations").Create(map[string]any{"cart_id": ref.CartID, "operation_id": in.OperationID, "state": "staged", "manifest_json": string(raw), "manifest_sha256": digest[:], "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return PaymentCompensation{}, ErrStorage
		}
	}
	result := PaymentCompensation{in.OperationID, "staged", in.Version, saved.Replacements, exists}
	return result, nil
}
