package cart

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"strconv"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ContextPaymentPreview is private preparation, never a save/provider receipt.
// SourceSHA256 authenticates the locked original collection/session fields.
// The caller must stage durable coordination and revalidate before dispatch;
// this result alone grants no permission to cancel, unfreeze or complete.
type ContextPaymentPreview struct {
	CartID, Version, RegionID, Currency, Locale, CheckoutOperationID string
	Payment                                                          PaymentRefreshPlan
	SourceSHA256                                                     [32]byte
}

// This capability is confined to the owned rollback-only transaction below.
// It lets the existing repricing participants inspect an authenticated frozen
// checkout without clearing its durable freeze or changing public write rules.
type contextPreviewScopeKey struct{}
type contextPreviewScope struct {
	tx                                                  gorm.ConnPool
	store, version, checkoutOperation, paymentOperation string
	ref                                                 sessionstore.CartReference
}

// PreviewContextPayment computes the actual post-change prices, delivery,
// taxes and promotions using the same participants as SaveContext. Every SQL
// change is rolled back, including on success; no version/receipt/freeze or
// provider effect is committed. Only an ordinary DB pool is accepted.
func PreviewContextPayment(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (ContextPaymentPreview, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return ContextPaymentPreview{}, err
	}
	tx := db.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return ContextPaymentPreview{}, ErrStorage
	}
	rolledBack := false
	defer func() {
		if !rolledBack {
			tx.Rollback()
		}
	}()
	out, err := previewContextPayment(ctx, tx, store, ref, in, options, "")
	rollback := tx.Rollback().Error
	rolledBack = true
	if rollback != nil {
		return ContextPaymentPreview{}, ErrStorage
	}
	if err != nil {
		return ContextPaymentPreview{}, err
	}
	return out, nil
}

func contextPaymentInput(db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) error {
	if _, err := validateContext(in); err != nil || !refOK(store, ref) || db == nil || db.Statement == nil {
		return ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return ErrInput
	}
	if err := initialOptions(options); err != nil {
		return err
	}
	if len(options.ManualProviders) > 100 {
		return ErrInput
	}
	for _, id := range options.ManualProviders {
		if !idPattern.MatchString(id) {
			return ErrInput
		}
	}
	return nil
}

func previewContextPayment(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions, paymentOperation string) (ContextPaymentPreview, error) {
	fail := func(err error) (ContextPaymentPreview, error) { return ContextPaymentPreview{}, err }
	if err := commandReady(ctx, tx, "cart.access"); err != nil {
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
	// Follow the existing owner-first lock order. Reading a freeze identifier
	// is routing only; current authority and its protected binding follow.
	var owner struct{ CheckoutAttemptID *string }
	if tx.Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
		return fail(ErrDenied)
	}
	checkout := ""
	if owner.CheckoutAttemptID != nil {
		checkout = *owner.CheckoutAttemptID
	}
	current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", paymentOperation, checkout)
	if err != nil {
		return fail(err)
	}
	if current.Version != in.Version {
		return fail(ErrConflict)
	}
	if err := validateContextPaymentCheckout(ctx, tx, ref, current, checkout, options); err != nil {
		return fail(err)
	}
	workCtx := context.WithValue(ctx, contextPreviewScopeKey{}, contextPreviewScope{tx: tx.Statement.ConnPool, store: store, version: in.Version, checkoutOperation: checkout, paymentOperation: paymentOperation, ref: ref})
	if err := applyCustomerPaymentTarget(workCtx, tx, ref); err != nil {
		return fail(err)
	}
	plan, err := resolveContextPlan(workCtx, tx, store, current, in)
	if err != nil {
		return fail(err)
	}
	current, err = applyContextPlan(workCtx, tx, store, ref, current, in, plan, options.ManualProviders)
	if err != nil {
		return fail(err)
	}
	payment, err := PreparePaymentRefresh(workCtx, tx, store, ref, in.Version)
	if err != nil {
		return fail(err)
	}
	// Validate original source ownership and unpaid status even for a no-op
	// payment plan. Unchanged amount/currency must retain all its sessions.
	sourcePlan := payment
	if sourcePlan.CollectionID != "" {
		sourcePlan.UpdateCollection = true
		if tx.Table("payment_sessions").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", sourcePlan.CollectionID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Scan(&sourcePlan.DeleteSessionIDs).Error != nil || len(sourcePlan.DeleteSessionIDs) > 100 {
			return fail(ErrSchema)
		}
	}
	_, sessions, digest, err := paymentSources(workCtx, tx, current.ID, sourcePlan)
	if err != nil {
		return fail(err)
	}
	for _, session := range sessions {
		if _, err := options.Adapters.Lookup(session.ProviderID); err != nil {
			return fail(err)
		}
		number, err := nonNegativeAmount(session.Amount, &session.RawAmount)
		if err != nil {
			return fail(err)
		}
		amount, err := totals.Resolve(number)
		if err != nil {
			return fail(ErrSchema)
		}
		request := payments.SessionRequest{ProviderID: session.ProviderID, CollectionID: session.PaymentCollectionID, SessionID: session.ID, Amount: amount, Currency: session.CurrencyCode}
		if _, err := payments.OpenSessionFields(options.Ring, request, session.Data, session.Context); err != nil {
			return fail(err)
		}
	}
	return ContextPaymentPreview{CartID: current.ID, Version: current.Version, RegionID: current.RegionID, Currency: current.CurrencyCode, Locale: current.Locale, CheckoutOperationID: checkout, Payment: payment, SourceSHA256: digest}, nil
}

// validateContextPaymentCheckout checks only the original frozen payment,
// independent of current catalogue quotes needed by a new target.
func validateContextPaymentCheckout(ctx context.Context, tx *gorm.DB, ref sessionstore.CartReference, current Snapshot, checkout string, options InitialPaymentOptions) error {
	if checkout == "" {
		return nil
	}
	if err := initialReady(ctx, tx); err != nil {
		return err
	}
	var row initialAttempt
	if initialWhere(tx, ref, InitialPaymentCommand{OperationID: checkout}).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
		return ErrStorage
	}
	version, _ := strconv.ParseUint(current.Version, 10, 64)
	if row.currentVersion() != version || row.InputVersion == 0 || row.InputVersion+1 != row.CartVersion || subtle.ConstantTimeCompare([]byte(row.Generation), []byte(ref.Generation)) != 1 || !row.Activated || row.NeedsReview || statusState(row) == "authorized" || statusState(row) == "paid_pending_order" {
		return ErrConflict
	}
	command := InitialPaymentCommand{OperationID: checkout, Version: strconv.FormatUint(row.InputVersion, 10)}
	key, err := validateInitialBinding(ctx, tx, ref, command, row, options)
	if err != nil {
		return err
	}
	created, err := payments.ReadSessionCreation(ctx, tx, key, options.Ring)
	if err != nil {
		return err
	}
	if created.Status != "pending" && created.Status != "requires_more" {
		return ErrConflict
	}
	amounts, err := assembleAmounts(tx, current)
	if err != nil || amounts.Totals["total"] != row.CartAmount || amounts.CurrencyCode != row.CurrencyCode {
		return ErrConflict
	}
	return nil
}
