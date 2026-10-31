// Package refunds owns native after-sales refund commands. Provider calls never
// run inside SQL and an uncertain dispatch never grants another POST.
package refunds

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInput       = errors.New("invalid refund command")
	ErrConflict    = errors.New("refund command conflict")
	ErrUnavailable = errors.New("refund unavailable")
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,191}$`)
	moneyPattern   = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,2})?$`)
)

type Input struct {
	PreviousOperationID string `json:"previous_operation_id,omitempty"`
	OperationID         string `json:"operation_id"`
	OrderID             string `json:"order_id"`
	RequestID           string `json:"request_id"`
	PaymentID           string `json:"payment_id"`
	Amount              string `json:"amount"`
}

// Result exposes local facts only. External references and recovery material are private.
type Result struct {
	PreviousOperationID *string `json:"previous_operation_id,omitempty"`
	CanCancel           bool    `json:"can_cancel"`
	CanRetry            bool    `json:"can_retry"`
	OperationID         string  `json:"operation_id"`
	OrderID             string  `json:"order_id"`
	RequestID           string  `json:"request_id"`
	PaymentID           string  `json:"payment_id"`
	Amount              string  `json:"amount"`
	Currency            string  `json:"currency_code"`
	State               string  `json:"state"`
	Applied             bool    `json:"applied"`
}
type Service struct {
	StoreID  string
	DB       *gorm.DB
	Ring     *protected.Ring
	Adapters *payments.CreateAdapters
}
type operation struct {
	PreviousOperationID                                                                     *string
	CanceledAt                                                                              *time.Time
	CanceledBy                                                                              *string
	ClaimID                                                                                 *string
	OperationID, ActorID, OrderID, RequestID, PaymentID, CollectionID, Amount, CurrencyCode string
	InputSHA256                                                                             []byte
	ScopeID, CheckoutOperationID, SessionID, ProviderID, RemotePaymentID                    string
	RequestSHA256                                                                           []byte
	RemoteRefundID                                                                          *string
	State                                                                                   string
	DispatchedAt, LeaseUntil                                                                *time.Time
	Observation                                                                             *string
	Applied                                                                                 bool
}

func (operation) TableName() string { return "order_refund_operations" }
func result(r operation) Result {
	if r.CanceledAt != nil {
		r.State = "canceled"
	}
	return Result{OperationID: r.OperationID, OrderID: r.OrderID, RequestID: r.RequestID, PaymentID: r.PaymentID, Amount: r.Amount, Currency: r.CurrencyCode, State: r.State, Applied: r.Applied, PreviousOperationID: r.PreviousOperationID, CanCancel: r.DispatchedAt == nil && r.CanceledAt == nil && r.State == "pending", CanRetry: r.State == "failed" || r.State == "canceled"}
}
func positive(s string) bool { n, ok := new(big.Rat).SetString(s); return ok && n.Sign() > 0 }
func valid(in Input) bool {
	id, e := uuid.Parse(in.OperationID)
	return e == nil && id.Version() == 4 && id.String() == in.OperationID && idPattern.MatchString(in.OrderID) && idPattern.MatchString(in.RequestID) && idPattern.MatchString(in.PaymentID) && moneyPattern.MatchString(in.Amount) && positive(in.Amount) && (in.PreviousOperationID == "" || validUUID(in.PreviousOperationID))
}
func gate(tx *gorm.DB) error {
	var r struct {
		Version int
		Enabled bool
	}
	if tx.Table("order_refund_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&r).Error != nil || r.Version != 3 || !r.Enabled {
		return ErrUnavailable
	}
	return nil
}
func authority(ctx context.Context, tx *gorm.DB, actor identity.User) error {
	return authorization.CheckTx(ctx, tx, actor, authorization.OrderRead, authorization.OrderPaymentRead, authorization.OrderAfterSalesWrite, authorization.OrderRefundWrite)
}
func digest(in Input) [32]byte { b, _ := json.Marshal(in); return sha256.Sum256(b) }
func key(r operation) payments.EffectKey {
	k := payments.EffectKey{ScopeID: r.ScopeID, OperationID: r.CheckoutOperationID, Kind: "create_session", ResourceID: r.SessionID}
	copy(k.Digest[:], r.RequestSHA256)
	return k
}
func (s Service) request(ctx context.Context, tx *gorm.DB, r operation) (payments.RefundRequest, error) {
	if len(r.RequestSHA256) != 32 {
		return payments.RefundRequest{}, ErrUnavailable
	}
	req, e := payments.ReadSessionRequest(ctx, tx, key(r), s.Ring)
	if e != nil || req.SessionID != r.SessionID || req.CollectionID != r.CollectionID || req.ProviderID != r.ProviderID || req.Currency != r.CurrencyCode {
		return payments.RefundRequest{}, ErrUnavailable
	}
	return payments.RefundRequest{Session: req, RemotePaymentID: r.RemotePaymentID, OperationID: r.OperationID, OrderID: r.OrderID, RequestID: r.RequestID, Amount: r.Amount}, nil
}

// Prepare reserves captured money and binds one immutable instruction per return.
// No external request occurs here. Current grants are checked even on replay.
func (s Service) Prepare(ctx context.Context, actor identity.User, in Input) (Result, error) {
	var out Result
	if !valid(in) {
		return out, ErrInput
	}
	if s.DB == nil || s.Ring == nil || s.Adapters == nil || !idPattern.MatchString(s.StoreID) {
		return out, ErrUnavailable
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authority(ctx, tx, actor); e != nil {
			return e
		}
		if e := gate(tx); e != nil {
			return e
		}
		if e := s.scope(tx, in.OrderID); e != nil {
			return e
		}
		var ord struct {
			Version      int
			Status       string
			IsDraftOrder bool
		}
		if tx.Table("orders").Where("id=? AND deleted_at IS NULL", in.OrderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&ord).Error != nil {
			return ErrConflict
		}
		sum := digest(in)
		var prior operation
		e := tx.Where("operation_id=?", in.OperationID).Take(&prior).Error
		if e == nil {
			if prior.ActorID != actor.ID || subtle.ConstantTimeCompare(prior.InputSHA256, sum[:]) != 1 {
				return ErrConflict
			}
			out = result(prior)
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if ord.Version < 1 || ord.Status == "canceled" || ord.IsDraftOrder {
			return ErrConflict
		}
		if e := returned(tx, in.OrderID, in.RequestID); e != nil {
			return e
		}
		var used int64
		if tx.Model(&operation{}).Where("request_id=? AND canceled_at IS NULL AND state NOT IN ('failed','canceled')", in.RequestID).Count(&used).Error != nil {
			return ErrUnavailable
		}
		if used != 0 {
			return ErrConflict
		}
		var history int64
		if tx.Model(&operation{}).Where("request_id=?", in.RequestID).Count(&history).Error != nil {
			return ErrUnavailable
		}
		if history >= 100 {
			return ErrConflict
		}
		if history > 0 && in.PreviousOperationID == "" || history == 0 && in.PreviousOperationID != "" {
			return ErrConflict
		}
		if in.PreviousOperationID != "" {
			var parent operation
			if tx.Where("operation_id=? AND order_id=? AND request_id=? AND payment_id=?", in.PreviousOperationID, in.OrderID, in.RequestID, in.PaymentID).Take(&parent).Error != nil || (parent.CanceledAt == nil && parent.State != "failed" && parent.State != "canceled") {
				return ErrConflict
			}
			var successors int64
			if tx.Model(&operation{}).Where("previous_operation_id=?", in.PreviousOperationID).Count(&successors).Error != nil {
				return ErrUnavailable
			}
			if successors != 0 {
				return ErrConflict
			}
		}
		var p struct {
			ID, PaymentCollectionID, PaymentSessionID, ProviderID, CurrencyCode string
			CapturedAt, CanceledAt                                              *time.Time
		}
		if tx.Table("payments p").Select("p.*").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id AND l.deleted_at IS NULL").Where("p.id=? AND l.order_id=? AND p.deleted_at IS NULL", in.PaymentID, in.OrderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&p).Error != nil || p.CapturedAt == nil || p.CanceledAt != nil {
			return ErrConflict
		}
		// Collection lock serializes shared balance updates across its payments.
		var collection struct{ ID string }
		if tx.Table("payment_collections").Where("id=? AND deleted_at IS NULL", p.PaymentCollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&collection).Error != nil {
			return ErrConflict
		}
		if _, e := s.Adapters.Refunder(p.ProviderID); e != nil {
			return ErrUnavailable
		}
		r := operation{OperationID: in.OperationID, ActorID: actor.ID, OrderID: in.OrderID, RequestID: in.RequestID, PaymentID: in.PaymentID, CollectionID: p.PaymentCollectionID, Amount: in.Amount, CurrencyCode: p.CurrencyCode, ProviderID: p.ProviderID, InputSHA256: sum[:], State: "pending"}
		if in.PreviousOperationID != "" {
			r.PreviousOperationID = &in.PreviousOperationID
		}
		var trace struct {
			ScopeID, OperationID, SessionID, RemoteID string
			RequestSHA256                             []byte
		}
		e = tx.Table("cart_completion_receipts c").Select("a.cart_id AS scope_id,a.operation_id,a.session_id,r.remote_id,a.request_sha256").Joins("JOIN cart_checkout_attempts a ON a.cart_id=c.cart_id AND a.operation_id=c.operation_id").Joins("JOIN mollie_payment_references r ON r.scope_id=a.cart_id AND r.operation_id=a.operation_id AND r.resource_id=a.session_id AND r.effect_kind='create_session' AND r.request_sha256=a.request_sha256").Where("c.order_id=? AND c.payment_id=? AND a.collection_id=? AND a.session_id=? AND r.provider_id=?", in.OrderID, in.PaymentID, p.PaymentCollectionID, p.PaymentSessionID, p.ProviderID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&trace).Error
		if e != nil {
			return ErrConflict
		}
		r.ScopeID, r.CheckoutOperationID, r.SessionID, r.RemotePaymentID, r.RequestSHA256 = trace.ScopeID, trace.OperationID, trace.SessionID, trace.RemoteID, trace.RequestSHA256
		req, e := s.request(ctx, tx, r)
		if e != nil {
			return e
		}
		defer clear(req.Session.Data)
		defer clear(req.Session.Context)
		if e := exchangeAmount(tx, r); e != nil {
			return e
		}
		if e := reserve(tx, in.PaymentID, in.Amount); e != nil {
			return e
		}
		if tx.Model(&operation{}).Create(map[string]any{"previous_operation_id": r.PreviousOperationID, "operation_id": r.OperationID, "actor_id": r.ActorID, "order_id": r.OrderID, "request_id": r.RequestID, "payment_id": r.PaymentID, "collection_id": r.CollectionID, "amount": r.Amount, "currency_code": r.CurrencyCode, "input_sha256": r.InputSHA256, "scope_id": r.ScopeID, "checkout_operation_id": r.CheckoutOperationID, "session_id": r.SessionID, "request_sha256": r.RequestSHA256, "provider_id": r.ProviderID, "remote_payment_id": r.RemotePaymentID, "state": "pending", "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrConflict
		}
		out = result(r)
		return nil
	})
	return out, e
}

// returned checks every explicitly selected original device, with no ownership inference.
func returned(tx *gorm.DB, o, r string) error {
	var req struct{ Type, Status, Items string }
	if tx.Table("after_sales_requests").Where("id=? AND order_id=? AND deleted_at IS NULL", r, o).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&req).Error != nil || (req.Type != "return" && req.Type != "exchange") || req.Status != "processing" {
		return ErrConflict
	}
	var items struct {
		Lines []struct {
			ItemID    string   `json:"item_id"`
			Quantity  int      `json:"quantity"`
			DeviceIDs []string `json:"device_ids"`
		} `json:"lines"`
	}
	if json.Unmarshal([]byte(req.Items), &items) != nil || len(items.Lines) == 0 || len(items.Lines) > 100 {
		return ErrConflict
	}
	seen := map[string]bool{}
	for _, l := range items.Lines {
		if !idPattern.MatchString(l.ItemID) || l.Quantity < 1 || l.Quantity != len(l.DeviceIDs) {
			return ErrConflict
		}
		for _, d := range l.DeviceIDs {
			if !idPattern.MatchString(d) || seen[d] {
				return ErrConflict
			}
			seen[d] = true
			var count int64
			if tx.Table("device_assignments a").Joins("JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Where("a.order_id=? AND a.item_id=? AND a.device_id=? AND a.after_sales_request_id=? AND a.status='returned' AND a.deleted_at IS NULL", o, l.ItemID, d, r).Count(&count).Error != nil || count != 1 {
				return ErrConflict
			}
			if req.Type == "exchange" {
				var native int64
				if tx.Table("device_assignments a").Joins("JOIN order_exchanges e ON e.order_id=a.order_id AND e.deleted_at IS NULL AND JSON_UNQUOTE(JSON_EXTRACT(e.metadata,'$.replacement_of'))=a.id AND JSON_UNQUOTE(JSON_EXTRACT(e.metadata,'$.after_sales_request_id'))=a.after_sales_request_id").Joins("JOIN returns r ON r.id=e.return_id AND r.exchange_id=e.id AND r.status='received' AND r.deleted_at IS NULL").Where("a.order_id=? AND a.item_id=? AND a.device_id=? AND a.after_sales_request_id=? AND a.status='returned' AND a.deleted_at IS NULL", o, l.ItemID, d, r).Count(&native).Error != nil || native != 1 {
					return ErrConflict
				}
			}
		}
	}
	return nil
}

// payment lock held by caller. Raw exact values, never cached collection totals,
// are the authority for captures/refunds. All unresolved commands reserve funds.
func reserve(tx *gorm.DB, payment, amount string) error {
	balance := new(big.Rat)
	for _, tab := range []string{"payment_captures", "payment_refunds"} {
		var rows []struct{ Amount, RawAmount string }
		if tx.Table(tab).Select("CAST(amount AS CHAR) AS amount,raw_amount").Where("payment_id=? AND deleted_at IS NULL", payment).Limit(10001).Find(&rows).Error != nil || len(rows) > 10000 {
			return ErrUnavailable
		}
		for _, r := range rows {
			v, e := exact(r.Amount, r.RawAmount)
			if e != nil {
				return e
			}
			n, ok := new(big.Rat).SetString(v)
			if !ok || n.Sign() < 0 {
				return ErrUnavailable
			}
			if tab == "payment_captures" {
				balance.Add(balance, n)
			} else {
				balance.Sub(balance, n)
			}
		}
	}
	var holds []operation
	if tx.Where("payment_id=? AND canceled_at IS NULL AND applied=FALSE AND state NOT IN ('failed','canceled')", payment).Limit(10001).Find(&holds).Error != nil || len(holds) > 10000 {
		return ErrUnavailable
	}
	for _, r := range holds {
		n, ok := new(big.Rat).SetString(r.Amount)
		if !ok || n.Sign() <= 0 {
			return ErrUnavailable
		}
		balance.Sub(balance, n)
	}
	n, ok := new(big.Rat).SetString(amount)
	if !ok || n.Sign() <= 0 || balance.Cmp(n) < 0 {
		return ErrConflict
	}
	return nil
}
func exact(decimal, raw string) (string, error) {
	n, e := totals.StoredNumber(decimal, &raw)
	if e != nil {
		return "", ErrUnavailable
	}
	v, e := totals.Resolve(n)
	if e != nil {
		return "", ErrUnavailable
	}
	return v, nil
}

func validUUID(s string) bool {
	id, e := uuid.Parse(s)
	return e == nil && id.Version() == 4 && id.String() == s
}

func (s Service) scope(tx *gorm.DB, order string) error {
	if !idPattern.MatchString(s.StoreID) {
		return ErrUnavailable
	}
	// Current locking reads must precede the order lock. A plain count here
	// would freeze a repeatable-read snapshot before a concurrent command commits,
	// hiding its receipt even after this transaction acquires the order lock.
	var rows []struct{ OrderID string }
	if tx.Table("cart_completion_receipts r").Select("r.order_id").Joins("JOIN cart_ownership o ON o.cart_id=r.cart_id").Where("r.order_id=? AND o.store_id=?", order, s.StoreID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(2).Find(&rows).Error != nil {
		return ErrUnavailable
	}
	if len(rows) != 1 {
		return ErrConflict
	}
	return nil
}
