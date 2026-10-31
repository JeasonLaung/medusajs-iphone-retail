package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/refunds"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// A rollback-only native exchange on the actual completed checkout verifies
// reservation and first-dispatch rejection. It intentionally never dispatches
// a provider call from the uncommitted fixture. Browser/CLI acceptance is separate.
func checkExchangeRefundCredit(t *testing.T, ctx context.Context, db *gorm.DB, s refunds.Service, actor identity.User, in refunds.Input, channel *refundChannel) {
	t.Helper()
	rollback := errors.New("owned exchange refund fixture rollback")
	e := db.Transaction(func(tx *gorm.DB) error {
		exec := func(q string, args ...any) {
			t.Helper()
			if e := tx.Exec(q, args...).Error; e != nil {
				t.Fatal("exchange refund fixture", e)
			}
		}
		exec("UPDATE after_sales_requests SET type='exchange' WHERE id=?", in.RequestID)
		exec(`UPDATE order_items SET fulfilled_quantity=1,raw_fulfilled_quantity='{"value":"1","precision":100}',shipped_quantity=1,raw_shipped_quantity='{"value":"1","precision":100}' WHERE order_id=? AND version=1`, in.OrderID)
		var location string
		if tx.Table("inventory_locations").Select("id").Where("deleted_at IS NULL").Order("id").Limit(1).Scan(&location).Error != nil || location == "" {
			t.Fatal("exchange refund location")
		}
		var line string
		if tx.Table("order_items").Select("item_id").Where("order_id=? AND version=1", in.OrderID).Scan(&line).Error != nil {
			t.Fatal("exchange refund line")
		}
		exec("UPDATE tax_rates SET rate=15 WHERE is_default=TRUE AND deleted_at IS NULL")
		native, e := orders.ApplyDeviceExchangeTx(ctx, tx, orders.DeviceExchangeInput{OrderID: in.OrderID, OriginalItemID: line, LocationID: location, OriginalAssignmentID: "refund-assignment", RequestID: in.RequestID, ActorID: actor.ID, ExpectedVersion: 1})
		if e != nil {
			t.Fatal("exchange refund native change", e)
		}
		if native.Amounts.Summary == nil || native.Amounts.Summary.PendingDifference != "-12" {
			t.Fatal("exchange refund expected credit", native.Amounts.Summary)
		}
		local := s
		local.DB = tx
		credit, e := local.Read(ctx, actor, in.OrderID, in.RequestID)
		if e != nil || credit.ExchangeCredit == nil || *credit.ExchangeCredit != "12.00" || credit.RequestType != "exchange" {
			t.Fatal("exchange refund context", credit, e)
		}
		// A cached summary must not invent extra refundable cash.
		exec(`UPDATE order_summaries SET totals=JSON_SET(totals,'$.transaction_total','99999','$.raw_transaction_total',JSON_OBJECT('value','99999','precision',100)) WHERE order_id=? AND version=3`, in.OrderID)
		for _, amount := range []string{"0.01", "11.99", "12.01", "158.75"} {
			bad := in
			bad.OperationID = uuid.NewString()
			bad.Amount = amount
			if _, e := local.Prepare(ctx, actor, bad); e != refunds.ErrConflict {
				t.Fatal("exchange credit bound", amount, e)
			}
		}
		good := in
		good.OperationID = uuid.NewString()
		good.Amount = "12.00"
		if _, e := local.Prepare(ctx, actor, good); e != nil {
			t.Fatal("exchange full credit prepare", e)
		}
		credit, e = local.Read(ctx, actor, in.OrderID, in.RequestID)
		if e != nil || credit.ExchangeCredit == nil || *credit.ExchangeCredit != "0.00" {
			t.Fatal("exchange reserved credit", credit, e)
		}
		// A changed current line balance invalidates an undispatched instruction.
		exec(`UPDATE order_line_tax_lines SET rate=25,raw_rate='{"value":"25","precision":100}' WHERE item_id=?`, native.ItemID)
		if _, e := local.Run(ctx, good.OperationID); e != refunds.ErrConflict || channel.posts.Load() != 0 {
			t.Fatal("stale exchange credit dispatched", e)
		}
		if _, e := local.Cancel(ctx, actor, in.OrderID, in.RequestID, refunds.CancelInput{OperationID: uuid.NewString(), RefundOperationID: good.OperationID}); e != nil {
			t.Fatal("stale exchange cancellation", e)
		}
		for _, rate := range []string{"25", "30"} {
			exec("UPDATE order_line_tax_lines SET rate=?,raw_rate=JSON_OBJECT('value',?,'precision',100) WHERE item_id=?", rate, rate, native.ItemID)
			bad := good
			bad.OperationID = uuid.NewString()
			bad.PreviousOperationID = good.OperationID
			if _, e := local.Prepare(ctx, actor, bad); e != refunds.ErrConflict {
				t.Fatal("nonnegative exchange refunded", rate, e)
			}
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal("exchange refund rollback", e)
	}
	t.Log(`NATIVE_EXCHANGE_REFUND_CREDIT_REHEARSAL {"actual_completed_checkout":true,"native_exchange_change":true,"credit":"12.00","exact_full_credit_only":true,"cached_summary_not_cash_authority":true,"order_wide_hold_projection":true,"stale_first_dispatch_denied":true,"zero_positive_difference_denied":true,"fixture_rolled_back":true,"provider_dispatch":false,"browser_cli_acceptance":false}`)
}
