package cart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"strconv"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func readContextPaymentOperation(tx *gorm.DB, ref sessionstore.CartReference, operation string) (paymentOperationRow, contextPaymentManifest, error) {
	where := func() *gorm.DB {
		return tx.Table("cart_payment_refresh_operations").Where("cart_id=? AND operation_id=?", ref.CartID, operation).Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var sizes []struct{ Bytes int64 }
	if where().Select("OCTET_LENGTH(manifest_json) AS bytes").Scan(&sizes).Error != nil || len(sizes) != 1 || sizes[0].Bytes > 1<<20 {
		return paymentOperationRow{}, contextPaymentManifest{}, ErrStorage
	}
	var row paymentOperationRow
	if where().Take(&row).Error != nil {
		return row, contextPaymentManifest{}, ErrStorage
	}
	raw := []byte(row.ManifestJSON)
	digest := sha256.Sum256(raw)
	var manifest contextPaymentManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if subtle.ConstantTimeCompare(digest[:], row.ManifestSHA256) != 1 || d.Decode(&manifest) != nil || d.Decode(new(any)) != io.EOF || manifest.Format != 2 || len(manifest.Effects) > 100 {
		return row, contextPaymentManifest{}, ErrStorage
	}
	return row, manifest, nil
}

// CompleteContextPaymentCompensation restores the ORIGINAL context. Only a
// succeeded, authenticated live replacement can become a new checkout. Its
// CREATE operation/session/digest and input/cart versions form a new immutable
// attempt; the old attempt is retained. Zero deleted sessions keep the original
// checkout with a new effective revision. No provider is called or cash assumed.
func CompleteContextPaymentCompensation(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, options InitialPaymentOptions) (PaymentCompensation, error) {
	if err := contextPaymentInput(db, store, ref, in, options); err != nil {
		return PaymentCompensation{}, err
	}
	version, _ := strconv.ParseUint(in.Version, 10, 64)
	digest := contextRequestDigest(ctx, in)
	var result PaymentCompensation
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
		where := func() *gorm.DB {
			return compensationWhere(tx, ref, PaymentRefreshCommand{OperationID: in.OperationID}).Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := validateCustomerPaymentScope(ctx, tx, ref); err != nil {
			return err
		}
		var sizes []struct{ Bytes int64 }
		if where().Select("OCTET_LENGTH(manifest_json) AS bytes").Scan(&sizes).Error != nil || len(sizes) > 1 || len(sizes) == 1 && sizes[0].Bytes > 1<<20 {
			return ErrStorage
		}
		if len(sizes) == 1 {
			var row compensationRow
			if where().Take(&row).Error != nil {
				return ErrStorage
			}
			if row.State == "completed" {
				var saved contextCompensationManifest
				raw := []byte(row.ManifestJSON)
				hash := sha256.Sum256(raw)
				d := json.NewDecoder(bytes.NewReader(raw))
				d.DisallowUnknownFields()
				if subtle.ConstantTimeCompare(hash[:], row.ManifestSHA256) != 1 || d.Decode(&saved) != nil || d.Decode(new(any)) != io.EOF || saved.Format != 2 || len(saved.Replacements) > 1 {
					return ErrStorage
				}
				op, manifest, err := readContextPaymentOperation(tx, ref, in.OperationID)
				if err != nil {
					return err
				}
				v, _ := strconv.ParseUint(current.Version, 10, 64)
				if row.ResultVersion == nil || *row.ResultVersion != version+1 || v < *row.ResultVersion || op.Generation != ref.Generation || op.CartVersion != version || op.State != "completed" || op.ResultVersion == nil || *op.ResultVersion != *row.ResultVersion || manifest.RequestSHA256 != digest || manifest.Target.CartID != ref.CartID || manifest.Target.Version != in.Version {
					return ErrConflict
				}
				attemptID := manifest.Target.CheckoutOperationID
				if len(saved.Replacements) == 1 {
					attemptID = in.OperationID
				}
				var attempt initialAttempt
				command := InitialPaymentCommand{OperationID: attemptID}
				if initialWhere(tx, ref, command).Clauses(clause.Locking{Strength: "SHARE"}).Take(&attempt).Error != nil || attempt.Generation != ref.Generation || !attempt.Activated {
					return ErrConflict
				}
				key := initialKey(ref, command, attempt)
				if len(saved.Replacements) == 1 && (saved.Replacements[0].Create != key || attempt.InputVersion != version || attempt.CartVersion != version+1) {
					return ErrConflict
				}
				if _, err := payments.ReadSessionRequest(ctx, tx, key, options.Ring); err != nil {
					return err
				}
				if _, err := payments.ReadSessionCreation(ctx, tx, key, options.Ring); err != nil {
					return err
				}
				result = PaymentCompensation{in.OperationID, "completed", strconv.FormatUint(*row.ResultVersion, 10), saved.Replacements, true}
				return nil
			}
		}
		operation, err := stageContextPaymentCompensation(ctx, tx, store, ref, in, options)
		if err != nil {
			return err
		}
		if len(operation.Replacements) > 1 || checkout == "" || checkout == in.OperationID {
			return ErrConflict
		}
		var original initialAttempt
		if initialWhere(tx, ref, InitialPaymentCommand{OperationID: checkout}).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&original).Error != nil {
			return ErrStorage
		}
		nextCheckout := checkout
		if len(operation.Replacements) == 0 {
			r = initialWhere(tx, ref, InitialPaymentCommand{OperationID: checkout}).Where("COALESCE(effective_cart_version,cart_version)=?", version).Update("effective_cart_version", version+1)
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrConflict
			}
		} else {
			replacement := operation.Replacements[0]
			request, err := payments.ReadSessionRequest(ctx, tx, replacement.Create, options.Ring)
			if err != nil {
				return err
			}
			created, err := payments.ReadSessionCreation(ctx, tx, replacement.Create, options.Ring)
			if err != nil {
				return err
			}
			if created.Status != "pending" && created.Status != "requires_more" && created.Status != "authorized" {
				return ErrConflict
			}
			// Discovery may find an already paid/authorized replacement without
			// a checkout link. Rebind it without classifying either as cash:
			// the normal observer must persist fresh, verified paid evidence.
			if created.Status != "authorized" {
				if request.ProviderID == hostedMollie {
					if _, err := initialCheckoutURL(created.Data); err != nil {
						return err
					}
				} else if _, err := payments.PayPalCreatedOrderID(request, created); err != nil {
					return err
				}
			}
			request.Data = created.Data
			data, protectedContext, err := payments.SealSessionFields(options.Ring, request)
			if err != nil {
				return err
			}
			amount, rawAmount, err := sqlAmount(request.Amount)
			if err != nil {
				return err
			}
			if tx.Table("payment_sessions").Create(map[string]any{"id": request.SessionID, "payment_collection_id": request.CollectionID, "provider_id": request.ProviderID, "currency_code": request.Currency, "amount": amount, "raw_amount": rawAmount, "data": data, "context": protectedContext, "status": created.Status, "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
				return ErrStorage
			}
			if tx.Table("cart_checkout_attempts").Create(map[string]any{"cart_id": ref.CartID, "operation_id": in.OperationID, "generation": ref.Generation, "input_version": version, "cart_version": version + 1, "collection_id": original.CollectionID, "session_id": request.SessionID, "cart_amount": original.CartAmount, "settlement_amount": original.SettlementAmount, "currency_code": original.CurrencyCode, "request_sha256": replacement.Create.Digest[:], "activated": true, "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
				return ErrStorage
			}
			r = tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", replacement.OriginalID, original.CollectionID).Updates(map[string]any{"deleted_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrStorage
			}
			nextCheckout = in.OperationID
		}
		r = tx.Exec("UPDATE cart_ownership SET version=?,checkout_attempt_id=?,payment_refresh_operation_id=NULL,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=? AND checkout_attempt_id=? AND payment_refresh_operation_id=?", version+1, nextCheckout, ref.CartID, version, checkout, in.OperationID)
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		for _, table := range []string{"cart_payment_refresh_compensations", "cart_payment_refresh_operations"} {
			r = tx.Table(table).Where("cart_id=? AND operation_id=? AND state='staged'", ref.CartID, in.OperationID).Updates(map[string]any{"state": "completed", "result_version": version + 1, "completed_at": gorm.Expr("UTC_TIMESTAMP(6)")})
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrStorage
			}
		}
		// Validate the resulting current checkout with the normal consumer path
		// in this same transaction. Any failure rolls back every activation write.
		inputVersion := in.Version
		createKey := initialKey(ref, InitialPaymentCommand{OperationID: checkout}, original)
		if len(operation.Replacements) == 0 {
			inputVersion = strconv.FormatUint(original.InputVersion, 10)
		} else {
			createKey = operation.Replacements[0].Create
		}
		// Replay the authenticated original/replacement provider. An omitted
		// provider selects hosted Mollie and cannot validate a PayPal checkout.
		request, err := payments.ReadSessionRequest(ctx, tx, createKey, options.Ring)
		if err != nil {
			return err
		}
		if _, _, err := stageInitialPayment(ctx, tx, store, ref, InitialPaymentCommand{OperationID: nextCheckout, Version: inputVersion, ProviderID: request.ProviderID}, options); err != nil {
			return err
		}
		result = PaymentCompensation{in.OperationID, "completed", strconv.FormatUint(version+1, 10), operation.Replacements, false}
		return nil
	})
	if err != nil {
		return PaymentCompensation{}, err
	}
	return result, nil
}
