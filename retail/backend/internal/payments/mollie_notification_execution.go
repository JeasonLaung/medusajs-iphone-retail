package payments

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/protected"
	"gorm.io/gorm"
)

type MollieNotificationExecution struct {
	Receipt MollieNotification
	Called  bool
}

// ProcessMollieNotification is a private server worker entry point, not browser
// authorization. Ordinary DB pool only. Commit claim, GET outside transactions,
// then atomically store encrypted evidence/ACK. No order/payment/stock mutation.
// A future scheduler must supply the reviewed merchant allowlist and key ring.
func ProcessMollieNotification(ctx context.Context, db *gorm.DB, provider, remote string, ring *protected.Ring, adapters *CreateAdapters) (MollieNotificationExecution, error) {
	if db == nil || db.Statement == nil || adapters == nil {
		return MollieNotificationExecution{}, ErrEffectInput
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return MollieNotificationExecution{}, ErrEffectInput
	}
	var execution MollieNotificationExecution
	var observer SessionObserver
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		execution.Receipt, err = ClaimMollieNotification(ctx, tx, provider, remote, ring, 30*time.Second)
		if err != nil {
			return err
		}
		if execution.Receipt.Dispatch {
			observer, err = adapters.Observer(provider)
		}
		return err
	})
	if err != nil {
		return MollieNotificationExecution{}, err
	}
	if !execution.Receipt.Dispatch {
		return execution, nil
	}
	execution.Receipt.Dispatch = false
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	execution.Called = true
	observed, callErr := InvokeSessionObservation(callCtx, observer, execution.Receipt.Request, remote)
	cancel()
	savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err = db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
		if callErr != nil {
			return RetryMollieNotification(savedCtx, tx, execution.Receipt, time.Minute)
		}
		return CompleteMollieNotification(savedCtx, tx, execution.Receipt, observed, ring)
	})
	if err != nil {
		return execution, err
	}
	// Receipt describes the authorized claim, not a fresh read of current state.
	// Concurrent notifications can already have advanced the durable generation.
	return execution, callErr
}
