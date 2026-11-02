package cart

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type paypalSettlementRow struct {
	SessionID, CartID, CheckoutOperationID                      string
	RequestSHA256                                               []byte
	RemoteOrderID, ProviderID, Intent, ProviderRequestID, State string
	LeaseUntil                                                  time.Time
	ResultEnvelope                                              []byte
}

func paypalSettlementContext(id string) protected.Context {
	return protected.Context{Purpose: "paypal-settlement-v1", Table: "cart_paypal_settlements", Column: "result_envelope", Row: id}
}

// SettlePayPal owns the durable single-dispatch claim. HTTP retries cannot
// choose a fresh key/order/action. Expired/uncertain claims only GET; they never
// redispatch a capture/authorization. External facts persist before rechecking
// cart authority, even when the browser disconnects during the provider call.
func SettlePayPal(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, o InitialPaymentOptions) (PaymentStatus, error) {
	if db == nil || db.Statement == nil {
		return PaymentStatus{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return PaymentStatus{}, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return PaymentStatus{}, err
	}
	var in InitialPaymentCommand
	var attempt initialAttempt
	var row paypalSettlementRow
	var request payments.SessionRequest
	var adapter payments.PayPalSettler
	dispatch, recovering := false, false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		in, err = currentInitialCommand(ctx, tx, store, ref, o.Ring)
		if err != nil {
			return err
		}
		if in.ProviderID == hostedMollie {
			return ErrConfiguration
		}
		var key payments.EffectKey
		attempt, key, err = stageInitialPayment(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		if !attempt.Activated || attempt.NeedsReview {
			return ErrConflict
		}
		created, err := payments.ReadSessionCreation(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		request, err = payments.ReadSessionRequest(ctx, tx, key, o.Ring)
		if err != nil {
			return err
		}
		id, err := payments.PayPalCreatedOrderID(request, created)
		if err != nil {
			return ErrConflict
		}
		registered, err := o.Adapters.Lookup(request.ProviderID)
		if err != nil {
			return ErrConfiguration
		}
		var ok bool
		adapter, ok = registered.(payments.PayPalSettler)
		if !ok {
			return ErrConfiguration
		}
		intent := adapter.SettlementIntent()
		requestID, err := payments.PayPalSettlementRequestID(request, id, intent)
		if err != nil {
			return ErrConfiguration
		}
		var existing []paypalSettlementRow
		if tx.Table("cart_paypal_settlements").Where("session_id=?", attempt.SessionID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Find(&existing).Error != nil {
			return ErrSchema
		}
		if len(existing) == 0 {
			row = paypalSettlementRow{SessionID: attempt.SessionID, CartID: ref.CartID, CheckoutOperationID: in.OperationID, RequestSHA256: attempt.RequestSHA256, RemoteOrderID: id, ProviderID: request.ProviderID, Intent: intent, ProviderRequestID: requestID, State: "inflight", LeaseUntil: time.Now().UTC().Add(30 * time.Second)}
			now := gorm.Expr("UTC_TIMESTAMP(6)")
			if tx.Table("cart_paypal_settlements").Create(map[string]any{"session_id": row.SessionID, "cart_id": row.CartID, "checkout_operation_id": row.CheckoutOperationID, "request_sha256": row.RequestSHA256, "remote_order_id": id, "provider_id": row.ProviderID, "intent": intent, "provider_request_id": requestID, "state": "inflight", "lease_until": row.LeaseUntil, "created_at": now, "updated_at": now}).Error != nil {
				return ErrStorage
			}
			dispatch = true
			return nil
		}
		row = existing[0]
		if row.CartID != ref.CartID || row.CheckoutOperationID != in.OperationID || row.RemoteOrderID != id || row.ProviderID != request.ProviderID || row.Intent != intent || row.ProviderRequestID != requestID || subtle.ConstantTimeCompare(row.RequestSHA256, attempt.RequestSHA256) != 1 {
			return ErrConflict
		}
		if row.State == "succeeded" {
			if len(row.ResultEnvelope) == 0 {
				return ErrStorage
			}
			return nil
		}
		if row.State != "inflight" && row.State != "uncertain" {
			return ErrStorage
		}
		if row.State == "inflight" && time.Now().UTC().Before(row.LeaseUntil) {
			return nil
		}
		if tx.Table("cart_paypal_settlements").Where("session_id=?", row.SessionID).Update("state", "uncertain").Error != nil {
			return ErrStorage
		}
		recovering = true
		return nil
	})
	if err != nil {
		return PaymentStatus{}, err
	}
	out := PaymentStatus{CartID: ref.CartID, Version: strconv.FormatUint(attempt.currentVersion(), 10), State: "processing"}
	if !dispatch && !recovering && row.State != "succeeded" {
		return out, nil
	}
	var observed payments.SessionObservation
	if dispatch || recovering {
		callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		if dispatch {
			observed, err = payments.InvokePayPalSettlement(callCtx, adapter, request, row.RemoteOrderID)
		} else {
			observed, err = payments.InvokeSessionObservation(callCtx, adapter, request, row.RemoteOrderID)
		}
		cancel()
		if err == nil {
			err = payments.ValidatePayPalObservation(request, row.RemoteOrderID, observed)
		}
		// A still-approved order after a lost response does not prove that the
		// original settlement POST never happened. Keep it uncertain.
		if err == nil && recovering && observed.ProviderStatus == "pending" {
			var data struct {
				PayPal struct {
					Order struct {
						Units []struct {
							Payments struct {
								Captures       []any `json:"captures"`
								Authorizations []any `json:"authorizations"`
							} `json:"payments"`
						} `json:"purchase_units"`
					} `json:"order"`
				} `json:"paypal"`
			}
			if json.Unmarshal(observed.Session.Data, &data) != nil || len(data.PayPal.Order.Units) != 1 || len(data.PayPal.Order.Units[0].Payments.Captures)+len(data.PayPal.Order.Units[0].Payments.Authorizations) == 0 {
				err = ErrConflict
			}
		}
		savedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var envelope []byte
		if err == nil {
			raw, e := json.Marshal(observed)
			if e != nil {
				return PaymentStatus{}, ErrStorage
			}
			envelope, e = o.Ring.Seal(paypalSettlementContext(row.SessionID), raw)
			clear(raw)
			if e != nil {
				return PaymentStatus{}, ErrStorage
			}
			defer clear(envelope)
		}
		saveErr := db.WithContext(savedCtx).Transaction(func(tx *gorm.DB) error {
			var current paypalSettlementRow
			if tx.Table("cart_paypal_settlements").Where("session_id=?", row.SessionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error != nil {
				return ErrStorage
			}
			if current.State == "succeeded" {
				row = current
				return nil
			}
			state := "uncertain"
			if err == nil {
				state = "succeeded"
			}
			var savedEnvelope any = gorm.Expr("NULL")
			if err == nil {
				savedEnvelope = envelope
			}
			if tx.Table("cart_paypal_settlements").Where("session_id=?", row.SessionID).Updates(map[string]any{"state": state, "result_envelope": savedEnvelope, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
				return ErrStorage
			}
			row.State = state
			row.ResultEnvelope = envelope
			return nil
		})
		if saveErr != nil {
			return PaymentStatus{}, saveErr
		}
		if row.State != "succeeded" {
			out.State = "requires_review"
			return out, nil
		}
	}
	raw, err := o.Ring.Open(paypalSettlementContext(row.SessionID), row.ResultEnvelope)
	if err != nil {
		return PaymentStatus{}, ErrStorage
	}
	defer clear(raw)
	if json.Unmarshal(raw, &observed) != nil || payments.ValidatePayPalObservation(request, row.RemoteOrderID, observed) != nil {
		return PaymentStatus{}, ErrStorage
	}
	// Apply the authenticated fact through the same existing observation ledger.
	id := uuid.NewString()
	envelope, err := o.Ring.Seal(observationContext(id), raw)
	if err != nil {
		return PaymentStatus{}, ErrStorage
	}
	defer clear(envelope)
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, _, err := stageInitialPayment(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		if tx.Table("cart_checkout_observations").Create(map[string]any{"id": id, "cart_id": ref.CartID, "operation_id": in.OperationID, "request_sha256": attempt.RequestSHA256, "result_envelope": envelope, "created_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrStorage
		}
		current, err = applyInitialObservation(tx, ref, in, current, id, observed)
		if err != nil {
			return err
		}
		out.State = statusState(current)
		out.Version = strconv.FormatUint(current.currentVersion(), 10)
		return nil
	})
	return out, err
}
