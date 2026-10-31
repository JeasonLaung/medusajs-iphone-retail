package orderpayments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Run commits one creation claim BEFORE calling the provider. Subsequent calls
// only read/discover the original session, even after process loss or account
// revocation. An ordinary pool is required: a caller transaction cannot authorize
// an external effect. This entry does not consume paid evidence or book cash.
func (s Service) Run(ctx context.Context, id string) (Result, error) {
	if !validUUID(id) {
		return Result{}, ErrInput
	}
	if s.DB == nil || s.DB.Statement == nil || txReady(s.DB) || s.Ring == nil || s.Adapters == nil || !idPattern.MatchString(s.StoreID) {
		return Result{}, ErrUnavailable
	}
	var locator operation
	if e := s.DB.WithContext(ctx).Where("operation_id=?", id).Take(&locator).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return Result{}, ErrConflict
		}
		return Result{}, ErrUnavailable
	}
	if locator.StoreID != s.StoreID {
		return Result{}, ErrConflict
	}
	var row operation
	var req payments.SessionRequest
	defer func() { clear(req.Data); clear(req.Context) }()
	var receipt payments.EffectReceipt
	var adapter payments.CreateSessionAdapter
	var discoverer payments.SessionDiscoverer
	var created payments.CreatedSession
	post, recoverRemote, known := false, false, false
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := runtime(tx, false); e != nil {
			return e
		}
		// A stale unsent locator may conservatively reject this call. A later
		// read sees the committed dispatch and can recover without this authority.
		if locator.DispatchedAt == nil && locator.State == "pending" {
			p := Principal{locator.AccountID, locator.AuthVersion}
			if e := authority(tx, p); e != nil {
				return e
			}
			cart, e := s.scope(tx, p, locator.OrderID)
			if e != nil {
				return e
			}
			if cart != locator.CartID {
				return ErrConflict
			}
		}
		if e := lockRecoveryOrder(tx, locator.OrderID); e != nil {
			return e
		}
		if e := orders.Ready(ctx, tx); e != nil {
			return ErrUnavailable
		}
		if tx.Where("operation_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
			return ErrUnavailable
		}
		if row.StoreID != s.StoreID || row.OrderID != locator.OrderID || row.CartID != locator.CartID || row.SessionID != locator.SessionID {
			return ErrConflict
		}
		if terminal(row.State) || row.Applied {
			return nil
		}
		var e error
		req, e = s.request(ctx, tx, row)
		if e != nil {
			return e
		}
		receipt, e = payments.InspectEffect(ctx, tx, key(row))
		if e != nil {
			return e
		}
		adapter, e = s.Adapters.Lookup(row.ProviderID)
		if e != nil {
			return ErrUnavailable
		}
		if row.DispatchedAt == nil {
			if row.State != "pending" || receipt.State != "pending" || receipt.Attempts != 0 || receipt.AttemptID != "" {
				return ErrConflict
			}
			if e = runtime(tx, true); e != nil {
				return e
			}
			if e = s.beforeDispatch(ctx, tx, row, req); e != nil {
				return e
			}
			receipt, req, e = payments.ClaimSessionRequest(ctx, tx, key(row), s.Ring, 30*time.Second)
			if e != nil {
				return e
			}
			if !receipt.Dispatch || receipt.Attempts != 1 {
				return ErrConflict
			}
			var now time.Time
			if tx.Raw("SELECT UTC_TIMESTAMP(6)").Scan(&now).Error != nil {
				return ErrUnavailable
			}
			r := tx.Model(&operation{}).Where("operation_id=? AND dispatched_at IS NULL AND state='pending'", id).Updates(map[string]any{"state": "uncertain", "dispatched_at": now, "updated_at": now})
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrUnavailable
			}
			row.State, row.DispatchedAt, post = "uncertain", &now, true
			return nil
		}
		switch receipt.State {
		case "succeeded":
			created, e = payments.ReadSessionCreation(ctx, tx, key(row), s.Ring)
			known = e == nil
			return e
		case "inflight", "uncertain":
			receipt, req, e = payments.ClaimSessionRequest(ctx, tx, key(row), s.Ring, 30*time.Second)
			if e != nil {
				return e
			}
			if receipt.Dispatch || receipt.Attempts != 1 {
				return ErrConflict
			}
			if receipt.State != "uncertain" {
				return nil
			}
			var ok bool
			discoverer, ok = adapter.(payments.SessionDiscoverer)
			if !ok {
				return ErrUnavailable
			}
			recoverRemote = true
			return nil
		default:
			// Generic journal 'failed' supports a retry. This order command does
			// not: only a new explicit successor may ever create another session.
			return ErrConflict
		}
	})
	if e != nil {
		return Result{}, e
	}
	if post || recoverRemote {
		outcome := payments.CreateUncertain
		var callErr error
		callCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
		if post {
			outcome, created, callErr = payments.InvokeCreate(callCtx, adapter, req)
		} else {
			var observed payments.SessionObservation
			observed, callErr = payments.InvokeDiscovery(callCtx, discoverer, req)
			if callErr == nil {
				outcome, created = payments.CreateSucceeded, observed.Session
			}
		}
		cancel()
		// Persist external facts even if the caller disconnected or lost access.
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		e = s.saveCreation(saveCtx, &row, receipt, outcome, created)
		cancel()
		if e != nil {
			return result(row), e
		}
		if callErr != nil {
			return result(row), payments.ErrCreateExecution
		}
		known = outcome == payments.CreateSucceeded
	}
	if known {
		// Queue separately: disabling notification intake must not discard a
		// creation receipt. A later Run safely queues the saved reference again.
		e = s.queue(ctx, row, created)
	}
	return result(row), e
}

func (s Service) beforeDispatch(ctx context.Context, tx *gorm.DB, r operation, req payments.SessionRequest) error {
	bound, e := orders.ReadSupplementalPaymentTx(ctx, tx, r.OrderID, r.OrderVersion)
	if e != nil || bound.CollectionID != r.CollectionID || bound.Amount != r.Amount || bound.CurrencyCode != r.CurrencyCode {
		return ErrConflict
	}
	var competing []struct{ OperationID string }
	if tx.Table("order_refund_operations").Select("operation_id").Where("order_id=? AND canceled_at IS NULL AND applied=FALSE AND state NOT IN('failed','canceled')", r.OrderID).Limit(1).Clauses(clause.Locking{Strength: "SHARE"}).Find(&competing).Error != nil {
		return ErrUnavailable
	}
	if len(competing) != 0 {
		return ErrConflict
	}
	if e = s.originalProvider(tx, r.OrderID, r.CartID); e != nil {
		return e
	}
	var session struct {
		ProviderID, CurrencyCode, Amount, RawAmount, Data, Status string
		Context                                                   *string
	}
	if tx.Table("payment_sessions").Select("provider_id,currency_code,CAST(amount AS CHAR) AS amount,raw_amount,data,context,status").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL", r.SessionID, r.CollectionID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&session).Error != nil {
		return ErrUnavailable
	}
	if session.ProviderID != req.ProviderID || session.CurrencyCode != req.Currency || session.Status != "pending" {
		return ErrConflict
	}
	n, e := totals.StoredNumber(session.Amount, &session.RawAmount)
	if e != nil {
		return ErrConflict
	}
	amount, e := totals.Resolve(n)
	if e != nil || amount != req.Amount {
		return ErrConflict
	}
	opened, e := payments.OpenSessionFields(s.Ring, req, session.Data, session.Context)
	if e != nil || !bytes.Equal(opened.Data, req.Data) || !bytes.Equal(opened.Context, req.Context) {
		return ErrUnavailable
	}
	return nil
}

func sameRemote(a, b payments.CreatedSession) bool {
	var x, y struct {
		ID string `json:"id"`
	}
	return json.Unmarshal(a.Data, &x) == nil && json.Unmarshal(b.Data, &y) == nil && x.ID != "" && x.ID == y.ID
}

func (s Service) saveCreation(ctx context.Context, row *operation, receipt payments.EffectReceipt, outcome payments.CreateOutcome, created payments.CreatedSession) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := runtime(tx, false); e != nil {
			return e
		}
		if e := lockRecoveryOrder(tx, row.OrderID); e != nil {
			return e
		}
		var current operation
		if tx.Where("operation_id=?", row.OperationID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error != nil {
			return ErrUnavailable
		}
		if current.DispatchedAt == nil || current.StoreID != s.StoreID {
			return ErrConflict
		}
		effect, e := payments.InspectEffect(ctx, tx, key(current))
		if e != nil {
			return e
		}
		if effect.AttemptID != receipt.AttemptID || effect.Attempts != 1 {
			return ErrConflict
		}
		if effect.State == "succeeded" {
			saved, e := payments.ReadSessionCreation(ctx, tx, key(current), s.Ring)
			if e != nil {
				return e
			}
			if outcome != payments.CreateSucceeded || !sameRemote(saved, created) {
				return ErrConflict
			}
			*row = current
			return nil
		}
		if outcome == payments.CreateSucceeded {
			if _, e = payments.RecordSessionCreation(ctx, tx, key(current), receipt.AttemptID, s.Ring, created); e != nil {
				return e
			}
		} else {
			if _, e = payments.RecordEffect(ctx, tx, key(current), receipt.AttemptID, string(outcome)); e != nil {
				return e
			}
			if outcome == payments.CreateFailed {
				if current.State != "uncertain" || current.Applied {
					return ErrConflict
				}
				r := tx.Table("payment_sessions").Where("id=? AND payment_collection_id=? AND status='pending' AND deleted_at IS NULL", current.SessionID, current.CollectionID).Updates(map[string]any{"status": "error", "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
				if r.Error != nil || r.RowsAffected != 1 {
					return ErrConflict
				}
				if tx.Model(&operation{}).Where("operation_id=?", current.OperationID).Updates(map[string]any{"state": "failed", "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
					return ErrUnavailable
				}
				current.State = "failed"
			}
		}
		*row = current
		return nil
	})
}

func (s Service) queue(ctx context.Context, row operation, created payments.CreatedSession) error {
	var remote struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(created.Data, &remote) != nil || remote.ID == "" {
		return ErrUnavailable
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		known, e := payments.EnqueueMollieNotification(ctx, tx, row.ProviderID, remote.ID)
		if e != nil {
			return e
		}
		if !known {
			return ErrConflict
		}
		return tx.Model(&operation{}).Where("operation_id=? AND applied=FALSE", row.OperationID).Update("updated_at", gorm.Expr("UTC_TIMESTAMP(6)")).Error
	})
}

// External facts survive soft deletion/cancellation of the original order.
// New dispatch still validates active state in beforeDispatch.
func lockRecoveryOrder(tx *gorm.DB, id string) error {
	var row struct{ ID string }
	if tx.Table("orders").Select("id").Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil {
		return ErrUnavailable
	}
	return nil
}
