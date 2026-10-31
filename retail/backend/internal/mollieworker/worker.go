// Package mollieworker schedules durable authoritative reads and checkout
// consumption. API startup never runs this worker or creates payments.
package mollieworker

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
)

const provider = "pp_mollie-hosted-checkout_mollie"

var ErrConfiguration = errors.New("explicit Mollie worker configuration required")

type Worker struct {
	DB      *gorm.DB
	StoreID string
	Payment cart.InitialPaymentOptions
	// Only scheduling fairness is in memory. All work, leases, observations
	// and consumption receipts remain in MySQL and survive worker restarts.
	cursor string
}

type Result struct{ Queried, Consumed, Review, Failed int }

// RunOnce processes at most ten provider queries and ten saved observations.
// A single Worker is called serially; independent processes may run concurrently
// because provider claims and business commits are fenced by durable SQL locks.
func (w *Worker) RunOnce(ctx context.Context) (Result, error) {
	var out Result
	if w == nil || w.DB == nil || w.DB.Statement == nil || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`).MatchString(w.StoreID) || w.Payment.Ring == nil || w.Payment.Adapters == nil {
		return out, ErrConfiguration
	}
	if _, nested := w.DB.Statement.ConnPool.(gorm.TxCommitter); nested {
		return out, ErrConfiguration
	}
	if _, err := w.Payment.Adapters.Observer(provider); err != nil {
		return out, ErrConfiguration
	}
	var due []payments.MollieNotificationLocator
	var attempts []string
	err := w.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var gate struct{ Ready bool }
		r := tx.Raw(`SELECT (n.version=1 AND n.enabled AND c.version=1 AND c.enabled AND o.version=1 AND o.enabled) AS ready FROM mollie_notification_runtime n JOIN cart_checkout_runtime c ON c.id=n.id JOIN cart_completion_runtime o ON o.id=n.id WHERE n.id=1 FOR SHARE`).Scan(&gate)
		if r.Error != nil || r.RowsAffected != 1 || !gate.Ready || payments.Ready(ctx, tx) != nil {
			return payments.ErrSchema
		}
		// Store and provider limits come from trusted process configuration.
		// Known non-checkout references and other shops cannot occupy the batch.
		r = tx.Table("mollie_notification_inbox n").Select("n.provider_id,n.remote_id").Joins("JOIN mollie_payment_references r ON r.provider_id=n.provider_id AND r.remote_id=n.remote_id").Joins("JOIN cart_checkout_attempts a ON a.cart_id=r.scope_id AND a.operation_id=r.operation_id AND a.session_id=r.resource_id").Joins("JOIN cart_ownership o ON o.cart_id=a.cart_id").Where("o.store_id=? AND n.provider_id=? AND ((n.state='pending' AND n.available_at<=UTC_TIMESTAMP(6)) OR (n.state='inflight' AND n.lease_until<=UTC_TIMESTAMP(6)))", w.StoreID, provider).Order("n.available_at,n.remote_id").Limit(10).Scan(&due)
		if r.Error != nil {
			return payments.ErrEffectStorage
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	for _, locator := range due {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		r, err := payments.ProcessMollieNotification(call, w.DB, locator.ProviderID, locator.RemoteID, w.Payment.Ring, w.Payment.Adapters)
		cancel()
		if r.Called {
			out.Queried++
		}
		if err != nil {
			out.Failed++
		}
	}
	// Includes evidence saved before a crash or while business gates were
	// closed. Failed evidence remains durable; advance the cursor so a bad
	// oldest record cannot starve later legitimate checkouts.
	err = w.DB.WithContext(ctx).Table("mollie_notification_observations n").Select("n.attempt_id").Joins("JOIN mollie_payment_references r ON r.provider_id=n.provider_id AND r.remote_id=n.remote_id").Joins("JOIN cart_checkout_attempts a ON a.cart_id=r.scope_id AND a.operation_id=r.operation_id AND a.session_id=r.resource_id").Joins("JOIN cart_ownership o ON o.cart_id=a.cart_id").Where("o.store_id=? AND n.provider_id=? AND n.consumed_at IS NULL AND n.attempt_id>?", w.StoreID, provider, w.cursor).Order("n.attempt_id").Limit(10).Scan(&attempts).Error
	if err != nil {
		return out, payments.ErrEffectStorage
	}
	for _, attempt := range attempts {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		r, err := cart.ConsumeMollieCheckoutObservation(call, w.DB, w.StoreID, attempt, w.Payment)
		cancel()
		w.cursor = attempt
		if err != nil {
			out.Failed++
			continue
		}
		out.Consumed++
		if r.State == "requires_review" {
			out.Review++
		}
	}
	if len(attempts) < 10 {
		w.cursor = ""
	}
	return out, nil
}
