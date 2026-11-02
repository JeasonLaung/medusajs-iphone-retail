package cart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"io"
	"reflect"
	"strconv"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ContextPaymentOperation is a durable, private staged intent. Even zero
// effects require final context/checkout coordination before completion.
// This entry point never grants dispatch or claims that the context was saved.
type ContextPaymentOperation struct {
	OperationID, State, Version string
	Target                      ContextPaymentPreview
	Effects                     []payments.EffectKey
	Replayed                    bool
}

// Format2 is deliberately distinct from the original standalone Format1
// payment refresh. Its existing decoder refuses these fields/format; the old
// execution/completion entry points cannot consume this context operation.
type contextPaymentManifest struct {
	Format        int
	RequestSHA256 [32]byte
	Target        ContextPaymentPreview
	Effects       []payments.EffectKey
}

// StageContextPayment commits only an immutable target/source manifest,
// protected delete requests and the existing payment-refresh freeze. The
// cart, collection, sessions and original checkout attempt remain unchanged.
// Every replay recalculates the same target with current configuration/source;
// disagreement leaves the original durable operation frozen for resolution.
// No HTTP registration, external call, claim or final save is performed here.
func StageContextPayment(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (ContextPaymentOperation, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return ContextPaymentOperation{}, err
	}
	var result ContextPaymentOperation
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = stageContextPayment(ctx, tx, store, ref, in, options)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ContextPaymentOperation{}, err
	}
	return result, nil
}

func stageContextPayment(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (ContextPaymentOperation, error) {
	fail := func(err error) (ContextPaymentOperation, error) { return ContextPaymentOperation{}, err }
	if err := paymentOperationReady(ctx, tx, PaymentRefreshCommand{OperationID: in.OperationID, Version: in.Version}); err != nil {
		return fail(err)
	}
	channel, err := storeChannel(ctx, tx, store)
	if err != nil {
		return fail(err)
	}
	// Acquire durable owner authority before the savepoint, so no other cart
	// command can change it while temporary repricing is rolled back below.
	var owner struct{ CheckoutAttemptID, PaymentRefreshOperationID *string }
	if tx.Table("cart_ownership").Select("checkout_attempt_id,payment_refresh_operation_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
		return fail(ErrDenied)
	}
	checkout := ""
	if owner.CheckoutAttemptID != nil {
		checkout = *owner.CheckoutAttemptID
	}
	if checkout == in.OperationID {
		return fail(ErrConflict)
	}
	current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", in.OperationID, checkout)
	if err != nil {
		return fail(err)
	}
	if current.Version != in.Version {
		return fail(ErrConflict)
	}
	request, _ := json.Marshal(in)
	digest := sha256.Sum256(request)
	where := func() *gorm.DB {
		return tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID)
	}
	var size struct{ Bytes int64 }
	r := where().Select("OCTET_LENGTH(manifest_json) AS bytes").Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&size)
	if r.Error != nil || size.Bytes > 1<<20 {
		return fail(ErrStorage)
	}
	exists := r.RowsAffected == 1
	var row paymentOperationRow
	var saved contextPaymentManifest
	if exists {
		// The size check is a current locking read. Keep the manifest read
		// current as well: an RR snapshot established before waiting for the
		// owner could otherwise hide a concurrently committed first stage.
		if where().Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
			return fail(ErrStorage)
		}
		raw := []byte(row.ManifestJSON)
		hash := sha256.Sum256(raw)
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if subtle.ConstantTimeCompare(row.ManifestSHA256, hash[:]) != 1 || d.Decode(&saved) != nil || d.Decode(new(any)) != io.EOF || saved.Format != 2 || len(saved.Effects) > 100 {
			return fail(ErrStorage)
		}
		version, _ := strconv.ParseUint(in.Version, 10, 64)
		if row.Generation != ref.Generation || row.CartVersion != version || row.State != "staged" || row.ResultVersion != nil || subtle.ConstantTimeCompare(saved.RequestSHA256[:], digest[:]) != 1 || owner.PaymentRefreshOperationID == nil || *owner.PaymentRefreshOperationID != in.OperationID {
			return fail(ErrConflict)
		}
	} else if owner.PaymentRefreshOperationID != nil {
		return fail(ErrConflict)
	}
	// A committed context receipt cannot be repurposed as a staged operation.
	var receipts int64
	if tx.Table("cart_context_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Count(&receipts).Error != nil {
		return fail(ErrStorage)
	}
	if receipts != 0 {
		return fail(ErrConflict)
	}
	// Compensation irreversibly owns this intent. Target repricing and delete
	// dispatch cannot resume after replacement preparation has committed.
	var compensations int64
	if tx.Table("cart_payment_refresh_compensations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Count(&compensations).Error != nil {
		return fail(ErrStorage)
	}
	if compensations != 0 {
		return fail(ErrConflict)
	}
	if tx.Exec("SAVEPOINT retail_context_payment_target").Error != nil {
		return fail(ErrStorage)
	}
	target, previewErr := previewContextPayment(ctx, tx, store, ref, in, options, in.OperationID)
	if tx.Exec("ROLLBACK TO SAVEPOINT retail_context_payment_target").Error != nil || tx.Exec("RELEASE SAVEPOINT retail_context_payment_target").Error != nil {
		return fail(ErrStorage)
	}
	if previewErr != nil {
		return fail(previewErr)
	}
	if target.Payment.CollectionID == "" {
		return fail(ErrConflict)
	}
	// Re-lock the original source after rolling back the scratch repricing.
	// Validate all sessions even when the target keeps its original payment.
	sourcePlan := target.Payment
	sourcePlan.UpdateCollection = true
	if tx.Table("payment_sessions").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", sourcePlan.CollectionID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&sourcePlan.DeleteSessionIDs).Error != nil || len(sourcePlan.DeleteSessionIDs) > 100 {
		return fail(ErrSchema)
	}
	_, sources, sourceSHA, err := paymentSources(ctx, tx, ref.CartID, sourcePlan)
	if err != nil {
		return fail(err)
	}
	if sourceSHA != target.SourceSHA256 {
		return fail(ErrConflict)
	}
	manifest := contextPaymentManifest{Format: 2, RequestSHA256: digest, Target: target, Effects: []payments.EffectKey{}}
	if exists {
		if !reflect.DeepEqual(target, saved.Target) || len(saved.Effects) != len(target.Payment.DeleteSessionIDs) {
			return fail(ErrConflict)
		}
		for i, key := range saved.Effects {
			if key.ScopeID != ref.CartID || key.OperationID != in.OperationID || key.Kind != "delete_session" || key.ResourceID != target.Payment.DeleteSessionIDs[i] || key.Digest == ([32]byte{}) {
				return fail(ErrStorage)
			}
			if _, err := payments.ReadSessionRequest(ctx, tx, key, options.Ring); err != nil {
				return fail(err)
			}
		}
		return ContextPaymentOperation{in.OperationID, "staged", in.Version, target, saved.Effects, true}, nil
	}
	if target.Payment.UpdateCollection {
		if len(sources) != len(target.Payment.DeleteSessionIDs) {
			return fail(ErrConflict)
		}
		for i, source := range sources {
			if source.ID != target.Payment.DeleteSessionIDs[i] {
				return fail(ErrConflict)
			}
			number, err := nonNegativeAmount(source.Amount, &source.RawAmount)
			if err != nil {
				return fail(err)
			}
			amount, err := totals.Resolve(number)
			if err != nil {
				return fail(ErrSchema)
			}
			request, err := payments.OpenSessionFields(options.Ring, payments.SessionRequest{ProviderID: source.ProviderID, CollectionID: source.PaymentCollectionID, SessionID: source.ID, Amount: amount, Currency: source.CurrencyCode}, source.Data, source.Context)
			if err != nil {
				return fail(err)
			}
			key, _, err := payments.StageSessionRequest(ctx, tx, payments.EffectKey{ScopeID: ref.CartID, OperationID: in.OperationID, Kind: "delete_session", ResourceID: source.ID}, options.Ring, request)
			if err != nil {
				return fail(err)
			}
			manifest.Effects = append(manifest.Effects, key)
		}
	}
	raw, err := json.Marshal(manifest)
	if err != nil || len(raw) > 1<<20 {
		return fail(ErrStorage)
	}
	hash := sha256.Sum256(raw)
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	if tx.Table("cart_payment_refresh_operations").Create(map[string]any{"cart_id": ref.CartID, "operation_id": in.OperationID, "generation": ref.Generation, "cart_version": version, "state": "staged", "manifest_json": string(raw), "manifest_sha256": hash[:], "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
		return fail(ErrStorage)
	}
	r = tx.Table("cart_ownership").Where("cart_id=? AND version=? AND payment_refresh_operation_id IS NULL", ref.CartID, version).Update("payment_refresh_operation_id", in.OperationID)
	if r.Error != nil || r.RowsAffected != 1 {
		return fail(ErrConflict)
	}
	return ContextPaymentOperation{in.OperationID, "staged", in.Version, target, manifest.Effects, false}, nil
}
