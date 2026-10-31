package refunds

import (
	"context"
	"gorm.io/gorm"
	"time"
)

type WorkResult struct{ Selected, Processed, Failed int }

// RunDue is serial per process and bounded to ten durable instructions. Separate
// workers are safe: SQL claims fence all provider calls. The cursor gives broken
// instructions fair retries without starving later work; it carries no authority.
func (s Service) RunDue(ctx context.Context, cursor string) (WorkResult, string, error) {
	var out WorkResult
	if s.DB == nil || s.Ring == nil || s.Adapters == nil || !idPattern.MatchString(s.StoreID) {
		return out, cursor, ErrUnavailable
	}
	if _, nested := s.DB.Statement.ConnPool.(gorm.TxCommitter); nested {
		return out, cursor, ErrUnavailable
	}
	var due []string
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := gate(tx); e != nil {
			return e
		}
		q := tx.Table("order_refund_operations r").Select("r.operation_id").Joins("JOIN cart_completion_receipts c ON c.order_id=r.order_id").Joins("JOIN cart_ownership o ON o.cart_id=c.cart_id").Where("o.store_id=? AND r.canceled_at IS NULL AND r.applied=FALSE AND r.state NOT IN ('failed','canceled') AND r.operation_id>? AND (r.lease_until IS NULL OR r.lease_until<=UTC_TIMESTAMP(6) OR r.state='refunded')", s.StoreID, cursor).Order("r.operation_id").Limit(10)
		return q.Scan(&due).Error
	})
	if e != nil {
		return out, cursor, ErrUnavailable
	}
	if len(due) == 0 {
		return out, "", nil
	}
	for _, id := range due {
		if e := ctx.Err(); e != nil {
			return out, cursor, e
		}
		out.Selected++
		cursor = id
		call, cancel := context.WithTimeout(ctx, 40*time.Second)
		_, e := s.Run(call, id)
		cancel()
		if e != nil {
			out.Failed++
		} else {
			out.Processed++
		}
	}
	return out, cursor, nil
}
