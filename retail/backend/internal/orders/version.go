package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrVersionConflict = errors.New("order version changed or conflicting snapshot exists")

// AdvanceVersionTx is an INTERNAL participant for order changes. The owning
// command must first authorize and lock its business objects, persist its own
// immutable receipt, and roll back the WHOLE transaction on ANY later error.
// This does not create an exchange, reserve stock, authorize an actor or commit.
// Snapshot rows in every prior version remain immutable. Non-versioned tax and
// product snapshots remain shared; later new-item tax rows use a new item ID.
func AdvanceVersionTx(ctx context.Context, tx *gorm.DB, orderID string, expected int) (int, error) {
	if !checkoutID.MatchString(orderID) || expected < 1 || expected >= 2147483647 {
		return 0, ErrVersionConflict
	}
	if e := numberTransaction(ctx, tx); e != nil {
		return 0, e
	}
	var order struct {
		Version      int
		Status       string
		IsDraftOrder bool
		CanceledAt   *time.Time
	}
	if e := tx.Table("orders").Select("version,status,is_draft_order,canceled_at").Where("id=? AND deleted_at IS NULL", orderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error; e != nil {
		return 0, e
	}
	if order.Version != expected || order.IsDraftOrder || order.Status == "draft" || order.Status == "canceled" || order.CanceledAt != nil {
		return 0, ErrVersionConflict
	}
	before, e := ReadAmountSnapshot(tx, orderID)
	if e != nil {
		return 0, e
	}
	next := expected + 1
	// A pending native change must be resolved by its owning command. Never
	// silently put a second change into a version that already has staged data.
	var active int64
	if tx.Table("order_changes").Where("order_id=? AND status IN ('pending','requested') AND deleted_at IS NULL", orderID).Count(&active).Error != nil {
		return 0, ErrSchema
	}
	if active != 0 {
		return 0, ErrVersionConflict
	}
	for _, table := range []string{"order_items", "order_shipping_versions", "order_credit_lines", "order_summaries"} {
		var count int64
		if tx.Table(table).Where("order_id=? AND version>=?", orderID, next).Count(&count).Error != nil {
			return 0, ErrSchema
		}
		if count != 0 {
			return 0, ErrVersionConflict
		}
	}
	ids := make([]string, 0, len(before.Lines))
	for _, line := range before.Lines {
		ids = append(ids, line.ItemID)
	}
	shipping := make([]string, 0, len(before.ShippingMethods))
	for _, method := range before.ShippingMethods {
		shipping = append(shipping, method.ShippingMethodID)
	}
	for _, scope := range []struct {
		table, where string
		args         []any
	}{
		{"order_items", "order_id=? AND version=?", []any{orderID, expected}},
		{"order_shipping_versions", "order_id=? AND version=?", []any{orderID, expected}},
		{"order_credit_lines", "order_id=? AND version=?", []any{orderID, expected}},
		{"order_summaries", "order_id=? AND version=?", []any{orderID, expected}},
		{"order_line_adjustments", "item_id IN ? AND version=?", []any{ids, expected}},
		{"order_shipping_adjustments", "shipping_method_id IN ? AND version=?", []any{shipping, expected}},
	} {
		if e = copyVersionRows(tx, scope.table, scope.where, scope.args, next); e != nil {
			return 0, fmt.Errorf("copy %s: %w", scope.table, e)
		}
	}
	r := tx.Exec("UPDATE orders SET version=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND version=? AND deleted_at IS NULL", next, orderID, expected)
	if r.Error != nil {
		return 0, ErrSchema
	}
	if r.RowsAffected != 1 {
		return 0, ErrVersionConflict
	}
	after, e := ReadAmountSnapshot(tx, orderID)
	if e != nil {
		return 0, e
	}
	if !sameSnapshotAmounts(before, after) {
		return 0, fmt.Errorf("version amount mismatch: %w", ErrSchema)
	}
	return next, nil
}

// Only compile-time table/where identifiers above reach this helper. Copy every
// stored field (including nullable references, exact scalar/raw values and
// metadata) instead of reconstructing money from floating-point projections.
func copyVersionRows(tx *gorm.DB, table, where string, args []any, next int) error {
	target := append([]any(nil), args...)
	target[len(target)-1] = next
	var occupied int64
	if tx.Table(table).Where(strings.Replace(where, "version=?", "version>=?", 1), target...).Count(&occupied).Error != nil {
		return ErrSchema
	}
	if occupied != 0 {
		return ErrVersionConflict
	}
	var rows []map[string]any
	if tx.Table(table).Where(where, args...).Where("deleted_at IS NULL").Order("id").Limit(10001).Find(&rows).Error != nil || len(rows) > 10000 {
		return ErrSchema
	}
	for _, row := range rows {
		// MySQL owns this generated active uniqueness key.
		if table == "order_shipping_adjustments" {
			delete(row, "active_key_ea58f9d7444a63c8765037c7")
		}
		row["id"] = "ordver_" + uuid.NewString()
		row["version"] = next
		row["created_at"] = time.Now().UTC()
		row["updated_at"] = row["created_at"]
		if e := tx.Table(table).Create(row).Error; e != nil {
			var driver *mysql.MySQLError
			if errors.As(e, &driver) {
				return fmt.Errorf("insert code %d: %w", driver.Number, ErrSchema)
			}
			return fmt.Errorf("insert failed: %w", ErrSchema)
		}
	}
	return nil
}

func sameSnapshotAmounts(a, b AmountDetail) bool {
	if a.OrderID != b.OrderID || a.CurrencyCode != b.CurrencyCode || len(a.Lines) != len(b.Lines) || len(a.ShippingMethods) != len(b.ShippingMethods) || !equalAmounts(a.Totals, b.Totals) {
		return false
	}
	if (a.Summary == nil) != (b.Summary == nil) || a.Summary != nil && *a.Summary != *b.Summary {
		return false
	}
	lines := map[string]AmountLine{}
	for _, l := range a.Lines {
		lines[l.ItemID] = l
	}
	for _, l := range b.Lines {
		prior, ok := lines[l.ItemID]
		if !ok || l.UnitPrice != prior.UnitPrice || l.Quantity != prior.Quantity || !equalAmounts(l.Totals, prior.Totals) {
			return false
		}
		delete(lines, l.ItemID)
	}
	methods := map[string]AmountShipping{}
	for _, s := range a.ShippingMethods {
		methods[s.ShippingMethodID] = s
	}
	for _, s := range b.ShippingMethods {
		prior, ok := methods[s.ShippingMethodID]
		if !ok || s.Name != prior.Name || s.Amount != prior.Amount || s.IsTaxInclusive != prior.IsTaxInclusive || !equalAmounts(s.Totals, prior.Totals) {
			return false
		}
		delete(methods, s.ShippingMethodID)
	}
	return len(lines) == 0 && len(methods) == 0
}
func equalAmounts(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || v != w {
			return false
		}
	}
	return true
}
