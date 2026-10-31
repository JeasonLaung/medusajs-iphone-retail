package cart

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MollieCheckoutConsumption struct {
	State      string
	Completion CompletionResult
	Replayed   bool
}

// ConsumeMollieCheckoutObservation is a server-only entry point. Store comes
// from worker configuration; attempt is an encrypted, durable observation ID.
// It never accepts customer-supplied payment facts or calls a payment provider.
// Consumption, state merge and all order effects share one local transaction.
func ConsumeMollieCheckoutObservation(ctx context.Context, db *gorm.DB, store, attempt string, o InitialPaymentOptions) (MollieCheckoutConsumption, error) {
	if db == nil || db.Statement == nil || !idPattern.MatchString(store) {
		return MollieCheckoutConsumption{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return MollieCheckoutConsumption{}, ErrInput
	}
	id, err := uuid.Parse(attempt)
	if err != nil || id.Version() != 4 || id.String() != attempt {
		return MollieCheckoutConsumption{}, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return MollieCheckoutConsumption{}, err
	}
	// Routing only. Lock the owner before observations, matching browser order
	// and preventing concurrent notifications from inverting completion locks.
	var locator struct{ ScopeID, OperationID string }
	r := db.WithContext(ctx).Table("mollie_notification_observations n").Select("r.scope_id,r.operation_id").Joins("JOIN mollie_payment_references r ON r.provider_id=n.provider_id AND r.remote_id=n.remote_id").Where("n.attempt_id=?", attempt).Take(&locator)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return MollieCheckoutConsumption{}, ErrDenied
	}
	if r.Error != nil {
		return MollieCheckoutConsumption{}, ErrStorage
	}
	var out MollieCheckoutConsumption
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := initialReady(ctx, tx); err != nil {
			return err
		}
		var owner struct {
			StoreID, Generation                          string
			Active                                       bool
			CheckoutAttemptID, PaymentRefreshOperationID *string
		}
		if tx.Table("cart_ownership").Where("cart_id=?", locator.ScopeID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner).Error != nil {
			return ErrDenied
		}
		var row initialAttempt
		if tx.Table("cart_checkout_attempts").Where("cart_id=? AND operation_id=?", locator.ScopeID, locator.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
			return ErrConflict
		}
		ref := sessionstore.CartReference{CartID: locator.ScopeID, StoreID: store, Generation: row.Generation}
		in := InitialPaymentCommand{OperationID: locator.OperationID, Version: strconv.FormatUint(row.InputVersion, 10)}
		if !refOK(store, ref) || !owner.Active || owner.StoreID != store || owner.Generation != row.Generation || owner.CheckoutAttemptID == nil || *owner.CheckoutAttemptID != in.OperationID || owner.PaymentRefreshOperationID != nil || len(row.RequestSHA256) != 32 || row.InputVersion == 0 || row.CartVersion != row.InputVersion+1 {
			return ErrConflict
		}
		var region struct{ RegionID string }
		if tx.Table("carts").Select("region_id").Where("id=?", ref.CartID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&region).Error != nil {
			return ErrStorage
		}
		if err := initialProvider(tx, Snapshot{RegionID: region.RegionID}); err != nil {
			return err
		}
		var notification struct {
			ProviderID, RemoteID string
			ConsumedAt           *time.Time
		}
		if tx.Table("mollie_notification_observations").Where("attempt_id=?", attempt).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&notification).Error != nil {
			return ErrStorage
		}
		observed, err := payments.ReadMollieNotificationObservation(ctx, tx, attempt, o.Ring)
		if err != nil {
			return err
		}
		var remote struct {
			ScopeID, OperationID, EffectKind, ResourceID string
			RequestSHA256                                []byte
		}
		if tx.Table("mollie_payment_references").Where("provider_id=? AND remote_id=?", notification.ProviderID, notification.RemoteID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&remote).Error != nil || remote.ScopeID != ref.CartID || remote.OperationID != in.OperationID || remote.EffectKind != "create_session" || remote.ResourceID != row.SessionID || subtle.ConstantTimeCompare(remote.RequestSHA256, row.RequestSHA256) != 1 || notification.ProviderID != hostedMollie {
			return ErrConflict
		}
		key, err := validateInitialBinding(ctx, tx, ref, in, row, o)
		if err != nil {
			return err
		}
		request, err := payments.ReadSessionRequest(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		created, err := payments.ReadSessionCreation(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		var identity struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(created.Data, &identity) != nil || identity.ID != notification.RemoteID || payments.ValidateMollieObservation(request, identity.ID, observed) != nil {
			return ErrConflict
		}
		var completed int64
		if tx.Table("cart_completion_receipts").Where("cart_id=?", ref.CartID).Count(&completed).Error != nil {
			return ErrStorage
		}
		if completed == 0 {
			// Reuse current cart, store, provider, amount and frozen-version checks.
			if _, _, err := stageInitialPayment(ctx, tx, store, ref, in, o); err != nil {
				return err
			}
		} else {
			// Closed-cart replay still requires current ownership and the original
			// immutable completion receipt; it cannot create another order.
			if err := completePaidOrder(ctx, tx, store, ref, in, row.InputVersion, o, &out.Completion); err != nil {
				return err
			}
		}
		if notification.ConsumedAt == nil {
			raw, err := json.Marshal(observed)
			if err != nil {
				return ErrStorage
			}
			defer clear(raw)
			envelope, err := o.Ring.Seal(observationContext(attempt), raw)
			if err != nil {
				return err
			}
			defer clear(envelope)
			if tx.Table("cart_checkout_observations").Create(map[string]any{"id": attempt, "cart_id": ref.CartID, "operation_id": in.OperationID, "request_sha256": key.Digest[:], "result_envelope": envelope, "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
				return ErrStorage
			}
			row, err = applyInitialObservation(tx, ref, in, row, attempt, observed)
			if err != nil {
				return err
			}
			if completed == 0 && !row.NeedsReview && row.ObservedStatus != nil && *row.ObservedStatus == "paid" {
				if err := completePaidOrder(ctx, tx, store, ref, in, row.InputVersion, o, &out.Completion); err != nil {
					return err
				}
			}
			r := tx.Table("mollie_notification_observations").Where("attempt_id=? AND consumed_at IS NULL", attempt).Update("consumed_at", gorm.Expr("UTC_TIMESTAMP(6)"))
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrStorage
			}
		} else {
			out.Replayed = true
		}
		out.State = statusState(row)
		if out.Completion.OrderID != "" && !row.NeedsReview {
			out.State = "completed"
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return MollieCheckoutConsumption{}, err
	}
	return out, nil
}
