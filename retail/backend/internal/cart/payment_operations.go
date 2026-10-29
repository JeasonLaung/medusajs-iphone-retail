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
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Internal server command; never accept provider facts, store or ownership
// fields from browser input. The caller owns commit/rollback and HTTP checks.
type PaymentRefreshCommand struct{ OperationID, Version string }
type PaymentRefreshOperation struct {
	OperationID, State, Version string
	Plan                        PaymentRefreshPlan
	Effects                     []payments.EffectKey
	Replayed                    bool
}
type paymentManifest struct {
	Format       int
	Plan         PaymentRefreshPlan
	SourceSHA256 [32]byte
	Effects      []payments.EffectKey
}
type paymentOperationRow struct {
	Generation, State, ManifestJSON string
	CartVersion                     uint64
	ResultVersion                   *uint64
	ManifestSHA256                  []byte
}
type paymentCollectionSource struct {
	ID, CurrencyCode, Amount, RawAmount, Status                              string
	AuthorizedAmount, RawAuthorizedAmount, CapturedAmount, RawCapturedAmount *string
	CompletedAt                                                              *time.Time
}
type paymentSessionSource struct {
	ID, ProviderID, PaymentCollectionID, CurrencyCode, Amount, RawAmount, Data, Status string
	Context                                                                            *string
	AuthorizedAt                                                                       *time.Time
}

func paymentOperationReady(ctx context.Context, tx *gorm.DB, in PaymentRefreshCommand) error {
	id, err := uuid.Parse(in.OperationID)
	version, e := strconv.ParseUint(in.Version, 10, 64)
	if id.Version() != 4 || err != nil || id.String() != in.OperationID || e != nil || version == 0 || version == ^uint64(0) || strconv.FormatUint(version, 10) != in.Version {
		return ErrInput
	}
	if err := commandReady(ctx, tx, "cart.access"); err != nil {
		return err
	}
	if payments.Ready(ctx, tx) != nil {
		return ErrSchema
	}
	var guard struct {
		Version int
		Enabled bool
	}
	r := tx.WithContext(ctx).Raw("SELECT version,enabled FROM cart_payment_refresh_runtime WHERE id=1 FOR SHARE").Scan(&guard)
	if r.Error != nil || r.RowsAffected != 1 || guard.Version != 1 || !guard.Enabled {
		return ErrSchema
	}
	return nil
}

func paymentSources(ctx context.Context, tx *gorm.DB, plan PaymentRefreshPlan) (paymentCollectionSource, []paymentSessionSource, [32]byte, error) {
	var c paymentCollectionSource
	var sessions []paymentSessionSource
	failed := func(err error) (paymentCollectionSource, []paymentSessionSource, [32]byte, error) {
		return paymentCollectionSource{}, nil, [32]byte{}, err
	}
	if !plan.UpdateCollection {
		return c, sessions, [32]byte{}, nil
	}
	if tx.WithContext(ctx).Table("payment_collections").Select("id,currency_code,CAST(amount AS CHAR) AS amount,raw_amount,status,CAST(authorized_amount AS CHAR) AS authorized_amount,raw_authorized_amount,CAST(captured_amount AS CHAR) AS captured_amount,raw_captured_amount,completed_at").Where("id=? AND deleted_at IS NULL", plan.CollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&c).Error != nil {
		return failed(ErrSchema)
	}
	if c.CompletedAt != nil || c.Status == "authorized" || c.Status == "partially_authorized" || c.Status == "partially_captured" || c.Status == "completed" {
		return failed(ErrConflict)
	}
	for _, pair := range [][2]*string{{c.AuthorizedAmount, c.RawAuthorizedAmount}, {c.CapturedAmount, c.RawCapturedAmount}} {
		if pair[0] == nil {
			if pair[1] != nil {
				return failed(ErrSchema)
			}
			continue
		}
		n, err := nonNegativeAmount(*pair[0], pair[1])
		if err != nil {
			return failed(err)
		}
		value, err := totals.Resolve(n)
		if err != nil {
			return failed(ErrSchema)
		}
		if value != "0" {
			return failed(ErrConflict)
		}
	}
	// Bound ciphertext reads before loading provider material into memory. The
	// enclosing preparation already holds all current session row locks.
	var sizes []struct{ DataSize, ContextSize int64 }
	if tx.WithContext(ctx).Table("payment_sessions").Select("OCTET_LENGTH(data) AS data_size,COALESCE(OCTET_LENGTH(context),0) AS context_size").Where("payment_collection_id=? AND deleted_at IS NULL", plan.CollectionID).Limit(101).Scan(&sizes).Error != nil || len(sizes) > 100 {
		return failed(ErrSchema)
	}
	var total int64
	for _, size := range sizes {
		if size.DataSize < 1 || size.DataSize > 2*protected.MaxPlaintext || size.ContextSize > 2*protected.MaxPlaintext {
			return failed(ErrSchema)
		}
		total += size.DataSize + size.ContextSize
		if total > 8*protected.MaxPlaintext {
			return failed(ErrSchema)
		}
	}
	if tx.WithContext(ctx).Table("payment_sessions").Select("id,provider_id,payment_collection_id,currency_code,CAST(amount AS CHAR) AS amount,raw_amount,data,context,status,authorized_at").Where("payment_collection_id=? AND deleted_at IS NULL", plan.CollectionID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&sessions).Error != nil || len(sessions) > 100 || len(sessions) != len(plan.DeleteSessionIDs) {
		return failed(ErrSchema)
	}
	for i, s := range sessions {
		if s.ID != plan.DeleteSessionIDs[i] || s.AuthorizedAt != nil || s.Status == "authorized" || s.Status == "captured" {
			return failed(ErrConflict)
		}
	}
	if len(plan.DeleteSessionIDs) > 0 {
		var paid int64
		if tx.WithContext(ctx).Table("payments").Where("payment_session_id IN ? AND deleted_at IS NULL", plan.DeleteSessionIDs).Count(&paid).Error != nil {
			return failed(ErrSchema)
		}
		if paid != 0 {
			return failed(ErrConflict)
		}
	}
	raw, err := json.Marshal(struct {
		Collection paymentCollectionSource
		Sessions   []paymentSessionSource
	}{c, sessions})
	if err != nil || len(raw) > 100*protected.MaxPlaintext {
		return failed(ErrSchema)
	}
	defer clear(raw)
	return c, sessions, sha256.Sum256(raw), nil
}

func readPaymentOperation(tx *gorm.DB, ref sessionstore.CartReference, in PaymentRefreshCommand) (paymentOperationRow, paymentManifest, bool, error) {
	var row paymentOperationRow
	result := tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return row, paymentManifest{}, false, nil
	}
	if result.Error != nil {
		return row, paymentManifest{}, false, ErrStorage
	}
	if row.Generation != ref.Generation || strconv.FormatUint(row.CartVersion, 10) != in.Version {
		return row, paymentManifest{}, false, ErrConflict
	}
	raw := []byte(row.ManifestJSON)
	digest := sha256.Sum256(raw)
	if len(raw) > 1<<20 || subtle.ConstantTimeCompare(row.ManifestSHA256, digest[:]) != 1 {
		return row, paymentManifest{}, false, ErrStorage
	}
	var manifest paymentManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&manifest) != nil || d.Decode(new(any)) != io.EOF || manifest.Format != 1 || len(manifest.Effects) != len(manifest.Plan.DeleteSessionIDs) || len(manifest.Effects) > 100 {
		return row, paymentManifest{}, false, ErrStorage
	}
	for i, key := range manifest.Effects {
		if key.ScopeID != ref.CartID || key.OperationID != in.OperationID || key.Kind != "delete_session" || key.ResourceID != manifest.Plan.DeleteSessionIDs[i] || key.Digest == ([32]byte{}) {
			return row, paymentManifest{}, false, ErrStorage
		}
	}
	if row.State == "staged" {
		if row.ResultVersion != nil || !manifest.Plan.UpdateCollection || manifest.SourceSHA256 == ([32]byte{}) {
			return row, paymentManifest{}, false, ErrStorage
		}
	} else if row.State == "completed" {
		expected := row.CartVersion
		if manifest.Plan.UpdateCollection {
			expected++
		}
		if row.ResultVersion == nil || *row.ResultVersion != expected {
			return row, paymentManifest{}, false, ErrStorage
		}
	} else {
		return row, paymentManifest{}, false, ErrStorage
	}
	return row, manifest, true, nil
}

func operationResult(in PaymentRefreshCommand, row paymentOperationRow, manifest paymentManifest, replayed bool) PaymentRefreshOperation {
	version := in.Version
	if row.ResultVersion != nil {
		version = strconv.FormatUint(*row.ResultVersion, 10)
	}
	return PaymentRefreshOperation{OperationID: in.OperationID, State: row.State, Version: version, Plan: manifest.Plan, Effects: manifest.Effects, Replayed: replayed}
}

// StagePaymentRefresh atomically freezes an owned cart, the original refresh
// target and encrypted server-read session requests. Public writes conflict
// while the durable operation is active; read-only inspection stays available.
// This entry point does not call any provider. Historical/plaintext session
// payloads cannot be promoted to active runtime authority by this operation.
func StagePaymentRefresh(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, ring *protected.Ring) (PaymentRefreshOperation, error) {
	failed := PaymentRefreshOperation{}
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
	row, manifest, found, err := readPaymentOperation(tx, ref, in)
	if err != nil {
		return failed, err
	}
	if found && row.State == "completed" {
		v, _ := strconv.ParseUint(current.Version, 10, 64)
		if v < *row.ResultVersion {
			return failed, ErrConflict
		}
		return operationResult(in, row, manifest, true), nil
	}
	if current.Version != in.Version {
		return failed, ErrConflict
	}
	plan, err := preparePaymentRefreshOperation(ctx, tx, store, ref, in.Version, in.OperationID)
	if err != nil {
		return failed, err
	}
	_, sources, fingerprint, err := paymentSources(ctx, tx, plan)
	if err != nil {
		return failed, err
	}
	if found {
		var frozen struct{ PaymentRefreshOperationID *string }
		if tx.Table("cart_ownership").Select("payment_refresh_operation_id").Where("cart_id=?", ref.CartID).Take(&frozen).Error != nil || frozen.PaymentRefreshOperationID == nil || *frozen.PaymentRefreshOperationID != in.OperationID {
			return failed, ErrConflict
		}
		if !reflect.DeepEqual(plan, manifest.Plan) || fingerprint != manifest.SourceSHA256 {
			return failed, ErrConflict
		}
		for _, key := range manifest.Effects {
			if _, err := payments.ReadSessionRequest(ctx, tx, key, ring); err != nil {
				return failed, err
			}
		}
		return operationResult(in, row, manifest, true), nil
	}
	manifest = paymentManifest{Format: 1, Plan: plan, SourceSHA256: fingerprint, Effects: []payments.EffectKey{}}
	for _, source := range sources {
		n, err := nonNegativeAmount(source.Amount, &source.RawAmount)
		if err != nil {
			return failed, err
		}
		amount, err := totals.Resolve(n)
		if err != nil {
			return failed, ErrSchema
		}
		request, err := payments.OpenSessionFields(ring, payments.SessionRequest{ProviderID: source.ProviderID, CollectionID: source.PaymentCollectionID, SessionID: source.ID, Amount: amount, Currency: source.CurrencyCode}, source.Data, source.Context)
		if err != nil {
			return failed, err
		}
		key, _, err := payments.StageSessionRequest(ctx, tx, payments.EffectKey{ScopeID: ref.CartID, OperationID: in.OperationID, Kind: "delete_session", ResourceID: source.ID}, ring, request)
		if err != nil {
			return failed, err
		}
		manifest.Effects = append(manifest.Effects, key)
	}
	raw, err := json.Marshal(manifest)
	if err != nil || len(raw) > 1<<20 {
		return failed, ErrStorage
	}
	digest := sha256.Sum256(raw)
	row = paymentOperationRow{Generation: ref.Generation, State: "staged", ManifestJSON: string(raw)}
	row.CartVersion, _ = strconv.ParseUint(in.Version, 10, 64)
	var resultVersion any
	var completedAt any
	if !plan.UpdateCollection {
		row.State = "completed"
		row.ResultVersion = &row.CartVersion
		resultVersion = row.CartVersion
		completedAt = gorm.Expr("UTC_TIMESTAMP(6)")
	}
	if tx.Table("cart_payment_refresh_operations").Create(map[string]any{"cart_id": ref.CartID, "operation_id": in.OperationID, "generation": ref.Generation, "cart_version": row.CartVersion, "state": row.State, "manifest_json": string(raw), "manifest_sha256": digest[:], "result_version": resultVersion, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "completed_at": completedAt}).Error != nil {
		return failed, ErrStorage
	}
	if plan.UpdateCollection {
		r := tx.Table("cart_ownership").Where("cart_id=? AND payment_refresh_operation_id IS NULL", ref.CartID).Update("payment_refresh_operation_id", in.OperationID)
		if r.Error != nil || r.RowsAffected != 1 {
			return failed, ErrConflict
		}
	}
	return operationResult(in, row, manifest, false), nil
}

// ClaimPaymentRefreshSession rechecks current ownership/version, frozen target,
// source rows and encrypted request before claiming one provider action. Commit
// before dispatch; provider work is outside this transaction. An ID alone grants
// no recovery authority. Unknown results are returned without dispatch.
func ClaimPaymentRefreshSession(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, sessionID string, ring *protected.Ring) (payments.EffectReceipt, payments.SessionRequest, error) {
	fail := func(err error) (payments.EffectReceipt, payments.SessionRequest, error) {
		return payments.EffectReceipt{}, payments.SessionRequest{}, err
	}
	operation, err := StagePaymentRefresh(ctx, tx, store, ref, in, ring)
	if err != nil {
		return fail(err)
	}
	if operation.State != "staged" {
		return fail(ErrConflict)
	}
	for _, key := range operation.Effects {
		if key.ResourceID == sessionID {
			return payments.ClaimSessionRequest(ctx, tx, key, ring, 30*time.Second)
		}
	}
	return fail(ErrInput)
}

// CompletePaymentRefresh applies local deletion/amount/version ONLY after every
// exact provider delete receipt is succeeded. No local soft delete substitutes
// for a provider call. Any error requires rollback of the entire transaction.
// Partial/uncertain failure stays frozen for reconciliation/compensation.
func CompletePaymentRefresh(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, ring *protected.Ring) (PaymentRefreshOperation, error) {
	failed := PaymentRefreshOperation{}
	operation, err := StagePaymentRefresh(ctx, tx, store, ref, in, ring)
	if err != nil {
		return failed, err
	}
	if operation.State == "completed" {
		return operation, nil
	}
	for _, key := range operation.Effects {
		receipt, err := payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return failed, err
		}
		if receipt.State != "succeeded" {
			return failed, ErrConflict
		}
	}
	scalar, raw, err := sqlAmount(operation.Plan.Amount)
	if err != nil {
		return failed, err
	}
	r := tx.Table("payment_collections").Where("id=? AND deleted_at IS NULL", operation.Plan.CollectionID).Updates(map[string]any{"amount": scalar, "raw_amount": raw, "currency_code": operation.Plan.Currency, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return failed, ErrStorage
	}
	for _, id := range operation.Plan.DeleteSessionIDs {
		r := tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", id, operation.Plan.CollectionID).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != 1 {
			return failed, ErrStorage
		}
	}
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	version++
	r = tx.Table("cart_ownership").Where("cart_id=? AND version=? AND payment_refresh_operation_id=?", ref.CartID, in.Version, in.OperationID).Updates(map[string]any{"version": version, "payment_refresh_operation_id": nil, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return failed, ErrConflict
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

// RecordPaymentRefreshSession binds a verified adapter outcome to the still
// authorized frozen operation and exact current attempt. Adapter error
// classification follows payments.RecordEffect; ambiguous errors are uncertain.
func RecordPaymentRefreshSession(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in PaymentRefreshCommand, sessionID, attempt, outcome string, ring *protected.Ring) (payments.EffectReceipt, error) {
	operation, err := StagePaymentRefresh(ctx, tx, store, ref, in, ring)
	if err != nil {
		return payments.EffectReceipt{}, err
	}
	if operation.State != "staged" {
		return payments.EffectReceipt{}, ErrConflict
	}
	for _, key := range operation.Effects {
		if key.ResourceID == sessionID {
			return payments.RecordEffect(ctx, tx, key, attempt, outcome)
		}
	}
	return payments.EffectReceipt{}, ErrInput
}
