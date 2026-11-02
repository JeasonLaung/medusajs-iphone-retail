package refunds

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Whole-order cancellation and after-sales are separate business objects. Only
// the existing refund executor and its immutable monetary journal are shared.
type CancellationInput struct {
	OperationID     string `json:"operation_id"`
	ExpectedVersion string `json:"expected_version"`
	Reason          string `json:"reason"`
}
type orderCancellation struct {
	OperationID, OrderID, ActorID, Reason string
	InputSHA256                           []byte
	ExpectedVersion                       int
	CompletedAt                           *time.Time
}

func (orderCancellation) TableName() string { return "order_cancellations" }

type CancellationView struct {
	OrderID     string     `json:"order_id"`
	OperationID string     `json:"operation_id,omitempty"`
	Version     int        `json:"version"`
	Enabled     bool       `json:"enabled"`
	Eligible    bool       `json:"eligible"`
	State       string     `json:"state"`
	Reason      string     `json:"reason,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Refunds     []Result   `json:"refunds"`
}

func validCancellation(in CancellationInput) bool {
	v, err := strconv.Atoi(in.ExpectedVersion)
	return validUUID(in.OperationID) && err == nil && v > 0 && v < 2147483647 && strconv.Itoa(v) == in.ExpectedVersion && utf8.ValidString(in.Reason) && strings.TrimSpace(in.Reason) == in.Reason && utf8.RuneCountInString(in.Reason) > 0 && utf8.RuneCountInString(in.Reason) <= 1000
}
func cancellationAuthority(ctx context.Context, tx *gorm.DB, actor identity.User) error {
	return authorization.CheckTx(ctx, tx, actor, authorization.OrderRead, authorization.OrderPaymentRead, authorization.OrderAfterSalesWrite, authorization.OrderRefundWrite, authorization.OrderAllocationWrite, authorization.OrderFulfillmentRead)
}
func cancellationGate(tx *gorm.DB) error {
	var row struct {
		Version int
		Enabled bool
	}
	if tx.Raw("SELECT version,enabled FROM order_cancellation_runtime WHERE id=1 FOR SHARE").Scan(&row).Error != nil || row.Version != 1 || !row.Enabled {
		return ErrUnavailable
	}
	return gate(tx)
}

// The order lock serializes packing, order changes and cancellation admission.
// Every linked fulfillment must already be canceled, exactly as in the source
// workflow. A shipped/completed order follows returns rather than cancellation.
func cancellationOrder(tx *gorm.DB, id string, expected int) error {
	var o struct {
		Version      int
		Status       string
		IsDraftOrder bool
		CanceledAt   *time.Time
	}
	if tx.Table("orders").Select("version,status,is_draft_order,canceled_at").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&o).Error != nil {
		return ErrConflict
	}
	if o.Version != expected || o.Version < 1 || o.Status == "completed" || o.Status == "canceled" || o.Status == "draft" || o.IsDraftOrder || o.CanceledAt != nil {
		return ErrConflict
	}
	var parcels []struct{ ID string }
	if tx.Table("order_fulfillments l").Select("f.id").Joins("JOIN fulfillments f ON f.id=l.fulfillment_id AND f.deleted_at IS NULL").Where("l.order_id=? AND l.deleted_at IS NULL AND f.canceled_at IS NULL", id).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&parcels).Error != nil {
		return ErrUnavailable
	}
	if len(parcels) != 0 {
		return ErrConflict
	}
	for _, q := range []struct{ table, where string }{
		{"order_changes", "order_id=? AND status IN ('pending','requested') AND deleted_at IS NULL"},
		{"after_sales_requests", "order_id=? AND status IN ('pending','processing') AND deleted_at IS NULL"},
	} {
		var rows []struct{ ID string }
		if tx.Table(q.table).Select("id").Where(q.where, id).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&rows).Error != nil {
			return ErrUnavailable
		}
		if len(rows) != 0 {
			return ErrConflict
		}
	}
	return nil
}

func cancellationRefundReady(ctx context.Context, tx *gorm.DB, r operation) error {
	if r.CancellationID == nil {
		return ErrConflict
	}
	if err := cancellationGate(tx); err != nil {
		return err
	}
	var parent orderCancellation
	if tx.Where("operation_id=? AND order_id=? AND completed_at IS NULL", *r.CancellationID, r.OrderID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&parent).Error != nil {
		return ErrConflict
	}
	var actor identity.User
	if tx.Where("id=?", r.ActorID).Take(&actor).Error != nil {
		return ErrUnavailable
	}
	if err := cancellationAuthority(ctx, tx, actor); err != nil {
		return err
	}
	return cancellationOrder(tx, r.OrderID, parent.ExpectedVersion)
}

func cancellationView(tx *gorm.DB, id string, enabled bool) (CancellationView, error) {
	out := CancellationView{OrderID: id, Enabled: enabled, State: "not_started", Refunds: []Result{}}
	var order struct{ Version int }
	if tx.Table("orders").Select("version").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error != nil {
		return out, ErrConflict
	}
	out.Version = order.Version
	var root orderCancellation
	err := tx.Where("order_id=?", id).Clauses(clause.Locking{Strength: "SHARE"}).Take(&root).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = cancellationOrder(tx, id, order.Version)
		if err != nil && !errors.Is(err, ErrConflict) {
			return out, err
		}
		out.Eligible = err == nil && enabled
		if out.Eligible {
			if err := orders.EnsureNoPendingPaymentTx(tx, id); err != nil {
				out.Eligible = false
			}
		}
		return out, nil
	}
	if err != nil {
		return out, ErrUnavailable
	}
	out.OperationID, out.Reason, out.CompletedAt = root.OperationID, root.Reason, root.CompletedAt
	out.State = "processing"
	if root.CompletedAt != nil {
		out.State = "canceled"
	}
	var rows []operation
	if tx.Where("cancellation_id=?", root.OperationID).Order("created_at,operation_id").Limit(1001).Find(&rows).Error != nil || len(rows) > 1000 {
		return out, ErrUnavailable
	}
	successors := map[string]bool{}
	for _, row := range rows {
		if row.PreviousOperationID != nil {
			successors[*row.PreviousOperationID] = true
		}
	}
	for _, row := range rows {
		view := result(row)
		// Children are owned by cancellation, not the after-sales cancel/retry API.
		view.CanCancel, view.CanRetry = false, !successors[row.OperationID] && (row.State == "failed" || row.State == "canceled")
		out.Refunds = append(out.Refunds, view)
		if view.CanRetry {
			out.State = "requires_action"
		}
	}
	return out, nil
}

func (s Service) ReadCancellation(ctx context.Context, actor identity.User, order string) (CancellationView, error) {
	var out CancellationView
	if !idPattern.MatchString(order) {
		return out, ErrInput
	}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := cancellationAuthority(ctx, tx, actor); err != nil {
			return err
		}
		if err := s.scope(tx, order); err != nil {
			return err
		}
		var err error
		out, err = cancellationView(tx, order, cancellationGate(tx) == nil)
		return err
	})
	return out, err
}

// CancelOrder atomically admits the immutable whole-order instruction and ALL
// original-payment refund children. No network call or inventory release occurs
// until admission commits. Replay never derives another set of child UUIDs.
func (s Service) CancelOrder(ctx context.Context, actor identity.User, order string, in CancellationInput) (CancellationView, error) {
	var out CancellationView
	if !idPattern.MatchString(order) || !validCancellation(in) {
		return out, ErrInput
	}
	if s.DB == nil || s.Ring == nil || s.Adapters == nil {
		return out, ErrUnavailable
	}
	digestInput := Input{OperationID: in.OperationID, OrderID: order, RequestID: in.Reason, Amount: in.ExpectedVersion}
	sum := digest(digestInput)
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := cancellationAuthority(ctx, tx, actor); err != nil {
			return err
		}
		if err := cancellationGate(tx); err != nil {
			return err
		}
		if err := s.scope(tx, order); err != nil {
			return err
		}
		var o struct{ ID string }
		if tx.Table("orders").Select("id").Where("id=? AND deleted_at IS NULL", order).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&o).Error != nil {
			return ErrConflict
		}
		var old orderCancellation
		err := tx.Where("operation_id=?", in.OperationID).Take(&old).Error
		if err == nil {
			if old.OrderID != order || old.ActorID != actor.ID || subtle.ConstantTimeCompare(old.InputSHA256, sum[:]) != 1 {
				return ErrConflict
			}
			out, err = cancellationView(tx, order, true)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		version, _ := strconv.Atoi(in.ExpectedVersion)
		if err := cancellationOrder(tx, order, version); err != nil {
			return err
		}
		if err := orders.EnsureNoPendingPaymentTx(tx, order); err != nil {
			return ErrConflict
		}
		var busy []struct{ OperationID string }
		if tx.Model(&operation{}).Select("operation_id").Where("order_id=? AND canceled_at IS NULL AND applied=FALSE AND state NOT IN ('failed','canceled')", order).Clauses(clause.Locking{Strength: "SHARE"}).Limit(1).Find(&busy).Error != nil {
			return ErrUnavailable
		}
		if len(busy) != 0 {
			return ErrConflict
		}
		if _, err := cancellationAssignments(tx, order); err != nil {
			return err
		}
		root := orderCancellation{OperationID: in.OperationID, OrderID: order, ActorID: actor.ID, Reason: in.Reason, ExpectedVersion: version, InputSHA256: sum[:]}
		if tx.Model(&orderCancellation{}).Create(map[string]any{"operation_id": root.OperationID, "order_id": order, "actor_id": actor.ID, "reason": in.Reason, "expected_version": version, "input_sha256": sum[:], "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrConflict
		}
		if err := s.stageCancellationRefunds(ctx, tx, root); err != nil {
			return err
		}
		out, err = cancellationView(tx, order, true)
		return err
	})
	return out, err
}

type cancellationPayment struct {
	ID, PaymentCollectionID, PaymentSessionID, ProviderID, CurrencyCode string
	CapturedAt, CanceledAt                                              *time.Time
}

func paymentBalance(tx *gorm.DB, payment string) (*big.Rat, error) {
	balance := new(big.Rat)
	for _, table := range []string{"payment_captures", "payment_refunds"} {
		var rows []struct{ Amount, RawAmount string }
		if tx.Table(table).Select("CAST(amount AS CHAR) AS amount,raw_amount").Where("payment_id=? AND deleted_at IS NULL", payment).Clauses(clause.Locking{Strength: "SHARE"}).Limit(10001).Find(&rows).Error != nil || len(rows) > 10000 {
			return nil, ErrUnavailable
		}
		for _, row := range rows {
			text, err := exact(row.Amount, row.RawAmount)
			if err != nil {
				return nil, err
			}
			amount, ok := new(big.Rat).SetString(text)
			if !ok || amount.Sign() < 0 {
				return nil, ErrUnavailable
			}
			if table == "payment_captures" {
				balance.Add(balance, amount)
			} else {
				balance.Sub(balance, amount)
			}
		}
	}
	if balance.Sign() < 0 {
		return nil, ErrConflict
	}
	return balance, nil
}

func (s Service) stageCancellationRefunds(ctx context.Context, tx *gorm.DB, root orderCancellation) error {
	var currency string
	if tx.Table("orders").Select("currency_code").Where("id=? AND deleted_at IS NULL", root.OrderID).Scan(&currency).Error != nil || currency == "" {
		return ErrUnavailable
	}
	var rows []cancellationPayment
	if tx.Table("payments p").Select("p.id,p.payment_collection_id,p.payment_session_id,p.provider_id,p.currency_code,p.captured_at,p.canceled_at").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id AND l.deleted_at IS NULL").Where("l.order_id=? AND p.deleted_at IS NULL", root.OrderID).Order("p.id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(101).Find(&rows).Error != nil || len(rows) > 100 {
		return ErrUnavailable
	}
	for _, p := range rows {
		if p.CurrencyCode != currency {
			return ErrConflict
		}
		balance, err := paymentBalance(tx, p.ID)
		if err != nil {
			return err
		}
		if p.CapturedAt == nil && p.CanceledAt == nil {
			// Admission must not claim an unverified authorization is canceled.
			// Its separate provider-void participant is required before release.
			return ErrUnavailable
		}
		if balance.Sign() == 0 {
			continue
		}
		if p.CapturedAt == nil || p.CanceledAt != nil {
			return ErrConflict
		}
		if _, err := s.Adapters.Refunder(p.ProviderID); err != nil {
			return ErrUnavailable
		}
		amount := balance.FloatString(2)
		canonical, ok := new(big.Rat).SetString(amount)
		if !ok || canonical.Cmp(balance) != 0 {
			return ErrConflict
		}
		if err := reserve(tx, p.ID, amount); err != nil {
			return err
		}
		var collection struct{ ID string }
		if tx.Table("payment_collections").Select("id").Where("id=? AND deleted_at IS NULL", p.PaymentCollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&collection).Error != nil {
			return ErrConflict
		}
		var trace supplementalTrace
		err = tx.Table("cart_completion_receipts c").Select("a.cart_id AS scope_id,a.operation_id,a.session_id,r.remote_id,a.request_sha256").Joins("JOIN cart_checkout_attempts a ON a.cart_id=c.cart_id AND a.operation_id=c.operation_id").Joins("JOIN mollie_payment_references r ON r.scope_id=a.cart_id AND r.operation_id=a.operation_id AND r.resource_id=a.session_id AND r.effect_kind='create_session' AND r.request_sha256=a.request_sha256").Where("c.order_id=? AND c.payment_id=? AND a.collection_id=? AND a.session_id=? AND r.provider_id=?", root.OrderID, p.ID, p.PaymentCollectionID, p.PaymentSessionID, p.ProviderID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&trace).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			trace, err = s.supplementalTrace(tx, root.OrderID, p.ID, p.PaymentCollectionID, p.PaymentSessionID, p.ProviderID)
		}
		if err != nil {
			return err
		}
		child := operation{CancellationID: &root.OperationID, OperationID: uuid.NewString(), ActorID: root.ActorID, OrderID: root.OrderID, PaymentID: p.ID, CollectionID: p.PaymentCollectionID, Amount: amount, CurrencyCode: p.CurrencyCode, ProviderID: p.ProviderID, ScopeID: trace.ScopeID, CheckoutOperationID: trace.OperationID, SessionID: trace.SessionID, RemotePaymentID: trace.RemoteID, RequestSHA256: trace.RequestSHA256, InputSHA256: root.InputSHA256, State: "pending"}
		request, err := s.request(ctx, tx, child)
		clear(request.Session.Data)
		clear(request.Session.Context)
		if err != nil {
			return err
		}
		if tx.Model(&operation{}).Create(map[string]any{"cancellation_id": root.OperationID, "request_id": nil, "operation_id": child.OperationID, "actor_id": child.ActorID, "order_id": child.OrderID, "payment_id": child.PaymentID, "collection_id": child.CollectionID, "amount": child.Amount, "currency_code": child.CurrencyCode, "input_sha256": child.InputSHA256, "scope_id": child.ScopeID, "checkout_operation_id": child.CheckoutOperationID, "session_id": child.SessionID, "request_sha256": child.RequestSHA256, "provider_id": child.ProviderID, "remote_payment_id": child.RemotePaymentID, "state": "pending", "created_at": gorm.Expr("UTC_TIMESTAMP(6)"), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}).Error != nil {
			return ErrConflict
		}
	}
	return nil
}

// Keep reason text as a string in the metadata object; never interpolate JSON.
func cancellationMetadata(previous *string, root orderCancellation, now time.Time) (string, error) {
	object := map[string]json.RawMessage{}
	if previous != nil && json.Unmarshal([]byte(*previous), &object) != nil {
		return "", ErrUnavailable
	}
	if object == nil {
		object = map[string]json.RawMessage{}
	}
	reason, err := json.Marshal(map[string]any{"reason": root.Reason, "actor_id": root.ActorID, "recorded_at": now.Format(time.RFC3339Nano)})
	if err != nil {
		return "", ErrUnavailable
	}
	object["cancellation"] = reason
	raw, err := json.Marshal(object)
	return string(raw), err
}
