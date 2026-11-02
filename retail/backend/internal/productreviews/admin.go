package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminReview struct {
	PublicReview
	Status   string  `json:"status"`
	OrderID  *string `json:"order_id"`
	Revision string  `json:"revision"`
}
type AdminPage struct {
	Reviews []AdminReview `json:"reviews"`
	Count   int64         `json:"count"`
	Offset  int           `json:"offset"`
	Limit   int           `json:"limit"`
	Enabled bool          `json:"enabled"`
}
type Moderation struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	Status           string `json:"status"`
}

func DecodeModeration(r io.Reader) (Moderation, error) {
	var in Moderation
	b, e := decodeObject(r, "operation_id", "expected_revision", "status")
	if e != nil {
		return in, e
	}
	if json.Unmarshal(b, &in) != nil || !validModeration(in) {
		return in, ErrInput
	}
	return in, nil
}
func validModeration(in Moderation) bool {
	return operationOK(in.OperationID) && revisionOK(in.ExpectedRevision) && statusOK(in.Status)
}
func (s Service) AdminPage(ctx context.Context, actor identity.User, q ListQuery) (AdminPage, error) {
	out := AdminPage{Reviews: []AdminReview{}, Offset: q.Offset, Limit: q.Limit}
	if !idOK(q.ProductID) || q.Offset < 0 || q.Offset > 100000 || q.Limit < 1 || q.Limit > 20 {
		return out, ErrInput
	}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead); e != nil {
			return e
		}
		var e error
		out.Enabled, e = runtime(tx)
		if e != nil {
			return e
		}
		base := func() *gorm.DB {
			return tx.Table("product_reviews").Where("product_id=? AND deleted_at IS NULL", q.ProductID)
		}
		if e := base().Count(&out.Count).Error; e != nil {
			return e
		}
		var rows []reviewRecord
		if e := base().Order("created_at DESC,id DESC").Offset(q.Offset).Limit(q.Limit).Find(&rows).Error; e != nil {
			return e
		}
		for _, row := range rows {
			out.Reviews = append(out.Reviews, AdminReview{row.PublicReview, row.Status, row.OrderID, digest(row)})
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return AdminPage{}, e
	}
	return out, nil
}
func (s Service) Moderate(ctx context.Context, actor identity.User, id string, in Moderation) (Result, error) {
	var out Result
	if !idOK(id) || !validModeration(in) {
		return out, ErrInput
	}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	sha := digest(struct {
		ID    string
		Input Moderation
	}{id, in})
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := authorization.CheckTx(ctx, tx, actor, authorization.ProductRead, authorization.ProductWrite); e != nil {
			return e
		}
		enabled, e := runtime(tx)
		if e != nil {
			return e
		}
		var row reviewRecord
		if e := tx.Where("id=? AND deleted_at IS NULL", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrProduct
			}
			return e
		}
		var receipt command
		e = tx.Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
		if e == nil {
			if receipt.Command != "moderate" || receipt.ActorID != actor.ID || receipt.ReviewID != id || receipt.RequestSHA256 != sha {
				return ErrConflict
			}
			out = Result{id, receipt.ResultStatus, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !enabled {
			return ErrUnavailable
		}
		if digest(row) != in.ExpectedRevision {
			return ErrConflict
		}
		before := row.Status
		stamp := time.Now().UTC()
		if e := tx.Table("product_reviews").Where("id=?", id).Updates(map[string]any{"status": in.Status, "updated_at": stamp}).Error; e != nil {
			return e
		}
		receipt = command{OperationID: in.OperationID, Command: "moderate", ActorID: actor.ID, ReviewID: id, RequestSHA256: sha, ResultStatus: in.Status, PreviousStatus: &before, CreatedAt: stamp}
		if e := tx.Create(&receipt).Error; e != nil {
			return e
		}
		out = Result{id, in.Status, false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return Result{}, e
	}
	return out, nil
}
