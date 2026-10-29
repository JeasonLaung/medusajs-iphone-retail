package cart

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCompletionPromotion = errors.New("checkout promotion registration is not configured")

type CompletionResult struct {
	OrderID  string `json:"order_id"`
	Version  string `json:"version"`
	Replayed bool   `json:"-"`
}

// CompletePaidOrder owns the entire LOCAL transaction. It never calls a payment
// provider. A sealed, matching paid observation must already exist. Replay is
// authorized against current possession even after the cart has been closed.
// This is internal and gated; HTTP/customer ownership and coupon usage remain
// separate work. Never turn a rejection into a second payment or successful UI.
func CompletePaidOrder(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, o InitialPaymentOptions) (CompletionResult, error) {
	failed := CompletionResult{}
	if db == nil || db.Statement == nil || !refOK(store, ref) {
		return failed, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return failed, ErrInput
	}
	op, err := uuid.Parse(in.OperationID)
	version, verr := strconv.ParseUint(in.Version, 10, 64)
	if err != nil || op.Version() != 4 || op.String() != in.OperationID || verr != nil || version == 0 || strconv.FormatUint(version, 10) != in.Version {
		return failed, ErrInput
	}
	if err = initialOptions(o); err != nil {
		return failed, err
	}
	var result CompletionResult
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := initialReady(ctx, tx); err != nil {
			return err
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_completion_runtime WHERE id=1 FOR SHARE").Scan(&gate)
		if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
			return ErrSchema
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		var owner struct {
			StoreID, Generation                          string
			Version                                      uint64
			Active                                       bool
			CheckoutAttemptID, PaymentRefreshOperationID *string
		}
		r = tx.Table("cart_ownership").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&owner)
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return ErrDenied
		}
		if r.Error != nil {
			return ErrStorage
		}
		if !owner.Active || owner.StoreID != store || subtle.ConstantTimeCompare([]byte(owner.Generation), []byte(ref.Generation)) != 1 {
			return ErrDenied
		}
		if owner.CheckoutAttemptID == nil || *owner.CheckoutAttemptID != in.OperationID || owner.PaymentRefreshOperationID != nil {
			return ErrConflict
		}
		var cart struct {
			SalesChannelID         string
			CustomerID             *string
			CompletedAt, DeletedAt *time.Time
		}
		if tx.Table("carts").Select("sales_channel_id,customer_id,completed_at,deleted_at").Where("id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&cart).Error != nil {
			return ErrStorage
		}
		if cart.DeletedAt != nil || cart.CustomerID != nil || cart.SalesChannelID != channel {
			return ErrDenied
		}
		var receipt struct {
			OperationID, Generation, OrderID string
			InputVersion, CompletedVersion   uint64
		}
		r = tx.Table("cart_completion_receipts").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&receipt)
		if r.Error == nil {
			if receipt.OperationID != in.OperationID || receipt.Generation != ref.Generation || receipt.InputVersion != version || receipt.CompletedVersion != owner.Version || cart.CompletedAt == nil {
				return ErrConflict
			}
			result = CompletionResult{receipt.OrderID, strconv.FormatUint(receipt.CompletedVersion, 10), true}
			return nil
		}
		if !errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return ErrStorage
		}
		if cart.CompletedAt != nil {
			return ErrConflict
		}
		// Coupons must not silently skip budget/attribute usage. Reject until their
		// exact frozen registration is connected to this transaction.
		var n int64
		if tx.Table("cart_promotions").Where("cart_id=? AND deleted_at IS NULL", ref.CartID).Count(&n).Error != nil {
			return ErrStorage
		}
		if n != 0 {
			return ErrCompletionPromotion
		}
		var promoAdjustments int64
		if tx.Raw(`SELECT COUNT(*) FROM (
   SELECT a.id FROM cart_line_adjustments a JOIN cart_line_items i ON i.id=a.item_id WHERE i.cart_id=? AND i.deleted_at IS NULL AND a.deleted_at IS NULL AND a.promotion_id IS NOT NULL
   UNION ALL SELECT a.id FROM cart_shipping_adjustments a JOIN cart_shipping_methods s ON s.id=a.shipping_method_id WHERE s.cart_id=? AND s.deleted_at IS NULL AND a.deleted_at IS NULL AND a.promotion_id IS NOT NULL
  ) p`, ref.CartID, ref.CartID).Scan(&promoAdjustments).Error != nil {
			return ErrStorage
		}
		if promoAdjustments != 0 {
			return ErrCompletionPromotion
		}
		staged, err := StagePaidOrderAndStock(ctx, tx, store, ref, in, o)
		if err != nil {
			return err
		}
		ledger, err := StagePaidOrderLedger(ctx, tx, store, ref, in, o, staged.Snapshot.OrderID)
		if err != nil {
			return err
		}
		var recipient struct {
			Email  string
			Locale *string
		}
		if tx.Table("orders").Select("email,locale").Where("id=?", staged.Snapshot.OrderID).Take(&recipient).Error != nil || recipient.Email == "" {
			return ErrConflict
		}
		notificationID := uuid.NewString()
		payload, err := json.Marshal(struct {
			OrderID, Email string
			Locale         *string
		}{staged.Snapshot.OrderID, recipient.Email, recipient.Locale})
		if err != nil {
			return ErrStorage
		}
		envelope, err := o.Ring.Seal(protected.Context{Purpose: "order-notification-v1", Table: "order_notification_outbox", Column: "payload_envelope", Row: notificationID}, payload)
		clear(payload)
		if err != nil {
			return ErrStorage
		}
		if tx.Exec("INSERT INTO order_notification_outbox(id,order_id,event,payload_envelope,status,attempts,created_at) VALUES(?,?,'order.placed',?,'pending',0,UTC_TIMESTAMP(6))", notificationID, staged.Snapshot.OrderID, envelope).Error != nil {
			return ErrStorage
		}
		next := owner.Version + 1
		if next <= owner.Version {
			return ErrConflict
		}
		r = tx.Exec("UPDATE carts SET completed_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE id=? AND completed_at IS NULL AND deleted_at IS NULL", ref.CartID)
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		r = tx.Exec("UPDATE cart_ownership SET version=?,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=?", next, ref.CartID, owner.Version)
		if r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		if tx.Exec("INSERT INTO cart_completion_receipts(cart_id,operation_id,generation,input_version,completed_version,order_id,payment_id,capture_id,completed_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6))", ref.CartID, in.OperationID, ref.Generation, version, next, staged.Snapshot.OrderID, ledger.PaymentID, ledger.CaptureID).Error != nil {
			return ErrStorage
		}
		result = CompletionResult{staged.Snapshot.OrderID, strconv.FormatUint(next, 10), false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return failed, err
	}
	return result, nil
}
