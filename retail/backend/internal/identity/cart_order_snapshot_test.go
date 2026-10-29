package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkPaidOrderSnapshots(t *testing.T, ctx context.Context, db *gorm.DB, ref sessionstore.CartReference, options cart.InitialPaymentOptions) {
	t.Helper()
	var attempt struct {
		OperationID       string
		InputVersion      uint64
		PaidObservationID *string
	}
	if db.Table("cart_checkout_attempts").Where("cart_id=?", ref.CartID).Take(&attempt).Error != nil || attempt.PaidObservationID == nil {
		t.Fatal("paid proof missing")
	}
	in := cart.InitialPaymentCommand{OperationID: attempt.OperationID, Version: strconv.FormatUint(attempt.InputVersion, 10)}
	rollback := errors.New("owned snapshot rollback")
	run := func(fn func(*gorm.DB) error) error {
		return db.WithContext(ctx).Transaction(fn, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	counts := func() map[string]int64 {
		m := map[string]int64{}
		for _, table := range []string{"orders", "order_addresses", "order_carts", "order_items", "order_line_items", "order_line_adjustments", "order_line_tax_lines", "order_shipping_versions", "order_shipping_methods", "order_shipping_adjustments", "order_shipping_tax_lines", "order_promotions", "inventory_reservations"} {
			var n int64
			if db.Table(table).Count(&n).Error != nil {
				t.Fatal("snapshot counts", table)
			}
			m[table] = n
		}
		return m
	}
	// Allocation is intentionally closed before reviewed cutover, even with paid
	// evidence. The caller rolls back the preliminary address copies as well.
	before := counts()
	if err := run(func(tx *gorm.DB) error {
		_, err := cart.StagePaidOrderSnapshot(ctx, tx, "shop", ref, in, options)
		return err
	}); err != orders.ErrNumbering {
		t.Fatal("unseeded paid order", err)
	}
	for table, n := range counts() {
		if n != before[table] {
			t.Fatal("unseeded stage leaked rows", table)
		}
	}
	if err := run(func(tx *gorm.DB) error {
		return orders.SeedOrderNumbers(ctx, tx, orders.OrderNumberSeed{LastDisplayID: 1000, LastTick: 200000000000000, EvidenceSHA256: sha256.Sum256([]byte("owned checkout source snapshot"))})
	}); err != nil {
		t.Fatal("snapshot seed", err)
	}
	// Full length codes must survive the actual INSERT path, not only DDL checks.
	code := strings.Repeat("优惠", 90) + "LONG-CODE"
	var line string
	if db.Table("cart_line_items").Select("id").Where("cart_id=? AND deleted_at IS NULL", ref.CartID).Limit(1).Scan(&line).Error != nil || line == "" {
		t.Fatal("snapshot source line")
	}
	if db.Exec("INSERT INTO cart_line_adjustments(id,item_id,code,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES('snapshot-zero',?,?,0,'{\"value\":\"0\",\"precision\":20}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", line, code).Error != nil {
		t.Fatal("owned long code")
	}
	defer db.Exec("DELETE FROM cart_line_adjustments WHERE id='snapshot-zero'")
	before = counts()
	var staged orders.CheckoutSnapshot
	if err := run(func(tx *gorm.DB) error {
		var err error
		staged, err = cart.StagePaidOrderSnapshot(ctx, tx, "shop", ref, in, options)
		if err != nil {
			return err
		}
		amounts, err := orders.ReadAmountSnapshot(tx, staged.OrderID)
		if err != nil || amounts.Totals["total"] != "158.75" || len(amounts.Lines) != 1 || len(amounts.ShippingMethods) != 1 {
			t.Error("order financial projection differs", err)
			return rollback
		}
		if staged.LineIDs[line] == "" || staged.LineIDs[line] == line || len(staged.ShippingIDs) != 1 {
			t.Error("new line mapping missing")
		}
		var order struct {
			DisplayID            int
			CustomDisplayID      string
			CustomerID, Metadata *string
		}
		if tx.Table("orders").Where("id=?", staged.OrderID).Take(&order).Error != nil || order.DisplayID != 1001 || order.CustomDisplayID != "2033051811332000001" || order.CustomerID != nil || order.Metadata != nil {
			t.Error("order numbering or private principal copied")
		}
		var copied string
		if tx.Table("order_line_adjustments").Select("code").Where("item_id=?", staged.LineIDs[line]).Scan(&copied).Error != nil || copied != code {
			t.Error("long adjustment code truncated")
		}
		var linked int64
		if tx.Table("order_line_items").Where("id=? AND totals_id IS NOT NULL", staged.LineIDs[line]).Count(&linked).Error != nil || linked != 1 {
			t.Error("order item totals cycle missing")
		}
		// Snapshot values, not just totals, retain JSON text and exact raw money.
		var equal bool
		if tx.Raw(`SELECT (o.title <=> c.title) AND (o.variant_option_values <=> c.variant_option_values)
 AND (o.raw_unit_price <=> c.raw_unit_price) AND (o.raw_compare_at_unit_price <=> c.raw_compare_at_unit_price)
 AND (o.variant_sku <=> c.variant_sku) AND (o.metadata <=> c.metadata)
 FROM order_line_items o JOIN cart_line_items c ON c.id=? WHERE o.id=?`, line, staged.LineIDs[line]).Scan(&equal).Error != nil || !equal {
			t.Error("line snapshot raw values changed")
		}
		for source, target := range staged.ShippingIDs {
			if tx.Raw(`SELECT (o.raw_amount <=> c.raw_amount) AND (o.description <=> c.description)
 AND (o.data <=> c.data) AND (o.shipping_option_id <=> c.shipping_option_id) AND o.is_custom_amount=FALSE
 FROM order_shipping_methods o JOIN cart_shipping_methods c ON c.id=? WHERE o.id=?`, source, target).Scan(&equal).Error != nil || !equal {
				t.Error("shipping snapshot values changed")
			}
		}
		if tx.Raw(`SELECT (oa.first_name <=> ca.first_name) AND (oa.address_1 <=> ca.address_1)
 AND (oa.postal_code <=> ca.postal_code) AND (oa.country_code <=> ca.country_code)
 AND oa.id<>ca.id AND oa.customer_id IS NULL
 FROM orders o JOIN order_addresses oa ON oa.id=o.shipping_address_id
 JOIN carts c ON c.id=? JOIN cart_addresses ca ON ca.id=c.shipping_address_id WHERE o.id=?`, ref.CartID, staged.OrderID).Scan(&equal).Error != nil || !equal {
			t.Error("shipping address not copied independently")
		}
		return rollback
	}); err != rollback {
		t.Fatal("paid snapshot staging", err)
	}
	for table, n := range counts() {
		if n != before[table] {
			t.Fatal("snapshot rollback leaked rows", table)
		}
	}
	var display int64
	if db.Table("order_number_allocator").Select("last_display_id").Where("id=1").Scan(&display).Error != nil || display != 1000 {
		t.Fatal("snapshot rollback consumed order number")
	}
	// Missing/sealed evidence cannot be replaced by setting observed_status.
	if db.Exec("UPDATE cart_checkout_attempts SET paid_observation_id=NULL WHERE cart_id=?", ref.CartID).Error != nil {
		t.Fatal("proof failure setup")
	}
	if err := run(func(tx *gorm.DB) error {
		_, err := cart.StagePaidOrderSnapshot(ctx, tx, "shop", ref, in, options)
		return err
	}); err != cart.ErrConflict {
		t.Fatal("paid string used without evidence", err)
	}
	if db.Exec("UPDATE cart_checkout_attempts SET paid_observation_id=? WHERE cart_id=?", *attempt.PaidObservationID, ref.CartID).Error != nil {
		t.Fatal("proof restoration")
	}
	for _, sample := range []struct {
		query string
		want  error
	}{
		{"UPDATE cart_checkout_attempts SET observed_status='authorized' WHERE cart_id=?", cart.ErrConflict},
		{"UPDATE cart_checkout_attempts SET needs_review=TRUE WHERE cart_id=?", cart.ErrConflict},
		{"UPDATE cart_checkout_observations SET request_sha256=UNHEX(REPEAT('00',32)) WHERE id=(SELECT paid_observation_id FROM cart_checkout_attempts WHERE cart_id=?)", cart.ErrConflict},
		{"UPDATE cart_checkout_observations SET result_envelope='tampered' WHERE id=(SELECT paid_observation_id FROM cart_checkout_attempts WHERE cart_id=?)", cart.ErrStorage},
	} {
		if err := run(func(tx *gorm.DB) error {
			if tx.Exec(sample.query, ref.CartID).Error != nil {
				return errors.New("owned evidence mutation failed")
			}
			_, err := cart.StagePaidOrderSnapshot(ctx, tx, "shop", ref, in, options)
			return err
		}); err != sample.want {
			t.Fatal("unsafe paid proof accepted", err)
		}
	}
	if db.Exec("CREATE TRIGGER fail_order_snapshot BEFORE INSERT ON order_shipping_versions FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned late snapshot failure'").Error != nil {
		t.Fatal("snapshot failure injection")
	}
	if err := run(func(tx *gorm.DB) error {
		_, err := cart.StagePaidOrderSnapshot(ctx, tx, "shop", ref, in, options)
		return err
	}); err != orders.ErrCheckoutSnapshot {
		t.Fatal("late snapshot failure hidden", err)
	}
	if db.Exec("DROP TRIGGER fail_order_snapshot").Error != nil {
		t.Fatal("snapshot trigger cleanup")
	}
	for table, n := range counts() {
		if n != before[table] {
			t.Fatal("late snapshot rollback leaked", table)
		}
	}
	wrong := ref
	wrong.Generation = strings.Repeat("a", 64)
	if err := run(func(tx *gorm.DB) error {
		_, err := cart.StagePaidOrderSnapshot(ctx, tx, "shop", wrong, in, options)
		return err
	}); err != cart.ErrDenied {
		t.Fatal("foreign snapshot authority", err)
	}
	if _, err := cart.StagePaidOrderSnapshot(ctx, db, "shop", ref, in, options); err != cart.ErrInput {
		t.Fatal("snapshot accepted ordinary pool")
	}
	checkPaidOrderStock(t, ctx, db, ref, in, options)
	checkPaidOrderLedger(t, ctx, db, ref, in, options)
	t.Log(`CART_ORDER_SNAPSHOT_REHEARSAL {"real_mysql":true,"current_possession_and_frozen_binding":true,"sealed_paid_proof_required":true,"authorized_review_tamper_and_digest_denied":true,"server_saved_payment_identity":true,"all_cart_totals_compared":true,"original_two_order_numbers":true,"copied_addresses_lines_shipping_tax_adjustments":true,"raw_money_and_json_unchanged":true,"unicode_189_character_code":true,"new_line_ids_for_reservations":true,"cyclic_item_totals_link":true,"caller_rollback_all_rows_and_number":true,"late_sql_failure_rollback":true,"no_stock_no_capture_no_complete_no_mail":true}`)
}
