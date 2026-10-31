package orderpayments

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
)

// Worker is called serially per process. Its cursors/timer provide fairness and
// reduce discovery traffic; SQL claims and immutable evidence own correctness.
// Separate processes may run concurrently without granting a second POST.
type Worker struct {
	Service                                             Service
	operationCursor, observationCursor, discoveryCursor string
	nextDiscovery                                       time.Time
}
type WorkResult struct{ Selected, Processed, Queried, Consumed, Failed int }

// RunOnce bounds each of command, GET and consumption batches to ten. Unknown
// creation discovery is slower: at most one per minute per process, up to five
// bounded provider reads. Retiring a cart/account does not hide durable work.
func (w *Worker) RunOnce(ctx context.Context) (WorkResult, error) {
	var out WorkResult
	if w == nil {
		return out, ErrUnavailable
	}
	s := w.Service
	if s.DB == nil || s.DB.Statement == nil || txReady(s.DB) || s.Ring == nil || s.Adapters == nil || !idPattern.MatchString(s.StoreID) {
		return out, ErrUnavailable
	}
	if _, e := s.Adapters.Observer(provider); e != nil {
		return out, ErrUnavailable
	}
	var enabled bool
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := runtime(tx, false); e != nil {
			return e
		}
		if tx.Table("order_payment_runtime").Select("enabled").Where("id=1").Scan(&enabled).Error != nil {
			return ErrUnavailable
		}
		var intake struct {
			Version int
			Enabled bool
		}
		if tx.Table("mollie_notification_runtime").Where("id=1").Take(&intake).Error != nil || intake.Version != 1 || !intake.Enabled || payments.Ready(ctx, tx) != nil {
			return ErrUnavailable
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	var instructions []struct{ OperationID, State string }
	e = s.DB.WithContext(ctx).Table("order_payment_operations o").Select("o.operation_id,e.state").Joins("JOIN payment_effect_journal e ON e.scope_id=o.order_id AND e.operation_id=o.operation_id AND e.resource_id=o.session_id AND e.effect_kind='create_session' AND e.request_sha256=o.request_sha256").Where("o.store_id=? AND o.applied=FALSE AND o.state NOT IN('failed','canceled','expired') AND o.operation_id>? AND ((? AND o.dispatched_at IS NULL) OR (o.dispatched_at IS NOT NULL AND e.state='succeeded' AND o.updated_at<=UTC_TIMESTAMP(6)-INTERVAL 1 MINUTE))", s.StoreID, w.operationCursor, enabled).Order("o.operation_id").Limit(10).Scan(&instructions).Error
	if e != nil {
		return out, ErrUnavailable
	}
	for _, row := range instructions {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		w.operationCursor = row.OperationID
		out.Selected++
		call, cancel := context.WithTimeout(ctx, 45*time.Second)
		_, e := s.Run(call, row.OperationID)
		cancel()
		if e != nil {
			out.Failed++
		} else {
			out.Processed++
		}
	}
	if len(instructions) < 10 {
		w.operationCursor = ""
	}
	if !time.Now().Before(w.nextDiscovery) {
		w.nextDiscovery = time.Now().Add(time.Minute)
		var recovery []string
		e = s.DB.WithContext(ctx).Table("order_payment_operations o").Select("o.operation_id").Joins("JOIN payment_effect_journal e ON e.scope_id=o.order_id AND e.operation_id=o.operation_id AND e.resource_id=o.session_id AND e.effect_kind='create_session' AND e.request_sha256=o.request_sha256").Where("o.store_id=? AND o.dispatched_at IS NOT NULL AND o.applied=FALSE AND o.state NOT IN('failed','canceled','expired') AND o.operation_id>? AND (e.state='uncertain' OR (e.state='inflight' AND e.lease_until<=UTC_TIMESTAMP(6)))", s.StoreID, w.discoveryCursor).Order("o.operation_id").Limit(1).Scan(&recovery).Error
		if e != nil {
			return out, ErrUnavailable
		}
		if len(recovery) == 0 {
			w.discoveryCursor = ""
		}
		for _, id := range recovery {
			w.discoveryCursor = id
			out.Selected++
			call, cancel := context.WithTimeout(ctx, 45*time.Second)
			_, e := s.Run(call, id)
			cancel()
			if e != nil {
				out.Failed++
			} else {
				out.Processed++
			}
		}
	}
	var due []payments.MollieNotificationLocator
	e = s.DB.WithContext(ctx).Table("mollie_notification_inbox n").Select("n.provider_id,n.remote_id").Joins("JOIN mollie_payment_references r ON r.provider_id=n.provider_id AND r.remote_id=n.remote_id").Joins("JOIN order_payment_operations o ON o.order_id=r.scope_id AND o.operation_id=r.operation_id AND o.session_id=r.resource_id AND o.request_sha256=r.request_sha256 AND r.effect_kind='create_session'").Where("o.store_id=? AND n.provider_id=? AND ((n.state='pending' AND n.available_at<=UTC_TIMESTAMP(6)) OR (n.state='inflight' AND n.lease_until<=UTC_TIMESTAMP(6)))", s.StoreID, provider).Order("n.available_at,n.remote_id").Limit(10).Scan(&due).Error
	if e != nil {
		return out, ErrUnavailable
	}
	for _, locator := range due {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		r, e := payments.ProcessMollieNotification(call, s.DB, locator.ProviderID, locator.RemoteID, s.Ring, s.Adapters)
		cancel()
		if r.Called {
			out.Queried++
		}
		if e != nil {
			out.Failed++
		}
	}
	var attempts []string
	e = s.DB.WithContext(ctx).Table("mollie_notification_observations n").Select("n.attempt_id").Joins("JOIN mollie_payment_references r ON r.provider_id=n.provider_id AND r.remote_id=n.remote_id").Joins("JOIN order_payment_operations o ON o.order_id=r.scope_id AND o.operation_id=r.operation_id AND o.session_id=r.resource_id AND o.request_sha256=r.request_sha256 AND r.effect_kind='create_session'").Where("o.store_id=? AND n.provider_id=? AND n.consumed_at IS NULL AND n.attempt_id>?", s.StoreID, provider, w.observationCursor).Order("n.attempt_id").Limit(10).Scan(&attempts).Error
	if e != nil {
		return out, ErrUnavailable
	}
	for _, attempt := range attempts {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		w.observationCursor = attempt
		call, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, e := s.Consume(call, attempt)
		cancel()
		if e != nil {
			out.Failed++
		} else {
			out.Consumed++
		}
	}
	if len(attempts) < 10 {
		w.observationCursor = ""
	}
	return out, nil
}
