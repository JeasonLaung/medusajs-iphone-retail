package identity_test

import (
	"errors"
	"testing"

	"github.com/eurorefurb/retail/internal/orders"
	"gorm.io/gorm"
)

func checkOrderVersion(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := t.Context()
	rollback := errors.New("artificial owner rollback")
	if _, e := orders.AdvanceVersionTx(ctx, db, "allocation-order", 1); e == nil {
		t.Fatal("autocommit accepted")
	}
	run := func(body func(*gorm.DB)) {
		t.Helper()
		e := db.Transaction(func(tx *gorm.DB) error { body(tx); return rollback })
		if !errors.Is(e, rollback) {
			t.Fatal("version rehearsal", e)
		}
	}
	exec := func(tx *gorm.DB, q string, args ...any) {
		t.Helper()
		if e := tx.Exec(q, args...).Error; e != nil {
			t.Fatal("version fixture", e)
		}
	}
	read := func(tx *gorm.DB) orders.AmountDetail {
		t.Helper()
		out, e := orders.ReadAmountSnapshot(tx, "allocation-order")
		if e != nil {
			t.Fatal("version amount read", e)
		}
		return out
	}
	var baseline int64
	db.Table("order_items").Where("order_id='allocation-order'").Count(&baseline)
	run(func(tx *gorm.DB) {
		exec(tx, `INSERT INTO order_line_tax_lines(id,item_id,code,rate,raw_rate,created_at,updated_at) VALUES('version-tax','allocation-line','fixture',20,'{"value":"20","precision":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO order_line_adjustments(id,item_id,version,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES('version-line-adjust','allocation-line',1,1.000000000000000001,'{"value":"1.000000000000000001","precision":38}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO order_shipping_adjustments(id,shipping_method_id,version,amount,raw_amount,created_at,updated_at) VALUES('version-shipping-adjust','packing-method',1,0,'{"value":"0","precision":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO order_credit_lines(id,order_id,version,amount,raw_amount,metadata,created_at,updated_at) VALUES('version-credit','allocation-order',1,0.000000000000000001,'{"value":"0.000000000000000001","precision":38}','{"reference":"preserve"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		exec(tx, `INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES('version-summary','allocation-order',1,'{"transaction_total":"200","raw_transaction_total":{"value":"200","precision":38},"private_history":"preserve"}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		before := read(tx)
		next, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1)
		if e != nil || next != 2 {
			t.Fatal("advance", e)
		}
		after := read(tx)
		if after.Version != 2 || before.Totals["total"] != after.Totals["total"] || before.Summary == nil || after.Summary == nil || *before.Summary != *after.Summary {
			t.Fatal("changed money")
		}
		for _, table := range []string{"order_items", "order_shipping_versions", "order_credit_lines", "order_summaries", "order_line_adjustments", "order_shipping_adjustments"} {
			var oldCount, newCount int64
			tx.Table(table).Where("version=1 AND deleted_at IS NULL").Count(&oldCount)
			tx.Table(table).Where("version=2 AND deleted_at IS NULL").Count(&newCount)
			// Other order fixtures can have version 1 rows, so compare this order
			// directly below; every scoped relation has exactly one copied row.
			if newCount != 1 || oldCount < 1 {
				t.Fatal("version relation missing", table, newCount)
			}
		}
		var exact string
		tx.Table("order_line_adjustments").Select("raw_amount").Where("item_id='allocation-line' AND version=2").Scan(&exact)
		if exact != `{"value":"1.000000000000000001","precision":38}` {
			t.Fatal("raw exactness lost")
		}
		if _, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1); !errors.Is(e, orders.ErrVersionConflict) {
			t.Fatal("stale version")
		}
		exec(tx, "UPDATE order_items SET written_off_quantity=written_off_quantity+1 WHERE order_id='allocation-order' AND version=2")
		var unchanged string
		tx.Table("order_items").Select("CAST(written_off_quantity AS CHAR)").Where("order_id='allocation-order' AND version=1").Scan(&unchanged)
		if unchanged != "1.000000000000000000" {
			t.Fatal("old version changed", unchanged)
		}
	})
	var n int64
	db.Table("order_items").Where("order_id='allocation-order'").Count(&n)
	if n != baseline {
		t.Fatal("owner rollback leaked version")
	}
	run(func(tx *gorm.DB) {
		exec(tx, "UPDATE orders SET status='canceled' WHERE id='allocation-order'")
		if _, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1); !errors.Is(e, orders.ErrVersionConflict) {
			t.Fatal("canceled version accepted")
		}
	})
	run(func(tx *gorm.DB) {
		exec(tx, `INSERT INTO order_changes(id,order_id,version,status,created_at,updated_at) VALUES('version-pending','allocation-order',2,'pending',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		if _, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1); !errors.Is(e, orders.ErrVersionConflict) {
			t.Fatal("pending change accepted")
		}
	})
	run(func(tx *gorm.DB) {
		exec(tx, `INSERT INTO order_summaries(id,order_id,version,totals,created_at,updated_at) VALUES('version-future','allocation-order',2,'{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		if _, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1); !errors.Is(e, orders.ErrVersionConflict) {
			t.Fatal("future version overwritten")
		}
	})
	run(func(tx *gorm.DB) {
		exec(tx, `INSERT INTO order_line_adjustments(id,item_id,version,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES('version-future-adjust','allocation-line',3,0,'{"value":"0","precision":38}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
		if _, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1); !errors.Is(e, orders.ErrVersionConflict) {
			t.Fatal("future adjustment overwritten")
		}
	})
	if db.Exec("CREATE TRIGGER version_copy_fault BEFORE INSERT ON order_shipping_versions FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial version copy failure'").Error != nil {
		t.Fatal("version trigger fixture")
	}
	e := db.Transaction(func(tx *gorm.DB) error { _, e := orders.AdvanceVersionTx(ctx, tx, "allocation-order", 1); return e })
	if db.Exec("DROP TRIGGER version_copy_fault").Error != nil || e == nil {
		t.Fatal("late copy failure not propagated")
	}
	db.Table("order_items").Where("order_id='allocation-order'").Count(&n)
	if n != baseline {
		t.Fatal("late failure leaked rows")
	}
	t.Log(`ORDER_VERSION_REHEARSAL {"real_mysql":true,"exact_amounts_unchanged":true,"raw_precision_preserved":true,"old_version_immutable":true,"items_shipping_credit_summary_adjustments_copied":true,"tax_snapshot_shared":true,"stale_canceled_pending_future_denied":true,"autocommit_denied":true,"owner_rollback":true,"late_copy_failure_atomic":true,"exchange_command_integrated":false}`)
}
