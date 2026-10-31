package cart

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// RecoverInitialPayment discovers a lost creation response through read-only
// merchant APIs. This private coordinator accepts the original authority and
// command, never a browser candidate ID. It does not create/retry a payment,
// activate checkout, reserve stock or create an order. A recovered reference is
// queued for the normal authoritative observation and checkout consumer.
func RecoverInitialPayment(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, o InitialPaymentOptions) (bool, error) {
	if db == nil || db.Statement == nil {
		return false, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return false, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return false, err
	}
	var key payments.EffectKey
	var request payments.SessionRequest
	var receipt payments.EffectReceipt
	var discoverer payments.SessionDiscoverer
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		var receiving struct{ Enabled bool }
		gate := tx.Raw("SELECT enabled FROM mollie_notification_runtime WHERE id=1 AND version=1 FOR SHARE").Scan(&receiving)
		if gate.Error != nil || gate.RowsAffected != 1 || !receiving.Enabled {
			return ErrSchema
		}
		// Recovery must not stage a fresh checkout or freeze an untouched cart.
		// This routing read grants no authority; stage rechecks the current
		// owner and immutable binding under its normal lock order below.
		var existing struct{ SessionID string }
		lookup := initialWhere(tx, ref, in).Select("session_id").Limit(1).Scan(&existing)
		if lookup.Error != nil {
			return ErrStorage
		}
		if lookup.RowsAffected != 1 {
			return ErrConflict
		}
		_, key, err = stageInitialPayment(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		receipt, err = payments.InspectEffect(ctx, tx, key)
		if err != nil {
			return err
		}
		if receipt.State != "inflight" && receipt.State != "uncertain" {
			return nil
		}
		receipt, request, err = payments.ClaimSessionRequest(ctx, tx, key, o.Ring, 30*time.Second)
		if err != nil {
			return err
		}
		if receipt.Dispatch {
			return ErrConflict
		}
		if receipt.State != "uncertain" {
			return nil
		}
		adapter, err := o.Adapters.Lookup(request.ProviderID)
		if err != nil {
			return err
		}
		var ok bool
		discoverer, ok = adapter.(payments.SessionDiscoverer)
		if !ok {
			return ErrConfiguration
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil || discoverer == nil {
		return false, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	observed, err := payments.InvokeDiscovery(callCtx, discoverer, request)
	cancel()
	if err != nil {
		return false, err
	}
	// Keep an authorized external fact even if the HTTP caller disconnects or
	// ownership changes while querying. Applying it remains separately gated.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err = db.WithContext(saveCtx).Transaction(func(tx *gorm.DB) error {
		current, err := payments.InspectEffect(saveCtx, tx, key)
		if err != nil {
			return err
		}
		if current.AttemptID != receipt.AttemptID {
			return ErrConflict
		}
		if current.State == "succeeded" {
			// Another recovery may have observed an earlier provider status. Never
			// overwrite its immutable result; validate the same remote identity.
			created, err := payments.ReadSessionCreation(saveCtx, tx, key, o.Ring)
			if err != nil {
				return err
			}
			return sameRecoveredPayment(created, observed.PaymentID)
		}
		if _, err = payments.RecordSessionCreation(saveCtx, tx, key, receipt.AttemptID, o.Ring, observed.Session); err != nil {
			return err
		}
		known, err := payments.EnqueueMollieNotification(saveCtx, tx, request.ProviderID, observed.PaymentID)
		if err != nil {
			return err
		}
		if !known {
			return ErrConflict
		}
		return nil
	})
	return err == nil, err
}

func sameRecoveredPayment(created payments.CreatedSession, remote string) error {
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(created.Data, &identity) != nil || identity.ID != remote {
		return ErrConflict
	}
	return nil
}
