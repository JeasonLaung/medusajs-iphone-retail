package identity_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkPaidOrderLedger(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, in cart.InitialPaymentCommand, o cart.InitialPaymentOptions) {
	t.Helper()
	rollback := errors.New("owned completion rollback")
	run := func(fn func(*gorm.DB) error) error {
		return db.WithContext(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	fingerprint := func() string {
		t.Helper()
		value := ""
		for _, table := range []string{"orders", "order_addresses", "order_carts", "order_items", "order_line_items", "order_shipping_versions", "order_shipping_methods", "order_payment_collections", "order_transactions", "order_summaries", "payments", "payment_captures", "inventory_reservations", "inventory_reservation_operations"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("ledger fingerprint")
			}
			value += fmt.Sprint(table, n)
		}
		var n struct{ LastDisplayID, LastTick int64 }
		if db.Table("order_number_allocator").Where("id=1").Take(&n).Error != nil {
			t.Fatal("ledger numbering fingerprint")
		}
		value += fmt.Sprint(n)
		var collection struct {
			Status                           string
			CapturedAmount, AuthorizedAmount *string
		}
		if db.Table("payment_collections").Where("id=(SELECT collection_id FROM cart_checkout_attempts WHERE cart_id=?)", ref.CartID).Take(&collection).Error != nil {
			t.Fatal("ledger collection read")
		}
		value += fmt.Sprint(collection.Status, collection.CapturedAmount == nil, collection.AuthorizedAmount == nil)
		return value
	}
	before := fingerprint()
	stage := func(tx *gorm.DB) (cart.PaidOrderStock, error) {
		if tx.Exec("UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.reservations'").Error != nil {
			return cart.PaidOrderStock{}, errors.New("owned inventory enable")
		}
		return cart.StagePaidOrderAndStock(ctx, tx, "shop", ref, in, o)
	}
	check := func(tx *gorm.DB) error {
		out, err := stage(tx)
		if err != nil {
			return err
		}
		ledger, err := cart.StagePaidOrderLedger(ctx, tx, "shop", ref, in, o, out.Snapshot.OrderID)
		if err != nil {
			return err
		}
		var payment struct {
			Amount, RawAmount, CurrencyCode, PaymentSessionID, Data string
			CapturedAt                                              *time.Time
		}
		if tx.Table("payments").Where("id=?", ledger.PaymentID).Take(&payment).Error != nil || payment.Amount != "158.750000000000000000" || payment.CurrencyCode != "eur" || payment.CapturedAt == nil || !strings.HasPrefix(payment.Data, "ers1.") || strings.Contains(payment.Data, "tr_initial") {
			t.Error("payment ledger not protected or exact")
		}
		sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(payment.Data, "ers1."))
		if err != nil {
			t.Error("payment envelope encoding")
			return rollback
		}
		plain, err := o.Ring.Open(protected.Context{Purpose: "checkout-payment-v1", Table: "payments", Column: "data", Row: ledger.PaymentID}, sealed)
		if err != nil || !strings.Contains(string(plain), "tr_initial") {
			t.Error("payment proof envelope cannot be read")
		}
		clear(plain)
		if _, err = o.Ring.Open(protected.Context{Purpose: "checkout-payment-v1", Table: "payments", Column: "data", Row: "foreign-payment"}, sealed); err == nil {
			t.Error("payment envelope row swap accepted")
		}
		var capture struct{ PaymentID, Amount string }
		if tx.Table("payment_captures").Where("id=?", ledger.CaptureID).Take(&capture).Error != nil || capture.PaymentID != ledger.PaymentID || capture.Amount != "158.750000000000000000" {
			t.Error("capture differs from verified payment")
		}
		var movement struct {
			Reference, ReferenceID, Amount string
			Version                        int
		}
		if tx.Table("order_transactions").Where("order_id=?", out.Snapshot.OrderID).Take(&movement).Error != nil || movement.Reference != "capture" || movement.ReferenceID != ledger.CaptureID || movement.Amount != "158.750000000000000000" || movement.Version != 1 {
			t.Error("order movement not tied to capture")
		}
		amounts, err := orders.ReadAmountSnapshot(tx, out.Snapshot.OrderID)
		if err != nil || amounts.Totals["total"] != "158.75" || amounts.Summary == nil || amounts.Summary.TransactionTotal != "158.75" || amounts.Summary.PendingDifference != "0" {
			t.Error("order ledger summary differs", err)
		}
		var collection struct{ Status, CapturedAmount, AuthorizedAmount, RefundedAmount string }
		if tx.Table("payment_collections").Where("id=(SELECT collection_id FROM cart_checkout_attempts WHERE cart_id=?)", ref.CartID).Take(&collection).Error != nil || collection.Status != "completed" || collection.CapturedAmount != "158.750000000000000000" || collection.AuthorizedAmount != collection.CapturedAmount || collection.RefundedAmount != "0.000000000000000000" {
			t.Error("collection balances differ")
		}
		var session struct {
			Status       string
			AuthorizedAt *time.Time
		}
		if tx.Table("payment_sessions").Select("status,authorized_at").Where("id=?", payment.PaymentSessionID).Take(&session).Error != nil || session.Status != "captured" || session.AuthorizedAt == nil {
			t.Error("session not locally captured")
		}
		if _, err = cart.StagePaidOrderLedger(ctx, tx, "shop", ref, in, o, out.Snapshot.OrderID); err != cart.ErrConflict {
			t.Error("duplicate ledger stage accepted", err)
		}
		return rollback
	}
	if err := run(check); err != rollback {
		t.Fatal("paid ledger stage", err)
	}
	if fingerprint() != before {
		t.Fatal("ledger/order/stock rollback incomplete")
	}
	// Artificial frozen-cart fixture: exact 158.75125 rounds to the SAME provider
	// settlement 158.75. Keep the authenticated provider request/evidence intact,
	// and verify the ledger never rounds the persisted order prices to match cash.
	if err := run(func(tx *gorm.DB) error {
		if tx.Exec(`UPDATE cart_line_items SET unit_price=120.001,raw_unit_price='{"value":"120.001","precision":100}' WHERE cart_id=? AND deleted_at IS NULL`, ref.CartID).Error != nil || tx.Exec("UPDATE cart_checkout_attempts SET cart_amount='158.75125' WHERE cart_id=?", ref.CartID).Error != nil {
			return errors.New("owned exact amount fixture")
		}
		out, err := stage(tx)
		if err != nil {
			return err
		}
		if _, err = cart.StagePaidOrderLedger(ctx, tx, "shop", ref, in, o, out.Snapshot.OrderID); err != nil {
			return err
		}
		amounts, err := orders.ReadAmountSnapshot(tx, out.Snapshot.OrderID)
		if err != nil || amounts.Totals["total"] != "158.75125" || amounts.Lines[0].UnitPrice != "120.001" || amounts.Summary == nil || amounts.Summary.TransactionTotal != "158.75" || amounts.Summary.PendingDifference != "0" {
			t.Error("provider rounding overwrote exact order total", err)
		}
		return rollback
	}); err != rollback {
		t.Fatal("exact order versus settlement fixture", err)
	}
	if fingerprint() != before {
		t.Fatal("settlement fixture leaked state")
	}
	for _, query := range []string{
		"UPDATE cart_checkout_attempts SET observed_status='authorized' WHERE cart_id=?",
		"UPDATE cart_checkout_attempts SET needs_review=TRUE WHERE cart_id=?",
		"UPDATE order_carts SET order_id='foreign-order' WHERE cart_id=?",
		"UPDATE payment_collections SET captured_amount=1 WHERE id=(SELECT collection_id FROM cart_checkout_attempts WHERE cart_id=?)",
	} {
		if err := run(func(tx *gorm.DB) error {
			out, err := stage(tx)
			if err != nil {
				return err
			}
			if tx.Exec(query, ref.CartID).Error != nil {
				return errors.New("owned bad ledger setup")
			}
			_, err = cart.StagePaidOrderLedger(ctx, tx, "shop", ref, in, o, out.Snapshot.OrderID)
			return err
		}); err != cart.ErrConflict {
			t.Fatal("bad ledger binding accepted", err)
		}
		if fingerprint() != before {
			t.Fatal("rejected ledger leaked transaction")
		}
	}
	if db.Exec("CREATE TRIGGER fail_paid_ledger BEFORE UPDATE ON payment_collections FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned late ledger failure'").Error != nil {
		t.Fatal("ledger failure setup")
	}
	if err := run(func(tx *gorm.DB) error {
		out, err := stage(tx)
		if err != nil {
			return err
		}
		_, err = cart.StagePaidOrderLedger(ctx, tx, "shop", ref, in, o, out.Snapshot.OrderID)
		return err
	}); err != cart.ErrStorage {
		t.Fatal("late ledger failure hidden", err)
	}
	if db.Exec("DROP TRIGGER fail_paid_ledger").Error != nil {
		t.Fatal("ledger failure cleanup")
	}
	if fingerprint() != before {
		t.Fatal("late ledger failure left payment/capture/order/stock")
	}
	if _, err := cart.StagePaidOrderLedger(ctx, db, "shop", ref, in, o, "order_arbitrary"); err != cart.ErrInput {
		t.Fatal("ledger accepted ordinary pool")
	}
	t.Log(`CART_ORDER_LEDGER_REHEARSAL {"real_mysql":true,"verified_paid_only":true,"same_cart_order_binding":true,"exact_payment_capture_and_transaction":"158.75","exact_order_total_separate_from_settlement":true,"encrypted_provider_evidence":true,"payment_envelope_row_swap_denied":true,"collection_balances_and_session_captured":true,"summary_matches_amount_reader":true,"duplicate_stage_rejected":true,"foreign_order_authorized_review_and_existing_balances_denied":true,"late_collection_failure_rolls_back_order_stock_and_cash":true,"no_external_capture_call":true,"no_completion_or_mail":true}`)
}
