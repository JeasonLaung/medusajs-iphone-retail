package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SQL-only prerequisites on the actual completed checkout. These never initiate
// a provider payment, insert a successful payment or release replacement stock.
func checkExchangePaymentBalance(t *testing.T, ctx context.Context, db *gorm.DB, order, request, actor string, options cart.InitialPaymentOptions) {
	t.Helper()
	if _, e := orders.ReadSupplementalPaymentTx(ctx, db, order, 1); e != orders.ErrSchema {
		t.Fatal("supplemental autocommit accepted", e)
	}
	checkCurrentBalanceAfterSnapshot(t, ctx, db, order)
	rollback := errors.New("supplemental balance fixture rollback")
	e := db.Transaction(func(tx *gorm.DB) error {
		exec := func(q string, args ...any) {
			t.Helper()
			if e := tx.Exec(q, args...).Error; e != nil {
				t.Fatal("supplemental fixture", e)
			}
		}
		if _, e := orders.ReadSupplementalPaymentTx(ctx, tx, order, 1); e != orders.ErrVersionConflict {
			t.Fatal("paid order accepted", e)
		}
		exec("UPDATE after_sales_requests SET type='exchange' WHERE id=?", request)
		exec(`UPDATE order_items SET fulfilled_quantity=1,raw_fulfilled_quantity='{"value":"1","precision":100}',shipped_quantity=1,raw_shipped_quantity='{"value":"1","precision":100}' WHERE order_id=? AND version=1`, order)
		var location, line string
		if tx.Table("inventory_locations").Select("id").Where("deleted_at IS NULL").Order("id").Limit(1).Scan(&location).Error != nil || location == "" {
			t.Fatal("supplemental location")
		}
		if tx.Table("order_items").Select("item_id").Where("order_id=? AND version=1", order).Scan(&line).Error != nil || line == "" {
			t.Fatal("supplemental line")
		}
		exec("UPDATE tax_rates SET rate=30 WHERE is_default=TRUE AND deleted_at IS NULL")
		native, e := orders.ApplyDeviceExchangeTx(ctx, tx, orders.DeviceExchangeInput{OrderID: order, OriginalItemID: line, LocationID: location, OriginalAssignmentID: "refund-assignment", RequestID: request, ActorID: actor, ExpectedVersion: 1})
		if e != nil || native.Amounts.Summary == nil || native.Amounts.Summary.PendingDifference != "6" {
			t.Fatal("positive native exchange", native.Amounts.Summary, e)
		}
		read := func() orders.SupplementalPayment {
			t.Helper()
			out, e := orders.ReadSupplementalPaymentTx(ctx, tx, order, 3)
			if e != nil || out.OrderID != order || out.Version != 3 || out.Amount != "6.00" || out.CurrencyCode != "eur" || out.CollectionID == "" {
				t.Fatal("exact supplemental balance", out, e)
			}
			return out
		}
		out := read()
		if _, e := orders.ReadSupplementalPaymentTx(ctx, tx, order, 1); e != orders.ErrVersionConflict {
			t.Fatal("stale version accepted", e)
		}
		// Cached summary cash and rounded SQL scalars cannot change the exact due.
		exec(`UPDATE order_summaries SET totals=JSON_SET(totals,'$.transaction_total','99999','$.raw_transaction_total',JSON_OBJECT('value','99999','precision',100)) WHERE order_id=? AND version=3`, order)
		exec("UPDATE payment_collections SET amount=99999 WHERE id=?", out.CollectionID)
		read()
		tests := []struct {
			name, sql string
			args      []any
		}{
			{"collection_amount", `UPDATE payment_collections SET raw_amount='{"value":"5.99","precision":100}' WHERE id=?`, []any{out.CollectionID}},
			{"collection_raw_invalid", `UPDATE payment_collections SET raw_amount='{}' WHERE id=?`, []any{out.CollectionID}},
			{"currency", "UPDATE payment_collections SET currency_code='dkk' WHERE id=?", []any{out.CollectionID}},
			{"authorized_state", "UPDATE payment_collections SET status='authorized' WHERE id=?", []any{out.CollectionID}},
			{"partial_capture", "UPDATE payment_collections SET status='partially_captured' WHERE id=?", []any{out.CollectionID}},
			{"cash_without_status", `UPDATE payment_collections SET captured_amount=0,raw_captured_amount='{"value":"0.01","precision":100}' WHERE id=?`, []any{out.CollectionID}},
			{"authorization_without_status", "UPDATE payment_collections SET authorized_amount=1 WHERE id=?", []any{out.CollectionID}},
			{"refund_without_status", "UPDATE payment_collections SET refunded_amount=1 WHERE id=?", []any{out.CollectionID}},
			{"orphan_cash_raw", `UPDATE payment_collections SET raw_captured_amount='{"value":"0","precision":100}' WHERE id=?`, []any{out.CollectionID}},
			{"completed_timestamp", "UPDATE payment_collections SET completed_at=UTC_TIMESTAMP(6) WHERE id=?", []any{out.CollectionID}},
			{"deleted_collection", "UPDATE payment_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", []any{out.CollectionID}},
			{"missing_link", "UPDATE order_payment_collections SET deleted_at=UTC_TIMESTAMP(6) WHERE payment_collection_id=?", []any{out.CollectionID}},
			{"existing_payment", "UPDATE payments p JOIN cart_completion_receipts r ON r.payment_id=p.id SET p.payment_collection_id=? WHERE r.order_id=?", []any{out.CollectionID, order}},
			{"pending_change", "UPDATE order_changes SET status='pending' WHERE order_id=? AND version=3", []any{order}},
			{"canceled_order", "UPDATE orders SET status='canceled' WHERE id=?", []any{order}},
			{"draft_order", "UPDATE orders SET is_draft_order=TRUE WHERE id=?", []any{order}},
			{"deleted_order", "UPDATE orders SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", []any{order}},
			{"zero_difference", `UPDATE order_line_tax_lines SET rate=25,raw_rate='{"value":"25","precision":100}' WHERE item_id=?`, []any{native.ItemID}},
			{"negative_difference", `UPDATE order_line_tax_lines SET rate=15,raw_rate='{"value":"15","precision":100}' WHERE item_id=?`, []any{native.ItemID}},
			{"fractional_cent", `UPDATE order_line_tax_lines SET rate=30.001,raw_rate='{"value":"30.001","precision":100}' WHERE item_id=?`, []any{native.ItemID}},
			{"ledger_currency", "UPDATE order_transactions SET currency_code='dkk' WHERE order_id=?", []any{order}},
			{"ledger_future_version", "UPDATE order_transactions SET version=4 WHERE order_id=?", []any{order}},
			{"ledger_raw_invalid", "UPDATE order_transactions SET raw_amount='{}' WHERE order_id=?", []any{order}},
		}
		for _, test := range tests {
			if tx.SavePoint("supplemental_case").Error != nil {
				t.Fatal("supplemental savepoint")
			}
			exec(test.sql, test.args...)
			if _, e := orders.ReadSupplementalPaymentTx(ctx, tx, order, 3); e == nil {
				t.Fatal("invalid supplemental accepted", test.name)
			}
			if tx.RollbackTo("supplemental_case").Error != nil {
				t.Fatal("supplemental rollback", test.name)
			}
			read()
		}
		if tx.SavePoint("supplemental_ambiguous").Error != nil {
			t.Fatal("ambiguous savepoint")
		}
		second := "paycol_" + uuid.NewString()
		exec("INSERT INTO payment_collections(id,currency_code,amount,raw_amount,status,created_at,updated_at) SELECT ?,currency_code,amount,raw_amount,status,created_at,updated_at FROM payment_collections WHERE id=?", second, out.CollectionID)
		exec("INSERT INTO order_payment_collections(id,order_id,payment_collection_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", uuid.NewString(), order, second)
		if _, e := orders.ReadSupplementalPaymentTx(ctx, tx, order, 3); e != orders.ErrVersionConflict {
			t.Fatal("ambiguous unpaid collections accepted", e)
		}
		if tx.RollbackTo("supplemental_ambiguous").Error != nil {
			t.Fatal("ambiguous rollback")
		}
		read()
		// Explicit exact zero projections and awaiting do not pretend a capture.
		exec(`UPDATE payment_collections SET status='awaiting',authorized_amount=0,raw_authorized_amount='{"value":"0","precision":100}',captured_amount=0,refunded_amount=0 WHERE id=?`, out.CollectionID)
		read()
		var collections, payments int64
		if tx.Table("order_payment_collections").Where("order_id=? AND deleted_at IS NULL", order).Count(&collections).Error != nil || tx.Table("payments p").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id").Where("l.order_id=? AND p.deleted_at IS NULL", order).Count(&payments).Error != nil || collections != 2 || payments != 1 {
			t.Fatal("supplemental reader changed cash or collections", collections, payments)
		}
		checkSupplementalPaymentCommands(t, ctx, tx, out, options)
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal("supplemental fixture rollback", e)
	}
	t.Log(`NATIVE_EXCHANGE_PAYMENT_BALANCE_REHEARSAL {"native_positive_exchange":true,"order_version":3,"due":"6.00","original_capture":"158.75","current_total":"164.75","exact_ledger_authority":true,"cached_summary_not_authority":true,"raw_precedence":true,"stale_version_denied":true,"invalid_cases":24,"zero_negative_fractional_cent_denied":true,"existing_services_preserved":true,"fixture_rolled_back":true,"provider_dispatch":false,"customer_authorization":false,"browser_cli_acceptance":false}`)
}

func checkCurrentBalanceAfterSnapshot(t *testing.T, ctx context.Context, db *gorm.DB, order string) {
	t.Helper()
	if _, e := orders.CurrentPendingDifferenceTx(db, order); e != orders.ErrSchema {
		t.Fatal("balance autocommit accepted", e)
	}
	var tax struct{ ID, Rate, RawRate string }
	if db.Table("order_line_tax_lines t").Select("t.id,CAST(t.rate AS CHAR) AS rate,t.raw_rate").Joins("JOIN order_items i ON i.item_id=t.item_id AND i.version=1 AND i.deleted_at IS NULL").Where("i.order_id=? AND t.deleted_at IS NULL", order).Take(&tax).Error != nil {
		t.Fatal("snapshot original tax")
	}
	defer func() {
		if e := db.Exec("UPDATE order_line_tax_lines SET rate=?,raw_rate=? WHERE id=?", tax.Rate, tax.RawRate, tax.ID).Error; e != nil {
			t.Fatal("snapshot tax cleanup", e)
		}
	}()
	id := "snapshot-fixture_" + uuid.NewString()
	defer func() {
		if e := db.Exec("DELETE FROM order_transactions WHERE id=?", id).Error; e != nil {
			t.Fatal("snapshot fixture cleanup", e)
		}
	}()
	e := db.Transaction(func(tx *gorm.DB) error {
		var isolation string
		if tx.Raw("SELECT @@transaction_isolation").Scan(&isolation).Error != nil || isolation != "REPEATABLE-READ" {
			t.Fatal("repeatable-read fixture isolation", isolation)
		}
		original, e := orders.ReadAmountSnapshot(tx, order)
		if e != nil || original.Totals["total"] != "158.75" {
			t.Fatal("snapshot original amount", original.Totals, e)
		}
		var before int64
		if tx.Table("order_transactions").Where("order_id=? AND deleted_at IS NULL", order).Count(&before).Error != nil || before != 1 {
			t.Fatal("original capture fixture")
		}
		// Commit from a different connection AFTER establishing the old snapshot.
		// This is an explicitly artificial ledger row, not a verified cash receipt.
		if e := db.Exec(`INSERT INTO order_transactions(id,order_id,version,amount,raw_amount,currency_code,created_at,updated_at) VALUES(?,?,1,0.01,'{"value":"0.01","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, order).Error; e != nil {
			t.Fatal("concurrent snapshot fixture", e)
		}
		if e := db.Exec(`UPDATE order_line_tax_lines SET rate=30,raw_rate='{"value":"30","precision":100}' WHERE id=?`, tax.ID).Error; e != nil {
			t.Fatal("concurrent tax fixture", e)
		}
		stale, e := orders.ReadAmountSnapshot(tx, order)
		if e != nil || stale.Totals["total"] != "158.75" {
			t.Fatal("old amount snapshot absent", stale.Totals, e)
		}
		var old int64
		if tx.Table("order_transactions").Where("order_id=? AND deleted_at IS NULL", order).Count(&old).Error != nil || old != before {
			t.Fatal("old repeatable-read snapshot absent")
		}
		var locked string
		if tx.Raw("SELECT id FROM orders WHERE id=? FOR UPDATE", order).Scan(&locked).Error != nil || locked != order {
			t.Fatal("snapshot order lock")
		}
		balance, e := orders.CurrentPendingDifferenceTx(tx.WithContext(ctx), order)
		if e != nil || balance.RatString() != "599/100" {
			t.Fatal("current ledger hidden by old snapshot", balance, e)
		}
		return nil
	})
	if e != nil {
		t.Fatal("current balance snapshot fixture", e)
	}
	t.Log(`CURRENT_ORDER_BALANCE_REHEARSAL {"repeatable_read_old_snapshot_proven":true,"separate_connection_committed_ledger":true,"current_locked_ledger_read":true,"current_locked_line_amount_read":true,"artificial_ledger_only":true,"provider_dispatch":false}`)
}
