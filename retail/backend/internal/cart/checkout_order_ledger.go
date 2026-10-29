package cart

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaidOrderLedger struct{ PaymentID, CaptureID string }

// StagePaidOrderLedger records an ALREADY verified external paid fact locally.
// This is not a provider capture request. The caller must first stage the order
// and stock, then invoke this participant within the SAME transaction, complete
// promotion usage/receipt/cart close/outbox before commit and roll back the whole
// transaction on ANY error. No HTTP route, provider call or implicit commit.
// Repeated invocation conflicts instead of creating a second payment; completion
// idempotency must be handled by the owning workflow BEFORE staging any rows.
func StagePaidOrderLedger(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, in InitialPaymentCommand, o InitialPaymentOptions, orderID string) (PaidOrderLedger, error) {
	if tx == nil || tx.Statement == nil || !idPattern.MatchString(orderID) {
		return PaidOrderLedger{}, ErrInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return PaidOrderLedger{}, ErrInput
	}
	if err := initialOptions(o); err != nil {
		return PaidOrderLedger{}, err
	}
	row, key, err := stageInitialPayment(ctx, tx, store, ref, in, o)
	if err != nil {
		return PaidOrderLedger{}, err
	}
	observed, err := readPaidOrderEvidence(ctx, tx, ref, in, o, row, key)
	if err != nil {
		return PaidOrderLedger{}, err
	}
	// This order must be the sole new-order cart link, not a browser-named order
	// with a coincidentally equal total. Legacy or soft-deleted links also conflict.
	var links []string
	if tx.Table("order_carts").Select("order_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(2).Scan(&links).Error != nil {
		return PaidOrderLedger{}, ErrStorage
	}
	if len(links) != 1 || links[0] != orderID {
		return PaidOrderLedger{}, ErrConflict
	}
	var order struct {
		Version              int
		Status, CurrencyCode string
		CustomerID           *string
		IsDraftOrder         bool
	}
	if tx.Table("orders").Select("version,status,currency_code,customer_id,is_draft_order").Where("id=? AND deleted_at IS NULL", orderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error != nil || order.Version != 1 || order.Status != "pending" || order.CurrencyCode != row.CurrencyCode || order.CustomerID != nil || order.IsDraftOrder {
		return PaidOrderLedger{}, ErrConflict
	}
	amounts, err := orders.ReadAmountSnapshot(tx, orderID)
	if err != nil || amounts.Totals["total"] != row.CartAmount {
		return PaidOrderLedger{}, ErrConflict
	}
	// No repair/overwrite of preexisting payment, order settlement or collection
	// balances. Unknown historical or partially committed facts require review.
	for _, query := range []struct {
		table, where string
		args         []any
	}{
		{"payments", "payment_collection_id=? OR payment_session_id=?", []any{row.CollectionID, row.SessionID}},
		{"order_payment_collections", "order_id=? OR payment_collection_id=?", []any{orderID, row.CollectionID}},
		{"order_transactions", "order_id=?", []any{orderID}},
		{"order_summaries", "order_id=?", []any{orderID}},
	} {
		var ids []string
		if tx.Table(query.table).Select("id").Where(query.where, query.args...).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Scan(&ids).Error != nil {
			return PaidOrderLedger{}, ErrStorage
		}
		if len(ids) != 0 {
			return PaidOrderLedger{}, ErrConflict
		}
	}
	var collection struct {
		Status                                                                                                      string
		AuthorizedAmount, CapturedAmount, RefundedAmount, RawAuthorizedAmount, RawCapturedAmount, RawRefundedAmount *string
	}
	if tx.Table("payment_collections").Select("status,authorized_amount,captured_amount,refunded_amount,raw_authorized_amount,raw_captured_amount,raw_refunded_amount").Where("id=? AND deleted_at IS NULL AND completed_at IS NULL", row.CollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&collection).Error != nil || collection.Status != "not_paid" || collection.AuthorizedAmount != nil || collection.CapturedAmount != nil || collection.RefundedAmount != nil || collection.RawAuthorizedAmount != nil || collection.RawCapturedAmount != nil || collection.RawRefundedAmount != nil {
		return PaidOrderLedger{}, ErrConflict
	}
	var session struct{ Status string }
	if tx.Table("payment_sessions").Select("status").Where("id=? AND payment_collection_id=? AND deleted_at IS NULL AND authorized_at IS NULL", row.SessionID, row.CollectionID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&session).Error != nil || session.Status != "authorized" {
		return PaidOrderLedger{}, ErrConflict
	}
	out := PaidOrderLedger{PaymentID: "pay_" + uuid.NewString(), CaptureID: "cap_" + uuid.NewString()}
	// Bind protected provider payload to this exact payment row. Do not copy the
	// session envelope (different AAD), provider ID or address into public metadata.
	envelope, err := o.Ring.Seal(protected.Context{Purpose: "checkout-payment-v1", Table: "payments", Column: "data", Row: out.PaymentID}, observed.Session.Data)
	if err != nil {
		return PaidOrderLedger{}, ErrStorage
	}
	defer clear(envelope)
	scalar, raw, err := sqlAmount(row.SettlementAmount)
	if err != nil {
		return PaidOrderLedger{}, err
	}
	zero, zeroRaw, err := sqlAmount("0")
	if err != nil {
		return PaidOrderLedger{}, err
	}
	now := gorm.Expr("UTC_TIMESTAMP(6)")
	if tx.Table("payments").Create(map[string]any{"id": out.PaymentID, "amount": scalar, "raw_amount": raw, "currency_code": row.CurrencyCode, "provider_id": hostedMollie, "data": "ers1." + base64.StdEncoding.EncodeToString(envelope), "payment_collection_id": row.CollectionID, "payment_session_id": row.SessionID, "captured_at": now, "created_at": now, "updated_at": now}).Error != nil {
		return PaidOrderLedger{}, ErrStorage
	}
	if tx.Table("payment_captures").Create(map[string]any{"id": out.CaptureID, "payment_id": out.PaymentID, "amount": scalar, "raw_amount": raw, "created_by": "checkout-verified-paid", "created_at": now, "updated_at": now}).Error != nil {
		return PaidOrderLedger{}, ErrStorage
	}
	if tx.Table("order_payment_collections").Create(map[string]any{"id": uuid.NewString(), "order_id": orderID, "payment_collection_id": row.CollectionID, "created_at": now, "updated_at": now}).Error != nil {
		return PaidOrderLedger{}, ErrStorage
	}
	if tx.Table("order_transactions").Create(map[string]any{"id": "ordtr_" + uuid.NewString(), "order_id": orderID, "version": 1, "amount": scalar, "raw_amount": raw, "currency_code": row.CurrencyCode, "reference": "capture", "reference_id": out.CaptureID, "created_at": now, "updated_at": now}).Error != nil {
		return PaidOrderLedger{}, ErrStorage
	}
	// Preserve exact cart/order total independently of provider minor-unit
	// settlement; never silently change prices to make the accounting match.
	summary, _ := json.Marshal(map[string]any{"transaction_total": row.SettlementAmount, "raw_transaction_total": json.RawMessage(raw)})
	if tx.Table("order_summaries").Create(map[string]any{"id": "ordsum_" + uuid.NewString(), "order_id": orderID, "version": 1, "totals": string(summary), "created_at": now, "updated_at": now}).Error != nil {
		return PaidOrderLedger{}, ErrStorage
	}
	if r := tx.Table("payment_collections").Where("id=? AND status='not_paid' AND completed_at IS NULL AND deleted_at IS NULL", row.CollectionID).Updates(map[string]any{"status": "completed", "authorized_amount": scalar, "raw_authorized_amount": raw, "captured_amount": scalar, "raw_captured_amount": raw, "refunded_amount": zero, "raw_refunded_amount": zeroRaw, "completed_at": now, "updated_at": now}); r.Error != nil || r.RowsAffected != 1 {
		return PaidOrderLedger{}, ErrStorage
	}
	if r := tx.Table("payment_sessions").Where("id=? AND status='authorized' AND deleted_at IS NULL", row.SessionID).Updates(map[string]any{"status": "captured", "authorized_at": now, "updated_at": now}); r.Error != nil || r.RowsAffected != 1 {
		return PaidOrderLedger{}, ErrStorage
	}
	return out, nil
}
