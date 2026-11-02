package cart

import (
	"context"
	"crypto/subtle"
	"reflect"
	"strconv"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CompleteContextPayment owns the local final save. An unchanged amount keeps
// the original checkout; changed amount/currency requires every protected delete
// receipt to succeed before updating the collection and retiring its checkout.
// No provider is called by this command.
func CompleteContextPayment(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (ContextResult, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return ContextResult{}, err
	}
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	digest := contextRequestDigest(ctx, in)
	var result ContextResult
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := paymentOperationReady(ctx, tx, PaymentRefreshCommand{OperationID: in.OperationID, Version: in.Version}); err != nil {
			return err
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_context_runtime WHERE id=1 FOR SHARE").Scan(&gate)
		if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
			return ErrSchema
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		var owner struct{ CheckoutAttemptID *string }
		if tx.Table("cart_ownership").Select("checkout_attempt_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
			return ErrDenied
		}
		checkout := ""
		if owner.CheckoutAttemptID != nil {
			checkout = *owner.CheckoutAttemptID
		}
		current, err := lockCurrentAuthority(ctx, tx, store, channel, ref, "UPDATE", in.OperationID, checkout)
		if err != nil {
			return err
		}
		var receipts []struct {
			Generation    string
			RequestSHA256 []byte
			ResultVersion uint64
		}
		if err := validateCustomerPaymentScope(ctx, tx, ref); err != nil {
			return err
		}
		if tx.Table("cart_context_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Find(&receipts).Error != nil {
			return ErrStorage
		}
		if len(receipts) == 1 {
			saved := receipts[0]
			currentVersion, _ := strconv.ParseUint(current.Version, 10, 64)
			var operation paymentOperationRow
			if tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=?", ref.CartID, in.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&operation).Error != nil {
				return ErrStorage
			}
			if saved.Generation != ref.Generation || subtle.ConstantTimeCompare(saved.RequestSHA256, digest[:]) != 1 || saved.ResultVersion != version+1 || saved.ResultVersion > currentVersion || operation.State != "completed" || operation.ResultVersion == nil || *operation.ResultVersion != saved.ResultVersion {
				return ErrConflict
			}
			result = ContextResult{ref.CartID, strconv.FormatUint(saved.ResultVersion, 10), true}
			return nil
		}
		operation, err := stageContextPayment(ctx, tx, store, ref, in, options)
		if err != nil {
			return err
		}
		if checkout == "" || operation.Target.CheckoutOperationID != checkout {
			return ErrConflict
		}
		for _, key := range operation.Effects {
			receipt, err := payments.InspectEffect(ctx, tx, key)
			if err != nil {
				return err
			}
			if receipt.State != "succeeded" {
				return ErrConflict
			}
		}
		workCtx := context.WithValue(ctx, contextPreviewScopeKey{}, contextPreviewScope{tx: tx.Statement.ConnPool, store: store, version: in.Version, checkoutOperation: checkout, paymentOperation: in.OperationID, ref: ref})
		if err := applyCustomerPaymentTarget(workCtx, tx, ref); err != nil {
			return err
		}
		plan, err := resolveContextPlan(workCtx, tx, store, current, in)
		if err != nil {
			return err
		}
		current, err = applyContextPlan(workCtx, tx, store, ref, current, in, plan, options.ManualProviders)
		if err != nil {
			return err
		}
		payment, err := PreparePaymentRefresh(workCtx, tx, store, ref, in.Version)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(payment, operation.Target.Payment) {
			return ErrConflict
		}
		nextCheckout := any(checkout)
		if payment.UpdateCollection {
			scalar, rawAmount, err := sqlAmount(payment.Amount)
			if err != nil {
				return err
			}
			r = tx.Table("payment_collections").Where("id=? AND deleted_at IS NULL", payment.CollectionID).Updates(map[string]any{"amount": scalar, "raw_amount": rawAmount, "currency_code": payment.Currency, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrStorage
			}
			for _, id := range payment.DeleteSessionIDs {
				r = tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", id, payment.CollectionID).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
				if r.Error != nil || r.RowsAffected != 1 {
					return ErrStorage
				}
			}
			nextCheckout = nil
		} else {
			r = tx.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=? AND activated=TRUE AND COALESCE(effective_cart_version,cart_version)=?", ref.CartID, checkout, version).Update("effective_cart_version", version+1)
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrConflict
			}
		}
		r = tx.Exec("UPDATE cart_ownership SET version=?,checkout_attempt_id=?,payment_refresh_operation_id=NULL,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=? AND payment_refresh_operation_id=? AND checkout_attempt_id=?", version+1, nextCheckout, ref.CartID, version, in.OperationID, checkout)
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}

		r = tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=? AND state='staged'", ref.CartID, in.OperationID).Updates(map[string]any{"state": "completed", "result_version": version + 1, "completed_at": gorm.Expr("UTC_TIMESTAMP(6)")})
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrStorage
		}
		if tx.Exec("INSERT INTO cart_context_operations(cart_id,operation_id,generation,request_sha256,result_version,created_at) VALUES(?,?,?,?,?,UTC_TIMESTAMP(6))", ref.CartID, in.OperationID, ref.Generation, digest[:], version+1).Error != nil {
			return ErrStorage
		}
		if err := saveCustomerPaymentReceipt(ctx, tx, ref, in); err != nil {
			return err
		}
		result = ContextResult{ref.CartID, strconv.FormatUint(version+1, 10), false}
		return nil
	})
	if err != nil {
		return ContextResult{}, err
	}
	return result, nil
}
