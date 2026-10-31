package orders

import (
	"context"
	"math/big"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SupplementalPayment is a private native order snapshot, not customer authority
// or paid evidence. The future payment coordinator must bind this exact version
// and collection to its durable instruction, then recheck before first dispatch.
type SupplementalPayment struct {
	OrderID, CollectionID, Amount, CurrencyCode string
	Version                                     int
}

// ReadSupplementalPaymentTx locks and validates an existing order's whole unpaid
// balance. The owning command must authorize the new customer, store and original
// completed checkout BEFORE calling this participant, and retain this transaction
// through staging. No cart, collection, payment, stock or summary is changed.
// Unknown, partial and ambiguous settlement facts require explicit recovery;
// this reader never repairs them or rounds away an unpaid fraction.
func ReadSupplementalPaymentTx(ctx context.Context, tx *gorm.DB, id string, expected int) (out SupplementalPayment, err error) {
	if !checkoutID.MatchString(id) || expected < 1 {
		return out, ErrVersionConflict
	}
	if numberTransaction(ctx, tx) != nil {
		return out, ErrSchema
	}
	var order struct {
		Version              int
		Status, CurrencyCode string
		IsDraftOrder         bool
		CanceledAt           *time.Time
	}
	if tx.Table("orders").Select("version,status,currency_code,is_draft_order,canceled_at").Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error != nil {
		return out, ErrSchema
	}
	if order.Version != expected || order.IsDraftOrder || order.CanceledAt != nil || order.Status == "draft" || order.Status == "canceled" || order.Status == "archived" {
		return out, ErrVersionConflict
	}
	current := currentAmountReads(tx)
	var changes []struct{ ID string }
	if current.Table("order_changes").Select("id").Where("order_id=? AND status IN ('pending','requested') AND deleted_at IS NULL", id).Limit(1).Find(&changes).Error != nil {
		return out, ErrSchema
	}
	if len(changes) != 0 {
		return out, ErrVersionConflict
	}
	balance, e := CurrentPendingDifferenceTx(current, id)
	if e != nil {
		return out, e
	}
	if balance.Sign() <= 0 {
		return out, ErrVersionConflict
	}
	// The existing hosted provider settles in cents. Preserve native exact totals:
	// a fractional cent is neither silently forgiven nor rounded into a charge.
	amount, e := payments.MollieSettlementAmount(balance.FloatString(18))
	settlement, ok := new(big.Rat).SetString(amount)
	if e != nil || !ok || settlement.Cmp(balance) != 0 {
		return out, ErrVersionConflict
	}
	var collections []struct {
		ID, Status, CurrencyCode, Amount, RawAmount                                                                 string
		AuthorizedAmount, RawAuthorizedAmount, CapturedAmount, RawCapturedAmount, RefundedAmount, RawRefundedAmount *string
		CompletedAt                                                                                                 *time.Time
	}
	if tx.Table("payment_collections p").Select("p.id,p.status,p.currency_code,CAST(p.amount AS CHAR) AS amount,p.raw_amount,p.authorized_amount,p.raw_authorized_amount,p.captured_amount,p.raw_captured_amount,p.refunded_amount,p.raw_refunded_amount,p.completed_at").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.id AND l.deleted_at IS NULL").Where("l.order_id=? AND p.deleted_at IS NULL AND p.status NOT IN ('completed','failed','canceled')", id).Order("p.id").Limit(2).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&collections).Error != nil {
		return out, ErrSchema
	}
	if len(collections) != 1 {
		return out, ErrVersionConflict
	}
	c := collections[0]
	if c.CurrencyCode != order.CurrencyCode || (c.Status != "not_paid" && c.Status != "awaiting") || c.CompletedAt != nil {
		return out, ErrVersionConflict
	}
	value, e := exactSupplementalNumber(c.Amount, &c.RawAmount)
	if e != nil || value.Cmp(balance) != 0 {
		return out, ErrVersionConflict
	}
	for _, pair := range [][2]*string{{c.AuthorizedAmount, c.RawAuthorizedAmount}, {c.CapturedAmount, c.RawCapturedAmount}, {c.RefundedAmount, c.RawRefundedAmount}} {
		if pair[0] == nil {
			if pair[1] != nil {
				return out, ErrVersionConflict
			}
			continue
		}
		n, e := exactSupplementalNumber(*pair[0], pair[1])
		if e != nil || n.Sign() != 0 {
			return out, ErrVersionConflict
		}
	}
	// A collection cannot be borrowed from another order. Existing payments are
	// recovery work, including a canceled row, rather than authority for a new POST.
	var links []struct{ OrderID string }
	if current.Table("order_payment_collections").Select("order_id").Where("payment_collection_id=? AND deleted_at IS NULL", c.ID).Limit(2).Find(&links).Error != nil {
		return out, ErrSchema
	}
	if len(links) != 1 || links[0].OrderID != id {
		return out, ErrVersionConflict
	}
	var existing []struct{ ID string }
	if current.Table("payments").Select("id").Where("payment_collection_id=? AND deleted_at IS NULL", c.ID).Limit(1).Find(&existing).Error != nil {
		return out, ErrSchema
	}
	if len(existing) != 0 {
		return out, ErrVersionConflict
	}
	return SupplementalPayment{OrderID: id, CollectionID: c.ID, Amount: amount, CurrencyCode: order.CurrencyCode, Version: order.Version}, nil
}

func exactSupplementalNumber(scalar string, raw *string) (*big.Rat, error) {
	n, e := totals.StoredNumber(scalar, raw)
	if e != nil {
		return nil, ErrSchema
	}
	s, e := totals.Resolve(n)
	if e != nil {
		return nil, ErrSchema
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, ErrSchema
	}
	return v, nil
}

// EnsureNoPendingPaymentTx is an internal participant. Caller owns the order
// lock; switching the feature off never releases a durable monetary hold.
func EnsureNoPendingPaymentTx(tx *gorm.DB, id string) error {
	if tx == nil || tx.Statement == nil || !checkoutID.MatchString(id) {
		return ErrSchema
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrSchema
	}
	var holds []struct{ OperationID string }
	if tx.Table("order_payment_operations").Select("operation_id").Where("active_order=?", id).Limit(1).Clauses(clause.Locking{Strength: "SHARE"}).Find(&holds).Error != nil {
		return ErrSchema
	}
	if len(holds) != 0 {
		return ErrVersionConflict
	}
	return nil
}
