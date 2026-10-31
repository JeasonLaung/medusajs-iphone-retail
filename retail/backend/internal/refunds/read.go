package refunds

import (
	"context"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Context struct {
	OrderID    string   `json:"order_id"`
	RequestID  string   `json:"request_id"`
	Enabled    bool     `json:"enabled"`
	Operations []Result `json:"operations"`
}

func (s Service) Read(ctx context.Context, actor identity.User, order, request string) (Context, error) {
	out := Context{OrderID: order, RequestID: request, Operations: []Result{}}
	if !idPattern.MatchString(order) || !idPattern.MatchString(request) {
		return Context{}, ErrInput
	}
	if s.DB == nil {
		return Context{}, ErrUnavailable
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authority(ctx, tx, actor); e != nil {
			return e
		}
		if e := s.scope(tx, order); e != nil {
			return e
		}
		var runtime struct {
			Version int
			Enabled bool
		}
		if tx.Table("order_refund_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&runtime).Error != nil || runtime.Version != 2 {
			return ErrUnavailable
		}
		out.Enabled = runtime.Enabled
		var n int64
		if tx.Table("after_sales_requests").Where("id=? AND order_id=? AND deleted_at IS NULL", request, order).Count(&n).Error != nil {
			return ErrUnavailable
		}
		if n != 1 {
			return ErrConflict
		}
		var rows []operation
		if tx.Where("order_id=? AND request_id=?", order, request).Order("created_at,operation_id").Limit(101).Find(&rows).Error != nil || len(rows) > 100 {
			return ErrUnavailable
		}
		successors := map[string]bool{}
		for _, r := range rows {
			if r.PreviousOperationID != nil {
				successors[*r.PreviousOperationID] = true
			}
		}
		for _, r := range rows {
			projected := result(r)
			if successors[r.OperationID] {
				projected.CanRetry = false
			}
			out.Operations = append(out.Operations, projected)
		}
		return nil
	})
	if e != nil {
		return Context{}, e
	}
	return out, nil
}
